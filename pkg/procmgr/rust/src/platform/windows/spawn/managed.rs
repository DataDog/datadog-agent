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
use super::credential::SpawnCredential;
use super::primary_token::spawn_as_primary_token;
use super::privileged;

pub(crate) fn spawn_child_handle(process: &mut ManagedProcess) -> Result<ProcessHandle> {
    let profile = profile_for(process.name());
    let request = SpawnRequest::from_config(process.name(), process.config(), profile)?;

    let process_name = process.name().to_owned();
    info!("[{process_name}] spawn profile: {profile}");
    if matches!(profile, SpawnProfile::Privileged) {
        privileged::validate_process_request(&process_name, &request)?;
    }

    let job = JobObject::new()
        .with_context(|| format!("[{process_name}] create job object for child supervision"))?;

    let account = match profile {
        SpawnProfile::Agent => resolve_agent_account()
            .with_context(|| format!("[{process_name}] resolve agent service account for spawn"))?,
        SpawnProfile::Privileged => AgentAccount::LocalSystem,
    };
    process.set_intended_user(account.display_name());

    let (suspended, user_profile) = spawn_as_primary_token(&process_name, &request, &credential)
        .with_context(|| format!("[{process_name}] CreateProcessAsUserW spawn failed"))?;
    if let Some(profile) = user_profile {
        process.set_user_profile_guard(profile);
    }

    suspended
        .supervise(process, job)
        .with_context(|| format!("[{process_name}] start supervised child"))
}
