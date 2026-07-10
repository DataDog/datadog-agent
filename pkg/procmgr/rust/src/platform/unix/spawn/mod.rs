// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod stdio;

use anyhow::{Context, Result};
use log::info;
use tokio::process::Command;

use crate::handle::ProcessHandle;
use crate::spawn::{SpawnProfile, SpawnRequest};
use crate::spawn_context;

/// Spawn a managed child. On Unix, procmgr already runs as `dd-agent`; both profiles
/// use the supervisor identity until a distinct host-privileged child is needed.
pub(crate) fn spawn_child(
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
    cmd.stdout(stdio::to_command_stdio(
        &stdout_setting,
        super::stdout_inheritable(),
    ));
    cmd.stderr(stdio::to_command_stdio(
        &stderr_setting,
        super::stderr_inheritable(),
    ));

    super::setup_process_group(&mut cmd);
    let child = cmd
        .spawn()
        .with_context(|| spawn_context::failed_message(process_name, &command))?;
    Ok(ProcessHandle::from_child(child))
}
