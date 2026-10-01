//! Tauri shell for the Naslos installer.
//!
//! The shell owns no install logic: it spawns the `naslos-install` sidecar,
//! forwards the engine's newline-delimited JSON progress to the webview as
//! `install://stdout`/`install://stderr` events, emits `install://exit` when the
//! child ends, and kills the child on `cancel_install` (docs/installer-contract.md
//! §5). All command arguments are defined here; the webview never supplies a
//! shell string (SEC-1).

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
    #[serde(default)]
    pub name: String,
    pub admin_user: String,
    pub admin_password: String,
    #[serde(default)]
    pub add_resolver: bool,
}

#[derive(Debug, Clone, Serialize)]
struct ExitPayload {
    code: Option<i32>,
    success: bool,
}

/// EngineState holds the running sidecar so it can be cancelled.
#[derive(Default)]
struct EngineState {
    child: Mutex<Option<CommandChild>>,
}

/// start_install spawns the engine sidecar and streams its output. It refuses to
/// start a second install while one is running.
#[tauri::command]
fn start_install(
    app: AppHandle,
    state: State<'_, EngineState>,
    input: InstallInput,
) -> Result<(), String> {
    {
        let guard = state.child.lock().map_err(|e| e.to_string())?;
        if guard.is_some() {
            return Err("an install is already running".into());
        }
    }

    let mut args = vec![
        "--node-ip".to_string(),
        input.node_ip,
        "--domain".to_string(),
        input.domain,
        "--admin-user".to_string(),
        input.admin_user,
        "--admin-password".to_string(),
        input.admin_password,
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
    let (mut rx, child) = sidecar.args(args).spawn().map_err(|e| e.to_string())?;
    *state.child.lock().map_err(|e| e.to_string())? = Some(child);

    let handle = app.clone();
    tauri::async_runtime::spawn(async move {
        let mut terminated = false;
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(bytes) => {
                    let line = String::from_utf8_lossy(&bytes).trim_end().to_string();
                    let _ = handle.emit("install://stdout", line);
                }
                CommandEvent::Stderr(bytes) => {
                    let line = String::from_utf8_lossy(&bytes).trim_end().to_string();
                    let _ = handle.emit("install://stderr", line);
                }
                CommandEvent::Error(err) => {
                    let _ = handle.emit("install://stderr", err);
                }
                CommandEvent::Terminated(payload) => {
                    let code = payload.code;
                    let _ = handle.emit(
                        "install://exit",
                        ExitPayload {
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
                    code: None,
                    success: false,
                },
            );
        }
        if let Some(state) = handle.try_state::<EngineState>() {
            if let Ok(mut guard) = state.child.lock() {
                *guard = None;
            }
        }
    });

    Ok(())
}

/// cancel_install kills the running engine, if any.
#[tauri::command]
fn cancel_install(state: State<'_, EngineState>) -> Result<(), String> {
    if let Some(child) = state.child.lock().map_err(|e| e.to_string())?.take() {
        child.kill().map_err(|e| e.to_string())?;
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
