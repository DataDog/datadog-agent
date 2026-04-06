// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;

/// Configure the child process for Windows: create a new process group
/// and assign to a Job Object so all descendants can be managed together.
pub fn setup_process_group(_cmd: &mut tokio::process::Command) {}

/// Send a graceful stop signal (CTRL_BREAK_EVENT via GenerateConsoleCtrlEvent).
pub fn send_graceful_stop(_pid: u32) -> Result<()> {
    unimplemented!("Windows graceful stop not yet implemented")
}

/// Force-kill the process and all descendants (TerminateJobObject / TerminateProcess).
pub fn send_force_kill(_pid: u32) -> Result<()> {
    unimplemented!("Windows force kill not yet implemented")
}

/// On Windows, processes don't have Unix signals.
pub fn last_signal(_status: &std::process::ExitStatus) -> Option<i32> {
    None
}

/// Wait for a shutdown trigger (Ctrl+C or service stop event).
pub async fn shutdown_signal() {
    tokio::signal::ctrl_c()
        .await
        .expect("failed to register Ctrl+C handler");
    log::info!("received Ctrl+C");
}
