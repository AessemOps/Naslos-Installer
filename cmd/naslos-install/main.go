// Command naslos-install is the headless Naslos install engine.
//
// It provisions a freshly-booted Talos node from a versioned install pack and
// streams progress as newline-delimited JSON on stdout. The Tauri desktop app
// bundles the same binary as a sidecar; running it headless makes the flow
// scriptable and testable (FR-INSTALL-12).
//
// This build implements the preflight + pack verification + machine-config
// rendering, the Talos lifecycle (apply-config, bootstrap, kubeconfig) and the
// cluster storage step (local-path provisioner + default StorageClass). The Helm
// install, admin + TOTP, resolver and recovery ZIP are the next increments and
// fail closed with a clear message.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	embeddedpack "github.com/AessemOps/Naslos-Installer/installpack"
	"github.com/AessemOps/Naslos-Installer/internal/config"
	"github.com/AessemOps/Naslos-Installer/internal/event"
	"github.com/AessemOps/Naslos-Installer/internal/installpack"
	"github.com/AessemOps/Naslos-Installer/internal/k8s"
	"github.com/AessemOps/Naslos-Installer/internal/preflight"
	"github.com/AessemOps/Naslos-Installer/internal/state"
	"github.com/AessemOps/Naslos-Installer/internal/talosclient"
	"github.com/AessemOps/Naslos-Installer/internal/talosconfig"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
)

// Version is set at build time with -ldflags.
var Version = "dev"

