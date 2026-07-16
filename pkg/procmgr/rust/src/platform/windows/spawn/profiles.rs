// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Profile-specific Windows spawn paths (`CreateProcessAsUserW` vs inherited LocalSystem).

use anyhow::{Context, Result};
use log::warn;

use crate::handle::ProcessHandle;
use crate::spawn::SpawnRequest;

use super::super::agent_credentials::{resolve_agent_account, AgentAccount};
use super::command::{build_command, spawn_as_local_system};
use super::primary_token::spawn_as_primary_token;

/// Agent-profile children must run as the configured service account via `CreateProcessAsUserW`.
///
/// `tokio::process::Command::spawn` always inherits the supervisor primary token (LocalSystem),
/// so impersonation cannot be used as a fallback here.
pub(super) fn spawn_agent_profile(
    process_name: &str,
    request: &SpawnRequest,
) -> Result<ProcessHandle> {
    let account = resolve_agent_account()
        .with_context(|| format!("[{process_name}] resolve agent service account for spawn"))?;

    if account.inherits_supervisor_token() {
        let (command, mut cmd) = build_command(request)?;
        return spawn_as_local_system(process_name, &command, &mut cmd)
            .with_context(|| format!("[{process_name}] spawn as LocalSystem (supervisor token)"));
    }

    spawn_as_primary_token(process_name, request, &account).with_context(|| {
        format!(
            "[{process_name}] agent-profile spawn requires CreateProcessAsUserW as the configured agent account"
        )
    })
}

/// Privileged children run as LocalSystem. Prefer `CreateProcessAsUserW`; if that fails,
/// fall back to inheriting the supervisor token (also LocalSystem).
pub(super) fn spawn_privileged_profile(
    process_name: &str,
    request: SpawnRequest,
) -> Result<ProcessHandle> {
    match spawn_as_primary_token(process_name, &request, &AgentAccount::LocalSystem) {
        Ok(handle) => Ok(handle),
        Err(e) => {
            warn!(
                "[{process_name}] primary-token LocalSystem spawn failed (trying inherited supervisor token): {e:#}"
            );
            let (command, mut cmd) = build_command(&request)?;
            spawn_as_local_system(process_name, &command, &mut cmd).with_context(|| {
                format!("[{process_name}] privileged spawn failed: could not run as LocalSystem")
            })
        }
    }
}
