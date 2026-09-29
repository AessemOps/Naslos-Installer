import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from 'tailwindcss';
import autoprefixer from 'autoprefixer';

// The Tauri project root is desktop/ (src-tauri/ is adjacent); the Svelte app
// lives in desktop/ui/ and is the Vite root. Tailwind's config is inlined so it
// never has to be discovered relative to a different root.
export default defineConfig({
  plugins: [svelte()],
  root: 'ui',
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2022',
  },
  css: {
    postcss: {
      plugins: [
        tailwindcss({
          content: ['./ui/index.html', './ui/src/**/*.{svelte,ts}'],
          theme: {
            extend: {
              fontFamily: {
                mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
              },
            },
          },
        }),
        autoprefixer(),
      ],
    },
  },
});