// Expected pack identity. A pack with a different Talos version or schematic is
// refused (FR-INSTALL-02); override at build time with -ldflags.
var (
	ExpectedTalosVersion = "v1.14.1"
	ExpectedSchematicID  = "4dd8e3a8b6203d3c14f049da8db4d3bb0d6d3e70c5e89dfcc1e709e81914f63c"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type options struct {
	input        config.Input
	packDir      string
	stateDir     string
	jsonProgress bool
	dryRun       bool
}

func run(args []string, stdout io.Writer) error {
	opts, err := parse(args)
	if err != nil {
		return err
	}
	em := event.New(stdout)
	emDone := func(msg string) error { return em.Done(msg) }

	if err := opts.input.Validate(); err != nil {
		return em.Fail("config", "invalid install inputs", err.Error())
	}
	if err := em.Step("config", "Inputs validated", 5); err != nil {
		return err
	}

	// --- milestone 2: load + verify the pack, persist state ---
	fsys, err := packFS(opts.packDir)
	if err != nil {
		return em.Fail("pack", "cannot open install pack", err.Error())
	}
	pack, err := installpack.Load(fsys, ExpectedTalosVersion, ExpectedSchematicID)
	if err != nil {
		return em.Fail("pack", "install pack rejected", err.Error())
	}
	if err := em.Step("pack", "Install pack verified ("+pack.Metadata().NaslosVersion+")", 10); err != nil {
		return err
	}
	if err := pack.Verify(); err != nil {
		return em.Fail("pack", "install pack checksum verification failed", err.Error())
	}

	statePath := filepath.Join(opts.stateDir, "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		return em.Fail("state", "cannot read install state", err.Error())
	}
	if st == nil {
		st = state.New(opts.input, pack.Metadata().NaslosVersion)
	} else if st.Input.NodeIP != opts.input.NodeIP {
		return em.Fail("state", "existing state targets a different node",
			fmt.Sprintf("state node %s, requested %s", st.Input.NodeIP, opts.input.NodeIP))
	}
	st.SetStep("pack", state.Done, pack.Metadata().NaslosVersion)
	if err := st.Save(statePath); err != nil {
		return em.Fail("state", "cannot persist install state", err.Error())
	}

	// --- milestone 1: node reachability (skipped in dry-run) ---
	if opts.dryRun {
		if err := em.Step("preflight", "dry-run: skipping node probe", 20); err != nil {
			return err
		}
	} else {
		nodeState, err := preflight.Check(opts.input.NodeIP, preflightTimeout)
		if err != nil {
			return em.Fail("preflight", "node is not reachable", err.Error())
		}
		if nodeState == preflight.Installed {
			return em.Fail("preflight", "node looks already installed",
				"kube-apiserver answered on :6443; refusing to re-key an installed node (use the state file to resume)")
		}
		st.SetStep("preflight", state.Done, string(nodeState))
		if err := st.Save(statePath); err != nil {
			return em.Fail("state", "cannot persist install state", err.Error())
		}
		if err := em.Step("preflight", "Node is in maintenance mode", 20); err != nil {
			return err
		}
	}

	// --- milestone 2 (continued): render machine config ---
	subnet, err := opts.input.Subnet24()
	if err != nil {
		return em.Fail("machine-config", "cannot derive the node subnet", err.Error())
	}
	rendered, err := pack.RenderMachineConfig(map[string]string{
		"NODE_SUBNET":  subnet,
		"INSTALL_DISK": installDisk,
	})
	if err != nil {
		return em.Fail("machine-config", "cannot render the machine config", err.Error())
	}
	mcPath := filepath.Join(opts.stateDir, "machine-config.yaml")
	if err := os.MkdirAll(opts.stateDir, 0o700); err != nil {
		return em.Fail("machine-config", "cannot create the state directory", err.Error())
	}
	if err := os.WriteFile(mcPath, rendered, 0o600); err != nil {
		return em.Fail("machine-config", "cannot write the machine config", err.Error())
	}
	st.SetStep("machine-config", state.Done, mcPath)
	if err := st.Save(statePath); err != nil {
		return em.Fail("state", "cannot persist install state", err.Error())
	}
	if err := em.Step("machine-config", "Machine config rendered", 25); err != nil {
		return err
	}

	// --- milestone 2 (continued): Talos PKI + control-plane config ---
	// The secrets bundle is persisted and reused, so a resume never re-keys an
	// installed node (FR-INSTALL-09).
	secretsPath := filepath.Join(opts.stateDir, "talos-secrets.json")
	bundle, reused, err := talosconfig.LoadOrCreateBundle(secretsPath, pack.Metadata().TalosVersion)
	if err != nil {
		return em.Fail("talos-config", "cannot prepare the Talos PKI", err.Error())
	}
	gen, err := talosconfig.Generate(talosconfig.Inputs{
		ClusterName:  "naslos",
		NodeIP:       opts.input.NodeIP,
		TalosVersion: pack.Metadata().TalosVersion,
		InstallDisk:  installDisk,
	}, rendered, bundle)
	if err != nil {
		return em.Fail("talos-config", "cannot generate the Talos config", err.Error())
	}
	cpPath := filepath.Join(opts.stateDir, "controlplane.yaml")
	tcPath := filepath.Join(opts.stateDir, "talosconfig")
	if err := os.WriteFile(cpPath, gen.ControlPlane, 0o600); err != nil {
		return em.Fail("talos-config", "cannot write controlplane.yaml", err.Error())
	}
	if err := os.WriteFile(tcPath, gen.Talosconfig, 0o600); err != nil {
		return em.Fail("talos-config", "cannot write talosconfig", err.Error())
	}
	msg := "Talos config generated"
	if reused {
		msg = "Talos config generated (reused existing PKI)"
	}
	st.SetStep("talos-config", state.Done, msg)
	if err := st.Save(statePath); err != nil {
		return em.Fail("state", "cannot persist install state", err.Error())
	}
	if err := em.Step("talos-config", msg, 35); err != nil {
		return err
	}

	if opts.dryRun {
		return emDone(fmt.Sprintf("dry-run complete for %s (pack %s; controlplane %s)",
			opts.input.NodeIP, pack.Metadata().NaslosVersion, cpPath))
	}

	// --- milestone 3: apply the config, bootstrap etcd, fetch kubeconfig ---
	ctx := context.Background()

	maintenance, err := talosclient.Dial(ctx, opts.input.NodeIP)
	if err != nil {
		return em.Fail("talos-install", "cannot open the Talos API", err.Error())
	}
	defer maintenance.Close()

	if err := em.Step("talos-install", "Waiting for the installer API", 40); err != nil {
		return err
	}
	if err := talosclient.WaitForAPI(ctx, maintenance, 2*time.Minute); err != nil {
		return em.Fail("talos-install", "the node did not answer on the Talos API", err.Error())
	}

	if err := em.Step("talos-install", "Applying the machine configuration", 45); err != nil {
		return err
	}
	if err := maintenance.Apply(ctx, gen.ControlPlane); err != nil {
		return em.Fail("talos-install", "apply-config failed", err.Error())
	}

	// The node installs to disk and reboots; the maintenance client can no
	// longer authenticate, so switch to the generated talosconfig.
	tcCfg, err := clientconfig.FromBytes(gen.Talosconfig)
	if err != nil {
		return em.Fail("talos-install", "cannot read the generated talosconfig", err.Error())
	}
	api, err := talosclient.DialAuthenticated(ctx, tcCfg)
	if err != nil {
		return em.Fail("talos-install", "cannot open the authenticated Talos API", err.Error())
	}
	defer api.Close()

	if err := em.Step("talos-install", "Node is installing; waiting for the API", 50); err != nil {
		return err
	}
	if err := talosclient.WaitForAPI(ctx, api, 25*time.Minute); err != nil {
		return em.Fail("talos-install", "the installed node did not come back", err.Error())
	}

	if err := em.Step("bootstrap", "Bootstrapping etcd", 60); err != nil {
		return err
	}
	if err := talosclient.Bootstrap(ctx, api); err != nil {
		return em.Fail("bootstrap", "bootstrap failed", err.Error())
	}
	if err := em.Step("bootstrap", "Waiting for the control plane", 65); err != nil {
		return err
	}
	if err := talosclient.WaitForServices(ctx, api, 5*time.Minute, "etcd", "kubelet"); err != nil {
		return em.Fail("bootstrap", "the control plane did not become ready", err.Error())
	}

	kc, err := api.Kubeconfig(ctx)
	if err != nil {
		return em.Fail("kubeconfig", "cannot fetch the kubeconfig", err.Error())
	}
	kubeconfigPath := filepath.Join(opts.stateDir, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, kc, 0o600); err != nil {
		return em.Fail("kubeconfig", "cannot write kubeconfig", err.Error())
	}
	st.SetStep("bootstrap", state.Done, "etcd bootstrapped")
	if err := st.Save(statePath); err != nil {
		return em.Fail("state", "cannot persist install state", err.Error())
	}
	if err := em.Step("kubeconfig", "Fetched kubeconfig", 70); err != nil {
		return err
	}

	// --- milestone 4: CNI readiness + cluster storage (local-path) ---
	// The node bootstraps with Flannel disabled and Cilium applied as a Talos
	// inline manifest, so no pod can be scheduled until its DaemonSet is ready.
	// The kubeconfig is already in memory; build the Kubernetes client from it.
	cluster, err := k8s.NewFromKubeconfig(kc)
	if err != nil {
		return em.Fail("storage", "cannot open the Kubernetes API", err.Error())
	}
	if err := em.Step("cni", "Waiting for the CNI (Cilium)", 72); err != nil {
		return err
	}
	if err := cluster.WaitForDaemonSet(ctx, "kube-system", "cilium", 5*time.Minute); err != nil {
		return em.Fail("cni", "the CNI did not become ready", err.Error())
	}

	if err := em.Step("storage", "Applying the local-path storage provisioner", 75); err != nil {
		return err
	}
	localPath, err := pack.ReadFile("manifests/local-path-v0.0.26.yaml")
	if err != nil {
		return em.Fail("storage", "install pack has no local-path manifest", err.Error())
	}
	if err := cluster.Apply(ctx, localPath, k8s.FieldManager); err != nil {
		return em.Fail("storage", "cannot apply the local-path manifest", err.Error())
	}
	// The provisioner's helper pod mounts hostPath volumes, which the default
	// "baseline" PodSecurity profile refuses (mirrors scripts/deploy-vm.sh).
	if err := cluster.LabelNamespace(ctx, "local-path-storage", map[string]string{
		"pod-security.kubernetes.io/enforce":         "privileged",
		"pod-security.kubernetes.io/enforce-version": "latest",
	}); err != nil {
		return em.Fail("storage", "cannot relax PodSecurity on the local-path namespace", err.Error())
	}
	if err := cluster.SetDefaultStorageClass(ctx, "local-path"); err != nil {
		return em.Fail("storage", "cannot make local-path the default StorageClass", err.Error())
	}
	if err := cluster.WaitForDeployment(ctx, "local-path-storage", "local-path-provisioner", 2*time.Minute); err != nil {
		return em.Fail("storage", "the local-path provisioner did not become ready", err.Error())
	}
	st.SetStep("storage", state.Done, "local-path applied and set as default")
	if err := st.Save(statePath); err != nil {
		return em.Fail("state", "cannot persist install state", err.Error())
	}
	if err := em.Step("storage", "Cluster storage ready", 80); err != nil {
		return err
	}

	// Helm install, admin + TOTP, resolver and the recovery ZIP are the next
	// increments. The Traefik CRDs are not applied here: they ship in the
	// traefik subchart's crds/ directory and Helm installs them before the
	// chart's Traefik custom resources (verified with `helm template --include-crds`).
	return em.Fail("helm", "Helm install is not implemented in this build",
		fmt.Sprintf("storage is ready; kubeconfig at %s", kubeconfigPath))
}

const installDisk = "/dev/vda"

var preflightTimeout = 5 * time.Second

func packFS(dir string) (fs.FS, error) {
	if dir == "" {
		return embeddedpack.FS, nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	return os.DirFS(dir), nil
}

func parse(args []string) (options, error) {
	var o options
	var addResolver, noJSON bool

	fs := flag.NewFlagSet("naslos-install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.input.NodeIP, "node-ip", "", "IPv4 address of the Talos node (required)")
	fs.StringVar(&o.input.Domain, "domain", "", "local domain / appliance name (required)")
	fs.StringVar(&o.input.Name, "name", "", "advertised SMB/mDNS name (default: domain's first label)")
	fs.StringVar(&o.input.AdminUser, "admin-user", "", "first administrator username (required)")
	fs.StringVar(&o.input.AdminPassword, "admin-password", "", "administrator password (required)")
	fs.BoolVar(&addResolver, "add-resolver", false, "add the name to the local hosts file (best effort)")
	fs.BoolVar(&o.dryRun, "dry-run", false, "validate inputs and the pack, then stop before touching the node")
	fs.BoolVar(&noJSON, "no-json-progress", false, "write human-readable progress instead of NDJSON")
	fs.StringVar(&o.packDir, "pack-dir", "", "use a pack directory instead of the embedded pack")
	fs.StringVar(&o.stateDir, "state-dir", "", "state directory (default: user config dir)")
	fs.BoolVar(&o.jsonProgress, "json-progress", true, "stream newline-delimited JSON progress on stdout")

	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	// --json-progress is on by default; keep the flag explicit for the contract,
	// while `--no-json-progress` is the human-readable escape hatch.
	o.jsonProgress = o.jsonProgress && !noJSON
	o.input.AddResolver = addResolver
	if o.stateDir == "" {
		p, err := state.DefaultPath()
		if err != nil {
			return o, err
		}
		o.stateDir = filepath.Dir(p)
	}
	return o, nil
}
