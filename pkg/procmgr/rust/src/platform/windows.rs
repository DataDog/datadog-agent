// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::path::PathBuf;
use windows_sys::Win32::Foundation::CloseHandle;
use windows_sys::Win32::System::Console::{CTRL_BREAK_EVENT, GenerateConsoleCtrlEvent};
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_PROCESS_GROUP, OpenProcess, PROCESS_TERMINATE, TerminateProcess,
};

/// Place the child in its own process group so `GenerateConsoleCtrlEvent`
/// can target it without affecting the daemon.
pub fn setup_process_group(cmd: &mut tokio::process::Command) {
    cmd.creation_flags(CREATE_NEW_PROCESS_GROUP);
}

/// Send CTRL_BREAK to the child's process group (graceful stop).
///
/// `GenerateConsoleCtrlEvent` targets the process group whose ID equals the
/// child's PID (because we created it with `CREATE_NEW_PROCESS_GROUP`).
pub fn send_graceful_stop(pid: u32) -> Result<()> {
    let ok = unsafe { GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid) };
    if ok == 0 {
        anyhow::bail!(
            "GenerateConsoleCtrlEvent(CTRL_BREAK, {pid}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(())
}

/// Force-kill the process via `TerminateProcess`.
///
/// Unlike the Unix implementation (which sends SIGKILL to the entire process
/// group), this only terminates the direct child. Full descendant cleanup
/// requires Job Objects, tracked as a future improvement.
pub fn send_force_kill(pid: u32) -> Result<()> {
    unsafe {
        let handle = OpenProcess(PROCESS_TERMINATE, 0, pid);
        if handle.is_null() {
            anyhow::bail!(
                "OpenProcess(TERMINATE, {pid}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        let ok = TerminateProcess(handle, 1);
        CloseHandle(handle);
        if ok == 0 {
            anyhow::bail!(
                "TerminateProcess({pid}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
    }
    Ok(())
}

/// On Windows, processes don't have Unix signals.
pub fn last_signal(_status: &std::process::ExitStatus) -> Option<i32> {
    None
}

pub fn default_config_dir() -> PathBuf {
    let base = std::env::var("ProgramData").unwrap_or_else(|_| r"C:\ProgramData".to_string());
    PathBuf::from(base).join(r"Datadog\dd-procmgr\processes.d")
}

/// Wait for a shutdown trigger (Ctrl+C or service stop event).
pub async fn shutdown_signal() {
    tokio::signal::ctrl_c()
        .await
        .expect("failed to register Ctrl+C handler");
    log::info!("received Ctrl+C");
}
