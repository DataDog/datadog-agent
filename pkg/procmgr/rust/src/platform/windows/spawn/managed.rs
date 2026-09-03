// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::{info, warn};
use std::process::Stdio;
use tokio::process::Command;

use crate::handle::ProcessHandle;
use crate::process::ManagedProcess;
use crate::spawn::{SpawnProfile, SpawnRequest, profile_for};
use crate::spawn_context;

use super::super::{
    apply_child_baseline_env, setup_process_group, JobObject, stdout_inheritable,
    stderr_inheritable,
};
use super::credential::SpawnCredential;
use super::primary_token::spawn_as_primary_token;
use super::stdio::to_command_stdio;

pub(crate) fn spawn_child_handle(process: &mut ManagedProcess) -> Result<ProcessHandle> {
    let profile = profile_for(process.name());
    let request = SpawnRequest::from_config(process.name(), process.config(), profile)?;

    let process_name = process.name().to_owned();
    info!("[{process_name}] spawn profile: {profile}");

    if matches!(profile, SpawnProfile::Privileged) {
        return spawn_privileged_inherit(process, &process_name, request);
    }

    spawn_agent_logon(process, &process_name, request)
}

fn spawn_agent_logon(
    process: &mut ManagedProcess,
    process_name: &str,
    request: SpawnRequest,
) -> Result<ProcessHandle> {
    let job = JobObject::new()
        .with_context(|| format!("[{process_name}] create job object for child supervision"))?;

    let credential = SpawnCredential::resolve(SpawnProfile::Agent)
        .with_context(|| format!("[{process_name}] resolve spawn credential"))?;
    process.set_intended_user(credential.display_name());

    let (suspended, user_profile) = spawn_as_primary_token(process_name, &request, &credential)
        .with_context(|| format!("[{process_name}] CreateProcessAsUserW spawn failed"))?;
    if let Some(profile) = user_profile {
        process.set_user_profile_guard(profile);
    }

    suspended
        .supervise(process, job)
        .with_context(|| format!("[{process_name}] start supervised child"))
}

fn spawn_privileged_inherit(
    process: &mut ManagedProcess,
    process_name: &str,
    request: SpawnRequest,
) -> Result<ProcessHandle> {
    process.set_intended_user(
        SpawnCredential::resolve(SpawnProfile::Privileged)
            .map(|credential| credential.display_name())
            .unwrap_or_else(|_| r"NT AUTHORITY\SYSTEM".to_string()),
    );

    let SpawnRequest {
        command,
        args,
        env,
        working_dir,
        stdout_setting,
        stderr_setting,
    } = request;

    let mut cmd = Command::new(&command);
    cmd.args(&args);
    cmd.env_clear();
    apply_child_baseline_env(&mut cmd);
    for (k, v) in env {
        cmd.env(k, v);
    }
    if let Some(dir) = working_dir {
        cmd.current_dir(dir);
    }
    cmd.stdin(Stdio::null());
    cmd.stdout(to_command_stdio(&stdout_setting, stdout_inheritable()));
    cmd.stderr(to_command_stdio(&stderr_setting, stderr_inheritable()));
    setup_process_group(&mut cmd);

    let child = cmd
        .spawn()
        .with_context(|| spawn_context::failed_message(process_name, &command))?;

    let pid = child.id().unwrap_or(0);
    let handle = ProcessHandle::from_tokio_child(child)?;

    match JobObject::new() {
        Ok(job) => match job.assign_process(pid) {
            Ok(()) => process.set_job_object(job),
            Err(e) => warn!("[{process_name}] failed to assign to job object: {e:#}"),
        },
        Err(e) => warn!("[{process_name}] failed to create job object: {e:#}"),
    }

    Ok(handle)
}
