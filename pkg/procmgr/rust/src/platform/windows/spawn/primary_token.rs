// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;

use crate::handle::ProcessHandle;
use crate::spawn::SpawnRequest;

use super::super::JobObject;
use super::create_process::{CreateProcessInvoker, spawn_managed_child};
use super::credential::SpawnCredential;
use super::inputs::SpawnInputs;
use super::logon::TokenHandle;
use super::user_profile::UserProfileGuard;

/// Spawn a child with `CreateProcessAsUserW` using a **primary access token**.
///
/// A primary token is the security context the new process runs under (user, groups,
/// privileges). This path is agent-profile `LogonUser`
/// only. Privileged and same-account agent spawns use `CreateProcessW` in
/// `inherit_supervisor`.
///
/// The child is created already assigned to `job` via `PROC_THREAD_ATTRIBUTE_JOB_LIST`.
///
/// Returns `(ProcessHandle, UserProfileGuard)`. The profile guard must stay alive
/// for the child's lifetime.
pub(super) fn spawn_as_primary_token(
    process_name: &str,
    request: &SpawnRequest,
    credential: &SpawnCredential,
    job: &JobObject,
) -> Result<(ProcessHandle, UserProfileGuard)> {
    let primary_token_guard = TokenHandle::new(credential.duplicate_primary_token(process_name)?);

    let profile_guard = UserProfileGuard::load(
        process_name,
        primary_token_guard.raw(),
        credential.account(),
    )?;

    let mut inputs =
        SpawnInputs::prepare(process_name, request, credential, primary_token_guard.raw())?;

    let handle = spawn_managed_child(
        process_name,
        &mut inputs,
        job,
        CreateProcessInvoker::AsUser(primary_token_guard.raw()),
    )?;

    Ok((handle, profile_guard))
}
