<script lang="ts">
  import QRCode from './QRCode.svelte';
  import { resolveLoginUrl } from '../lib/handoff';
  import type { Handoff } from '../lib/types';

  export let domain: string;
  export let adminUser: string;
  export let handoff: Handoff = {};
  export let onRestart: () => void;

  let copied = '';

  $: loginUrl = resolveLoginUrl(domain, handoff);

  async function copy(label: string, text: string | undefined) {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      copied = label;
      setTimeout(() => (copied = ''), 1500);
    } catch {
      copied = '';
    }
  }
</script>

<div class="space-y-6">
  <header class="text-center">
    <div class="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-emerald-500/15 text-2xl">
      ✓
    </div>
    <h1 class="text-2xl font-semibold">Naslos is installed</h1>
    <p class="mt-1 text-sm text-slate-400">Sign in as <span class="font-mono">{adminUser}</span> and save your recovery ZIP.</p>
  </header>

  <section class="card space-y-2">
    <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">First login</h2>
    <div class="flex flex-wrap items-center gap-3">
      <a class="font-mono text-sky-400 hover:underline" href={loginUrl} target="_blank" rel="noreferrer">{loginUrl}</a>
      <button class="btn-secondary" type="button" on:click={() => copy('url', loginUrl)}>
        {copied === 'url' ? 'Copied' : 'Copy'}
      </button>
    </div>
    <p class="hint">Accept the certificate warning if the local certificate is not trusted yet.</p>
  </section>

  <section class="card">
    <h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">Two-factor</h2>
    <div class="flex flex-col items-center gap-4 sm:flex-row sm:items-start">
      <QRCode value={handoff.otpauthUri ?? ''} />
      <div class="flex-1 space-y-3">
        <p class="text-sm text-slate-300">
          Scan the QR with an authenticator app, or add this secret manually.
        </p>
        <div class="rounded-lg border border-slate-800 bg-slate-950/60 p-3">
          <p class="hint mb-1">Base32 secret</p>
          <p class="break-all font-mono text-lg tracking-wider text-emerald-300">
            {handoff.secret ?? '—'}
          </p>
        </div>
        <button class="btn-secondary" type="button" on:click={() => copy('secret', handoff.secret)}>
          {copied === 'secret' ? 'Copied' : 'Copy secret'}
        </button>
        <p class="hint">
          This device is enrolled during install; the portal will ask for a code on first sign-in.
        </p>
      </div>
    </div>
  </section>

  <section class="card space-y-2">
    <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">Recovery ZIP</h2>
    {#if handoff.zipPath}
      <p class="break-all font-mono text-sm text-slate-200">{handoff.zipPath}</p>
      <button class="btn-secondary" type="button" on:click={() => copy('zip', handoff.zipPath)}>
        {copied === 'zip' ? 'Copied' : 'Copy path'}
      </button>
    {:else}
      <p class="hint">The recovery ZIP path will appear here once the engine writes it.</p>
    {/if}
    <p class="rounded-lg border border-amber-700/50 bg-amber-950/40 p-3 text-xs text-amber-200">
      The ZIP holds the Talos and Kubernetes credentials and the secrets bundle — a master credential.
      Store it offline. <span class="font-semibold">Never</span> share it, and note that
      <span class="font-mono">/var/lib/naslos/buddy-identity.json</span> on the node is the backup
      KEK: losing it makes every stored backup unreadable.
    </p>
  </section>

  <div class="flex justify-center">
    <button class="btn-secondary" type="button" on:click={onRestart}>Install another node</button>
  </div>
</div>
