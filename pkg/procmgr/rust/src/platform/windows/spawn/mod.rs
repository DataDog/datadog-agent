// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows child spawning by spawn profile (inherit vs agent-user impersonation).

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

use command::{spawn_as_agent_user, spawn_as_local_system};
use primary_token::spawn_as_primary_token;
use privileged::validate_privileged_process_request;

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
        validate_privileged_process_request(process_name, &request)?;
    }

    // Prefer primary-token spawning (explicit stdio handles). Privileged fallbacks inherit
    // the supervisor token; dd-procmgr-service runs as LocalSystem on Windows.
    match profile {
        SpawnProfile::Privileged => {
            match spawn_as_primary_token(process_name, &request, &AgentAccount::LocalSystem) {
                Ok(handle) => return Ok(handle),
                Err(e) => {
                    log::warn!(
                        "[{process_name}] primary-token LocalSystem spawn failed (trying inherited supervisor token): {e:#}"
                    );
                }
            }
        }
        SpawnProfile::Agent => {
            let account = resolve_agent_account().with_context(|| {
                format!("[{process_name}] resolve agent service account for spawn")
            })?;
            match spawn_as_primary_token(process_name, &request, &account) {
                Ok(handle) => return Ok(handle),
                Err(e) if matches!(account, AgentAccount::LocalSystem) => {
                    log::warn!(
                        "[{process_name}] primary-token LocalSystem spawn failed (trying inherited supervisor token): {e:#}"
                    );
                }
                Err(e) => {
                    return Err(e).with_context(|| {
                        format!(
                            "[{process_name}] agent-profile spawn requires CreateProcessAsUserW as the configured agent account"
                        )
                    });
                }
            }
        }
    }

    let (command, mut cmd) = command::build_command(request)?;
    match profile {
        SpawnProfile::Privileged => spawn_as_local_system(process_name, &command, &mut cmd)
            .with_context(|| {
                format!("[{process_name}] privileged spawn failed: could not run as LocalSystem")
            }),
        SpawnProfile::Agent => spawn_as_agent_user(process_name, &command, &mut cmd),
    }
}
