# Desktop GUI — Tauri v2 shell + Svelte wizard (I7)

Repo: `AessemOps/Naslos-Installer`. Plan: `desktop-installer-app.md` I7/I8b.
Companion: `Naslos-Linux/.kilo/plans/1790466900440-desktop-installer-app.md` §7.6.

## Goal

The installer is only a headless CLI. Ship the desktop app the plan calls for:
a Tauri v2 shell around the `naslos-install` engine with a Svelte 5 wizard —
input → confirm → progress bar + log → 2FA/QR handoff → recovery ZIP/first login
— plus cancel and error screens.

## Decisions

- **UI stack**: Svelte 5 (legacy syntax, matching the Naslos UI) + Vite + Tailwind
  3 + TypeScript, in `desktop/ui`. No SvelteKit: the wizard is a single-window
  SPA with no routes.
- **Engine coupling**: unchanged — NDJSON on the child's stdout. The Rust side
  spawns the sidecar (`tauri-plugin-shell`), forwards every stdout/stderr line
  to the webview as `install://stdout` / `install://stderr`, and emits
  `install://exit`. `cancel_install` kills the child. Commands:
  `start_install(input)` / `cancel_install()`.
- **Browser dev fallback**: `desktop/ui` runs in a plain browser too
  (`npm run dev`) with a **simulated** engine stream, so the wizard can be
  reviewed and e2e-tested without Rust. `isTauri()` picks the real path when
  running inside the shell.
- **Sidecar layout**: `desktop/src-tauri/binaries/naslos-install-<target-triple>`
  (gitignored), produced by `scripts/build-sidecar.sh`; `externalBin:
  ["binaries/naslos-install"]` in `tauri.conf.json`.

## Files

- `desktop/ui/**` — wizard (App.svelte + components + lib/engine.ts/types).
- `desktop/src-tauri/**` — Cargo project, `tauri.conf.json`, capabilities,
  `src/lib.rs` (spawn/stream/kill), generated icons.
- `scripts/build-sidecar.sh` — build the engine with the pack and copy it to the
  sidecar path for the host triple.
- `.github/workflows/ci.yml` — `desktop-ui` job (`npm ci`, check, build).
- Docs: installer `README.md`, `AI_HANDOFF.md`, plan I7/I8b.

## Verification

- `npm run check` (svelte-check) and `npm run build` in `desktop/ui`.
- `cargo build` in `desktop/src-tauri` (Rust installed user-level; Tauri's
  webkit2gtk/gtk/librsvg deps are already present).
- `tauri build` bundles (AppImage/deb/rpm, dmg/app, msi/exe) stay I8b; they need
  per-OS runners.