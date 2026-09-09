// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use std::process::Stdio;

use crate::config::ProcessConfig;
use crate::handle::ProcessHandle;
use crate::spawn::SpawnRequest;

use super::{setup_process_group, stderr_inheritable, stdout_inheritable};

pub(crate) fn spawn_child_handle(name: &str, config: &ProcessConfig) -> Result<ProcessHandle> {
    let request = SpawnRequest::from_config(name, config)?;
    let mut cmd = request.to_command(stdout_inheritable(), stderr_inheritable());
    cmd.stdin(Stdio::null());
    setup_process_group(&mut cmd);
    let child = cmd
        .spawn()
        .with_context(|| format!("[{name}] failed to spawn: {}", config.command))?;
    Ok(ProcessHandle::from_child(child))
}
