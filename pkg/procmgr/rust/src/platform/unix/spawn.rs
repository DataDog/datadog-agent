// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;

use crate::handle::ProcessHandle;
use crate::process::ManagedProcess;
use crate::spawn::SpawnRequest;

use super::{setup_process_group, stderr_inheritable, stdout_inheritable};

pub(crate) fn spawn_child_handle(process: &mut ManagedProcess) -> Result<ProcessHandle> {
    let process_name = process.name();
    info!("[{process_name}] spawn profile: {}", process.profile());
    let request = SpawnRequest::from_config(process_name, process.config())?;
    let mut cmd = request.to_command(stdout_inheritable(), stderr_inheritable());
    setup_process_group(&mut cmd);
    let child = cmd.spawn().with_context(|| {
        format!(
            "[{process_name}] failed to spawn: {}",
            process.config().command
        )
    })?;
    Ok(ProcessHandle::from_child(child))
}
