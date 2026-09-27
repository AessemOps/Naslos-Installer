// Command naslos-install is the headless Naslos install engine.
//
// It provisions a freshly-booted Talos node from a versioned install pack and
// streams progress as newline-delimited JSON on stdout. The Tauri desktop app
// bundles the same binary as a sidecar; running it headless makes the flow
// scriptable and testable (FR-INSTALL-12).
//
// This build implements the preflight + pack verification + machine-config
// rendering milestones and the state/resume scaffolding. The cluster lifecycle
// steps (apply-config, bootstrap, Helm, admin + TOTP, resolver, recovery ZIP)
// are the next increment and fail closed with a clear message.
package main

import (
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
	"github.com/AessemOps/Naslos-Installer/internal/preflight"
	"github.com/AessemOps/Naslos-Installer/internal/state"
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

	if opts.dryRun {
		return emDone(fmt.Sprintf("dry-run complete for %s (pack %s, machine config %s)",
			opts.input.NodeIP, pack.Metadata().NaslosVersion, mcPath))
	}

	// The cluster lifecycle (apply-config, bootstrap, kubeconfig, storage, CRDs,
	// Helm, admin, TOTP, resolver, recovery ZIP) is the next increment.
	return em.Fail("install", "cluster lifecycle is not implemented in this build",
		"this scaffold stops after machine-config rendering; use --dry-run to validate inputs and the pack")
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
