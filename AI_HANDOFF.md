# AI Handoff — Naslos Installer

Session-to-session state for `AessemOps/Naslos-Installer`. Companion repo:
`AessemOps/Naslos-Linux` (publishes the install pack). Contract:
[`Naslos-Linux/docs/installer-contract.md`](https://github.com/AessemOps/Naslos-Linux/blob/master/docs/installer-contract.md).
Plan: [`.kilo/plans/desktop-installer-app.md`](.kilo/plans/desktop-installer-app.md).
Spec: `Naslos-Linux/docs/spec.md` §FR-INSTALL.

## What this is

The desktop installer (Tauri v2 shell + Svelte wizard) plus the headless Go
engine `naslos-install` that provisions a Talos node booted from the Naslos ZFS
ISO. The engine is a standalone binary so it runs headless and bundles as a
Tauri sidecar. Nothing here touches a node without the user's inputs.

## Repo state (2026-09-27)

- `main` @ `f9fb079` (first commit — repo bootstrap).
- **Open PR [#1](https://github.com/AessemOps/Naslos-Installer/pull/1)**,
  branch `feat/engine-talos-config` (`main`..`823267e`):
  - `35287b4` `feat(engine): generate Talos PKI + machine config from the pack`
  - `391f225` `feat(engine): Talos lifecycle client (apply/bootstrap/kubeconfig)`
  - `876e5ee` `chore(engine): drop I3 work-in-progress and tidy modules`
  - `823267e` `feat(engine): apply cluster storage (local-path) after bootstrap`
- Working tree clean. `go build ./...`, `go vet ./...`, `go test -race ./...`
  all pass (`make check`).

## Live drill — 2026-10-03 (validated)

The engine was run against a **freshly booted maintenance Talos VM**
(`192.168.1.117`, Talos v1.14.0 maintenance; VNC console on `192.168.1.2:5900`)
with the published `v0.1.0` pack:

```
./dist/naslos-install --node-ip 192.168.1.117 --domain naslos.local \
  --admin-user admin --admin-password … --state-dir /tmp/naslos-drill-state
```

Steps config → pack → preflight → machine-config → talos-config →
talos-install → bootstrap → kubeconfig → cni → storage all succeeded. Verified
with the fetched kubeconfig: node `talos-6r3-5y2` **Ready** at Talos v1.14.1 /
k8s v1.37.0, Cilium + CoreDNS Running, StorageClass `local-path (default)`,
`local-path-provisioner` 1/1 Running, namespace `local-path-storage` labeled
`pod-security.kubernetes.io/enforce=privileged`.

**I4 (Helm) then validated against the same cluster** (branch
`feat/engine-helm-install`): `internal/helm` extracted the pack chart, merged
`values.yaml` → `values-installer.yaml` → engine overrides, pre-created the
`naslos` namespace + `naslos-talosconfig` Secret, and installed release `naslos`
(Helm revision 2, `deployed`). Live result: all pods Running, `/` → 401
redirect to Authelia, `/authelia/` → 200, `/api/health` → 200. Two SDK details
mattered: `SkipSchemaValidation` (the authelia subchart's schema has an offline
external `$ref`) and **not** using Helm `Wait` — the OpenLDAP bootstrap is a
post-install hook, so waiting on Authelia before the hook runs deadlocks; the
engine now runs the hook and waits for the workloads itself. The engine then
stops at the `admin` step (I5).

**Bug found and fixed in the same drill:** `//go:embed *` in
`installpack/embed.go` silently drops files whose names begin with `_` or `.`,
so Helm's `charts/naslos/templates/_helpers.tpl` was missing from the embedded
pack and every real install failed at "checksum lists a missing member". The
directive is now `//go:embed all:*`, with `TestEmbeddedPackVerifies` (runs when
a pack is present; wired into the ci.yml pack-build job). Note the drill
surfaced this only because the pack was actually embedded and verified end to
end — the unit tests use a synthetic FS.

Minor observation (not fixed): the local-path manifest's Deployment is applied
before the namespace PSA label, so the first apply logs a non-fatal
`would violate PodSecurity "restricted"` warning. Sequential (namespace → label
→ workload) apply would silence it.

### Implemented and verified

| Package | What | Verified by |
| --- | --- | --- |
| `internal/config` | install inputs + validation (IPv4, domain, NetBIOS name, Authelia password policy), derivations (`ShortName`, `Subnet24`, `AutheliaURL`) | unit tests |
| `internal/event` | NDJSON progress protocol (`step/status/pct/msg`, terminal `done`, `error`) | unit tests |
| `internal/installpack` | parse `metadata.json`, sha256-verify every member, refuse talosVersion/schematicId skew, render `{{NODE_SUBNET}}`/`{{INSTALL_DISK}}`, read members, extract | unit tests + real `0.1.0` pack |
| `internal/otpauth` | parse Authelia's `otpauth://` output (base32 + >20-byte secret) | unit tests |
| `internal/preflight` | classify node: `:6443` => installed, else `:50000` => maintenance | unit tests |
| `internal/state` | `state.json` (0600), resumable step status | unit tests |
| `internal/talosconfig` | persist/reuse Talos secrets bundle (never re-key), generate control plane from the pack patch via `pkg/machinery`, skip `UnattendedInstallConfig`, fill talosconfig endpoints | unit tests + parity vs `talosctl gen config` |
| `internal/talosclient` | maintenance-mode client: apply / wait-for-API / bootstrap / kubeconfig / service states; already-bootstrapped = success | unit tests (fake API) |
| `internal/k8s` | server-side apply of pack manifests (discovery-backed dynamic client), namespace label merge, default StorageClass annotation, Deployment/DaemonSet ready waits | unit tests (recording dynamic client) |
| `desktop/` | Tauri v2 shell + Svelte 5 wizard: sidecar spawn/stream/cancel, input → confirm → progress/log → 2FA/QR + recovery-ZIP handoff, error/retry; browser preview with a simulated engine | svelte-check + vite build; `cargo build`; wizard driven end-to-end in a headless browser |

`cmd/naslos-install` runs: validate → load/verify pack → preflight (skipped in
dry-run) → render machine config → generate Talos config → maintenance Dial →
Apply → authenticated Dial → Bootstrap → services → kubeconfig → wait for the
Cilium DaemonSet → apply local-path + PSA label + default StorageClass →
provisioner ready, streaming NDJSON. `--dry-run` stops after config generation;
the real flow runs through the Helm install and fails closed at the `admin` step.

### Not implemented / not wired

- The Talos + Kubernetes lifecycle **was live-drilled on 2026-10-03** (see the
  drill section above) through the Helm install; admin/TOTP, resolver and the
  recovery ZIP are still to do, so the real flow fails closed at `admin`.
- The desktop GUI (I7) and CI (I8a) are done — see below.
- The GUI is **not launched on a real display** in this environment and **not
  bundled per OS** (I8b). It was verified by building the Tauri app and driving
  the built wizard with a headless browser; the engine it drives now reaches the
  Naslos deploy, then stops at `admin` (no handoff screen yet).
- The Traefik CRDs are **not** applied by the engine: they ship in the traefik
  subchart's `crds/` directory and Helm installs them before the chart's Traefik
  custom resources (confirmed with `helm template --include-crds`). Re-check if
  the subchart's CRD packaging ever changes.

## Desktop GUI (I7, added 2026-09-29)

- `desktop/` is the Tauri project root; `desktop/ui/` is the Svelte 5 + Vite +
  Tailwind wizard and `desktop/src-tauri/` is the Rust shell. `desktop/README.md`
  has the dev/build commands.
- The shell spawns the engine as the `naslos-install` sidecar
  (`scripts/build-sidecar.sh` writes `src-tauri/binaries/naslos-install-<triple>`),
  emits `install://stdout` / `install://stderr` / `install://exit`, and kills it
  on `cancel_install`. It defines every engine argument; the webview never
  supplies a shell string (SEC-1).
- The wizard reads the engine's NDJSON. It uses the optional `data` payload
  (contract §5: `totp` → `otpauth`/`secret`, `archive` → `path`, `done` →
  `loginUrl`) with a `msg`-scan fallback; `internal/event` gained the `Data`
  field and `ProgressData` so the engine can emit it.
- `npm run dev` serves the wizard in a browser with a **simulated** engine for
  UI work without Rust; `npm run tauri dev` opens the real window.
- Verified: `npm run check` + `npm run build` clean; `cargo build` and
  `npm run tauri build -- --no-bundle` succeed; the wizard flow was driven with
  Playwright (screenshots of form → confirm → progress → handoff). Not run on a
  real display and not bundled per OS (I8b).

## CI & release (I8a, added 2026-09-27)

- **`ci.yml`** — `make check` on push/PR; a second job resolves the newest
  Naslos-Linux `vX.Y.Z` tag, fetches that pack and runs `make build`; a third
  (`desktop`) builds/type-checks the wizard and `cargo check`s the Tauri shell.
  If no semver tag exists yet the pack job warns and skips instead of failing.
- **`release.yml`** — on installer `v*` tag, `workflow_dispatch` (optional
  `naslos_version` input) and `repository_dispatch: naslos-release`. It resolves
  the newest Naslos-Linux `vX.Y.Z` tag, fetches + verifies the pack, cross-builds
  linux/darwin/windows (amd64 + arm64) with `make build VERSION=<release tag>`,
  packages tar.gz (zip on Windows) + `checksums.txt`, and publishes a GitHub
  release. Tag-push releases use the pushed tag; dispatch/manual releases use
  `naslos-v<X.Y.Z>`.
- **Always-latest pack.** `scripts/fetch-install-pack.sh` with no `PACK_VERSION`
  resolves the newest `vX.Y.Z` tag (`PACK_TAG` overrides; non-semver tags like
  the current `latest` release are ignored). The **Makefile** reads
  `installpack/metadata.json` and defaults `TALOS_VERSION` / `SCHEMATIC_ID` from
  it, so the FR-INSTALL-02 gate matches the embedded pack; the `v1.14.1` /
  schematic constants are only the no-pack fallback. Targets: `make
  fetch-latest-pack` (auto) and `make fetch-pack PACK_VERSION=x.y.z` (pin).
- **Cross-repo trigger.** `Naslos-Linux/.github/workflows/install-pack.yml` now
  sends a `repository_dispatch` to this repo after attaching the pack. It needs
  the `INSTALLER_DISPATCH_TOKEN` secret (fine-grained PAT with
  `Contents: read and write` / `Actions: write` on `AessemOps/Naslos-Installer`);
  the step is skipped when the secret is absent. **Not yet configured** — create
  the secret in Naslos-Linux to enable automatic rebuilds.
- **Naslos-Linux tags must be semver `vX.Y.Z`.** The only release so far is tag
  `latest` (name "Alpha001") with no assets, so the pack job currently warns and
  skips. Create a `vX.Y.Z` tag to publish a pack and light up both pipelines.

## Spike results (validated 2026-09-27)

- **TOTP (live VM `192.168.1.117`).** In `naslos-authelia-0`,
  `authelia storage user totp generate <uid> --issuer <domain>` works with the
  pod env (`X_AUTHELIA_CONFIG=/configuration.yaml`,
  `X_AUTHELIA_CONFIG_FILTERS=template`, `AUTHELIA_STORAGE_ENCRYPTION_KEY_FILE`),
  prints
  `... with URI 'otpauth://totp/<issuer>:<uid>?algorithm=SHA1&digits=6&issuer=<issuer>&period=30&secret=<BASE32>'`,
  and `... totp delete <uid>` removes it. Verified reversibly with a throwaway
  username; the admin device was untouched. The Authelia image **has `/bin/sh`**
  (the earlier “distroless” note was wrong), but stay binary-direct per SEC-1.
  Portal acceptance of a CLI device is still to confirm in the e2e drill.
- **Machine-config generation (offline, real `0.1.0` pack).**
  `internal/talosconfig` output matches `talosctl gen config` (same patch,
  UIC stripped) for `KubeNodeConfig`, `KubeProxyConfig`, `ResolverConfig`,
  `KubeInlineManifestConfig`, `KubeFlannelCNIConfig` and the machine
  install/kernel/network subset.

## Gotchas (hard-won — read before continuing)

1. **`NewInput`'s 3rd arg is the Kubernetes version, not the Talos version.**
   `generate.NewInput(cluster, endpoint, kubernetesVersion, ...)`. Passing the
   Talos version bakes `registry.k8s.io/kube-proxy:vv1.14.1`. Use
   `constants.DefaultKubernetesVersion` and set the Talos feature set with
   `generate.WithVersionContract(config.ParseContractFromVersion(talosVersion))`.
2. **`gen config` leaves `talosconfig` endpoints empty.** The engine fills
   `contexts[context].Endpoints = [nodeIP]` (mirrors `bootstrap/vm/talosconfig-pod`)
   so the recovery/API copies work without `--endpoints`.
3. **Never regenerate the secrets bundle.** It is persisted to
   `<state-dir>/talos-secrets.json` and reloaded; regenerating orphans an
   installed node (`certificate signed by unknown authority`).
4. **`UnattendedInstallConfig`** is skipped with
   `generate.WithSkipUnattendedInstallConfig(true)` (the Makefile otherwise
   strips it with Python).
5. **`bootstrap`**: treat “already bootstrapped”/gRPC `AlreadyExists` as success,
   but **do not** swallow `unknown authority` (wrong PKI) — see
   `internal/talosclient` tests.
6. **The pack is not published yet.** `make fetch-latest-pack` resolves the
   newest Naslos-Linux `vX.Y.Z` tag; there is none yet (only a non-semver
   `latest` release with no assets), so it fails. Until a `vX.Y.Z` release
   exists, build the pack locally in `Naslos-Linux` (`make install-pack`) and
   point the engine at it with `--pack-dir dist/…` (extract with
   `--strip-components=1`), or `PACK_VERSION=x.y.z make fetch-pack`.

## How to run it now (local)

```bash
# In Naslos-Linux:
make install-pack                       # -> dist/naslos-install-pack-0.1.0.tar.gz
mkdir -p /tmp/pack && tar -xzf dist/naslos-install-pack-0.1.0.tar.gz -C /tmp/pack --strip-components=1

# In Naslos-Installer:
make check
go run ./cmd/naslos-install \
  --node-ip 192.168.1.117 --domain naslos.local \
  --admin-user admin --admin-password 'Correct1' \
  --dry-run --pack-dir /tmp/pack --state-dir /tmp/installer-state
```

## Next steps (in order)

1. **I5** admin + TOTP: `remotecommand.Exec` into `deploy/naslos-terminal`
   (`naslos-privileged`) with `Remote-User`/`Remote-Groups`/
   `X-Naslos-Proxy-Secret` (read `naslos-proxy`) to `POST /api/users`, then
   `authelia storage user totp generate` in `naslos-authelia-0` and parse with
   `internal/otpauth`.
2. **I6** resolver + recovery ZIP; **I7** Tauri shell; **I8b** Tauri per-OS
   bundles (engine release CI, I8a, is done).
3. Create the `INSTALLER_DISPATCH_TOKEN` secret **and** tag Naslos-Linux
   `vX.Y.Z` so the `install-pack` workflow publishes a pack and dispatches the
   installer. Then add image digests to `values-installer.yaml` (NAS-022).

## Conventions / gates

- Every change: `gofmt -l .` clean, `go vet ./...`, `go test -race ./...`
  (`make check`). Add the module to its own audit script when one exists.
- Work on a branch, open a PR against `main`. Conventional Commits.
- Keep the contract in sync: a change to a name/payload/command in
  `Naslos-Linux/docs/installer-contract.md` must update the engine in the same
  release.

## Session caveat

The engine compiles and unit-tests pass. The Talos lifecycle and the storage
step are wired but **no node has been touched**. Treat PR #1 as **not
live-validated** — the next session must run the live drill on a fresh node
before relying on it.
