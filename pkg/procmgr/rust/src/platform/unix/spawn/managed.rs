// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;
use tokio::process::Command;

use crate::config::ProcessConfig;
use crate::handle::ProcessHandle;
use crate::process::ManagedChildSpawn;
use crate::spawn::{SpawnProfile, SpawnRequest, profile_for};
use crate::spawn_context;

pub(crate) fn spawn_managed_child(
    process_name: &str,
    config: &ProcessConfig,
) -> Result<ManagedChildSpawn> {
    let profile = profile_for(process_name);
    let request = SpawnRequest::from_config(process_name, config, profile)?;
    let intended_user = super::super::spawn_user_for_supervisor();
    let handle = spawn_child(process_name, request, profile)?;
    Ok(ManagedChildSpawn {
        handle,
        intended_user,
    })
}

fn spawn_child(
    process_name: &str,
    request: SpawnRequest,
    profile: SpawnProfile,
) -> Result<ProcessHandle> {
    info!("[{process_name}] spawn profile: {profile}");
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
    for (k, v) in env {
        cmd.env(k, v);
    }
    if let Some(dir) = working_dir {
        cmd.current_dir(dir);
    }
    cmd.stdout(super::stdio::to_command_stdio(
        &stdout_setting,
        super::super::stdout_inheritable(),
    ));
    cmd.stderr(super::stdio::to_command_stdio(
        &stderr_setting,
        super::super::stderr_inheritable(),
    ));

    super::super::setup_process_group(&mut cmd);
    let child = cmd
        .spawn()
        .with_context(|| spawn_context::failed_message(process_name, &command))?;
    Ok(ProcessHandle::from_child(child))
}
