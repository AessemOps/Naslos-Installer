<script lang="ts">
  import QRCode from 'qrcode';

  export let value = '';

  let canvas: HTMLCanvasElement;
  let failed = false;

  $: if (canvas && value) {
    failed = false;
    QRCode.toCanvas(canvas, value, {
      width: 220,
      margin: 1,
      color: { dark: '#0f172a', light: '#ffffff' },
    }).catch(() => {
      failed = true;
    });
  }
</script>

{#if value && !failed}
  <canvas bind:this={canvas} class="rounded-lg bg-white p-2" aria-label="Two-factor QR code"></canvas>
{:else}
  <div
    class="flex h-56 w-56 items-center justify-center rounded-lg border border-dashed border-slate-700 text-center text-xs text-slate-500"
  >
    QR code unavailable — enter the secret below manually.
  </div>
{/if}
