// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::sync::Mutex;
use std::time::{Duration, Instant};
use windows_sys::Win32::Foundation::{
    CloseHandle, GetLastError, INVALID_HANDLE_VALUE, NO_ERROR, SetLastError, TRUE,
};
use windows_sys::Win32::Storage::FileSystem::{
    CreateFileW, FILE_ATTRIBUTE_NORMAL, FILE_GENERIC_READ, FILE_GENERIC_WRITE, FILE_SHARE_READ,
    FILE_SHARE_WRITE, FILE_TYPE_UNKNOWN, GetFileType, OPEN_EXISTING,
};
use windows_sys::Win32::System::Console::{
    ATTACH_PARENT_PROCESS, AttachConsole, CTRL_BREAK_EVENT, FreeConsole, GenerateConsoleCtrlEvent,
    GetConsoleCP, GetStdHandle, STD_ERROR_HANDLE, STD_INPUT_HANDLE, STD_OUTPUT_HANDLE,
    SetConsoleCtrlHandler, SetStdHandle,
};

use super::wide;

static CONSOLE_LOCK: Mutex<()> = Mutex::new(());

pub(crate) fn console_lock() -> std::sync::MutexGuard<'static, ()> {
    CONSOLE_LOCK.lock().expect("console lock poisoned")
}

/// True when the std handle still refers to something usable.
///
/// After `FreeConsole`, `GetStdHandle` can still return stale console handles.
fn std_handle_live(handle: u32) -> bool {
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
    std_handle_live(STD_OUTPUT_HANDLE)
}

pub fn stderr_inheritable() -> bool {
    std_handle_live(STD_ERROR_HANDLE)
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

/// Std handles owned by a console, paired with the device that reopens them.
const CONSOLE_STD_HANDLES: [(u32, &str); 3] = [
    (STD_INPUT_HANDLE, "CONIN$"),
    (STD_OUTPUT_HANDLE, "CONOUT$"),
    (STD_ERROR_HANDLE, "CONOUT$"),
];

/// Reattaches the caller to its own console once signaling is done.
///
/// Signaling requires leaving that console (see `ChildConsoleGuard`). A supervisor started
/// from a terminal logs to stdout, so staying detached would silence its log for the rest
/// of the process lifetime.
struct CallerConsoleGuard {
    had_console: bool,
}

impl CallerConsoleGuard {
    fn capture() -> Self {
        Self {
            // GetConsoleWindow is also NULL for a windowless console. GetConsoleCP returns
            // 0 only when the process has no console at all, which is the service case.
            had_console: unsafe { GetConsoleCP() != 0 },
        }
    }
}

impl Drop for CallerConsoleGuard {
    fn drop(&mut self) {
        if !self.had_console {
            return;
        }
        // The console we left belongs to whoever launched us, so it is reachable through
        // the parent process. It is gone for good if that process already exited.
        if unsafe { AttachConsole(ATTACH_PARENT_PROCESS) } == 0 {
            log::warn!(
                "AttachConsole(ATTACH_PARENT_PROCESS) failed: {}, supervisor console output stays detached",
                std::io::Error::last_os_error()
            );
            return;
        }
        for (kind, device) in CONSOLE_STD_HANDLES {
            // Redirected handles survive FreeConsole untouched, and rebinding them would
            // discard the redirection. Only the ones the console owned come back dead.
            if !std_handle_live(kind) {
                rebind_std_handle(kind, device);
            }
        }
    }
}

/// Point a std handle back at the console, which `AttachConsole` leaves closed.
fn rebind_std_handle(kind: u32, device: &str) {
    let name = wide::null_terminated(device);
    let handle = unsafe {
        CreateFileW(
            name.as_ptr(),
            FILE_GENERIC_READ | FILE_GENERIC_WRITE,
            FILE_SHARE_READ | FILE_SHARE_WRITE,
            std::ptr::null(),
            OPEN_EXISTING,
            FILE_ATTRIBUTE_NORMAL,
            std::ptr::null_mut(),
        )
    };
    if handle == INVALID_HANDLE_VALUE || handle.is_null() {
        log::warn!(
            "CreateFileW({device}) failed: {}",
            std::io::Error::last_os_error()
        );
        return;
    }
    if unsafe { SetStdHandle(kind, handle) } == 0 {
        log::warn!(
            "SetStdHandle({device}) failed: {}",
            std::io::Error::last_os_error()
        );
        unsafe {
            CloseHandle(handle);
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

// Declaration order matters: guards drop in reverse, so the caller console is restored
// after leaving the child's and before the ctrl handler goes back to normal.
pub fn send_graceful_stop(pid: u32) -> Result<()> {
    let _guard = console_lock();
    let _ignore_ctrl = IgnoreCtrlGuard::install()?;
    let _caller_console = CallerConsoleGuard::capture();
    let _child_console = ChildConsoleGuard::attach(pid)?;
    signal_ctrl_break(pid)
}

pub fn send_force_kill(pid: u32) -> Result<()> {
    super::process::terminate_process_by_pid(pid)
}

pub fn last_signal(_status: &std::process::ExitStatus) -> Option<i32> {
    None
}
