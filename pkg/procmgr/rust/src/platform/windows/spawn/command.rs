// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use tokio::process::Command;

use crate::handle::ProcessHandle;
use crate::platform;
use crate::spawn::SpawnRequest;
use crate::spawn_context;

use super::super::{child_baseline_env_vars, merge_env_overrides, setup_process_group};
use super::stdio;

pub(super) fn build_command(request: &SpawnRequest) -> Result<(String, Command)> {
    let SpawnRequest {
        command,
        args,
        env,
        working_dir,
        stdout_setting,
        stderr_setting,
    } = request;

    let stdout = stdio::to_command_stdio(&stdout_setting, platform::stdout_inheritable());
    let stderr = stdio::to_command_stdio(&stderr_setting, platform::stderr_inheritable());

    let mut cmd = Command::new(command);
    cmd.args(args);
    // Ensure children don't see fleet installer environment.
    cmd.env_clear();
    let mut env_vars = child_baseline_env_vars();
    merge_env_overrides(&mut env_vars, env);
    for (k, v) in env_vars {
        cmd.env(k, v);
    }
    if let Some(dir) = working_dir {
        cmd.current_dir(dir.as_path());
    }

    // Don't inherit stdin: invalid after AttachConsole/FreeConsole on stop.
    cmd.stdin(std::process::Stdio::null());
    cmd.stdout(stdout);
    cmd.stderr(stderr);

    Ok((command.clone(), cmd))
}

/// Privileged fallback only: dd-procmgr-service runs as LocalSystem and the child inherits SYSTEM.
pub(super) fn spawn_as_local_system(
    process_name: &str,
    command: &str,
    cmd: &mut Command,
) -> Result<ProcessHandle> {
    setup_process_group(cmd);
    let child = cmd
        .spawn()
        .with_context(|| spawn_context::failed_message(process_name, command))?;
    Ok(ProcessHandle::from_child(child))
}
