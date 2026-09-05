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

impl SpawnProfile {
    pub fn profile_for(process_name: &str) -> Self {
        match process_name {
            DATADOG_AGENT_PROCESS if cfg!(windows) => Self::Privileged,
            _ => Self::Agent,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[cfg(windows)]
    #[test]
    fn datadog_agent_process_uses_privileged_profile_on_windows() {
        assert_eq!(
            SpawnProfile::profile_for("datadog-agent-process"),
            SpawnProfile::Privileged
        );
    }

    #[cfg(not(windows))]
    #[test]
    fn datadog_agent_process_uses_agent_profile_on_unix() {
        assert_eq!(
            SpawnProfile::profile_for("datadog-agent-process"),
            SpawnProfile::Agent
        );
    }

    #[test]
    fn other_process_names_use_agent_profile() {
        for name in [
            "datadog-agent-action",
            "datadog-agent-ddot",
            "datadog-agent-trace",
            "unknown-process",
        ] {
            assert_eq!(
                SpawnProfile::profile_for(name),
                SpawnProfile::Agent,
                "{name}"
            );
        }
    }
}
