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
- [ ] **I4 — Helm install.** Install the pack's chart with `values-installer.yaml`
  + engine overrides (domain, discovery name, CIDRs, `openldap.host`) via
  `helm.sh/helm/v3`; wait on Deployments/StatefulSets. Notes from the VM install
  path (`Naslos-Linux/scripts/deploy-vm.sh`, `make install-vm`):
  - **CRDs.** The VM path runs `make crds` (`helm show crds traefik | kubectl
    apply`, cert-manager rendered from its `templates/crds.yaml`) and then passes
    `--skip-crds`. The install pack ships **no** CRDs, so the engine must either
    let Helm install the traefik subchart's `crds/` (i.e. do **not** pass
    `--skip-crds`) or pre-apply them itself. `certManager.enabled` /
    `ovhWebhook.enabled` are `false` in `values-installer.yaml`.
  - **`naslos` namespace + `naslos-talosconfig`.** The chart has no template for
    the Secret `api.talosConfigSecret` mounts, so the engine must create the
    `naslos` namespace and the `naslos-talosconfig` Secret from the generated
    talosconfig (endpoints already filled) **before** the Helm install, then
    adopt the pre-created namespace (`--take-ownership`, mirroring deploy-vm.sh).
  - **Helm version.** The repo CLI is Helm 4 (server-side apply; that is what the
    `--force-conflicts`/`--take-ownership` comments refer to) while the Go module
    cache has `helm.sh/helm/v3` v3.18.5. Pick one deliberately and pin it; do not
    assume the v3 SDK's apply semantics match Helm 4.
- [ ] **I5 — Bootstrap admin + TOTP.** Exec `curl` into `deploy/naslos-terminal`
  to `POST /api/users` (owner headers from the `naslos-proxy` Secret), verify
  with `GET /api/users`; exec `authelia storage user totp generate <uid> --issuer
  <domain>` in `naslos-authelia-0`, parse the `otpauth://` URI
  (`internal/otpauth`), render a QR.
- [ ] **I6 — Resolver + recovery ZIP.** Best-effort elevated hosts entry per OS
  (marked line, always show the fallback record); recovery ZIP with
  talosconfig/controlplane/secrets bundle/kubeconfig/schematic/ISO.md/README.txt
  (no admin password; warn that `buddy-identity.json` is the backup KEK).
- [ ] **I7 — Tauri v2 shell + Svelte wizard.** Input → confirm → progress/log →
  2FA/QR → ZIP/first-login; sidecar wiring; per-OS bundles (AppImage + deb/rpm,
  .dmg/.app, .msi/.exe).
- [ ] **I8 — Release CI.** Build the engine per OS, run `tauri build`, attach
  bundles; fetch + pin the install pack version.

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
