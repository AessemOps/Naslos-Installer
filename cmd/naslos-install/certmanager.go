package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/AessemOps/Naslos-Installer/internal/event"
	"github.com/AessemOps/Naslos-Installer/internal/helm"
	"github.com/AessemOps/Naslos-Installer/internal/installpack"
	"github.com/AessemOps/Naslos-Installer/internal/k8s"
	"github.com/AessemOps/Naslos-Installer/internal/state"
)

// The certificate stack: cert-manager plus the OVH DNS-01 webhook. The desktop
// installer provisions both from the pack, the way `make install-vm` installs
// `crds`, `cert-manager` and `cert-manager-webhook-ovh`, so a fresh install can
// issue publicly trusted certificates instead of only the chart's generated
// self-signed one.
//
// Order matters and mirrors the Makefile: apply the CRDs, install cert-manager
// with `crds.enabled=false` (the CRDs are already there), install the webhook,
// then wait. The Naslos release comes afterwards: its values-installer profile
// enables ovhWebhook RBAC, which references the webhook ServiceAccount.
const (
	certManagerStep      = "cert-manager"
	certManagerNamespace = "cert-manager"
	certManagerManifest  = "manifests/cert-manager-crds.yaml"
	// ovhWebhookGroup must equal the OVH provider's webhook.groupName in
	// rust/api/src/providers/builtin/ovh.yaml, or Issuers never reconcile.
	ovhWebhookGroup = "ovh.naslos.local"
)

// packHasCertManager reports whether this pack carries the certificate stack.
// Packs built before 0.2.5 do not, and an install must not fail over a pack
// feature it was not built for.
func packHasCertManager(p *installpack.Pack) bool {
	_, err := p.ReadFile(certManagerManifest)
	return err == nil
}

// installCertManager provisions cert-manager and the OVH webhook in the
// `cert-manager` namespace. No operator credential is involved: the webhook is
// credential-less and OVH API credentials live in per-Domain Secrets the UI
// writes later. It needs image-pull egress to quay.io and Docker Hub.
func installCertManager(
	ctx context.Context,
	em *event.Emitter,
	p *installpack.Pack,
	cluster *k8s.Client,
	kubeconfigPath, statePath string,
	st *state.State,
) error {
	if !packHasCertManager(p) {
		// Record it as done so a resume does not retry, and say why in the log.
		st.SetStep(certManagerStep, state.Done, "skipped: pack carries no cert-manager material")
		if err := st.Save(statePath); err != nil {
			return em.Fail("state", "cannot persist install state", err.Error())
		}
		return em.Step(certManagerStep, "Pack carries no cert-manager material; skipping", 81)
	}

	crds, err := p.ReadFile(certManagerManifest)
	if err != nil {
		return em.Fail(certManagerStep, "cannot read the cert-manager CRDs", err.Error())
	}
	tmp, err := os.MkdirTemp("", "naslos-cert-manager-")
	if err != nil {
		return em.Fail(certManagerStep, "cannot create a temp dir for the charts", err.Error())
	}
	defer os.RemoveAll(tmp)
	if err := p.Extract(tmp); err != nil {
		return em.Fail(certManagerStep, "cannot extract the install pack", err.Error())
	}
	certManagerChart, err := singleChart(filepath.Join(tmp, "charts", "naslos", "charts"), "cert-manager-*.tgz")
	if err != nil {
		return em.Fail(certManagerStep, "install pack has no cert-manager chart", err.Error())
	}
	webhookChart, err := singleChart(filepath.Join(tmp, "charts"), "cert-manager-webhook-ovh-*.tgz")
	if err != nil {
		return em.Fail(certManagerStep, "install pack has no OVH webhook chart", err.Error())
	}

	if err := em.Step(certManagerStep, "Applying the cert-manager CRDs", 81); err != nil {
		return err
	}
	if err := cluster.Apply(ctx, crds, k8s.FieldManager); err != nil {
		return em.Fail(certManagerStep, "cannot apply the cert-manager CRDs", err.Error())
	}
	if err := cluster.EnsureNamespace(ctx, certManagerNamespace, nil); err != nil {
		return em.Fail(certManagerStep, "cannot create the cert-manager namespace", err.Error())
	}

	helmCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	if err := em.Step(certManagerStep, "Installing cert-manager", 82); err != nil {
		return err
	}
	if err := helm.Install(helmCtx, helm.Options{
		KubeconfigPath: kubeconfigPath,
		ChartDir:       certManagerChart,
		Release:        "cert-manager",
		Namespace:      certManagerNamespace,
		// The CRDs were applied above; a second install would conflict.
		Overrides: map[string]interface{}{"crds": map[string]interface{}{"enabled": false}},
		Timeout:   10 * time.Minute,
	}); err != nil {
		return em.Fail(certManagerStep, "cert-manager did not install", err.Error())
	}

	if err := em.Step(certManagerStep, "Installing the OVH DNS-01 webhook", 83); err != nil {
		return err
	}
	if err := helm.Install(helmCtx, helm.Options{
		KubeconfigPath: kubeconfigPath,
		ChartDir:       webhookChart,
		Release:        "cert-manager-webhook-ovh",
		Namespace:      certManagerNamespace,
		Overrides: map[string]interface{}{
			"groupName": ovhWebhookGroup,
			"certManager": map[string]interface{}{
				"namespace":          certManagerNamespace,
				"serviceAccountName": "cert-manager",
			},
		},
		Timeout: 10 * time.Minute,
	}); err != nil {
		return em.Fail(certManagerStep, "the OVH webhook did not install", err.Error())
	}

	// Helm does not wait (see internal/helm), so wait for the workloads. The
	// webhook's serving certificate is injected by the cainjector, which needs
	// the CRDs and cert-manager itself, hence this order.
	for _, deploy := range []string{"cert-manager", "cert-manager-cainjector", "cert-manager-webhook"} {
		if err := cluster.WaitForDeployment(ctx, certManagerNamespace, deploy, 5*time.Minute); err != nil {
			return em.Fail(certManagerStep, "cert-manager did not become ready", err.Error())
		}
	}
	if err := cluster.WaitForDeployment(ctx, certManagerNamespace, "cert-manager-webhook-ovh", 5*time.Minute); err != nil {
		return em.Fail(certManagerStep, "the OVH webhook did not become ready", err.Error())
	}

	st.SetStep(certManagerStep, state.Done, "cert-manager and the OVH webhook are ready")
	if err := st.Save(statePath); err != nil {
		return em.Fail("state", "cannot persist install state", err.Error())
	}
	return em.Step(certManagerStep, "Certificate stack ready", 84)
}

// singleChart returns the one file matching pattern, and fails on zero or many
// so a repacked chart cannot silently install the wrong version.
func singleChart(dir, pattern string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("expected exactly one %s in %s, found %d", pattern, dir, len(matches))
	}
	return matches[0], nil
}
