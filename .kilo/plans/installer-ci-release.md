# Installer CI + release pipeline

Repo: `AessemOps/Naslos-Installer`.
Related: `Naslos-Linux/.github/workflows/install-pack.yml` (publishes the pack),
`Naslos-Linux/docs/installer-contract.md` §1, plan `desktop-installer-app.md` I8.

## Goal

Build and release the installer from CI, always embedding the **newest
`vX.Y.Z` tag of Naslos-Linux** (not a hardcoded pin), and rebuild automatically
when Naslos-Linux publishes a new release.

## Decisions

- **Pack resolution**: resolve the highest `vX.Y.Z` git tag on
  `AessemOps/Naslos-Linux` and fetch
  `releases/download/<tag>/naslos-install-pack-<version>.tar.gz` (+ `.sha256`).
  Non-semver tags (the current `latest` release) are ignored.
- **Auto-rebuild**: `Naslos-Linux`'s `install-pack` workflow sends a
  `repository_dispatch` (`event_type: naslos-release`) to this repo after it
  attaches the pack, so a fresh pack produces a fresh installer release with no
  manual step. Requires an `INSTALLER_DISPATCH_TOKEN` secret with
  `contents:write` (or `Actions:write`) on `Naslos-Installer`; the step is
  skipped when the secret is absent.
- **Engine identity**: the build derives `ExpectedTalosVersion` /
  `ExpectedSchematicID` from the fetched pack's `metadata.json`, so the
  FR-INSTALL-02 gate always matches the embedded pack.

## Files

- `scripts/fetch-install-pack.sh` — resolve the newest semver tag when
  `PACK_VERSION` is unset (`PACK_TAG` override).
- `Makefile` — `fetch-latest-pack`; `TALOS_VERSION`/`SCHEMATIC_ID` default from
  `installpack/metadata.json` when present.
- `.github/workflows/ci.yml` — `make check` on push/PR; best-effort
  fetch+build with the latest pack.
- `.github/workflows/release.yml` — on installer `v*` tag, `workflow_dispatch`
  and `repository_dispatch`: fetch the latest pack, cross-compile
  linux/darwin/windows × amd64/arm64, package + checksum, publish a release.
- `Naslos-Linux/.github/workflows/install-pack.yml` — dispatch this repo.
- Docs: installer `README.md`, `AI_HANDOFF.md`, plan I8;
  `Naslos-Linux/docs/installer-contract.md` §1 and `docs/deployment.md`.

## Release naming

- Installer `v*` tag push → release that tag.
- `workflow_dispatch` / `repository_dispatch` → release tag
  `naslos-v<X.Y.Z>` (one per embedded Naslos version), marked latest.
