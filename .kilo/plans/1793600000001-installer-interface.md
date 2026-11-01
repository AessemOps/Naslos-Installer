# Installer NIC detection for 0.3.1

Status: implemented · Target release: **0.3.1** (version parity with the
Naslos-Linux `v0.3.1` pack)
Repos: `Naslos-Installer` (engine) + `Naslos-Linux` (pack template/contract).

## Problem found on the 0.3.0 live install

The 0.3.0 pack's machine-config template hardcoded `interface: eth0`. The
physical node's NIC is `ens3`, so the configured link did not exist. Talos
applied the config but etcd failed its pre-stage with:

```
failed to build initial etcd cluster: failed to build cluster arguments:
1 error(s) occurred: timeout
```

The installer then timed out waiting for `etcd`/`kubelet` ("the control plane
did not become ready"). Applying the same config with `interface: ens3` and
running `talosctl bootstrap` made the control plane healthy, confirming the
cause. The VM still uses `eth0`; the failure only appears on hardware with
predictable interface names.

## Fix

- The pack template uses `{{INTERFACE}}` instead of `eth0`.
- The engine detects the interface from the maintenance-mode Talos COSI
  resources: the `AddressStatus` whose address equals the node IP, falling back
  to the first physical `LinkStatus` that is up. It substitutes that value into
  `RenderMachineConfig`.
- Detection is skipped for `--dry-run` and for a resumed node whose bootstrap
  step is already done (the config is not applied again). A failure is a clear
  machine-config error rather than silently writing the wrong link.
- `docs/installer-contract.md` documents the third placeholder; the
  `render-installer-template.sh` placeholder guard includes it.
- The desktop Retry button now passes `--resume`. It previously re-ran the
  engine without it, so even after fixing the node the retry failed preflight
  with "node looks already installed". The Tauri command takes an explicit
  `resume` flag, and the wizard sets it only from the error screen's Retry.

## Tests

- `internal/talosclient`: the `API` interface and fake implement `Interface`.
- `internal/installpack`: the render test covers `{{INTERFACE}}`.
- `cmd/naslos-install`: dry-run still writes a machine config without dialing.
- Live check against the installed node: `Interface("192.168.1.117")` returned
  `ens3` through the authenticated Talos client.
- `make check` (vet + race tests) and a rebuilt engine/AppImage before release.

## Release order

1. Merge this engine change with the Naslos-Linux `{{INTERFACE}}` pack template.
2. Bump both repos to 0.3.1 and release the pack.
3. The release workflow dispatches the installer build; publish
   `naslos-v0.3.1` only after the 0.3.1 pack exists.

## Recovery already applied to the failed node

The node was manually recovered by applying the corrected `ens3` config,
bootstrapping etcd, fetching the kubeconfig, and marking the installer's
`bootstrap` step done. The 0.3.0 GUI can be retried to finish storage/cert-manager/
Helm; the 0.3.1 installer will do this without manual intervention.
