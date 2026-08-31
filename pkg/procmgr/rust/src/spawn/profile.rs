// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

pub const DATADOG_AGENT_PROCESS: &str = "datadog-agent-process";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SpawnProfile {
    Agent,
    Privileged,
}

impl std::fmt::Display for SpawnProfile {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(match self {
            Self::Agent => "agent",
            Self::Privileged => "privileged",
        })
    }
}

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
        assert_eq!(
            profile_for("datadog-agent-process"),
            SpawnProfile::Privileged
        );
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
