//! Tauri shell for the Naslos installer.
//!
//! The shell owns no install logic: it spawns the `naslos-install` sidecar,
//! forwards the engine's newline-delimited JSON progress to the webview as
//! `install://stdout`/`install://stderr` events, emits `install://exit` when the
//! child ends, and kills the child on `cancel_install` (docs/installer-contract.md
//! §5). All command arguments are defined here; the webview never supplies a
//! shell string (SEC-1).

use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Mutex;

use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter, Manager, State};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

/// InstallInput mirrors the engine's CLI inputs (internal/config.Input).
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InstallInput {
    pub node_ip: String,
    pub domain: String,
    /// Whole-disk device the node installs to; validated by the engine and
    /// confirmed with `--confirm-wipe`.
    pub install_disk: String,
    #[serde(default)]
    pub name: String,
    pub admin_user: String,
    pub admin_password: String,
    #[serde(default)]
    pub add_resolver: bool,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct ExitPayload {
    run_id: u64,
    code: Option<i32>,
    success: bool,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct OutputPayload {
    run_id: u64,
    line: String,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct StartedPayload {
    run_id: u64,
}

/// Whether the finishing run owns the slot. After cancel + retry the old run's
/// exit task must not clear the new run's slot, or the retry becomes
/// uncancellable and its events are attributed to the wrong attempt.
fn owns_slot(owner: Option<u64>, finished: u64) -> bool {
    owner == Some(finished)
}

/// One running engine. The id lets the exit task clear the slot only if it
/// still owns it: after cancel + retry the old task must not wipe the new run.
struct Running {
    run_id: u64,
    child: CommandChild,
}

/// EngineState holds the running sidecar so it can be cancelled.
#[derive(Default)]
struct EngineState {
    child: Mutex<Option<Running>>,
    next_run: AtomicU64,
}

/// start_install spawns the engine sidecar and streams its output. It refuses to
/// start a second install while one is running.
#[tauri::command]
fn start_install(
    app: AppHandle,
    state: State<'_, EngineState>,
    input: InstallInput,
) -> Result<u64, String> {
    // Hold the lock across check + spawn + store. Releasing between the check
    // and the store let two concurrent invokes both start an engine, and only
    // one was tracked (so the other could not be cancelled).
    let mut guard = state.child.lock().map_err(|e| e.to_string())?;
    if guard.is_some() {
        return Err("an install is already running".into());
    }
    let run_id = state.next_run.fetch_add(1, Ordering::SeqCst) + 1;

    let mut args = vec![
        "--node-ip".to_string(),
        input.node_ip,
        "--domain".to_string(),
        input.domain,
        "--admin-user".to_string(),
        input.admin_user,
        "--install-disk".to_string(),
        input.install_disk.clone(),
        // The wizard's confirm screen requires an explicit checkbox, so the
        // desktop always confirms; the engine still validates it matches.
        "--confirm-wipe".to_string(),
        input.install_disk,
        "--admin-password-stdin".to_string(),
        "--json-progress".to_string(),
    ];
    if !input.name.trim().is_empty() {
        args.push("--name".to_string());
        args.push(input.name);
    }
    if input.add_resolver {
        args.push("--add-resolver".to_string());
    }

    let sidecar = app
        .shell()
        .sidecar("naslos-install")
        .map_err(|e| e.to_string())?;
    let (mut rx, mut child) = sidecar.args(args).spawn().map_err(|e| e.to_string())?;
    // The password must never be an argv element: it is visible in the process
    // list and in any audit log of this process. One line on stdin instead.
    let mut line = format!("{}\n", input.admin_password);
    let write_result = child.write(line.as_bytes());
    line.clear();
    drop(line);
    write_result.map_err(|e| format!("sending the administrator password to the engine: {e}"))?;
    *guard = Some(Running { run_id, child });
    drop(guard);

    let _ = app.emit("install://run", StartedPayload { run_id });

    let handle = app.clone();
    tauri::async_runtime::spawn(async move {
        let mut terminated = false;
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(bytes) => {
                    let line = String::from_utf8_lossy(&bytes).trim_end().to_string();
                    let _ = handle.emit("install://stdout", OutputPayload { run_id, line });
                }
                CommandEvent::Stderr(bytes) => {
                    let line = String::from_utf8_lossy(&bytes).trim_end().to_string();
                    let _ = handle.emit("install://stderr", OutputPayload { run_id, line });
                }
                CommandEvent::Error(err) => {
                    let _ = handle.emit("install://stderr", OutputPayload { run_id, line: err });
                }
                CommandEvent::Terminated(payload) => {
                    let code = payload.code;
                    let _ = handle.emit(
                        "install://exit",
                        ExitPayload {
                            run_id,
                            code,
                            success: code == Some(0),
                        },
                    );
                    terminated = true;
                }
                _ => {}
            }
        }
        if !terminated {
            let _ = handle.emit(
                "install://exit",
                ExitPayload {
                    run_id,
                    code: None,
                    success: false,
                },
            );
        }
        if let Some(state) = handle.try_state::<EngineState>() {
            if let Ok(mut guard) = state.child.lock() {
                // Only this run may clear the slot. A cancelled run's exit task
                // used to set it to None after a retry had stored a new child,
                // making the retry uncancellable and delivering stale events.
                if owns_slot(guard.as_ref().map(|r| r.run_id), run_id) {
                    *guard = None;
                }
            }
        }
    });

    Ok(run_id)
}

/// cancel_install kills the running engine, if any.
#[tauri::command]
fn cancel_install(state: State<'_, EngineState>) -> Result<(), String> {
    if let Some(running) = state.child.lock().map_err(|e| e.to_string())?.take() {
        running.child.kill().map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .manage(EngineState::default())
        .invoke_handler(tauri::generate_handler![start_install, cancel_install])
        .run(tauri::generate_context!())
        .expect("error while running the Naslos installer");
}

#[cfg(test)]
mod tests {
    use super::owns_slot;

    #[test]
    fn a_finished_run_clears_only_its_own_slot() {
        assert!(owns_slot(Some(7), 7));
        // Cancel then retry: run 7's exit must not clear run 8.
        assert!(!owns_slot(Some(8), 7));
        assert!(!owns_slot(None, 7));
    }
}
