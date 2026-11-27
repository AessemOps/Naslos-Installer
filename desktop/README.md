# Naslos installer desktop app

The wizard half of the Naslos installer: a Tauri v2 shell around the headless
`naslos-install` engine, with a Svelte 5 + Vite + Tailwind UI. The shell spawns
the engine as a sidecar and turns its newline-delimited JSON progress into a
live install screen, a 2FA/QR handoff, and a recovery ZIP.

```
package.json            npm project + Tauri CLI scripts
vite.config.ts          Vite root ui/, Tailwind inlined
ui/                     the Svelte wizard (index.html + src/)
src-tauri/              the Tauri shell (Rust) + bundle config
src-tauri/binaries/     built engine sidecar (gitignored)
```

## Develop

```bash
# From the repo root: fetch the pack the engine embeds, then build the sidecar
# Tauri spawns.
make fetch-latest-pack
./scripts/build-sidecar.sh

# From desktop/: run the Tauri dev window (Vite dev server + Rust shell).
npm install
npm run tauri dev
```

`npm run dev` alone serves the wizard in a browser with a **simulated** engine
stream. That is the fast path for UI work and screenshots without Rust. In the
Tauri window the shell spawns the real sidecar and forwards its NDJSON progress
as `install://stdout` / `install://stderr` / `install://exit` events.

## Build

```bash
npm run check                 # svelte-check
npm run tauri build           # bundling needs per-OS tooling (I8b: AppImage, dmg, msi)
npm run tauri build -- --no-bundle   # just the app binary
```

## How the wizard talks to the engine

`ui/src/lib/engine.ts` starts the install (`start_install`) and parses the
engine's NDJSON lines. `ui/src/lib/handoff.ts` reads the optional `data` payload
(the `otpauth://` URI, base32 secret, recovery-ZIP path, and login URL) with a
`msg`-scanning fallback. The Rust side (`src-tauri/src/lib.rs`) defines every
engine argument and never takes a shell string from the webview.
