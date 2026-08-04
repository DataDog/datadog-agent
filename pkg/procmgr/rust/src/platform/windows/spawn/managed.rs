// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::{info, warn};

use crate::handle::ProcessHandle;
use crate::process::ManagedProcess;
use crate::spawn::{SpawnProfile, SpawnRequest, profile_for};

use super::super::JobObject;
use super::privileged;
use super::profiles::{spawn_agent_profile, spawn_privileged_profile};

/// Build a [`SpawnRequest`], spawn the child, and assign it to a supervision job.
///
/// Post-spawn `AssignProcessToJobObject` runs here (not in profile modules) because
/// `CreateProcessAsUserW` cannot pass `PROC_THREAD_ATTRIBUTE_JOB_LIST` under impersonation.
///
/// Caller must hold [`super::super::console_lock`] on Windows (see `ManagedProcess::try_spawn`).
pub(crate) fn spawn_child_handle(process: &mut ManagedProcess) -> Result<ProcessHandle> {
    let profile = profile_for(process.name());
    let request = SpawnRequest::from_config(process.name(), process.config(), profile)?;

    let process_name = process.name();
    info!("[{process_name}] spawn profile: {profile}");
    if matches!(profile, SpawnProfile::Privileged) {
        privileged::validate_process_request(process_name, &request)?;
    }

    // Create the job before spawning so a CreateJobObjectW / SetInformationJobObject
    // failure cannot leave a running child with no retained handle or watcher.
    let job = JobObject::new()
        .with_context(|| format!("[{process_name}] create job object for child supervision"))?;

    let handle = match profile {
        SpawnProfile::Agent => spawn_agent_profile(process_name, &request)?,
        SpawnProfile::Privileged => spawn_privileged_profile(process_name, request)?,
    };

    assign_supervision_job(process, &handle, job)?;
    Ok(handle)
}

/// Assign the child to a pre-created job and store it on the process only when assignment succeeds.
///
/// Assignment failure is best-effort: an empty job would make `force_kill`'s
/// `TerminateJobObject` a no-op while skipping the `TerminateProcess` fallback.
fn assign_supervision_job(
    process: &mut ManagedProcess,
    handle: &ProcessHandle,
    job: JobObject,
) -> Result<()> {
    let Some(pid) = handle.id() else {
        return Ok(());
    };
    let process_name = process.name();
    match job.assign_process(pid) {
        Ok(()) => process.set_job_object(job),
        Err(e) => warn!("[{process_name}] failed to assign to job object: {e:#}"),
    }
    Ok(())
}
