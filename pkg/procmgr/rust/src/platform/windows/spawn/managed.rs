// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::{info, warn};

use crate::config::ProcessConfig;
use crate::process::{ManagedChildSpawn, ManagedProcess};
use crate::spawn::{SpawnProfile, SpawnRequest, profile_for};

use super::super::JobObject;
use super::credential::SpawnCredential;
use super::primary_token::spawn_as_primary_token;
use super::privileged;

pub(crate) fn spawn_managed_child(
    process_name: &str,
    config: &ProcessConfig,
) -> Result<ManagedChildSpawn> {
    let profile = profile_for(process_name);
    let request = SpawnRequest::from_config(process_name, config, profile)?;

    info!("[{process_name}] spawn profile: {profile}");
    if matches!(profile, SpawnProfile::Privileged) {
        privileged::validate_process_request(process_name, &request)?;
    }

    let job = JobObject::new()
        .with_context(|| format!("[{process_name}] create job object for child supervision"))?;

    let credential = SpawnCredential::resolve(profile)
        .with_context(|| format!("[{process_name}] resolve spawn credential"))?;
    let intended_user = credential.display_name();

    let (suspended, user_profile) = spawn_as_primary_token(process_name, &request, &credential)
        .with_context(|| format!("[{process_name}] CreateProcessAsUserW spawn failed"))?;

    let (handle, job_object) = suspended
        .supervise(process_name, job)
        .with_context(|| format!("[{process_name}] start supervised child"))?;

    Ok(ManagedChildSpawn {
        handle,
        intended_user,
        job_object,
        user_profile,
    })
}

pub(crate) async fn abort_uncommitted_spawn(spawn: ManagedChildSpawn, process_name: &str) {
    let ManagedChildSpawn {
        mut handle,
        intended_user: _,
        job_object,
        user_profile,
    } = spawn;

    if let Err(e) = job_object.terminate() {
        warn!("[{process_name}] failed to terminate uncommitted spawn job: {e:#}");
    }
    let pid = handle.id().unwrap_or(0);
    if let Err(e) = handle.kill().await {
        warn!("[{process_name}] failed to terminate uncommitted spawn (pid={pid}): {e:#}");
    }
    if let Err(e) = handle.wait().await {
        warn!("[{process_name}] failed to wait for uncommitted spawn (pid={pid}): {e:#}");
    }

    let timeout = ManagedProcess::FORCE_KILL_TIMEOUT;
    let drained = tokio::task::spawn_blocking(move || job_object.wait_until_empty(timeout))
        .await
        .unwrap_or(false);
    if !drained {
        warn!(
            "[{process_name}] timed out waiting for uncommitted spawn job to drain before releasing profile"
        );
    }

    drop(user_profile);
}
