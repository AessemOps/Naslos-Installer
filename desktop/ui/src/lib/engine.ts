import { invoke } from '@tauri-apps/api/core';
import { listen, type UnlistenFn } from '@tauri-apps/api/event';
import type { EngineEvent, ExitPayload, InstallInput, RunHandlers } from './types';

// The engine is a Tauri sidecar that writes newline-delimited JSON to stdout.
// In a plain browser (`npm run dev`) there is no sidecar, so a simulated stream
// drives the wizard instead; it is clearly labelled in the UI. isTauri()
// distinguishes the two.

export function isTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
}

// parseLine turns one stdout line into an event, or null when it is not JSON
// (e.g. a panic or a plain log line), so the caller can route it to the log.
export function parseLine(line: string): EngineEvent | null {
  const trimmed = line.trim();
  if (!trimmed || trimmed[0] !== '{') return null;
  try {
    const value = JSON.parse(trimmed) as EngineEvent;
    return typeof value === 'object' && value !== null ? value : null;
  } catch {
    return null;
  }
}

// runInstall starts the install and returns a cancel function. Cancelling kills
// the engine (Tauri) or stops the simulation (browser).
export async function runInstall(input: InstallInput, handlers: RunHandlers): Promise<() => void> {
  if (isTauri()) {
    return runTauri(input, handlers);
  }
  return runSimulated(input, handlers);
}

async function runTauri(input: InstallInput, handlers: RunHandlers): Promise<() => void> {
  const unlisten: UnlistenFn[] = [];
  unlisten.push(
    await listen<string>('install://stdout', (event) => {
      const parsed = parseLine(event.payload);
      if (parsed) handlers.onEvent(parsed);
      else handlers.onLog(event.payload);
    }),
  );
  unlisten.push(await listen<string>('install://stderr', (event) => handlers.onLog(event.payload)));
  unlisten.push(await listen<ExitPayload>('install://exit', (event) => handlers.onExit(event.payload)));

  await invoke('start_install', { input });

  return () => {
    for (const off of unlisten) off();
    void invoke('cancel_install').catch(() => undefined);
  };
}

// runSimulated is the browser dev fallback: it replays a representative NDJSON
// stream for the whole flow (including the TOTP and recovery-ZIP payloads) so
// the wizard can be reviewed and tested without the Rust shell. The UI labels
// it as simulated.
function runSimulated(input: InstallInput, handlers: RunHandlers): () => void {
  const secret = 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP';
  const otpauth = `otpauth://totp/${input.domain}:${input.adminUser}?algorithm=SHA1&digits=6&issuer=${input.domain}&period=30&secret=${secret}`;
  const loginUrl = `https://${input.domain}/authelia`;

  const script: Array<{ event?: EngineEvent; log?: string; delay: number }> = [
    { event: { step: 'config', status: 'running', pct: 5, msg: 'Inputs validated' }, delay: 220 },
    { event: { step: 'pack', status: 'running', pct: 10, msg: 'Install pack verified (0.1.0)' }, delay: 300 },
    { event: { step: 'preflight', status: 'running', pct: 18, msg: 'Node is in maintenance mode' }, delay: 360 },
    { event: { step: 'machine-config', status: 'running', pct: 24, msg: 'Machine config rendered' }, delay: 300 },
    { event: { step: 'talos-config', status: 'running', pct: 32, msg: 'Talos config generated' }, delay: 300 },
    { event: { step: 'talos-install', status: 'running', pct: 42, msg: 'Applying the machine configuration' }, delay: 520 },
    { event: { step: 'talos-install', status: 'running', pct: 50, msg: 'Node is installing; waiting for the API' }, delay: 520 },
    { event: { step: 'bootstrap', status: 'running', pct: 60, msg: 'Bootstrapping etcd' }, delay: 480 },
    { event: { step: 'kubeconfig', status: 'running', pct: 68, msg: 'Fetched kubeconfig' }, delay: 300 },
    { event: { step: 'cni', status: 'running', pct: 72, msg: 'Waiting for the CNI (Cilium)' }, delay: 360 },
    { event: { step: 'storage', status: 'running', pct: 78, msg: 'Cluster storage ready' }, delay: 360 },
    { event: { step: 'helm', status: 'running', pct: 84, msg: 'Deploying Naslos (simulated)' }, delay: 520 },
    { event: { step: 'admin', status: 'running', pct: 89, msg: `Created administrator ${input.adminUser}` }, delay: 420 },
    { event: { step: 'totp', status: 'running', pct: 93, msg: 'Scan this code with your authenticator', data: { otpauth, secret } }, delay: 420 },
    { event: { step: 'resolver', status: 'running', pct: 95, msg: `Add ${input.domain} to your hosts file (simulated)` }, delay: 300 },
    { event: { step: 'archive', status: 'running', pct: 98, msg: 'Recovery ZIP written', data: { path: `~/Downloads/naslos-recovery-${input.domain}.zip` } }, delay: 420 },
    { event: { step: 'done', status: 'ok', pct: 100, msg: loginUrl, data: { loginUrl } }, delay: 360 },
  ];

  let timer: ReturnType<typeof setTimeout> | undefined;
  let index = 0;
  let cancelled = false;

  const pump = () => {
    if (cancelled || index >= script.length) return;
    const item = script[index++];
    timer = setTimeout(() => {
      if (cancelled) return;
      if (item.event) {
        handlers.onEvent(item.event);
        if (item.event.step === 'done') {
          handlers.onExit({ code: 0, success: true });
          return;
        }
      }
      if (item.log) handlers.onLog(item.log);
      pump();
    }, item.delay);
  };
  pump();

  return () => {
    cancelled = true;
    if (timer) clearTimeout(timer);
  };
}
