# Review remediation plan — Naslos-Installer (2026-10-31)

Executor: an autonomous coding agent (DeepSeek). Owner: Aessem.
Companion plan (product side, **read it first**):
`Naslos-Linux/.kilo/plans/1793474557133-review-remediation.md` — its §0 rules apply
here too (one PR per workstream, gates, no tags, stop-and-ask conditions, docs rules).

Baseline: `origin/main` **`84defd7`** (includes PR #12 resumed-bundle clock fix and
PR #13 version-parity guard). The local `feat/release-bundles` checkout (`95e4f4d`) is
stale — never branch from it. Line numbers below are from `84defd7`.

---

## 0. Executor rules specific to this repo

1. `git fetch origin && git switch -c fix/<slug> origin/main`; push with
   `git push -u origin fix/<slug>`; open a PR with `gh pr create`; do not merge.
2. Gates for every PR:
   ```bash
   make fetch-latest-pack   # needs a published Naslos-Linux vX.Y.Z; if it fails, say so in the PR
   make check               # go vet + go test -race ./...
   make build
   cd desktop && npm ci && npm run build          # plus the UI check script if package.json defines one
   cd desktop/src-tauri && cargo fmt -- --check && cargo clippy -- -D warnings
   ```
   Read `desktop/package.json` for the exact UI check/build script names before running.
3. Docs rule: update `README.md`, `AI_HANDOFF.md`, and — for any interface the product
   depends on — `Naslos-Linux/docs/installer-contract.md` **in a Naslos-Linux PR that lands
   first** (cross-repo release order, workspace `AGENTS.md`).
4. **Never push a tag.** In particular see I0.

---

## I0 — Hygiene (do first, no PR needed)

- A stray **local** tag `v0.2.3` points to `e1657cf`, a commit **before** the parity guard.
  The release workflow triggers on `v*` and runs the workflow file **from the tagged commit**,
  so pushing it (after Naslos-Linux `v0.2.3` exists) would publish an unguarded release.
  It is not on the remote (`git ls-remote origin 'refs/tags/v*'` is empty). Delete it locally:
  `git tag -d v0.2.3`. Do the same in every local worktree/clone you use. Report it in your
  first PR description.

---

## I1 — Credential lifecycle  (`fix/credential-lifecycle`)  — release-blocking

### I1.1 Admin password off argv
- Engine (`cmd/naslos-install/main.go:539`): replace `--admin-password` with
  `--admin-password-stdin` (bool). When set, read **one line** from stdin
  (`bufio.Reader.ReadString('\n')`, strip `\r\n`, max 1 KiB), before any other work.
  Also accept env `NASLOS_ADMIN_PASSWORD` (read, then `os.Unsetenv`) for headless use.
  If `--admin-password` is passed, fail with: "--admin-password was removed (visible in
  the process list); use --admin-password-stdin or NASLOS_ADMIN_PASSWORD".
- Desktop (`desktop/src-tauri/src/lib.rs:58-68`): pass `--admin-password-stdin` and, right
  after `spawn()`, `child.write(format!("{}\n", password).as_bytes())`. Never put the
  password into `args`. Drop the password `String` right after writing; never log it
  (no new crate for zeroization).
- Update README/AI_HANDOFF headless examples (`echo` is NOT acceptable in docs; show
  `read -rs NASLOS_ADMIN_PASSWORD && export NASLOS_ADMIN_PASSWORD && naslos-install ...`).
- Tests: flag parsing table test (stdin, env, removed flag error); a test that the arg vector
  built for the sidecar never contains the password (extract the arg builder into a pure fn).

### I1.2 Never persist the password; refresh inputs
- `internal/config/config.go:17-24`: tag `AdminPassword string \`json:"-"\``.
- `internal/state/state.go`: on `Load`, if the raw JSON contains an `AdminPassword` key,
  rewrite the file without it (scrub) and emit a warning event. Re-assert `0600` on load
  (`os.Chmod`).
