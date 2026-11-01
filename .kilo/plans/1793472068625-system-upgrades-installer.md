# Installer side of the 0.3.0 web upgrades

Companion to `Naslos-Linux/.kilo/plans/1793472068625-web-ui-system-upgrades.md`.
Version parity: this work ships as `naslos-v0.3.0` with the Naslos-Linux
`v0.3.0` pack.

## Changes

1. **Pack Kubernetes pin** — `installpack.Metadata` reads the additive
   `kubernetesVersion`. `talosconfig.Generate` passes it explicitly to
   `generate.NewInput` (stripping the leading `v`, which Talos adds), falling
   back to the machinery default only for pre-0.3.0 packs. This is what makes
   a fresh install and a web upgrade target the same Kubernetes minor.
2. **Resume convergence** — `helm.DeployedVersion` reads the deployed chart
   version. On `--resume`, when it is older than the pack's `chartVersion`, the
   engine re-runs the Helm step with the new pack (and `ResetValues`, so the
   release's old merged image digests are replaced). This is the bootstrap path
   from 0.2.x nodes to the first web-upgradable release.
3. **Conditional admin password** — `Input.Validate` allows an empty password;
   the engine requires it only while the `admin` step is pending, so a resume
   past that step needs no throwaway value.
4. **Docs** — README (resume/upgrade path and the Kubernetes pin),
   `AI_HANDOFF.md`.

## Tests

`make check` (go vet + `go test -race ./...`), including
`installpack.ChartVersionLess` and the empty-password validation case.
