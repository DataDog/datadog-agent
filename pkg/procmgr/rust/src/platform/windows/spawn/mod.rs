// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows child spawning by spawn profile (`CreateProcessAsUserW` vs inherited LocalSystem).

mod command;
mod logon;
mod primary_token;
mod privileged;
mod stdio;

use anyhow::{Context, Result};
use log::info;

use crate::handle::ProcessHandle;
use crate::spawn::{SpawnProfile, SpawnRequest};

use super::agent_credentials::{AgentAccount, resolve_agent_account};

use command::{build_command, spawn_as_local_system};
use primary_token::spawn_as_primary_token;

/// Spawn a managed child using the platform spawn profile for `process_name`.
///
/// Caller must hold [`super::console_lock`] on Windows (see `ManagedProcess::try_spawn`).
pub(crate) fn spawn_child(
    process_name: &str,
    request: SpawnRequest,
    profile: SpawnProfile,
) -> Result<ProcessHandle> {
    info!("[{process_name}] spawn profile: {profile}");

    if matches!(profile, SpawnProfile::Privileged) {
        privileged::validate_process_request(process_name, &request)?;
    }

    match profile {
        SpawnProfile::Agent => spawn_agent_profile(process_name, &request),
        SpawnProfile::Privileged => spawn_privileged_profile(process_name, request),
    }
}

/// Agent-profile children must run as the configured service account via `CreateProcessAsUserW`.
///
/// `tokio::process::Command::spawn` always inherits the supervisor primary token (LocalSystem),
/// so impersonation cannot be used as a fallback here.
fn spawn_agent_profile(process_name: &str, request: &SpawnRequest) -> Result<ProcessHandle> {
    let account = resolve_agent_account()
        .with_context(|| format!("[{process_name}] resolve agent service account for spawn"))?;
    spawn_as_primary_token(process_name, request, &account).with_context(|| {
        format!(
            "[{process_name}] agent-profile spawn requires CreateProcessAsUserW as the configured agent account"
        )
    })
}

/// Privileged children run as LocalSystem. Prefer `CreateProcessAsUserW`; if that fails,
/// fall back to inheriting the supervisor token (also LocalSystem).
fn spawn_privileged_profile(process_name: &str, request: SpawnRequest) -> Result<ProcessHandle> {
    match spawn_as_primary_token(process_name, &request, &AgentAccount::LocalSystem) {
        Ok(handle) => Ok(handle),
        Err(e) => {
            log::warn!(
                "[{process_name}] primary-token LocalSystem spawn failed (trying inherited supervisor token): {e:#}"
            );
            let (command, mut cmd) = build_command(request)?;
            spawn_as_local_system(process_name, &command, &mut cmd).with_context(|| {
                format!("[{process_name}] privileged spawn failed: could not run as LocalSystem")
            })
        }
    }
}
