// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::{info, warn};

use crate::handle::ProcessHandle;
use crate::process::ManagedProcess;
use crate::spawn::{SpawnProfile, SpawnRequest};

use super::super::JobObject;
use super::credential::SpawnCredential;
use super::inherit_supervisor::spawn_inherit_supervisor;
use super::primary_token::spawn_as_primary_token;

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
        return spawn_with_supervisor_inherit(process, process_name, request, &credential);
    }

    spawn_agent_logon(process, process_name, request, &credential)
}

fn spawn_agent_logon(
    process: &mut ManagedProcess,
    process_name: &str,
    request: SpawnRequest,
    credential: &SpawnCredential,
) -> Result<ProcessHandle> {
    let job = JobObject::new()
        .with_context(|| format!("[{process_name}] create job object for child supervision"))?;

    let (handle, user_profile) =
        spawn_as_primary_token(process_name, &request, credential, &job)
            .with_context(|| format!("[{process_name}] CreateProcessAsUserW spawn failed"))?;

    process.set_user_profile_guard(user_profile);

    process.set_job_object(job);
    Ok(handle)
}

fn spawn_privileged_inherit(
    process: &mut ManagedProcess,
    process_name: &str,
    request: SpawnRequest,
) -> Result<ProcessHandle> {
    spawn_with_supervisor_inherit(
        process,
        process_name,
        request,
        &SpawnCredential::privileged(),
    )
}

fn spawn_with_supervisor_inherit(
    process: &mut ManagedProcess,
    process_name: &str,
    request: SpawnRequest,
    credential: &SpawnCredential,
) -> Result<ProcessHandle> {
    let job = JobObject::new()
        .with_context(|| format!("[{process_name}] create job object for child supervision"))?;

    let handle = spawn_inherit_supervisor(process_name, &request, credential, &job)
        .with_context(|| format!("[{process_name}] supervisor-token inherit spawn failed"))?;

    process.set_job_object(job);
    Ok(handle)
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
