# Naslos-Installer — implementation plan

Repo: `AessemOps/Naslos-Installer` (this repo).
Companion plan: `Naslos-Linux/.kilo/plans/1790466900440-desktop-installer-app.md`.
Contract (stable interfaces): `Naslos-Linux/docs/installer-contract.md`.
Spec: `docs/spec.md` §FR-INSTALL.

The desktop installer provisions a Talos node booted from the Naslos ZFS ISO end
to end, with a progress bar, first-admin + 2FA handoff and a recovery ZIP. The
engine is a standalone Go binary (`naslos-install`) so it can run headless and be
bundled as a Tauri sidecar.

## Increments

- [x] **I1 — Engine foundation.** Module; `internal/{config,event,installpack,
  otpauth,preflight,state}`; `cmd/naslos-install` with NDJSON progress; pack
  load + checksum verification + Talos/schematic version gate; machine-config
  rendering; resumable `state.json` (0600); `scripts/fetch-install-pack.sh` +
  Makefile. Dry-run verified against the real `0.1.0` pack.
- [x] **I2a — Talos config generation.** `internal/talosconfig`: PKI/secrets bundle
  persisted and reused (never re-key), control-plane config generated with
  `siderolabs/talos/pkg/machinery` from the pack patch, `UnattendedInstallConfig`
  skipped, talosconfig endpoints filled. Verified against `talosctl gen config`
  on the real pack patch (all key docs MATCH). Note: `NewInput`'s third arg is
  the *Kubernetes* version; the Talos feature set comes from the version
  contract.
- [x] **I2b — Talos lifecycle client.** `internal/talosclient`:
  maintenance-mode apply, wait-for-API, authenticated switch, bootstrap
  (already-bootstrapped = success, auth errors not swallowed), service wait,
  kubeconfig. Wired into `cmd/naslos-install`. **Live drill still pending** on a
  freshly-booted node (not the installed `.117`).
- [x] **I3 — Cluster storage + CRDs.** `internal/k8s`: server-side apply of the
  pack's pinned local-path manifest, label its namespace `privileged`, patch
  `local-path` as default StorageClass, wait for the provisioner Deployment, and
  wait for the Cilium DaemonSet before scheduling workloads. The Traefik CRDs are
  **not** applied by the engine: they ship in the traefik subchart's `crds/`
  directory and Helm installs them before the chart's Traefik custom resources
  (verified with `helm template --include-crds`). Wired into `cmd/naslos-install`;
  unit-tested with a recording dynamic client (the client-go dynamic fake cannot
  server-side apply unstructured objects).
- [x] **I4 — Helm install (live-validated 2026-10-03).** `internal/helm` installs
  the pack's chart with `values.yaml` → `values-installer.yaml` → engine
  overrides (domain, discovery name, CIDRs, `openldap.host`) via
  `helm.sh/helm/v3`, and the engine pre-creates `naslos` + the
  `naslos-talosconfig` Secret before the install. Two SDK details found live:
  the authelia subchart's schema has an offline external `$ref` (so
  `SkipSchemaValidation`), and Helm `Wait` must stay **off** because the
  OpenLDAP bootstrap is a post-install hook Authelia depends on (waiting
  deadlocks); the engine runs the hook then waits for OpenLDAP, Authelia, API and
  UI itself. Deployed live: release `naslos` `deployed`, all pods Running, `/`
  → 401 to Authelia, `/authelia/` 200, `/api/health` 200.
  Notes from the VM install path (`Naslos-Linux/scripts/deploy-vm.sh`,
  `make install-vm`):
  - **CRDs.** The VM path runs `make crds` (`helm show crds traefik | kubectl
    apply`, cert-manager rendered from its `templates/crds.yaml`) and then passes
    `--skip-crds`. The install pack ships **no** CRDs, so the engine lets Helm
    install the traefik subchart's `crds/` (it does **not** pass `--skip-crds`).
    `certManager.enabled` / `ovhWebhook.enabled` are `false` in
    `values-installer.yaml`.
  - **`naslos` namespace + `naslos-talosconfig`.** The chart has no template for
    the Secret `api.talosConfigSecret` mounts, so the engine creates the
    `naslos` namespace and the `naslos-talosconfig` Secret from the generated
    talosconfig (endpoints already filled) **before** the Helm install, and
    adopts the pre-created namespace (`TakeOwnership`, mirroring deploy-vm.sh).
  - **Helm version.** The repo CLI is Helm 4 while the Go SDK used is
    `helm.sh/helm/v3` v3.18.5. Pinned deliberately; the v3 SDK's client-side
    apply is what the live validation exercised.
