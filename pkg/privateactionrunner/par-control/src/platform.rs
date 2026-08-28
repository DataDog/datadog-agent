// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#[cfg(windows)]
mod imp {
    use std::path::PathBuf;

    /// `fleet_policies_dir`, which config experiments repoint in the registry.
    /// Mirrors `FleetConfigOverride` in `pkg/config/setup/config_windows.go`,
    /// including letting environment and local YAML win over the registry.
    pub fn fleet_policies_dir() -> Option<String> {
        use windows_registry::LOCAL_MACHINE;
        use windows_sys::Win32::System::Registry::KEY_WOW64_64KEY;

        let key = LOCAL_MACHINE
            .options()
            .read()
            .access(KEY_WOW64_64KEY)
            .open(r"SOFTWARE\Datadog\Datadog Agent")
            .ok()?;
        let value: String = key.get_string("fleet_policies_dir").ok()?;
        if value.is_empty() { None } else { Some(value) }
    }

    /// Fallback for manual runs. dd-procmgrd always passes `--config`.
    pub fn default_config_path() -> PathBuf {
        let base = std::env::var("ProgramData").unwrap_or_else(|_| r"C:\ProgramData".to_string());
        PathBuf::from(base).join("Datadog").join("datadog.yaml")
    }
}

#[cfg(not(windows))]
mod imp {
    use std::path::PathBuf;

    pub fn fleet_policies_dir() -> Option<String> {
        None
    }

    pub fn default_config_path() -> PathBuf {
        PathBuf::from("/etc/datadog-agent/datadog.yaml")
    }
}

pub use imp::{default_config_path, fleet_policies_dir};
