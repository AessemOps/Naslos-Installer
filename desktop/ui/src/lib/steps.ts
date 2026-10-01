import type { InstallInput } from './types';

// Step ids are the stable part of the progress protocol
// (docs/installer-contract.md §5); the labels mirror the milestones in §6 of
// the desktop-installer plan.
export const STEP_LABELS: Record<string, string> = {
  config: 'Validate inputs',
  pack: 'Verify install pack',
  preflight: 'Check the node',
  'machine-config': 'Generate machine config',
  'talos-config': 'Generate Talos config',
  'talos-install': 'Install Talos',
  bootstrap: 'Bootstrap the cluster',
  kubeconfig: 'Fetch kubeconfig',
  cni: 'Wait for the network (Cilium)',
  storage: 'Install cluster storage',
  helm: 'Deploy Naslos',
  admin: 'Create the administrator',
  totp: 'Set up two-factor',
  resolver: 'Add the local name',
  archive: 'Write the recovery ZIP',
  done: 'Finished',
};

// STEP_ORDER is the display order in the stepper. Unknown steps are appended.
export const STEP_ORDER: string[] = [
  'config',
  'pack',
  'preflight',
  'machine-config',
  'talos-config',
  'talos-install',
  'bootstrap',
  'kubeconfig',
  'cni',
  'storage',
  'helm',
  'admin',
  'totp',
  'resolver',
  'archive',
];

export function stepLabel(step: string | undefined): string {
  if (!step) return '';
  return STEP_LABELS[step] ?? step;
}

// shortName mirrors config.Input.ShortName: the explicit name, else the first
// label of the domain, lowercased and capped at 15 chars (NetBIOS).
export function shortName(name: string, domain: string): string {
  if (name) return name;
  const first = domain.toLowerCase().split('.')[0] ?? '';
  return first.slice(0, 15);
}

export function autheliaURL(domain: string): string {
  return `https://${domain.toLowerCase()}/authelia`;
}

// defaultDomain is the placeholder the form starts with.
export const DEFAULT_DOMAIN = 'naslos.local';

// ISO_URL is the pinned Talos image the wizard points at
// (docs/installer-contract.md §6). A pack release can change the schematic, so
// the engine is the source of truth once it emits the pack's isoUrls; this is
// the value the current pack is built from.
export const ISO_URL =
  'https://factory.talos.dev/image/4dd8e3a8b6203d3c14f049da8db4d3bb0d6d3e70c5e89dfcc1e709e81914f63c/v1.14.1/metal-amd64.iso';

export function emptyInput(): InstallInput {
  return {
    nodeIp: '',
    domain: DEFAULT_DOMAIN,
    name: '',
    adminUser: 'admin',
    adminPassword: '',
    addResolver: false,
  };
}
