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
//! | process-agent | `User=dd-agent` → [`SpawnProfile::Agent`] | `LocalSystem` → [`SpawnProfile::Host`] |
//! | trace, PAR, DDOT, … | `User=dd-agent` → [`SpawnProfile::Agent`] | `ddagentuser` → [`SpawnProfile::Agent`] |
//!
//! - [`SpawnProfile::Agent`]: run as the Datadog agent service account (`dd-agent` on
//!   Linux, `ddagentuser` on Windows).
//! - [`SpawnProfile::Host`]: inherit the supervisor security context (for example
//!   `LocalSystem` on Windows when dd-procmgr runs as LocalSystem).

/// Procmgr process name for the process-agent (`processes.d` basename stem).
pub const DATADOG_AGENT_PROCESS: &str = "datadog-agent-process";

/// How a managed child process is spawned on the current platform.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SpawnProfile {
    /// Run as the agent service account (default for most processes).
    Agent,
    /// Inherit the supervisor security context.
    Host,
}

impl SpawnProfile {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Agent => "agent",
            Self::Host => "host",
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
/// as a host-privileged Windows SCM service use [`SpawnProfile::Host`].
pub fn profile_for(process_name: &str) -> SpawnProfile {
    match process_name {
        DATADOG_AGENT_PROCESS if cfg!(windows) => SpawnProfile::Host,
        _ => SpawnProfile::Agent,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[cfg(windows)]
    #[test]
    fn datadog_agent_process_matches_local_system_scm_service() {
        assert_eq!(profile_for("datadog-agent-process"), SpawnProfile::Host);
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