- `cmd/naslos-install/main.go:103-107`: when an existing state is loaded for the same node,
  replace `st.Input` with the current non-secret inputs (domain, name, admin user,
  add-resolver) so a re-run never keeps stale values.
- Atomic save (`state.go:89-98`): write `state.json.tmp` (0600) → `fsync` → `rename`;
  keep `state.json.bak` of the previous version; `Load` falls back to `.bak` on parse error.
  Take an exclusive `flock` (`syscall.Flock`, `LOCK_EX|LOCK_NB`) on `<state-dir>/.lock` for
  the engine's lifetime; on Windows use `LockFileEx` via `golang.org/x/sys/windows` only if
  already in `go.mod` — otherwise a lock file with `O_EXCL` + stale-PID check.
- Tests: marshal of `New(input)` has no password; scrub on load; atomic write survives a
  truncated `.tmp`; second engine instance fails to lock.

### I1.3 In-cluster admin creation without secrets in argv (Talos audit-logs exec URIs)
- `internal/k8s/exec.go:17,38`: add `ExecWithStdin(ctx, ns, pod, container, cmd, stdin io.Reader)`
  passing `Stdin` in `remotecommand.StreamOptions`; keep `Exec` as a wrapper with nil stdin.
  Update the `Execer` interface and fakes.
- `internal/bootstrap/bootstrap.go:49-78`: argv becomes exactly
  `["curl","-sS","--fail-with-body","-K","-"]`; stdin is a curl config:
  ```
  url = "http://.../api/users"
  request = "POST"
  header = "X-Naslos-Proxy-Secret: <secret>"
  header = "Remote-User: <uid>"
  header = "Remote-Groups: naslos_admins"
  header = "Content-Type: application/json"
  data-binary = "<json>"
  ```
  Write `curlQuote(s)` that escapes `\` → `\\`, `"` → `\"`, newline → `\n`, tab → `\t`
  (curl config quoting rules) and unit-test it with a password containing `"`, `\`, `$`, `'`.
  Same for the `users()` list call (headers via `-K -`).
- Verification: parse the list response with `encoding/json` and require an entry whose
  `uid` field **equals** `o.UID` (no substring). Treat a non-2xx (curl exit 22 with
  `--fail-with-body`) as failure and include the response body (never the request) in the error.
- Tests: fake Execer asserts argv contains no secret/password and stdin contains them;
  exact-uid matching (`admin` vs `administrator`); HTTP 500 body path.
- Requires curl ≥ 7.76 in the terminal image: alpine 3.24 — satisfied
  (`Naslos-Linux/terminal/image/Dockerfile:21,36`). Note this dependency in
  `Naslos-Linux/docs/installer-contract.md` §3 (first-admin call) via a Naslos-Linux PR,
  and fix the documented payload fields there (`firstName`/`lastName`, not `displayName`).

---

## I2 — Desktop shell correctness  (`fix/desktop-run-lifecycle`)

1. **Run identity** (`desktop/src-tauri/src/lib.rs:51-56,82-127,135-139`):
   `EngineState { slot: Mutex<Option<(u64 /*run_id*/, CommandChild)>>, next: AtomicU64 }`.
   `start_install` takes the lock **once**, rejects if occupied, spawns, stores
   `(run_id, child)`, and only then releases the lock (no TOCTOU). Every emitted event payload
   carries `runId`. The exit task clears the slot **only if** the stored `run_id` matches.
   `cancel_install` takes the slot and kills.
2. **JS side** (`desktop/ui/src/lib/engine.ts:36-53`, `App.svelte:39-49,69-78,112-130`,
   `components/ErrorScreen.svelte:31-34`): keep `currentRunId` from the `start_install`
   return value (make the command return it); ignore events with another `runId`; wrap
   `invoke` in try/catch and unlisten on failure; on an `error` event call `cancel()` and show
   "Stopping…" until `install://exit`, then enable Retry; treat `exit.success` without a
   prior `done` step as an error ("engine finished without a completion event").
