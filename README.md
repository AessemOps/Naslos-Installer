# Naslos Installer

**Boot a machine from the Talos ISO, run the wizard, and get a working NAS with
an admin account, 2FA, and a recovery ZIP.**

The installer writes the Talos machine config, creates the PKI, bootstraps the
cluster, installs CNI, a storage class, certificates, and the Helm release, then
creates the first administrator and a TOTP device. It ships as a desktop wizard
and as a headless CLI.

## Two repos, one release

This repo is the app half of a two-repo split:

- [`AessemOps/Naslos-Linux`](https://github.com/AessemOps/Naslos-Linux) owns the
  declarative install artifacts and publishes a versioned **install pack**
  (`naslos-install-pack-<version>.tar.gz`) as a GitHub release.
- This repo resolves the **newest `vX.Y.Z` tag** of Naslos-Linux, downloads and
  checksum-verifies that pack at build time, and `go:embed`s it
  (`scripts/fetch-install-pack.sh`). Naslos-Linux dispatches a rebuild here when
  it publishes a new pack, so a released pack always yields a released
  installer.

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

## Desktop app

`desktop/` is the Tauri v2 shell around the engine with a Svelte 5 wizard:
inputs, confirmation, a progress bar and log, the 2FA/QR handoff, and the
recovery ZIP. The shell spawns `naslos-install` as a sidecar and streams its
NDJSON progress to the window. See [`desktop/README.md`](desktop/README.md).

```bash
make fetch-latest-pack            # install pack the engine embeds
./scripts/build-sidecar.sh        # build the engine the shell spawns
cd desktop && npm install
npm run tauri dev                 # the desktop window
npm run dev                       # browser preview (simulated engine)
```

The sidecar build reads `installpack/metadata.json` to pin the Talos version
and schematic, so fetch the pack before the first build on a fresh checkout.

## Headless engine

```bash
make check                 # go vet + go test -race ./...
make fetch-latest-pack     # newest Naslos-Linux vX.Y.Z pack -> installpack/
make fetch-pack PACK_VERSION=0.1.0   # or pin an explicit version
make build                 # dist/naslos-install (Talos/schematic from the pack)
```

A dry run validates the inputs and prints the plan without touching the node:

```bash
read -rs NASLOS_ADMIN_PASSWORD && export NASLOS_ADMIN_PASSWORD
dist/naslos-install \
  --node-ip 192.0.2.10 --domain naslos.example.com \
  --admin-user admin --install-disk /dev/vda \
  --dry-run
unset NASLOS_ADMIN_PASSWORD
```

### Choose the install disk

The engine never infers the disk. List the node's whole disks first (this only
needs the node IP; nothing is written):

```bash
dist/naslos-install --node-ip 192.0.2.10 --list-disks
```

Then pass the chosen device as `--install-disk` and repeat it with
`--confirm-wipe` on the real run. Read-only devices and CD-ROMs are marked as
non-candidates in the listing.

The engine also detects the node's real network interface (the link that carries
the node IP) and substitutes it into the pack's `{{INTERFACE}}` placeholder.
This is `eth0` on a VM and `ens3`/`enp…` on hardware. Hardcoding `eth0` leaves
etcd without an advertised address and fails the bootstrap.

### Resume an interrupted install

`state.json` records each milestone. If the engine stops after the node is
installed (a failed Helm deploy, a lost connection, a cancelled run), re-run it
with the **same inputs and state directory** plus `--resume`:

```bash
read -rs NASLOS_ADMIN_PASSWORD && export NASLOS_ADMIN_PASSWORD
dist/naslos-install \
  --node-ip 192.0.2.10 --domain naslos.example.com \
  --admin-user admin --admin-password-stdin \
  --install-disk /dev/vda --confirm-wipe /dev/vda --resume
unset NASLOS_ADMIN_PASSWORD
```

Resume verifies the persisted `talosconfig` actually authenticates to the node
before touching it, then skips the steps already recorded as done
(bootstrapping, storage, certificate stack, Helm, administrator, TOTP).
Administrator creation is idempotent: an existing exact uid is not POSTed
again. A state that never reached an installed node is refused, as is re-running
an installed state without `--resume`. `--admin-password-stdin` /
`NASLOS_ADMIN_PASSWORD` is only required while the `admin` step is pending, so
a resume past it needs no throwaway value.

