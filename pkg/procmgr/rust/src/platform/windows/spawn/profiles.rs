// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Profile-specific Windows spawn paths (`CreateProcessAsUserW` vs inherited LocalSystem).

use anyhow::{Context, Result};
use log::warn;

use crate::handle::ProcessHandle;
use crate::spawn::SpawnRequest;

use super::super::JobObject;
use super::super::agent_credentials::{AgentAccount, resolve_agent_account};
use super::command::{build_command, spawn_as_local_system};
use super::primary_token::spawn_as_primary_token;

/// Agent-profile children must run as the configured service account via `CreateProcessAsUserW`.
///
/// `tokio::process::Command::spawn` always inherits the supervisor primary token (LocalSystem),
/// so impersonation cannot be used as a fallback here.
pub(super) fn spawn_agent_profile(
    process_name: &str,
    request: &SpawnRequest,
    job: &JobObject,
) -> Result<ProcessHandle> {
    let account = resolve_agent_account()
        .with_context(|| format!("[{process_name}] resolve agent service account for spawn"))?;
    spawn_as_primary_token(process_name, request, &account, job).with_context(|| {
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
    job: &JobObject,
) -> Result<ProcessHandle> {
    match spawn_as_primary_token(process_name, &request, &AgentAccount::LocalSystem, job) {
        Ok(handle) => Ok(handle),
        Err(e) => {
            warn!(
                "[{process_name}] primary-token LocalSystem spawn failed (trying inherited supervisor token): {e:#}"
            );
            let (command, mut cmd) = build_command(request)?;
            let handle =
                spawn_as_local_system(process_name, &command, &mut cmd).with_context(|| {
                    format!(
                        "[{process_name}] privileged spawn failed: could not run as LocalSystem"
                    )
                })?;
            assign_child_to_job(process_name, job, &handle);
            Ok(handle)
        }
    }
}

/// Post-spawn job assignment for backends that cannot pass `PROC_THREAD_ATTRIBUTE_JOB_LIST`.
fn assign_child_to_job(process_name: &str, job: &JobObject, handle: &ProcessHandle) {
    if let Some(pid) = handle.id()
        && let Err(e) = job.assign_process(pid)
    {
        warn!("[{process_name}] failed to assign to job object: {e:#}");
    }
}
