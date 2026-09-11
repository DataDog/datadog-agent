// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
use log::{info, warn};
use std::process::Stdio;

use crate::handle::ProcessHandle;
use crate::process::ManagedProcess;
use crate::spawn::{SpawnProfile, SpawnRequest};

use super::super::{
    JobObject, send_force_kill, setup_process_group, stderr_inheritable, stdout_inheritable,
};
use super::credential::SpawnCredential;
use super::inherit_supervisor::spawn_inherit_supervisor;

const PRIVILEGED_INTENDED_USER: &str = r"NT AUTHORITY\SYSTEM";

pub(crate) fn resolve_spawn_identity(
    process_name: &str,
    profile: SpawnProfile,
) -> (String, Option<SpawnCredential>) {
    match profile {
        SpawnProfile::Privileged => (PRIVILEGED_INTENDED_USER.to_string(), None),
        SpawnProfile::Agent => match SpawnCredential::resolve_agent() {
            Ok(credential) => (credential.display_name(), Some(credential)),
            Err(e) => {
                warn!("[{process_name}] could not resolve intended spawn user: {e:#}");
                ("unknown".to_string(), None)
            }
        },
    }
}

pub(crate) fn spawn_child_handle(process: &mut ManagedProcess) -> Result<ProcessHandle> {
    let profile = process.profile();
    let request = SpawnRequest::from_config(process.name(), process.config())?;

    let process_name = process.name().to_owned();
    info!("[{process_name}] spawn profile: {profile}");

    if matches!(profile, SpawnProfile::Privileged) {
        return spawn_privileged_inherit(process, &process_name, request);
    }

    spawn_agent(process, &process_name, request)
}

fn spawn_agent(
    process: &mut ManagedProcess,
    process_name: &str,
    request: SpawnRequest,
) -> Result<ProcessHandle> {
    let credential = match process.agent_credential().cloned() {
        Some(credential) => credential,
        None => {
            let credential = SpawnCredential::resolve_agent()
                .with_context(|| format!("[{process_name}] resolve spawn credential"))?;
            process.set_intended_user(credential.display_name());
            credential
        }
    };

    if credential.reuses_supervisor_token() {
        return spawn_agent_inherit(process, process_name, request, &credential);
    }

    let intended_user = credential.display_name();
    bail!(
        "[{process_name}] cannot spawn as {intended_user}: supervisor token does not match the \
         installed agent account; run dd-procmgr-service as {intended_user}"
    );
}

fn spawn_agent_inherit(
    process: &mut ManagedProcess,
    process_name: &str,
    request: SpawnRequest,
    credential: &SpawnCredential,
) -> Result<ProcessHandle> {
    let job = JobObject::new()
        .with_context(|| format!("[{process_name}] create job object for child supervision"))?;

    let suspended = spawn_inherit_supervisor(process_name, &request, credential)
        .with_context(|| format!("[{process_name}] supervisor-token inherit spawn failed"))?;

    suspended
        .supervise(process, job)
        .with_context(|| format!("[{process_name}] start supervised child"))
}

fn spawn_privileged_inherit(
    process: &mut ManagedProcess,
    process_name: &str,
    request: SpawnRequest,
) -> Result<ProcessHandle> {
    let mut cmd = request.to_command(stdout_inheritable(), stderr_inheritable());
    cmd.stdin(Stdio::null());
    setup_process_group(&mut cmd);

    let child = cmd
        .spawn()
        .with_context(|| format!("[{process_name}] failed to spawn: {}", request.command()))?;

    let pid = child.id().unwrap_or(0);
    let handle = match ProcessHandle::from_tokio_child(child) {
        Ok(handle) => handle,
        Err(e) => {
            terminate_unsupervised_child(process_name, pid);
            return Err(e);
        }
    };

    let job = JobObject::new()
        .inspect_err(|_| {
            terminate_unsupervised_child(process_name, pid);
        })
        .with_context(|| format!("[{process_name}] create job object for child supervision"))?;

    job.assign_process(pid)
        .inspect_err(|_| {
            terminate_unsupervised_child(process_name, pid);
        })
        .with_context(|| {
            format!("[{process_name}] failed to assign pid {pid} to supervision job")
        })?;

    process.set_job_object(job);
    Ok(handle)
}

fn terminate_unsupervised_child(process_name: &str, pid: u32) {
    if let Err(e) = send_force_kill(pid) {
        warn!(
            "[{process_name}] failed to terminate unsupervised child (pid={pid}) after job setup failure: {e:#}"
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::spawn::SpawnProfile;

    #[test]
    fn privileged_profile_spawn_user_is_local_system() {
        assert_eq!(
            resolve_spawn_identity("datadog-agent-process", SpawnProfile::Privileged).0,
            PRIVILEGED_INTENDED_USER
        );
    }
}
