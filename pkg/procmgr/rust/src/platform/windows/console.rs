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

/// True when the process is attached to a console.
///
/// `GetConsoleWindow` is also NULL for a windowless console. `GetConsoleCP` returns
/// 0 only when the process has no console at all, which is the service case.
fn has_console() -> bool {
    unsafe { GetConsoleCP() != 0 }
}

/// The console state a graceful stop has to leave exactly as it found it.
///
/// Signaling a child means leaving the caller's own console, so a regression in
/// `CallerConsoleGuard` leaves the supervisor running normally with nowhere to log.
/// Tests compare a snapshot taken before the stop against one taken after.
#[cfg(test)]
#[derive(Debug, PartialEq, Eq)]
pub(crate) struct CallerConsoleState {
    has_console: bool,
    stdout: bool,
    stderr: bool,
    /// Raw std handle values, and only for a process with no console of its own.
    ///
    /// Liveness alone accepts a slot left pointing at a closed console handle once
    /// Windows has reassigned that value to something else. Whether a graceful stop
    /// leaves that behind is the open question this is here to answer, so the values are
    /// not an invariant for a process that does have a console: reattaching to the parent
    /// reopens `CONOUT$`, and fresh handles there are correct.
    std_handles: Option<[usize; 3]>,
}

#[cfg(test)]
pub(crate) fn caller_console_state() -> CallerConsoleState {
    let has_console = has_console();
    CallerConsoleState {
        has_console,
        stdout: stdout_inheritable(),
        stderr: stderr_inheritable(),
        std_handles: (!has_console)
            .then(|| CONSOLE_STD_HANDLES.map(|(kind, _)| unsafe { GetStdHandle(kind) } as usize)),
    }
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
            had_console: has_console(),
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::mem;
    use windows_sys::Win32::Foundation::HANDLE;
    use windows_sys::Win32::System::Threading::{
        CREATE_NEW_CONSOLE, CreateProcessW, INFINITE, PROCESS_INFORMATION, STARTUPINFOW,
        TerminateProcess, WaitForSingleObject,
    };

    fn open_nul() -> HANDLE {
        let nul = wide::null_terminated("NUL");
        let handle = unsafe {
            CreateFileW(
                nul.as_ptr(),
                FILE_GENERIC_READ | FILE_GENERIC_WRITE,
                FILE_SHARE_READ | FILE_SHARE_WRITE,
                std::ptr::null(),
                OPEN_EXISTING,
                FILE_ATTRIBUTE_NORMAL,
                std::ptr::null_mut(),
            )
        };
        assert!(
            handle != INVALID_HANDLE_VALUE && !handle.is_null(),
            "CreateFileW(NUL) failed: {}",
            std::io::Error::last_os_error()
        );
        handle
    }

    /// Throwaway for AGENTRUN-1504. Answers whether `FreeConsole` leaves closed handle
    /// values in the std slots on this Windows image, which is the premise of the
    /// inherit/h2 diagnosis. Not a product invariant.
    ///
    /// Forces the console-less state the SCM case uses, puts a known live NUL in every
    /// slot, then runs the same AttachConsole/FreeConsole churn as a graceful stop. A
    /// failure means the slots hold closed values Windows can recycle onto a later named
    /// pipe. A pass means this image does not leave that residue, so the diagnosis is wrong.
    #[test]
    fn probe_freeconsole_leaves_closed_handles_in_std_slots() {
        let _lock = console_lock();
        let had_console = has_console();

        if had_console {
            leave_console();
        }
        assert!(
            !has_console(),
            "could not enter the console-less state the SCM case uses"
        );

        let nul = open_nul();
        for (kind, _) in CONSOLE_STD_HANDLES {
            assert_ne!(
                unsafe { SetStdHandle(kind, nul) },
                0,
                "SetStdHandle({kind}) failed: {}",
                std::io::Error::last_os_error()
            );
        }

        // Own console so AttachConsole has something to attach to. CREATE_NO_WINDOW
        // children used by managed spawns are attachable too, but a dedicated console
        // keeps this probe independent of spawn flags.
        let mut cmdline = wide::null_terminated("ping -n 60 127.0.0.1");
        let mut startup: STARTUPINFOW = unsafe { mem::zeroed() };
        startup.cb = mem::size_of::<STARTUPINFOW>() as u32;
        let mut process: PROCESS_INFORMATION = unsafe { mem::zeroed() };
        let ok = unsafe {
            CreateProcessW(
                std::ptr::null(),
                cmdline.as_mut_ptr(),
                std::ptr::null(),
                std::ptr::null(),
                0,
                CREATE_NEW_CONSOLE,
                std::ptr::null(),
                std::ptr::null(),
                &startup,
                &mut process,
            )
        };
        assert_ne!(
            ok,
            0,
            "CreateProcessW(CREATE_NEW_CONSOLE) failed: {}",
            std::io::Error::last_os_error()
        );
        let pid = process.dwProcessId;

        let deadline = Instant::now() + GRACEFUL_STOP_ATTACH_RETRY;
        let mut attached = false;
        while Instant::now() < deadline {
            if unsafe { AttachConsole(pid) } != 0 {
                attached = true;
                break;
            }
            std::thread::sleep(GRACEFUL_STOP_ATTACH_INTERVAL);
        }
        let attach_err = std::io::Error::last_os_error();
        assert!(attached, "AttachConsole({pid}) failed: {attach_err}");

        // The FreeConsole under test. Same call ChildConsoleGuard makes on drop.
        leave_console();

        let mut stale = Vec::new();
        for (kind, _) in CONSOLE_STD_HANDLES {
            let handle = unsafe { GetStdHandle(kind) };
            if !handle.is_null() && handle != INVALID_HANDLE_VALUE && !std_handle_live(kind) {
                stale.push((kind, handle as usize));
            }
        }

        unsafe {
            let _ = TerminateProcess(process.hProcess, 1);
            WaitForSingleObject(process.hProcess, INFINITE);
            CloseHandle(process.hProcess);
            CloseHandle(process.hThread);
        }

        // Put this process back so sibling tests keep a usable console.
        if had_console && unsafe { AttachConsole(ATTACH_PARENT_PROCESS) } != 0 {
            for (kind, device) in CONSOLE_STD_HANDLES {
                if !std_handle_live(kind) {
                    rebind_std_handle(kind, device);
                }
            }
        }
        unsafe {
            CloseHandle(nul);
        }

        assert!(
            stale.is_empty(),
            "FreeConsole left closed handle values in the std slots: {stale:?}. \
             Windows can reuse those values for a later named pipe, which is the \
             AGENTRUN-1504 inherit/h2 mechanism. Empty means the diagnosis is wrong \
             on this image."
        );
    }
}
