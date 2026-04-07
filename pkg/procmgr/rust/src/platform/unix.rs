// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use nix::sys::signal::{self, Signal};
use nix::unistd::Pid;
use std::os::unix::process::ExitStatusExt;

/// Place the child in its own process group so signals don't propagate
/// to the daemon itself and SIGTERM can target all descendants.
pub fn setup_process_group(cmd: &mut tokio::process::Command) {
    cmd.process_group(0);
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
