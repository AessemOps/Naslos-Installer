# Engineer: provision cert-manager + the OVH webhook from the install pack

Companion plan: `Naslos-Linux/.kilo/plans/1793555100000-installer-cert-manager-pack.md`
(pack contents, chart profile, contract §2.1). That change shipped as pack 0.2.5;
this repo consumes it.

## Change

- `cmd/naslos-install/certmanager.go`: new `cert-manager` step between `storage`
  and `helm`. Applies `manifests/cert-manager-crds.yaml` server-side, installs
  `charts/naslos/charts/cert-manager-*.tgz` with `crds.enabled=false`, installs
  `charts/cert-manager-webhook-ovh-*.tgz` with `groupName=ovh.naslos.local` plus
  the cert-manager namespace/SA, then waits for the four deployments. Chart
  versions are discovered from the pack, never hardcoded; `singleChart` fails on
  zero or many matches.
- Packs without the material (≤0.2.4) skip the step and record it as done, so an
  older pack cannot fail an install.
- `main.go` wires the step with the usual `StepDone` resume guard; interrupted
  runs converge. `steps.ts` gains the label for the desktop stepper.

## Verification

- `certmanager_test.go`: pack detection with/without the CRD manifest, and
  `singleChart` zero/one/many.
- Live drill (2026-11-01): resumed the 0.2.5 install on 192.168.1.117; both
  releases `deployed`, 4 deployments Ready, 6 CRDs, webhook RBAC in
  `naslos-apps`, install completed to `done` with no re-key. Recorded in
  `AI_HANDOFF.md`.

## Open follow-up

- `--admin-password` is required by input validation even when the admin step is
  done, so a resume needs a throwaway value.
