// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;

use crate::handle::ProcessHandle;
use crate::process::ManagedProcess;
use crate::spawn::{SpawnProfile, SpawnRequest, profile_for};

use super::super::JobObject;
use super::privileged;
use super::profiles::{spawn_agent_profile, spawn_privileged_profile};

/// Build a [`SpawnRequest`], create a supervision [`JobObject`], and spawn the child.
///
/// `CreateProcessAsUserW` assigns the job post-spawn (LocalSystem); the tokio fallback does the same.
///
/// Caller must hold [`super::super::console_lock`] on Windows (see `ManagedProcess::try_spawn`).
pub(crate) fn spawn_child_handle(process: &mut ManagedProcess) -> Result<ProcessHandle> {
    let profile = profile_for(process.name());
    let request = SpawnRequest::from_config(process.name(), process.config(), profile)?;
    let job = JobObject::new().with_context(|| {
        format!(
            "[{}] create job object for child supervision",
            process.name()
        )
    })?;

    let process_name = process.name();
    info!("[{process_name}] spawn profile: {profile}");
    if matches!(profile, SpawnProfile::Privileged) {
        privileged::validate_process_request(process_name, &request)?;
    }

    let handle = match profile {
        SpawnProfile::Agent => spawn_agent_profile(process_name, &request, &job)?,
        SpawnProfile::Privileged => spawn_privileged_profile(process_name, request, &job)?,
    };

    process.set_job_object(job);
    Ok(handle)
}
