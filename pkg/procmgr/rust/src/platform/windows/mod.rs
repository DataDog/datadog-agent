// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod child_env;
mod console;
mod job_object;
mod process;
mod runtime_user;
mod spawn;
mod wide;

use crate::spawn::SpawnProfile;
use anyhow::{Context, Result};
use std::path::PathBuf;
use std::sync::OnceLock;
use tokio::sync::Notify;

pub(crate) use spawn::spawn_child_handle;

pub use child_env::apply_child_baseline_env;
pub use console::{
    last_signal, send_force_kill, send_graceful_stop, setup_process_group, stderr_inheritable,
    stdout_inheritable,
};
pub use job_object::JobObject;
pub(crate) use runtime_user::runtime_user_for_pid;

static SHUTDOWN_NOTIFY: OnceLock<Notify> = OnceLock::new();

pub(crate) use console::console_lock;

pub fn shutdown_notify() -> &'static Notify {
    SHUTDOWN_NOTIFY.get_or_init(Notify::new)
}

fn open_datadog_agent_key() -> Option<windows_registry::Key> {
    use windows_registry::LOCAL_MACHINE;
    use windows_sys::Win32::System::Registry::KEY_WOW64_64KEY;

    LOCAL_MACHINE
        .options()
        .read()
        .access(KEY_WOW64_64KEY)
        .open(r"SOFTWARE\Datadog\Datadog Agent")
        .ok()
}

fn registry_nonempty_string(key: &windows_registry::Key, name: &str) -> Option<String> {
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

/// Wait for a shutdown trigger (Ctrl+C or SCM stop via [`shutdown_notify()`]).
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

const PRIVILEGED_SPAWN_USER: &str = r"NT AUTHORITY\SYSTEM";

pub(crate) fn intended_spawn_user(process_name: &str, profile: SpawnProfile) -> String {
    match profile {
        SpawnProfile::Privileged => PRIVILEGED_SPAWN_USER.to_string(),
        SpawnProfile::Agent => agent_account_display_from_registry(process_name),
    }
}

fn agent_account_display_from_registry(process_name: &str) -> String {
    match try_agent_account_display_from_registry() {
        Ok(display) => display,
        Err(e) => {
            log::warn!("[{process_name}] intended spawn user lookup failed: {e:#}");
            "unknown".to_string()
        }
    }
}

fn try_agent_account_display_from_registry() -> Result<String> {
    let key = open_datadog_agent_key().context("open Datadog Agent registry key")?;
    let user = registry_nonempty_string(&key, "installedUser")
        .context("read installedUser from registry")?;
    let domain = key.get_string("installedDomain").unwrap_or_default();
    Ok(format_account_display(&domain, &user))
}

fn format_account_display(domain: &str, user: &str) -> String {
    let domain = domain.trim();
    if domain.is_empty() || domain == "." {
        format!(r".\{user}")
    } else {
        format!(r"{domain}\{user}")
    }
}

#[cfg(test)]
mod intended_spawn_user_tests {
    use super::*;
    use crate::spawn::SpawnProfile;

    #[test]
    fn privileged_profile_uses_local_system() {
        assert_eq!(
            intended_spawn_user("datadog-agent-process", SpawnProfile::Privileged),
            PRIVILEGED_SPAWN_USER
        );
    }

    #[test]
    fn format_account_display_local_uses_dot_prefix() {
        assert_eq!(format_account_display("", "ddagentuser"), r".\ddagentuser");
        assert_eq!(format_account_display(".", "ddagentuser"), r".\ddagentuser");
    }

    #[test]
    fn format_account_display_domain() {
        assert_eq!(format_account_display("CORP", "gmsa$"), r"CORP\gmsa$");
    }
}
