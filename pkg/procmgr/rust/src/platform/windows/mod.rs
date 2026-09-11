// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod agent_service_sid;
mod child_env;
mod console;
mod job_object;
mod local_account;
mod local_agent_account;
mod process;
mod runtime_user;
mod sid;
mod spawn;
mod token_identity;
mod wide;

use std::path::PathBuf;
use std::sync::OnceLock;
use tokio::sync::Notify;

pub(crate) use spawn::{SpawnCredential, resolve_spawn_identity, spawn_child_handle};

pub(crate) use child_env::apply_child_baseline_env;
pub(crate) use child_env::apply_legacy_scm_env;
pub(crate) use child_env::{baseline_env_vars_for_spawn, merge_env_overrides};
pub(crate) use console::console_lock;
pub use console::{
    last_signal, send_force_kill, send_graceful_stop, setup_process_group, stderr_inheritable,
    stdout_inheritable,
};
pub use job_object::JobObject;
pub(crate) use process::{
    ProcessWaitOutcome, WAIT_INFINITE, terminate_process, wait_for_process_exit_ms,
};
pub(crate) use runtime_user::runtime_user_for_pid;

static SHUTDOWN_NOTIFY: OnceLock<Notify> = OnceLock::new();

pub fn shutdown_notify() -> &'static Notify {
    SHUTDOWN_NOTIFY.get_or_init(Notify::new)
}

pub async fn shutdown_signal() {
    tokio::select! {
        result = tokio::signal::ctrl_c() => {
            result.expect("failed to register Ctrl+C handler");
            log::info!("received Ctrl+C");
        }
        _ = shutdown_notify().notified() => {
            log::info!("received service stop request");
        }
    }
}

pub(crate) fn open_datadog_agent_key() -> Option<windows_registry::Key> {
    use windows_registry::LOCAL_MACHINE;
    use windows_sys::Win32::System::Registry::KEY_WOW64_64KEY;

    LOCAL_MACHINE
        .options()
        .read()
        .access(KEY_WOW64_64KEY)
        .open(r"SOFTWARE\Datadog\Datadog Agent")
        .ok()
}

pub(crate) fn registry_nonempty_string(key: &windows_registry::Key, name: &str) -> Option<String> {
    let value: String = key.get_string(name).ok()?;
    if value.is_empty() { None } else { Some(value) }
}

pub fn program_data_root() -> PathBuf {
    open_datadog_agent_key()
        .and_then(|k| registry_nonempty_string(&k, "ConfigRoot"))
        .map(PathBuf::from)
        .unwrap_or_else(default_program_data_dir)
}

fn default_program_data_dir() -> PathBuf {
    let base = std::env::var("ProgramData").unwrap_or_else(|_| r"C:\ProgramData".to_string());
    PathBuf::from(base).join("Datadog")
}

fn install_root_from_registry() -> Option<PathBuf> {
    open_datadog_agent_key()
        .and_then(|k| registry_nonempty_string(&k, "InstallPath"))
        .map(PathBuf::from)
}

fn default_install_root() -> PathBuf {
    let program_files =
        std::env::var("ProgramFiles").unwrap_or_else(|_| r"C:\Program Files".to_string());
    PathBuf::from(program_files)
        .join("Datadog")
        .join("Datadog Agent")
}

fn install_root() -> PathBuf {
    install_root_from_registry().unwrap_or_else(default_install_root)
}

pub fn default_config_dir() -> PathBuf {
    install_root().join("processes.d")
}