3. **CSP** (`desktop/src-tauri/tauri.conf.json:23-25`): set
   `"csp": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src ipc: http://ipc.localhost"`.
   Verify `tauri dev` and a built bundle still load (QR canvas, fonts).
4. **Link sink** (`components/Handoff.svelte:39`, `lib/handoff.ts:18`): accept `loginUrl` only
   if `new URL(x).protocol === 'https:'`; otherwise render plain text.
5. Tests: Rust unit test for slot ownership logic (extract into a pure struct); UI unit tests
   if a test runner exists, otherwise document manual steps in the PR (cancel → retry twice).

---

## I3 — Engine robustness  (`fix/engine-robustness`)

1. **Cancellation/timeouts** (`main.go:207`, `internal/talosclient/talosclient.go:111-166`,
   `internal/k8s/k8s.go:76-150`, `internal/helm/helm.go:64,90`): root context from
   `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`; each RPC in wait loops uses
   `context.WithTimeout(ctx, 10*time.Second)`; Helm install gets an overall
   `context.WithTimeout(ctx, 25*time.Minute)` (match the existing documented budget). On
   cancellation, record the current step as `Failed` with "cancelled" and emit a `fail` event.
2. **Readiness errors** (`k8s.go:76-105`): checks return `(false, err)`; `poll` retries
   `IsNotFound`/transient errors and fails fast after 3 consecutive `Forbidden`/`Unauthorized`.
3. **Node IP** (`internal/config/config.go:34`): reject unspecified, loopback, link-local,
   multicast, broadcast (`ip.IsUnspecified() || ip.IsLoopback() || ...`). Table test.
4. **Preflight identity** (`internal/preflight/preflight.go:46-57`): `Installed` only if a
   TLS handshake to :50000 succeeds and Talos `Version` answers (authenticated with the
   persisted talosconfig when present); a bare :6443 listener yields a distinct error
   "something answers on :6443 but it is not a Talos node we can reach".
5. **Recovery ZIP** (`main.go:470-489`, `internal/archive/archive.go:47-57,96-99`): write to
   `<dest>/.naslos-recovery-*.zip.tmp`, check `zw.Close`, `f.Sync`, `f.Close` errors, then
   rename; remove the temp on any error. Keep the destination logic but emit the final path
   in the `done` event and print a warning that the folder may be cloud-synced.
6. **`--no-json-progress`** (`main.go:542-553`): `o.jsonProgress` is never read — implement a
   human-readable emitter in `internal/event` (one line per step: `[ 40%] helm: Installing…`)
   and select it; test both modes.
7. **Pack hygiene** (`internal/installpack/pack.go:86-170`) — defence in depth, not a
   provenance fix: `Verify` fails if any pack member other than `metadata.json`/`README.md`
   is missing from `checksums`; `Extract` rejects non-regular entries. Tests with an unlisted
   member and an extra file. Document in README that provenance is unsigned (tracked as D6
   in the companion plan).

---

## I4 — Install-disk selection  (`feat/install-disk`)  — needs a contract doc change first

- Contract today: "v1 assumes `/dev/vda`" (`Naslos-Linux/docs/installer-contract.md` §1.1).
  First land a Naslos-Linux docs PR: `INSTALL_DISK` is chosen by the installer from the
  node's maintenance-mode disk list.
- Engine: `--install-disk` (default empty). Before apply, list disks from the maintenance
  API (machinery COSI `block.Disk` resources, insecure maintenance client already used by
  `talosclient.Dial`). Rules: must exist; not `readonly`, not `cdrom`; not the boot medium if
  identifiable. If the flag is empty and exactly one eligible disk exists, use it; otherwise
  fail listing candidates. Non-dry-run requires `--confirm-wipe=<disk>` equal to the chosen disk.
  Emit `data.disks` in a `preflight` event so the desktop can render a picker.
