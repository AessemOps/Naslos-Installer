# Naslos Installer

Desktop installer and headless install engine for **Naslos** — provisions the
Naslos node from a machine booted from the Talos ISO (the Naslos ZFS schematic),
end to end, with a progress bar, a first-admin + 2FA handoff and a recovery ZIP.

This repo is the app half of a two-repo split:

- [`AessemOps/Naslos-Linux`](https://github.com/AessemOps/Naslos-Linux) owns the
  declarative install artifacts and publishes a versioned **install pack**
  (`naslos-install-pack-<version>.tar.gz`) as a GitHub release.
- This repo resolves the **newest `vX.Y.Z` tag** of Naslos-Linux, downloads +
  checksum-verifies that pack at build time, and `go:embed`s it
  (`scripts/fetch-install-pack.sh`). Naslos-Linux dispatches a rebuild here when
  it publishes a new pack, so a released pack always yields a released installer.

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
scripts/build-sidecar.sh  build the engine into the Tauri sidecar path
.github/workflows/       ci.yml (test + pack + desktop) and release.yml
desktop/                  Tauri v2 shell (src-tauri/) + Svelte 5 wizard (ui/)
```

## Build & test

```bash
make check                 # go vet + go test -race ./...
make fetch-latest-pack     # newest Naslos-Linux vX.Y.Z pack -> installpack/
make fetch-pack PACK_VERSION=0.1.0   # or pin an explicit version
make build                 # dist/naslos-install (Talos/schematic from the pack)
```

The engine runs without the desktop shell:

```bash
dist/naslos-install \
  --node-ip 192.168.1.117 --domain naslos.local \
  --admin-user admin --admin-password 'Correct1' \
  --dry-run
```

## Desktop app

`desktop/` is the Tauri v2 shell around the engine with a Svelte 5 wizard
(input → confirm → progress bar + log → 2FA/QR handoff → recovery ZIP). The
shell spawns `naslos-install` as a sidecar and streams its NDJSON progress to the
window. See [`desktop/README.md`](desktop/README.md).

```bash
./scripts/build-sidecar.sh        # build the engine the shell spawns
cd desktop && npm install
npm run tauri dev                 # the desktop window
npm run dev                       # browser preview (simulated engine)
```

## CI & releases

- `ci.yml` runs `make check` on every push/PR, fetches the newest Naslos-Linux
  `vX.Y.Z` pack and builds against it, and type-checks/builds the wizard and
  `cargo check`s the Tauri shell.
- `release.yml` builds `linux`/`darwin`/`windows` (amd64 + arm64), packages each
  binary with per-file sha256 checksums, and publishes a GitHub release. It runs
  on an installer `v*` tag, on manual dispatch (with an optional
  `naslos_version` input), and on the `naslos-release` repository dispatch that
  Naslos-Linux sends when it publishes a pack. Every build embeds the newest
  Naslos-Linux `vX.Y.Z` tag and derives `ExpectedTalosVersion` /
  `ExpectedSchematicID` from that pack, so the FR-INSTALL-02 gate always matches.
  Tauri `.AppImage`/`.deb`/`.rpm`, `.dmg`/`.app` and `.msi`/`.exe` bundles are
  the next release increment (I8b).

## Status

Implemented: inputs/validation, pack load + checksum verification + version
gate, machine-config rendering, node preflight, resumable state, NDJSON
progress/`data` payloads, Talos PKI + control-plane/talosconfig generation via
`siderolabs/talos/pkg/machinery` (persisted secrets bundle; reuse never re-keys
a node), the Talos lifecycle (maintenance apply → wait → bootstrap →
kubeconfig), the cluster storage step (startup CNI wait, local-path provisioner
server-side apply, PodSecurity label, default StorageClass), the Helm install of
the pack's chart (values merge + engine overrides, pre-created `naslos` +
`naslos-talosconfig` Secret, explicit workload waits), the first administrator +
TOTP device (owner API via pod exec, `authelia … totp generate`), a best-effort
hosts entry and the recovery ZIP, and the desktop GUI (Tauri v2 shell + Svelte
wizard, verified against a simulated engine in a browser). The generated control
plane matches `talosctl gen config` on the pack patch (install image, Cilium
inline manifest, kube-proxy/flannel disabled, host-DNS, `KubeNodeConfig`). The
whole path through admin + TOTP was live-validated on 2026-10-03 (Talos node
Ready, Naslos pods Running, `/authelia/` 200, and an Authelia firstfactor +
CLI-generated TOTP login → 200); the engine completes to the `done` event.

Next increment (see `.kilo/plans/desktop-installer-app.md`): the per-OS Tauri
bundles (I8b).

## License

GNU Affero General Public License v3.0 (see Naslos-Linux `LICENSE`).
