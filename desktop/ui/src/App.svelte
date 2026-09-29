<script lang="ts">
  import { onDestroy } from 'svelte';
  import Stepper from './components/Stepper.svelte';
  import LogPane from './components/LogPane.svelte';
  import HandoffScreen from './components/Handoff.svelte';
  import ErrorScreen from './components/ErrorScreen.svelte';
  import { emptyInput, shortName, autheliaURL, ISO_URL } from './lib/steps';
  import { validate } from './lib/validate';
  import { isTauri, runInstall } from './lib/engine';
  import { mergeHandoff, secretFromURI } from './lib/handoff';
  import type { EngineEvent, ExitPayload, Handoff, InstallInput } from './lib/types';

  type Phase = 'form' | 'confirm' | 'progress' | 'handoff' | 'error';
  type StepState = 'running' | 'ok' | 'failed';

  const demo = !isTauri();

  let input: InstallInput = emptyInput();
  let phase: Phase = 'form';
  let touched = false;
  let currentStep = '';
  let pct = 0;
  let stepStatus: Record<string, StepState> = {};
  let logLines: string[] = [];
  let handoff: Handoff = {};
  let failure: { step: string; msg: string; output?: string } | null = null;
  let cancel: (() => void) | null = null;

  $: errors = validate(input);
  $: invalid = Object.keys(errors).length > 0;
  $: derivedName = shortName(input.name, input.domain);
  $: loginUrl = autheliaURL(input.domain);

  function pushLog(line: string) {
    if (!line) return;
    logLines = [...logLines, line];
  }

  function handleEvent(event: EngineEvent) {
    if (event.error) {
      if (event.error.step) stepStatus = { ...stepStatus, [event.error.step]: 'failed' };
      failure = {
        step: event.error.step ?? currentStep,
        msg: event.error.msg,
        output: event.error.output,
      };
      phase = 'error';
      return;
    }
    if (event.step) {
      currentStep = event.step;
      if (typeof event.pct === 'number') pct = event.pct;
      if (event.step !== 'done') {
        stepStatus = { ...stepStatus, [event.step]: event.status === 'ok' ? 'ok' : 'running' };
      }
    }
    handoff = mergeHandoff(handoff, event);
    if (event.msg) pushLog(event.msg);
    if (event.step === 'done') {
      handoff = { ...handoff, secret: handoff.secret ?? secretFromURI(handoff.otpauthUri) };
      stepStatus = Object.fromEntries(
        Object.keys(stepStatus).map((key) => [key, 'ok' as StepState]),
      );
      pct = 100;
      phase = 'handoff';
    }
  }

  function handleExit(exit: ExitPayload) {
    cancel = null;
    if (!exit.success && phase !== 'error' && phase !== 'handoff') {
      failure = failure ?? {
        step: currentStep,
        msg: `The installer exited with code ${exit.code ?? 'unknown'}.`,
      };
      phase = 'error';
    }
  }

  // review validates the form and moves to the confirmation screen; the install
  // itself only starts from there (start()).
  function review() {
    touched = true;
    if (invalid) return;
    phase = 'confirm';
  }

  async function start() {
    touched = true;
    if (invalid) return;
    phase = 'progress';
    currentStep = '';
    pct = 0;
    stepStatus = {};
    logLines = [];
    handoff = {};
    failure = null;
    try {
      cancel = await runInstall(
        { ...input, name: input.name || derivedName },
        { onEvent: handleEvent, onLog: pushLog, onExit: handleExit },
      );
    } catch (error) {
      failure = {
        step: 'engine',
        msg: error instanceof Error ? error.message : String(error),
      };
      phase = 'error';
    }
  }

  function requestCancel() {
    if (cancel) cancel();
    cancel = null;
    phase = 'error';
    failure = { step: currentStep, msg: 'Install cancelled.' };
  }

  function restart() {
    input = emptyInput();
    touched = false;
    phase = 'form';
    currentStep = '';
    pct = 0;
    stepStatus = {};
    logLines = [];
    handoff = {};
    failure = null;
    cancel = null;
  }

  onDestroy(() => {
    if (cancel) cancel();
  });
</script>

