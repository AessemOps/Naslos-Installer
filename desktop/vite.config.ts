import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/postcss';
import autoprefixer from 'autoprefixer';

// The Tauri project root is desktop/ (src-tauri/ is adjacent); the Svelte app
// lives in desktop/ui/ and is the Vite root. Tailwind 4 is CSS-first: the theme
// lives in ui/src/app.css, so the PostCSS plugin needs no options here.
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
      plugins: [tailwindcss(), autoprefixer()],
    },
  },
});
