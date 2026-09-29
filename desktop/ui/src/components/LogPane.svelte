<script lang="ts">
  export let lines: string[] = [];

  let pane: HTMLDivElement;

  // Keep the newest line in view as the engine streams.
  $: if (pane && lines.length) {
    pane.scrollTop = pane.scrollHeight;
  }
</script>

<div
  bind:this={pane}
  class="h-48 overflow-auto rounded-lg border border-slate-800 bg-slate-950/80 p-3 font-mono text-xs leading-relaxed text-slate-400"
>
  {#if lines.length === 0}
    <p class="text-slate-600">Waiting for the engine…</p>
  {:else}
    {#each lines as line, i (i)}
      <div class="whitespace-pre-wrap">{line}</div>
    {/each}
  {/if}
</div>