<div class="flex min-h-full flex-col">
  <header class="flex items-center justify-between border-b border-slate-800 px-6 py-4">
    <div class="flex items-center gap-3">
      <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-sky-500 font-bold text-slate-950">
        N
      </div>
      <div>
        <p class="text-sm font-semibold">Naslos Installer</p>
        <p class="text-xs text-slate-500">Provision a Talos node from the Naslos ISO</p>
      </div>
    </div>
    {#if demo}
      <span class="rounded-full border border-amber-700/60 bg-amber-950/50 px-3 py-1 text-xs text-amber-300">
        Browser preview — simulated engine
      </span>
    {/if}
  </header>

  <main class="flex flex-1 items-start justify-center p-6">
    <div class="w-full {phase === 'progress' ? 'max-w-5xl' : 'max-w-2xl'}">
      {#if phase === 'form'}
        <form class="card space-y-5" on:submit|preventDefault={review}>
          <div>
            <h1 class="text-xl font-semibold">Install Naslos</h1>
            <p class="mt-1 text-sm text-slate-400">
              Boot the target machine from the Naslos ISO, then enter its details.
            </p>
          </div>

          <div class="grid gap-4 sm:grid-cols-2">
            <div class="sm:col-span-2">
              <label class="label" for="node-ip">Node IPv4 address</label>
              <input
                id="node-ip"
                class="field font-mono"
                placeholder="192.168.1.50"
                autocomplete="off"
                bind:value={input.nodeIp}
                on:blur={() => (touched = true)}
              />
              {#if touched && errors.nodeIp}<p class="mt-1 text-xs text-rose-400">{errors.nodeIp}</p>{/if}
            </div>

            <div>
              <label class="label" for="domain">Local domain</label>
              <input id="domain" class="field font-mono" bind:value={input.domain} />
              {#if touched && errors.domain}<p class="mt-1 text-xs text-rose-400">{errors.domain}</p>{/if}
            </div>

            <div>
              <label class="label" for="name">Advertised name <span class="text-slate-500">(optional)</span></label>
              <input id="name" class="field font-mono" placeholder={derivedName} bind:value={input.name} />
              {#if touched && errors.name}<p class="mt-1 text-xs text-rose-400">{errors.name}</p>{/if}
            </div>

            <div>
              <label class="label" for="admin-user">Administrator username</label>
              <input id="admin-user" class="field font-mono" autocomplete="off" bind:value={input.adminUser} />
              {#if touched && errors.adminUser}<p class="mt-1 text-xs text-rose-400">{errors.adminUser}</p>{/if}
            </div>

            <div>
              <label class="label" for="admin-password">Administrator password</label>
              <input
                id="admin-password"
                class="field"
                type="password"
                autocomplete="new-password"
                bind:value={input.adminPassword}
              />
              {#if touched && errors.adminPassword}
                <p class="mt-1 text-xs text-rose-400">{errors.adminPassword}</p>
              {:else}
                <p class="hint mt-1">≥8 chars with upper, lower and a digit.</p>
              {/if}
            </div>
          </div>

          <label class="flex items-center gap-2 text-sm text-slate-300">
            <input class="h-4 w-4 rounded border-slate-600 bg-slate-900" type="checkbox" bind:checked={input.addResolver} />
            Add <span class="font-mono">{input.domain || 'the domain'}</span> to this computer's hosts file
          </label>

          <div class="flex justify-end">
            <button class="btn-primary" type="submit" disabled={touched && invalid}>Continue</button>
          </div>
        </form>
      {:else if phase === 'confirm'}
        <div class="card space-y-5">
          <div>
            <h1 class="text-xl font-semibold">Confirm the install</h1>
            <p class="mt-1 text-sm text-slate-400">
              This writes the Talos image to the node's disk. Make sure the machine is freshly booted
              from the Naslos ISO and has nothing to keep.
            </p>
          </div>

          <dl class="grid gap-3 text-sm sm:grid-cols-2">
            <div><dt class="hint">Node</dt><dd class="font-mono">{input.nodeIp}</dd></div>
            <div><dt class="hint">Domain</dt><dd class="font-mono">{input.domain}</dd></div>
            <div><dt class="hint">Advertised name</dt><dd class="font-mono">{input.name || derivedName}</dd></div>
            <div><dt class="hint">Administrator</dt><dd class="font-mono">{input.adminUser}</dd></div>
            <div><dt class="hint">First login</dt><dd class="font-mono text-sky-400">{loginUrl}</dd></div>
            <div><dt class="hint">Hosts entry</dt><dd>{input.addResolver ? 'yes (best effort)' : 'no'}</dd></div>
          </dl>

          <p class="rounded-lg border border-rose-800/60 bg-rose-950/40 p-3 text-xs text-rose-200">
            The node's install disk (<span class="font-mono">/dev/vda</span>) is overwritten. The
            engine refuses to re-key an already-installed node, but there is no undo for its existing data.
          </p>

          <p class="hint">
            Need the image?
            <a class="text-sky-400 hover:underline" href={ISO_URL} target="_blank" rel="noreferrer">Download the Naslos ISO</a>.
          </p>

          <div class="flex justify-between">
            <button class="btn-secondary" type="button" on:click={() => (phase = 'form')}>Back</button>
            <button class="btn-primary" type="button" on:click={start}>Start install</button>
          </div>
        </div>
      {:else if phase === 'progress'}
        <div class="grid gap-6 lg:grid-cols-[280px_1fr]">
          <aside class="card h-fit">
            <h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">Steps</h2>
            <Stepper {currentStep} status={stepStatus} />
          </aside>
          <section class="space-y-4">
            <div class="card space-y-3">
              <div class="flex items-baseline justify-between">
                <p class="text-lg font-medium">{currentStep ? stepStatus[currentStep] === 'running' ? 'Installing…' : 'Working…' : 'Starting…'}</p>
                <p class="font-mono text-sm text-slate-400">{pct}%</p>
              </div>
              <div class="h-2 overflow-hidden rounded-full bg-slate-800">
                <div class="h-full rounded-full bg-sky-500 transition-all" style="width: {pct}%"></div>
              </div>
              <LogPane lines={logLines} />
            </div>
            <div class="flex justify-end">
              <button class="btn-danger" type="button" on:click={requestCancel}>Cancel</button>
            </div>
          </section>
        </div>
      {:else if phase === 'handoff'}
        <HandoffScreen domain={input.domain} adminUser={input.adminUser} {handoff} onRestart={restart} />
      {:else}
        <ErrorScreen
          step={failure?.step ?? ''}
          message={failure?.msg ?? 'Unknown error.'}
          output={failure?.output ?? ''}
          onRetry={start}
          onBack={() => (phase = 'form')}
        />
      {/if}
    </div>
  </main>
</div>
