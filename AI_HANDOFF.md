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
  branch `feat/engine-talos-config` (`main`..`391f225`):
  - `35287b4` `feat(engine): generate Talos PKI + machine config from the pack`
  - `391f225` `feat(engine): Talos lifecycle client (apply/bootstrap/kubeconfig)`
- Working tree clean. `go build ./...`, `go vet ./...`, `go test -race ./...`
  all pass.

### Implemented and verified

| Package | What | Verified by |
| --- | --- | --- |
| `internal/config` | install inputs + validation (IPv4, domain, NetBIOS name, Authelia password policy), derivations (`ShortName`, `Subnet24`, `AutheliaURL`) | unit tests |
| `internal/event` | NDJSON progress protocol (`step/status/pct/msg`, terminal `done`, `error`) | unit tests |
| `internal/installpack` | parse `metadata.json`, sha256-verify every member, refuse talosVersion/schematicId skew, render `{{NODE_SUBNET}}`/`{{INSTALL_DISK}}`, extract | unit tests + real `0.1.0` pack |
| `internal/otpauth` | parse Authelia's `otpauth://` output (base32 + >20-byte secret) | unit tests |
| `internal/preflight` | classify node: `:6443` => installed, else `:50000` => maintenance | unit tests |
| `internal/state` | `state.json` (0600), resumable step status | unit tests |
| `internal/talosconfig` | persist/reuse Talos secrets bundle (never re-key), generate control plane from the pack patch via `pkg/machinery`, skip `UnattendedInstallConfig`, fill talosconfig endpoints | unit tests + parity vs `talosctl gen config` |
| `internal/talosclient` | maintenance-mode client: apply / wait-for-API / bootstrap / kubeconfig / service states; already-bootstrapped = success | unit tests (fake API) |

`cmd/naslos-install` runs: validate → load/verify pack → preflight (skipped in
dry-run) → render machine config → generate Talos config, streaming NDJSON.
`--dry-run` stops after config generation; the real flow fails closed at
“cluster lifecycle is not implemented in this build”.

### Not implemented / not wired

- `internal/talosclient` is wired into `cmd/naslos-install` (maintenance Dial →
  WaitForAPI → Apply → authenticated Dial → WaitForAPI → Bootstrap →
  WaitForServices(etcd,kubelet) → Kubeconfig), but is **not live-drilled**: the
  network half of spike 2 needs a freshly-booted node.
- Storage/CRDs, Helm install, admin creation, TOTP bootstrap, resolver,
  recovery ZIP, the Tauri shell and CI are all still to do (see the plan). After
  kubeconfig the engine fails closed at the `storage` step.

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
6. **The pack is not published yet.** `make fetch-pack` needs a `v*` release
   with `naslos-install-pack-<version>.tar.gz`. Until then, build the pack
   locally in `Naslos-Linux` (`make install-pack`) and point the engine at it
   with `--pack-dir dist/…` (extract with `--strip-components=1`).

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

1. **I2b live drill**: the client is wired (Dial → apply → authenticated Dial →
   bootstrap → services → kubeconfig). Validate on a freshly-booted node (wipe
   `/dev/vda` from the ISO). Do **not** point it at the installed `192.168.1.117`
   production node.
2. **I3** storage: apply `manifests/local-path-v0.0.26.yaml`, label its
   namespace `privileged`, patch `local-path` as default StorageClass; apply the
   Traefik CRDs.
3. **I4** Helm: install the pack chart with `values-installer.yaml` + overrides
   (domain, `shares.discovery.name`, `openldap.host`, `networkPolicy.*` CIDRs)
   via `helm.sh/helm/v3`; wait on Deployments/StatefulSets.
4. **I5** admin + TOTP: `remotecommand.Exec` into `deploy/naslos-terminal`
   (`naslos-privileged`) with `Remote-User`/`Remote-Groups`/
   `X-Naslos-Proxy-Secret` (read `naslos-proxy`) to `POST /api/users`, then
   `authelia storage user totp generate` in `naslos-authelia-0` and parse with
   `internal/otpauth`.
5. **I6** resolver + recovery ZIP; **I7** Tauri shell; **I8** release CI.
6. When the pack release exists, wire `make fetch-pack` + a CI job and pin the
   pack version; add digests to `values-installer.yaml` (NAS-022).

## Conventions / gates

- Every change: `gofmt -l .` clean, `go vet ./...`, `go test -race ./...`
  (`make check`). Add the module to its own audit script when one exists.
- Work on a branch, open a PR against `main`. Conventional Commits.
- Keep the contract in sync: a change to a name/payload/command in
  `Naslos-Linux/docs/installer-contract.md` must update the engine in the same
  release.

## Session caveat

This session hit tooling issues. The engine compiles and unit-tests pass; the
Talos lifecycle is wired but **no node has been touched**. Treat PR #1 as
**not live-validated** — the next session must run the I2b drill on a fresh
node before relying on it.
