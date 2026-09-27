// Package talosconfig generates Talos machine config and the talosconfig from
// the pack's machine-config patch, using the same machinery as `talosctl`.
//
// The secrets bundle (PKI) is persisted and reused: regenerating it against an
// already-installed node orphans the node's talosconfig and the next bootstrap
// fails with "certificate signed by unknown authority" (the trap in
// scripts/bootstrap-vm.sh / deploy-vm.sh). Callers MUST load an existing bundle
// when one is present (LoadOrCreateBundle).
package talosconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
)

// Inputs parameterise config generation.
type Inputs struct {
	ClusterName    string
	NodeIP         string
	TalosVersion   string
	InstallerImage string
	InstallDisk    string
}

// Result is the generated config material.
type Result struct {
	ControlPlane []byte
	Talosconfig  []byte
}

var docSepRE = regexp.MustCompile(`(?m)^---\s*$`)

// LoadOrCreateBundle loads a persisted secrets bundle from path, or creates and
// writes a new one. The bool reports whether an existing bundle was reused.
func LoadOrCreateBundle(path, talosVersion string) (*secrets.Bundle, bool, error) {
	if raw, err := os.ReadFile(path); err == nil {
		var b secrets.Bundle
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, false, fmt.Errorf("parsing persisted Talos secrets: %w", err)
		}
		if b.Cluster == nil || b.Secrets == nil || b.Certs == nil {
			return nil, false, errors.New("persisted Talos secrets bundle is incomplete")
		}
		return &b, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}

	contract, err := config.ParseContractFromVersion(talosVersion)
	if err != nil {
		return nil, false, fmt.Errorf("resolving the Talos version contract for %s: %w", talosVersion, err)
	}
	bundle, err := secrets.NewBundle(secrets.NewClock(), contract)
	if err != nil {
		return nil, false, fmt.Errorf("creating the Talos secrets bundle: %w", err)
	}
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(dir(path), 0o700); err != nil {
		return nil, false, err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, false, err
	}
	return bundle, false, nil
}

// Generate produces the control-plane machine config (with patch applied) and
// the talosconfig, reusing bundle for the PKI.
func Generate(in Inputs, patch []byte, bundle *secrets.Bundle) (*Result, error) {
	if in.NodeIP == "" || in.TalosVersion == "" {
		return nil, errors.New("talosconfig.Generate: NodeIP and TalosVersion are required")
	}
	endpoint := "https://" + in.NodeIP + ":6443"

	// NewInput's third argument is the *Kubernetes* version (Talos prepends the
	// "v"), while the Talos feature set comes from the version contract. Getting
	// this wrong bakes a bogus kube-proxy/kubelet image tag like `vv1.14.1`.
	contract, err := config.ParseContractFromVersion(in.TalosVersion)
	if err != nil {
		return nil, fmt.Errorf("resolving the Talos version contract for %s: %w", in.TalosVersion, err)
	}

	opts := []generate.Option{
		generate.WithVersionContract(contract),
		generate.WithSecretsBundle(bundle),
		generate.WithInstallDisk(in.InstallDisk),
		// v1.14 gen config otherwise emits a stock UnattendedInstallConfig that
		// is mutually exclusive with our machine.install block.
		generate.WithSkipUnattendedInstallConfig(true),
	}
	if in.InstallerImage != "" {
		opts = append(opts, generate.WithInstallImage(in.InstallerImage))
	}

	input, err := generate.NewInput(in.ClusterName, endpoint, constants.DefaultKubernetesVersion, opts...)
	if err != nil {
		return nil, fmt.Errorf("preparing config generation: %w", err)
	}
	provider, err := input.Config(machine.TypeControlPlane)
	if err != nil {
		return nil, fmt.Errorf("generating control-plane config: %w", err)
	}

	patched, err := applyPatch(provider, patch)
	if err != nil {
		return nil, err
	}
	cp, err := patched.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encoding control-plane config: %w", err)
	}

	talosconfig, err := input.Talosconfig()
	if err != nil {
		return nil, fmt.Errorf("generating talosconfig: %w", err)
	}
	// `gen config` leaves endpoints empty (the CLI passes --endpoints). Fill the
	// node's endpoint so the recovery talosconfig and the API pod's copy work
	// without extra flags (mirrors bootstrap/vm/talosconfig-pod).
	if ctx, ok := talosconfig.Contexts[talosconfig.Context]; ok {
		ctx.Endpoints = []string{in.NodeIP}
	}
	tc, err := talosconfig.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encoding talosconfig: %w", err)
	}

	return &Result{ControlPlane: cp, Talosconfig: tc}, nil
}

// applyPatch splits the multi-document patch and applies each document in order.
func applyPatch(provider config.Provider, patch []byte) (config.Provider, error) {
	patches := make([]configpatcher.Patch, 0, 8)
	for _, doc := range docSepRE.Split(string(patch), -1) {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		p, err := configpatcher.LoadPatch([]byte(doc))
		if err != nil {
			return nil, fmt.Errorf("loading machine-config patch document: %w", err)
		}
		patches = append(patches, p)
	}
	out, err := configpatcher.Apply(configpatcher.WithConfig(provider), patches)
	if err != nil {
		return nil, fmt.Errorf("applying the machine-config patch: %w", err)
	}
	patched, err := out.Config()
	if err != nil {
		return nil, fmt.Errorf("re-reading the patched config: %w", err)
	}
	return patched, nil
}

func dir(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[:i]
	}
	return "."
}