Resume also **converges** the release: when the deployed chart version is older
than the pack's, the engine re-runs the Helm step with the new pack's values and
its recomputed overrides (the release stores merged values, so it resets them).
This is how an installed node gains a step added by a later release (for
example a 0.2.5 install resumes once on 0.2.6 to get cert-manager and the OVH
webhook), and it is the **bootstrap path from 0.2.x to the first
web-upgradable 0.3.0 release**:

```bash
read -rs NASLOS_ADMIN_PASSWORD && export NASLOS_ADMIN_PASSWORD
dist/naslos-install --resume --node-ip 192.0.2.10 --domain naslos.example.com \
  --admin-user admin --install-disk /dev/vda --confirm-wipe /dev/vda
unset NASLOS_ADMIN_PASSWORD
```

The machine config is generated with the pack's explicit `kubernetesVersion`
(never the Talos machinery default), so an upgraded node and a fresh install
target the same Kubernetes minor.

### Re-key a node (reinstall)

If the saved state describes a completed install but the node is back in
maintenance mode (re-imaged, wiped, or a different node at the same address),
the engine refuses to guess. Pass `--reinstall`: it moves the engine's state
files into `<state-dir>/stale-<timestamp>/` and starts a fresh install with a
new PKI. Nothing is deleted, so the old kubeconfig and talosconfig remain
available under that backup directory.

```bash
read -rs NASLOS_ADMIN_PASSWORD && export NASLOS_ADMIN_PASSWORD
dist/naslos-install \
  --node-ip 192.0.2.10 --domain naslos.example.com \
  --admin-user admin --admin-password-stdin \
  --install-disk /dev/vda --confirm-wipe /dev/vda --reinstall
unset NASLOS_ADMIN_PASSWORD
```

The desktop offers the same as a **Start over** button on the error screen when
it sees this state/node mismatch.

## CI and releases

- `ci.yml` runs `make check` on every push and PR, fetches the newest
  Naslos-Linux `vX.Y.Z` pack and builds against it, type-checks and builds the
  wizard, and `cargo check`s the Tauri shell.
- `release.yml` builds `linux`/`darwin`/`windows` (amd64 + arm64), packages each
  binary with per-file sha256 checksums, and publishes a GitHub release. It runs
  on an installer `v*` tag, on manual dispatch (with an optional
  `naslos_version` input), and on the `naslos-release` repository dispatch that
  Naslos-Linux sends when it publishes a pack. Every build embeds the newest
  Naslos-Linux `vX.Y.Z` tag and derives `ExpectedTalosVersion` and
  `ExpectedSchematicID` from that pack, so the FR-INSTALL-02 gate always
  matches.
- The same workflow builds the desktop bundles next to the engine binaries:
  AppImage and `.deb` on Linux amd64, `.app.tar.gz` and `.dmg` on macOS arm64,
  and an NSIS setup `.exe` on Windows amd64. It does not produce `.rpm` or
  `.msi` packages.

## Status

Implemented: inputs and validation, pack load with checksum verification and a
version gate, machine-config rendering, node preflight, resumable state, NDJSON
progress and `data` payloads, Talos PKI plus control-plane/talosconfig
generation via `siderolabs/talos/pkg/machinery` (persisted secrets bundle;
reuse never re-keys a node), the full Talos lifecycle (maintenance apply, wait,
bootstrap, kubeconfig), the cluster storage step (startup CNI wait, local-path
provisioner server-side apply, PodSecurity label, default StorageClass), the
certificate stack (cert-manager CRDs applied server-side, cert-manager and the
OVH DNS-01 webhook installed from the pack into `cert-manager`, both waited on),
the Helm install of the pack's chart (values merge, engine overrides,
pre-created `naslos` and `naslos-talosconfig` Secrets, explicit workload waits),
the first administrator and TOTP device (owner API via pod exec,
`authelia … totp generate`), a best-effort hosts entry, the recovery ZIP, and
the desktop GUI (Tauri v2 shell plus Svelte wizard, verified against a simulated
engine in a browser).

The generated control plane matches `talosctl gen config` on the pack patch
(install image, Cilium inline manifest, kube-proxy/flannel disabled, host DNS,
`KubeNodeConfig`). The whole path through admin and TOTP was live-validated on
2026-10-03: Talos node Ready, Naslos pods Running, `/authelia/` returning 200,
and an Authelia first-factor plus CLI-generated TOTP login returning 200. The
engine completes to the `done` event.

## License

GNU Affero General Public License v3.0 (see Naslos-Linux `LICENSE`).
