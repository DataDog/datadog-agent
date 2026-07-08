// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;
use nix::sys::signal::{self, Signal};
use nix::unistd::Pid;
use std::os::unix::process::ExitStatusExt;
use std::path::PathBuf;
use tokio::process::Command;

use crate::spawn_context;
use crate::spawn_profile::SpawnProfile;
use crate::handle::ProcessHandle;
use crate::spawn_request::SpawnRequest;

/// Place the child in its own process group so signals don't propagate
/// to the daemon itself and SIGTERM can target all descendants.
pub fn setup_process_group(cmd: &mut tokio::process::Command) {
    cmd.process_group(0);
}

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
        stdout,
        stderr,
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
    cmd.stdout(stdout);
    cmd.stderr(stderr);

    setup_process_group(&mut cmd);
    let child = cmd.spawn().with_context(|| {
        spawn_context::failed_message(process_name, &command)
    })?;
    Ok(ProcessHandle::from_child(child))
}

/// Negate a PID to produce the process group ID for `kill(2)`.
/// Sending a signal to `-pgid` targets every process in the group.
pub(crate) fn process_group_id(pid: u32) -> Result<Pid> {
    let raw = i32::try_from(pid).context("PID overflows i32")?;
    Ok(Pid::from_raw(-raw))
}

/// Send SIGTERM to the entire process group (graceful stop).
pub fn send_graceful_stop(pid: u32) -> Result<()> {
    signal::kill(process_group_id(pid)?, Signal::SIGTERM)
        .with_context(|| format!("failed to send SIGTERM to pgid {pid}"))?;
    Ok(())
}

/// Send SIGKILL to the entire process group (force kill).
pub fn send_force_kill(pid: u32) -> Result<()> {
    signal::kill(process_group_id(pid)?, Signal::SIGKILL)
        .with_context(|| format!("failed to send SIGKILL to pgid {pid}"))?;
    Ok(())
}

/// Extract the signal number from an exit status, if the process was
/// terminated by a signal.
pub fn last_signal(status: &std::process::ExitStatus) -> Option<i32> {
    status.signal()
}

pub fn default_config_dir() -> PathBuf {
    PathBuf::from("/opt/datadog-agent/processes.d")
}

pub fn stdout_inheritable() -> bool {
    true
}

pub fn stderr_inheritable() -> bool {
    true
}

/// Wait for a shutdown trigger (SIGTERM or SIGINT).
pub async fn shutdown_signal() {
    use tokio::signal::unix::{SignalKind, signal};
    let mut sigterm = signal(SignalKind::terminate()).expect("failed to register SIGTERM handler");
    let mut sigint = signal(SignalKind::interrupt()).expect("failed to register SIGINT handler");
    tokio::select! {
        _ = sigterm.recv() => { log::info!("received SIGTERM"); }
        _ = sigint.recv() => { log::info!("received SIGINT"); }
    }
}
