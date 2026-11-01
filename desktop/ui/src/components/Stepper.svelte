<script lang="ts">
  import { STEP_ORDER, stepLabel } from '../lib/steps';

  export let currentStep = '';
  export let status: Record<string, 'running' | 'ok' | 'failed'> = {};

  $: steps =
    !currentStep || STEP_ORDER.includes(currentStep) ? STEP_ORDER : [...STEP_ORDER, currentStep];
  $: currentIndex = steps.indexOf(currentStep);

  type State = 'running' | 'ok' | 'failed' | 'pending';

  // Every input is a parameter on purpose. In Svelte's legacy reactivity a
  // `{@const x = fn(step, index)}` only re-runs when `step`/`index` change, so a
  // helper that read `status`/`currentStep` from the closure froze every step
  // at "pending" (all grey) for the whole install.
  function stateOf(
    step: string,
    index: number,
    statusMap: Record<string, 'running' | 'ok' | 'failed'>,
    active: string,
    activeIndex: number,
  ): State {
    if (statusMap[step] === 'failed') return 'failed';
    if (statusMap[step] === 'ok') return 'ok';
    if (step === active) return 'running';
    if (activeIndex >= 0 && index < activeIndex) return 'ok';
    return 'pending';
  }

  $: states = steps.map((step, index) => stateOf(step, index, status, currentStep, currentIndex));

  const dot: Record<State, string> = {
    running: 'border-sky-400 text-sky-300',
    ok: 'border-emerald-500 text-emerald-400',
    failed: 'border-rose-500 text-rose-400',
    pending: 'border-slate-700 text-slate-600',
  };
</script>

<ol class="space-y-1">
  {#each steps as step, index (step)}
    {@const state = states[index] ?? 'pending'}
    <li
      class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm {state === 'running'
        ? 'bg-sky-950/40'
        : ''}"
    >
      <span
        class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border text-[10px] {dot[
          state
        ]}"
      >
        {#if state === 'ok'}
          ✓
        {:else if state === 'failed'}
          ✕
        {:else if state === 'running'}
          ●
        {:else}
          {index + 1}
        {/if}
      </span>
      <span class={state === 'pending' ? 'text-slate-500' : 'text-slate-200'}>
        {stepLabel(step)}
      </span>
    </li>
  {/each}
</ol>
