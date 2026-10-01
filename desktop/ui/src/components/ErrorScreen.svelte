<script lang="ts">
  export let step = '';
  export let message = '';
  export let output = '';
  export let onRetry: () => void;
  export let onBack: () => void;
</script>

<div class="space-y-6">
  <header class="text-center">
    <div class="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-rose-500/15 text-2xl">
      ✕
    </div>
    <h1 class="text-2xl font-semibold">Install failed</h1>
    <p class="mt-1 text-sm text-slate-400">
      {#if step}The <span class="font-mono text-slate-300">{step}</span> step failed.{/if}
    </p>
  </header>

  <section class="card space-y-3">
    <p class="text-sm text-rose-200">{message}</p>
    {#if output}
      <pre class="max-h-48 overflow-auto rounded-lg border border-slate-800 bg-slate-950/80 p-3 font-mono text-xs text-slate-400">{output}</pre>
    {/if}
    <p class="hint">
      The install is resumable: the engine keeps its state and never regenerates the Talos PKI for an
      already-installed node. Fix the problem and retry.
    </p>
  </section>

  <div class="flex justify-center gap-3">
    <button class="btn-secondary" type="button" on:click={onBack}>Back</button>
    <button class="btn-primary" type="button" on:click={onRetry}>Retry</button>
  </div>
</div>
