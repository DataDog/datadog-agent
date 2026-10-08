// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use windows_sys::Win32::Security::{TOKEN_DUPLICATE, TOKEN_QUERY};

use crate::handle::ProcessHandle;
use crate::spawn::SpawnRequest;

use super::super::JobObject;
use super::super::token_identity::open_current_process_token;
use super::create_process::spawn_managed_child;
use super::credential::SpawnCredential;
use super::inputs::SpawnInputs;

/// Spawns a child in the supervisor's security context (`CreateProcessW`).
///
/// `CreateProcessAsUserW` is not used here: the child matches dd-procmgrd's identity, and that
/// API requires `SeIncreaseQuotaPrivilege` that the installed agent account does not hold.
///
/// Used for privileged spawn and for agent-profile spawn when the supervisor already
/// runs as the target account.
///
/// The child joins `job` at create time via `PROC_THREAD_ATTRIBUTE_JOB_LIST`.
pub(super) fn spawn_inherit_supervisor(
    process_name: &str,
    request: &SpawnRequest,
    credential: &SpawnCredential,
    job: &JobObject,
) -> Result<ProcessHandle> {
    let supervisor_token =
        open_current_process_token(TOKEN_QUERY | TOKEN_DUPLICATE).map_err(|e| {
            anyhow::anyhow!("[{process_name}] OpenProcessToken(GetCurrentProcess()) failed: {e}")
        })?;
    let mut inputs = SpawnInputs::prepare(
        process_name,
        request,
        credential,
        supervisor_token.as_handle(),
    )?;

    spawn_managed_child(process_name, &mut inputs, job)
}