- Desktop: disk picker step + typed confirmation; pass both flags.
- Tests: selection rules table test with fake disk lists.

---

## I5 — Release/CI pipeline  (`ci/release-hardening`)

1. **Trigger** (`.github/workflows/release.yml:21-35`): change `push.tags` to
   `["naslos-v*"]`. In `prepare`, a tag push must match `^naslos-v[0-9]+\.[0-9]+\.[0-9]+$`
   and derive `version` from it (strip `naslos-`); the parity step then compares that with the
   embedded Naslos tag (both normalised without `v`).
2. **Input injection** (`:56-71,81-86` and every `${{ steps.*.outputs.* }}` /
   `${{ needs.*.outputs.* }}` inside `run:` — currently `:81,82,86` and the build jobs):
   validate `NASLOS_VERSION_INPUT` with `^v?[0-9]+\.[0-9]+\.[0-9]+$` before use; move every
   expression used inside a `run:` script into `env:` and reference `$VAR`.
3. **Dispatch payload**: for `repository_dispatch`, read `github.event.client_payload.tag`
   via `env:`, validate it, and use it instead of `ls-remote`.
4. **Permissions** (`:34-35`): workflow-level `contents: read`; `contents: write` only on the
   `publish` job.
5. **CI gate**: in `publish`, require the installer `ci` workflow to be green for `$GITHUB_SHA`
   (same paginated, exact-name approach as Naslos-Linux W11.1).
6. **Bundle version** (`desktop/src-tauri/tauri.conf.json:4`, `src-tauri/Cargo.toml:3`,
   `desktop/package.json`): in each bundle job, before building, rewrite the version to the
   release version (`jq` for JSON; `sed` on the `[package] version` line) — or pass
   `npm run tauri -- build --config '{"version":"X.Y.Z"}'`. Assert the built artefact name/metadata
   carry X.Y.Z. Keep the in-tree files at the current release version too.
7. **Checksums**: produce `checksums.txt` (sha256) over **all** published assets in `publish`
   and attach it. State "unsigned, not notarized" in the release notes until signing exists.
8. **CI pack fetch** (`.github/workflows/ci.yml:37-43`): make `scripts/fetch-install-pack.sh`
   exit `3` only for "no vX.Y.Z tag yet"; CI skips on `3` only and fails on any other code.
9. `concurrency: { group: release, cancel-in-progress: false }`.

---

## I6 — Real resume  (`feat/resume`)  — after I1 and I3

- Today `StepDone` has no production caller and preflight refuses an installed node.
- Add `--resume`: requires an existing state for the same node whose `talos-config` step is
  `done`. Preflight accepts `Installed` only in resume mode, then verifies the node with the
  persisted talosconfig (Talos `Version` equals the pack). Each later milestone is skipped
  when `StepDone`. TOTP: before `authelia storage user totp generate`, check for an existing
  device and skip (or `delete` then `generate` only if the user passed `--reset-totp`).
- The admin step is idempotent: if the exact uid exists, skip create and only verify groups.
- Desktop: offer "Resume previous install" when a state file for the entered IP exists.
- Tests: fake Talos/K8s; failure after `helm` then `--resume` skips earlier steps.
- Update README/AI_HANDOFF (currently they claim resumable state).

---

## Do NOT fix

- `internal/k8s/prereq.go` `UpsertSecret` update without `resourceVersion`: **not a bug**
  (Secrets allow unconditional updates).
- Version parity between installer and Naslos: already enforced on `main` (PR #13); I5 only
  tightens the tag pattern.

## Release

Installer work releases as `naslos-v0.2.3` **after** Naslos-Linux `v0.2.3` is published
(companion plan §6). I1 + I2 + I5 are required for that release; I3/I4/I6 may follow in
0.2.4 (which then needs a Naslos-Linux 0.2.4 pack — version parity).
