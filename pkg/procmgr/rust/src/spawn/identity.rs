// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Spawn identity for operator-facing list/describe output.

use log::warn;

use super::profile::SpawnProfile;

/// Resolve the account procmgr intends to use (or last used) for spawn.
pub fn spawn_user_for(process_name: &str, profile: SpawnProfile) -> String {
    match resolve_spawn_user(process_name, profile) {
        Ok(user) => user,
        Err(e) => {
            warn!("[{process_name}] could not resolve spawn user: {e:#}");
            "unknown".to_string()
        }
    }
}

fn resolve_spawn_user(process_name: &str, profile: SpawnProfile) -> anyhow::Result<String> {
    #[cfg(windows)]
    {
        crate::platform::spawn_user_for_profile(process_name, profile)
    }
    #[cfg(unix)]
    {
        let _ = (process_name, profile);
        crate::platform::spawn_user_for_supervisor()
    }
    #[cfg(not(any(windows, unix)))]
    {
        let _ = (process_name, profile);
        anyhow::bail!("unsupported platform")
    }
}

#[cfg(test)]
mod tests {
    use super::super::profile::{SpawnProfile, profile_for};
    use super::*;

    #[test]
    fn profile_for_trace_is_agent() {
        assert_eq!(profile_for("datadog-agent-trace"), SpawnProfile::Agent);
    }

    #[cfg(windows)]
    #[test]
    fn privileged_profile_spawn_user_is_local_system() {
        assert_eq!(
            spawn_user_for("datadog-agent-process", SpawnProfile::Privileged),
            r"NT AUTHORITY\SYSTEM"
        );
    }

    #[cfg(not(windows))]
    #[test]
    fn spawn_user_matches_supervisor_on_unix() {
        use nix::unistd::{User, geteuid};

        let expected = User::from_uid(geteuid())
            .ok()
            .flatten()
            .map(|u| u.name)
            .unwrap_or_else(|| "unknown".to_string());
        assert_eq!(
            spawn_user_for("datadog-agent-trace", SpawnProfile::Agent),
            expected
        );
    }
}
