// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::path::PathBuf;
use std::sync::OnceLock;
use tokio::sync::Notify;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::System::Console::{CTRL_BREAK_EVENT, GenerateConsoleCtrlEvent};
use windows_sys::Win32::System::JobObjects::{
    AssignProcessToJobObject, CreateJobObjectW, JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION, JobObjectExtendedLimitInformation,
    SetInformationJobObject, TerminateJobObject,
};
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_PROCESS_GROUP, OpenProcess, PROCESS_SET_QUOTA, PROCESS_TERMINATE, TerminateProcess,
};

static SHUTDOWN_NOTIFY: OnceLock<Notify> = OnceLock::new();

/// Returns the global shutdown notifier. The SCM control handler calls
/// `notify_one()` on this from its OS thread to trigger graceful shutdown
/// inside the tokio runtime.
pub fn shutdown_notify() -> &'static Notify {
    SHUTDOWN_NOTIFY.get_or_init(Notify::new)
}

// ---------------------------------------------------------------------------
// Job Object — ensures all descendants are killed together
// ---------------------------------------------------------------------------

/// RAII wrapper around a Win32 Job Object handle.
///
/// When a child process is assigned to a Job Object, all of its descendants
/// automatically belong to the same job. `terminate()` kills every process
/// in the job, matching the Unix behavior of `SIGKILL` to `-pgid`.
///
/// The job is configured with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, which
/// means if the daemon process itself crashes (dropping the handle), the OS
/// will terminate all children — a safety net against orphaned processes.
pub struct JobObject {
    handle: HANDLE,
}

// SAFETY: The Win32 HANDLE is a plain pointer-sized value that is safe to
// send across threads. The kernel serialises concurrent operations on the
// same handle.
unsafe impl Send for JobObject {}
unsafe impl Sync for JobObject {}

impl JobObject {
    /// Create a new anonymous Job Object configured for kill-on-close.
    pub fn new() -> Result<Self> {
        unsafe {
            let handle = CreateJobObjectW(std::ptr::null(), std::ptr::null());
            if handle.is_null() {
                anyhow::bail!(
                    "CreateJobObjectW failed: {}",
                    std::io::Error::last_os_error()
                );
            }

            let mut info: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = std::mem::zeroed();
            info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;

            let ok = SetInformationJobObject(
                handle,
                JobObjectExtendedLimitInformation,
                &info as *const _ as *const _,
                std::mem::size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
            );
            if ok == 0 {
                let err = std::io::Error::last_os_error();
                CloseHandle(handle);
                anyhow::bail!("SetInformationJobObject failed: {err}");
            }

            Ok(Self { handle })
        }
    }

    /// Assign a process (by PID) to this Job Object. Must be called before
    /// the child spawns its own children for complete coverage.
    pub fn assign_process(&self, pid: u32) -> Result<()> {
        unsafe {
            let proc_handle = OpenProcess(PROCESS_SET_QUOTA | PROCESS_TERMINATE, 0, pid);
            if proc_handle.is_null() {
                anyhow::bail!(
                    "OpenProcess({pid}) for job assignment failed: {}",
                    std::io::Error::last_os_error()
                );
            }
            let ok = AssignProcessToJobObject(self.handle, proc_handle);
            CloseHandle(proc_handle);
            if ok == 0 {
                anyhow::bail!(
                    "AssignProcessToJobObject({pid}) failed: {}",
                    std::io::Error::last_os_error()
                );
            }
        }
        Ok(())
    }

    /// Terminate every process in the Job Object with exit code 1.
    pub fn terminate(&self) -> Result<()> {
        unsafe {
            let ok = TerminateJobObject(self.handle, 1);
            if ok == 0 {
                anyhow::bail!(
                    "TerminateJobObject failed: {}",
                    std::io::Error::last_os_error()
                );
            }
        }
        Ok(())
    }
}

impl Drop for JobObject {
    fn drop(&mut self) {
        unsafe {
            CloseHandle(self.handle);
        }
    }
}

// ---------------------------------------------------------------------------
// Platform functions
// ---------------------------------------------------------------------------

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

/// Force-kill a single process via `TerminateProcess`.
///
/// Prefer [`JobObject::terminate()`] when a job handle is available — it
/// kills all descendants. This function is the fallback when no job exists
/// (e.g. test helpers, or if job creation failed at spawn time).
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

/// Wait for a shutdown trigger: either Ctrl+C (console mode) or an SCM
/// stop request relayed through [`shutdown_notify()`].
pub async fn shutdown_signal() {
    tokio::select! {
        result = tokio::signal::ctrl_c() => {
            result.expect("failed to register Ctrl+C handler");
            log::info!("received Ctrl+C");
        }
        _ = shutdown_notify().notified() => {
            log::info!("received service stop request");
        }
    }
}
