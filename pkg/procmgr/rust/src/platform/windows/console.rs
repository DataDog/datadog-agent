// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::sync::Mutex;
use std::time::{Duration, Instant};
use windows_sys::Win32::Foundation::{
    GetLastError, INVALID_HANDLE_VALUE, NO_ERROR, SetLastError, TRUE,
};
use windows_sys::Win32::Storage::FileSystem::{FILE_TYPE_UNKNOWN, GetFileType};
use windows_sys::Win32::System::Console::{
    AttachConsole, CTRL_BREAK_EVENT, FreeConsole, GenerateConsoleCtrlEvent, GetStdHandle,
    STD_ERROR_HANDLE, STD_OUTPUT_HANDLE, SetConsoleCtrlHandler,
};

static CONSOLE_LOCK: Mutex<()> = Mutex::new(());

pub(crate) fn console_lock() -> std::sync::MutexGuard<'static, ()> {
    CONSOLE_LOCK.lock().expect("console lock poisoned")
}

/// True when the std handle is live enough for a child to inherit.
///
/// After `FreeConsole`, `GetStdHandle` can still return stale console handles.
fn std_handle_inheritable(handle: u32) -> bool {
    unsafe {
        let h = GetStdHandle(handle);
        if h.is_null() || h == INVALID_HANDLE_VALUE {
            return false;
        }
        // GetFileType reports UNKNOWN both for genuinely unknown types and for dead
        // handles; only the last-error value tells the two apart.
        SetLastError(NO_ERROR);
        GetFileType(h) != FILE_TYPE_UNKNOWN || GetLastError() == NO_ERROR
    }
}

pub fn stdout_inheritable() -> bool {
    std_handle_inheritable(STD_OUTPUT_HANDLE)
}

pub fn stderr_inheritable() -> bool {
    std_handle_inheritable(STD_ERROR_HANDLE)
}

/// Detach from the current console without clearing std handles.
fn leave_console() {
    unsafe {
        let _ = FreeConsole();
    }
}

unsafe extern "system" fn ignore_console_ctrl_events(ctrl: u32) -> i32 {
    if ctrl == CTRL_BREAK_EVENT { TRUE } else { 0 }
}

struct IgnoreCtrlGuard;

impl IgnoreCtrlGuard {
    fn install() -> Result<Self> {
        unsafe {
            if SetConsoleCtrlHandler(Some(ignore_console_ctrl_events), 1) == 0 {
                anyhow::bail!("SetConsoleCtrlHandler: {}", std::io::Error::last_os_error());
            }
        }
        Ok(Self)
    }
}

impl Drop for IgnoreCtrlGuard {
    fn drop(&mut self) {
        unsafe {
            if SetConsoleCtrlHandler(Some(ignore_console_ctrl_events), 0) == 0 {
                log::warn!(
                    "SetConsoleCtrlHandler(remove console ctrl ignore handler) failed: {}",
                    std::io::Error::last_os_error()
                );
            }
        }
    }
}

const GRACEFUL_STOP_ATTACH_RETRY: Duration = Duration::from_millis(500);
const GRACEFUL_STOP_ATTACH_INTERVAL: Duration = Duration::from_millis(10);
const GRACEFUL_STOP_SETTLE: Duration = Duration::from_millis(200);

/// Detaches from the caller console, attaches to the child's, and detaches again on drop.
struct ChildConsoleGuard;

impl ChildConsoleGuard {
    fn attach(pid: u32) -> Result<Self> {
        leave_console();

        // A stop requested right after spawn can race the child's own console setup:
        // until it has one, AttachConsole fails. Retry instead of falling through to
        // the stop_timeout force-kill.
        let deadline = Instant::now() + GRACEFUL_STOP_ATTACH_RETRY;
        let mut last_err = None;
        while Instant::now() < deadline {
            // SAFETY: Win32 attach to the target process console for signaling.
            if unsafe { AttachConsole(pid) != 0 } {
                return Ok(Self);
            }
            last_err = Some(std::io::Error::last_os_error());
            std::thread::sleep(GRACEFUL_STOP_ATTACH_INTERVAL);
        }
        anyhow::bail!(
            "AttachConsole({pid}) failed: {}",
            last_err.unwrap_or_else(std::io::Error::last_os_error)
        );
    }
}

impl Drop for ChildConsoleGuard {
    fn drop(&mut self) {
        leave_console();
    }
}

/// Send `CTRL_BREAK` to a process group. Managed children use `CREATE_NEW_PROCESS_GROUP`, so
/// `pgid` is the child pid.
fn signal_ctrl_break(pgid: u32) -> Result<()> {
    let ok = unsafe { GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pgid) != 0 };
    if !ok {
        anyhow::bail!(
            "GenerateConsoleCtrlEvent(CTRL_BREAK, {pgid}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    std::thread::sleep(GRACEFUL_STOP_SETTLE);
    Ok(())
}

pub fn send_graceful_stop(pid: u32) -> Result<()> {
    let _guard = console_lock();
    let _ignore_ctrl = IgnoreCtrlGuard::install()?;
    let _child_console = ChildConsoleGuard::attach(pid)?;
    signal_ctrl_break(pid)
}

pub fn send_force_kill(pid: u32) -> Result<()> {
    super::process::terminate_process_by_pid(pid)
}

pub fn last_signal(_status: &std::process::ExitStatus) -> Option<i32> {
    None
}
