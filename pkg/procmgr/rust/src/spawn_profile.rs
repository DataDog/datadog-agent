// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Platform spawn profiles for managed child processes.
//!
//! Process YAML stays portable (command, args, env, restart, etc.). Identity and
//! privilege level are chosen here from the process name, not from config fields.
//!
//! Profiles mirror legacy supervisor units (systemd / Windows SCM), not a single
//! account name across platforms:
//!
//! | Process | Linux (`datadog-agent-process.service`) | Windows (`datadog-process-agent`) |
//! |---------|----------------------------------------|-----------------------------------|
//! | process-agent | `User=dd-agent` → [`SpawnProfile::Agent`] | `LocalSystem` → [`SpawnProfile::Privileged`] |
//! | trace, PAR, DDOT, … | `User=dd-agent` → [`SpawnProfile::Agent`] | `ddagentuser` → [`SpawnProfile::Agent`] |
//!
//! - [`SpawnProfile::Agent`]: run as the Datadog agent service account (`dd-agent` on
//!   Linux, `ddagentuser` on Windows).
//! - [`SpawnProfile::Privileged`]: run with the legacy SCM-like privilege level for
//!   the process (on Windows that corresponds to `NT AUTHORITY\\SYSTEM`).

/// Procmgr process name for the process-agent (`processes.d` basename stem).
pub const DATADOG_AGENT_PROCESS: &str = "datadog-agent-process";

/// How a managed child process is spawned on the current platform.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SpawnProfile {
    /// Run as the agent service account (default for most processes).
    Agent,
    /// Run with the legacy privileged level for this process.
    ///
    /// Platform-specific implementations map this to the equivalent privilege
    /// source (e.g. Windows `LocalSystem` impersonation).
    Privileged,
}

impl SpawnProfile {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Agent => "agent",
            Self::Privileged => "privileged",
        }
    }
}

impl std::fmt::Display for SpawnProfile {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

/// Resolve the spawn profile for a process from its procmgr name.
///
/// Unknown processes default to [`SpawnProfile::Agent`]. Only processes that ran
/// with the legacy privileged SCM level use [`SpawnProfile::Privileged`].
pub fn profile_for(process_name: &str) -> SpawnProfile {
    match process_name {
        DATADOG_AGENT_PROCESS if cfg!(windows) => SpawnProfile::Privileged,
        _ => SpawnProfile::Agent,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[cfg(windows)]
    #[test]
    fn datadog_agent_process_matches_local_system_scm_service() {
        assert_eq!(profile_for("datadog-agent-process"), SpawnProfile::Privileged);
    }

    #[cfg(not(windows))]
    #[test]
    fn datadog_agent_process_matches_dd_agent_systemd_service() {
        assert_eq!(profile_for("datadog-agent-process"), SpawnProfile::Agent);
    }

    #[test]
    fn unknown_and_other_processes_use_agent_profile() {
        for name in [
            "datadog-agent-action",
            "datadog-agent-ddot",
            "datadog-agent-trace",
            "unknown-process",
        ] {
            assert_eq!(profile_for(name), SpawnProfile::Agent, "{name}");
        }
    }
}
