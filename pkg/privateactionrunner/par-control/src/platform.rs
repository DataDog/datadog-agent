// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Platform paths par-control needs before it has read any configuration.
//!
//! Deliberately a small copy of the equivalent helpers in
//! `pkg/procmgr/rust/src/platform/windows.rs`. Sharing them would mean
//! extracting a crate out of the shipped dd-procmgrd daemon; keep the two in
//! sync until that refactor happens.

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

    /// Root directory for agent program data (config, logs).
    ///
    /// Mirrors `pkg/util/winutil.GetProgramDataDir` in Go and
    /// `platform::program_data_root` in dd-procmgrd:
    /// `HKLM\SOFTWARE\Datadog\Datadog Agent\ConfigRoot`, else `%ProgramData%\Datadog`.
    pub fn program_data_root() -> PathBuf {
        open_datadog_agent_key()
            .and_then(|k| registry_nonempty_string(&k, "ConfigRoot"))
            .map(PathBuf::from)
            .unwrap_or_else(default_program_data_root)
    }

    /// `fleet_policies_dir` as written to the registry by the installer. Linux
    /// passes the equivalent through `DD_FLEET_POLICIES_DIR` in the unit file.
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

/// Default `datadog.yaml` location, matching the Agent's own default.
pub fn default_config_path() -> &'static str {
    #[cfg(windows)]
    {
        // The process-manager unit passes `--config` explicitly; this default only
        // matters for interactive runs.
        r"C:\ProgramData\Datadog\datadog.yaml"
    }
    #[cfg(not(windows))]
    {
        "/etc/datadog-agent/datadog.yaml"
    }
}

/// Where par-control should persist its own diagnostics, or `None` when stdout
/// is durable.
///
/// On Windows, services normally have no inheritable stdout/stderr handles, so
/// dd-procmgrd's `stdout: inherit` degrades to the null device
/// (`stdio_from_config` in `pkg/procmgr/rust/src/process.rs`). Write to the
/// standard agent log directory instead, exactly as dd-procmgrd does for
/// itself. The path is derived from the program-data root rather than from
/// `--config` so that relocating the config does not relocate the logs.
pub fn default_log_file() -> Option<std::path::PathBuf> {
    #[cfg(windows)]
    {
        Some(program_data_root().join("logs").join("par-control.log"))
    }
    #[cfg(not(windows))]
    {
        // dd-procmgrd inherits stdout into the agent's own logging.
        None
    }
}