- [x] **I5 — Bootstrap admin + TOTP (live-validated 2026-10-03).**
  `internal/bootstrap` execs `curl` into `deploy/naslos-terminal` to
  `POST /api/users` (owner headers + the `naslos-proxy` Secret's `secret` key),
  verifies with `GET /api/users`, then execs `authelia storage user totp
  generate <uid> --issuer <domain>` in `naslos-authelia-0` and parses the URI
  with `internal/otpauth`. `internal/k8s` gained SPDY `Exec`, `SecretValue` and
  `PodForDeployment`. Live: admin created, TOTP device generated, and the
  spike passed — `POST /authelia/api/firstfactor` (LDAP password) → 200 and
  `POST /authelia/api/secondfactor/totp` (code from the CLI-generated secret) →
  200 with a redirect, so no web enrolment is needed. The engine then fails
  closed at the `resolver` step.
- [x] **I6 — Resolver + recovery ZIP.** `internal/resolver` upserts/removes a
  single marker-owned hosts line (`# naslos-installer`) and writes `/etc/hosts`,
  returning a permission error the engine turns into a shown fallback line.
  `internal/archive` builds `naslos-recovery-<domain>-<stamp>.zip`
  (talosconfig, controlplane, talos-secrets.json, kubeconfig,
  schematic/naslos.yaml, ISO.md, README.txt) — no admin password, with the
  buddy-identity KEK warning. The engine finishes with a `done` event carrying
  `data.loginUrl`. Live: the ZIP built from the real drill state dir with all
  members. Elevation for the hosts write (pkexec/sudo/osascript/UAC) is
  best-effort by design; the fallback line is always shown.
- [x] **I7 — Tauri v2 shell + Svelte wizard.** `desktop/` (Tauri root) with
  `src-tauri/` (spawns the `naslos-install` sidecar, forwards NDJSON as
  `install://stdout`/`stderr`/`exit`, kills on `cancel_install`) and `ui/`
  (Svelte 5 + Vite + Tailwind: input → confirm → progress bar + log + cancel →
  2FA/QR + recovery-ZIP handoff; error screen with retry). Browser preview runs
  a simulated engine. `scripts/build-sidecar.sh` builds the sidecar; `ci.yml`
  has a `desktop` job (svelte-check + build + `cargo check`). Plan:
  `desktop-gui.md`. **Not yet bundled per OS (I8b) and not launched on a real
  display (no X server in this environment) — the wizard was verified by
  driving the built UI with a headless browser.**
- [x] **I8a — Engine release CI.** `.github/workflows/ci.yml` (`make check` +
  pack build) and `release.yml` (cross-build linux/darwin/windows amd64+arm64,
  per-file sha256, GitHub release). Builds always embed the newest Naslos-Linux
  `vX.Y.Z` pack: `scripts/fetch-install-pack.sh` resolves the tag, and the
  Makefile derives `ExpectedTalosVersion`/`ExpectedSchematicID` from the pack's
  `metadata.json`. Naslos-Linux's `install-pack` workflow sends a
  `repository_dispatch` (`naslos-release`) so a new pack rebuilds the installer
  automatically. Plan: `installer-ci-release.md`.
- [ ] **I8b — Tauri bundles.** Run `tauri build` per OS and attach
  AppImage/deb/rpm, .dmg/.app and .msi/.exe alongside the engine binaries.

## Spike results

- **TOTP (2026-09-27, live VM):** `authelia storage user totp generate <uid>
  --issuer <domain>` runs in `naslos-authelia-0` (config + encryption key
  auto-resolved from the pod env), prints
  `Successfully generated TOTP configuration for user '<uid>' with URI
  'otpauth://totp/<issuer>:<uid>?...&secret=<BASE32>'`, and `... totp delete
  <uid>` removes it. Verified reversibly with a throwaway username; the admin's
  device was untouched. Primary mechanism = CLI generate + parse URI + QR.
  Fallback = portal enrolment + read the elevated-session code from
  `/config/notification.txt`. Portal *acceptance* of a CLI-created device is
  confirmed in the end-to-end drill.
- **Machine-config generation (2026-09-27, offline):** `internal/talosconfig`
  renders the real `0.1.0` pack patch through `pkg/machinery` and the output
  matches `talosctl gen config` (with the same patch) for `KubeNodeConfig`,
  `KubeProxyConfig`, `ResolverConfig`, `KubeInlineManifestConfig`,
  `KubeFlannelCNIConfig` and the machine install/kernel/network subset. The
  insecure `apply-config` + `bootstrap` half of spike 2 still needs a live node.

## Constraints (from the contract)

- Never regenerate Talos PKI against an installed node.
- Engine defines all command arguments; never expose arbitrary shell input (SEC-1).
- `--json-progress` NDJSON on stdout is the only shell↔engine coupling.
- Record `naslosVersion` in the recovery README; refuse version-skewed packs.
