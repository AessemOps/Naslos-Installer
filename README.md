# Naslos Installer

Desktop installer and headless install engine for **Naslos** — provisions a
Talos node booted from the Naslos ZFS ISO, end to end, with a progress bar, a
first-admin + 2FA handoff and a recovery ZIP.

This repo is the app half of a two-repo split:

- [`AessemOps/Naslos-Linux`](https://github.com/AessemOps/Naslos-Linux) owns the
  declarative install artifacts and publishes a versioned **install pack**
  (`naslos-install-pack-<version>.tar.gz`) as a GitHub release.
- This repo pins a pack version, downloads + checksum-verifies it at build time,
  and `go:embed`s it (`scripts/fetch-install-pack.sh`).

The stable interfaces (pack schema, cluster object names, first-admin payload,
TOTP command, progress protocol) are the **contract**:
[`docs/installer-contract.md`](https://github.com/AessemOps/Naslos-Linux/blob/master/docs/installer-contract.md).

## Layout

```
cmd/naslos-install        headless CLI, newline-delimited JSON progress
internal/config           install inputs + validation/derivations
internal/event            NDJSON progress protocol
internal/installpack      load, verify and render an install pack
internal/k8s              server-side apply of pack manifests + workload waits
internal/otpauth          parse the Authelia otpauth:// enrolment output
internal/preflight        node reachability (:50000 maintenance / :6443 installed)
internal/state            resumable <app-data>/naslos-install/state.json (0600)
internal/talosclient      Talos lifecycle (apply-config/bootstrap/kubeconfig)
internal/talosconfig      PKI + machine-config/talosconfig generation (machinery)
installpack/              go:embed target (gitignored; populated by fetch)
scripts/fetch-install-pack.sh
desktop/                  Tauri v2 shell + Svelte wizard (next increment)
```

## Build & test

```bash
make check                 # go vet + go test -race ./...
make fetch-pack PACK_VERSION=0.1.0
make build                 # dist/naslos-install
```

The engine runs without the desktop shell:

```bash
dist/naslos-install \
  --node-ip 192.168.1.117 --domain naslos.local \
  --admin-user admin --admin-password 'Correct1' \
  --dry-run
```

## Status

Implemented: inputs/validation, pack load + checksum verification + version
gate, machine-config rendering, node preflight, resumable state, NDJSON
progress, Talos PKI + control-plane/talosconfig generation via
`siderolabs/talos/pkg/machinery` (persisted secrets bundle; reuse never re-keys
a node), the Talos lifecycle (maintenance apply → wait → bootstrap →
kubeconfig), and the cluster storage step (startup CNI wait, local-path
provisioner server-side apply, PodSecurity label, default StorageClass). The
generated control plane matches `talosctl gen config` on the pack patch (install
image, Cilium inline manifest, kube-proxy/flannel disabled, host-DNS,
`KubeNodeConfig`).

Next increments (see `.kilo/plans/desktop-installer-app.md`): Helm install from
the pack, admin + TOTP bootstrap, resolver hosts entry, recovery ZIP, then the
Tauri shell and per-OS bundles.

## License

GNU Affero General Public License v3.0 (see Naslos-Linux `LICENSE`).
