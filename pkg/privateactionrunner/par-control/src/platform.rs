// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Platform paths needed before configuration is loaded.
//!
//! Keep the Windows helpers aligned with `pkg/procmgr/rust/src/platform/windows.rs`.

#[cfg(windows)]
mod imp {
    use std::path::PathBuf;

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

    fn default_program_data_root() -> PathBuf {
        let base = std::env::var("ProgramData").unwrap_or_else(|_| r"C:\ProgramData".to_string());
        PathBuf::from(base).join("Datadog")
    }

    /// Agent program-data root from `ConfigRoot`, or `%ProgramData%\Datadog`.
    pub fn program_data_root() -> PathBuf {
        open_datadog_agent_key()
            .and_then(|k| registry_nonempty_string(&k, "ConfigRoot"))
            .map(PathBuf::from)
            .unwrap_or_else(default_program_data_root)
    }

    /// `fleet_policies_dir` written by the Windows installer.
    pub fn fleet_policies_dir() -> Option<String> {
        open_datadog_agent_key().and_then(|k| registry_nonempty_string(&k, "fleet_policies_dir"))
    }
}

#[cfg(not(windows))]
mod imp {
    pub fn fleet_policies_dir() -> Option<String> {
        None
    }
}

pub use imp::fleet_policies_dir;
#[cfg(windows)]
pub use imp::program_data_root;

pub fn default_config_path() -> &'static str {
    #[cfg(windows)]
    {
        r"C:\ProgramData\Datadog\datadog.yaml"
    }
    #[cfg(not(windows))]
    {
        "/etc/datadog-agent/datadog.yaml"
    }
}

/// Log to a file on Windows, where services normally lack stdout/stderr handles.
pub fn default_log_file() -> Option<std::path::PathBuf> {
    #[cfg(windows)]
    {
        Some(program_data_root().join("logs").join("par-control.log"))
    }
    #[cfg(not(windows))]
    {
        None
    }
}
