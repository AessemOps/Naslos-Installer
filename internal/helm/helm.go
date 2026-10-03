// Package helm installs the Naslos umbrella chart from the verified install
// pack.
//
// It is deliberately thin: it loads the chart directory the engine extracted
// from the pack, merges values.yaml -> values-installer.yaml -> the engine's
// per-install overrides, and runs a server-side Helm install with the pack's
// CRDs enabled (the VM Makefile pre-installs them and passes --skip-crds, but
// the pack ships no CRDs, so Helm must install the traefik subchart's crds/).
package helm

import (
	"context"
	"fmt"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/yaml"
)

// Options describe one release install.
type Options struct {
	KubeconfigPath string
	ChartDir       string
	Release        string
	Namespace      string
	ValueFiles     []string
	Overrides      map[string]interface{}
	Timeout        time.Duration
}

// Install runs (or upgrades) the release. TakeOwnership lets it adopt the
// namespace and other prerequisites the engine pre-created (the
// naslos-talosconfig Secret mounts before the API pod starts).
func Install(ctx context.Context, o Options) error {
	cfg := new(action.Configuration)
	getter := genericclioptions.NewConfigFlags(false)
	getter.KubeConfig = &o.KubeconfigPath
	if err := cfg.Init(getter, o.Namespace, "secret", func(string, ...interface{}) {}); err != nil {
		return fmt.Errorf("initialising Helm: %w", err)
	}

	ch, err := loader.Load(o.ChartDir)
	if err != nil {
		return fmt.Errorf("loading the chart from %s: %w", o.ChartDir, err)
	}

	vals, err := MergeValues(o.ValueFiles, o.Overrides)
	if err != nil {
		return err
	}

	// Re-run/upgrade if the release already exists, so an interrupted install
	// converges instead of failing with "cannot re-use a name".
	if _, err := action.NewStatus(cfg).Run(o.Release); err == nil {
		up := action.NewUpgrade(cfg)
		up.Namespace = o.Namespace
		up.TakeOwnership = true
		up.Wait = false
		up.Timeout = o.Timeout
		up.SkipSchemaValidation = true
		if _, err := up.RunWithContext(ctx, o.Release, ch, vals); err != nil {
			return fmt.Errorf("helm upgrade %s: %w", o.Release, err)
		}
		return nil
	}

	inst := action.NewInstall(cfg)
	inst.ReleaseName = o.Release
	inst.Namespace = o.Namespace
	inst.CreateNamespace = false
	inst.TakeOwnership = true
	// Wait must stay false: the OpenLDAP bootstrap Job is a post-install hook,
	// but Authelia's startup check binds the LDAP service account that hook
	// creates. Waiting for resources before running the hook deadlocks on a
	// fresh install. Helm still runs (and waits for) the hook; the engine waits
	// explicitly for the workloads afterwards.
	inst.Wait = false
	inst.WaitForJobs = false
	inst.Timeout = o.Timeout
	inst.SkipCRDs = false
	// The authelia subchart's values.schema.json has an external $ref
	// (https://charts.authelia.com/definitions.json) that Helm v3 tries to
	// fetch, so validation fails offline. Rendering still validates the values;
	// the chart is left to install without the JSON-schema pass.
	inst.SkipSchemaValidation = true

	if _, err := inst.RunWithContext(ctx, ch, vals); err != nil {
		return fmt.Errorf("helm install %s: %w", o.Release, err)
	}
	return nil
}

// MergeValues deep-merges value files left-to-right (later wins) and then the
// engine overrides (highest priority). Nested maps are merged recursively;
// scalars and lists are replaced.
func MergeValues(files []string, overrides map[string]interface{}) (map[string]interface{}, error) {
	merged := map[string]interface{}{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("reading values file %s: %w", f, err)
		}
		v := map[string]interface{}{}
		if err := yaml.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("parsing values file %s: %w", f, err)
		}
		mergeInto(merged, v)
	}
	if overrides != nil {
		mergeInto(merged, overrides)
	}
	return merged, nil
}

// mergeInto recursively copies src over dst (src wins).
func mergeInto(dst, src map[string]interface{}) {
	for k, v := range src {
		if vm, ok := v.(map[string]interface{}); ok {
			if dm, ok := dst[k].(map[string]interface{}); ok {
				mergeInto(dm, vm)
				continue
			}
			child := map[string]interface{}{}
			mergeInto(child, vm)
			dst[k] = child
			continue
		}
		dst[k] = v
	}
}

// Overrides builds the engine's per-install value overrides from the validated
// inputs. Kept here (not in main) so it is unit-tested.
func Overrides(domain, shortName, nodeCIDR, talosConfigSecret string) map[string]interface{} {
	return map[string]interface{}{
		"domain": domain,
		"sso": map[string]interface{}{
			"domains": []interface{}{domain},
		},
		"shares": map[string]interface{}{
			"discovery": map[string]interface{}{"name": shortName},
		},
		"openldap": map[string]interface{}{
			"host": "ldap." + domain,
		},
		"api": map[string]interface{}{
			"talosConfigSecret": talosConfigSecret,
		},
		"networkPolicy": map[string]interface{}{
			"nodeCIDR":           nodeCIDR,
			"ingressPluginsCIDR": nodeCIDR,
			"nfsClientCIDR":      nodeCIDR,
		},
	}
}
