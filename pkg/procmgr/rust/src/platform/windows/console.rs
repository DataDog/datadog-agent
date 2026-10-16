// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
#[cfg(test)]
use windows_sys::Win32::Foundation::{
    GetLastError, HANDLE, INVALID_HANDLE_VALUE, NO_ERROR, SetLastError,
};
#[cfg(test)]
use windows_sys::Win32::Storage::FileSystem::{FILE_TYPE_UNKNOWN, GetFileType};
#[cfg(test)]
use windows_sys::Win32::System::Console::{
    GetConsoleCP, GetStdHandle, STD_ERROR_HANDLE, STD_OUTPUT_HANDLE,
};

/// True when the handle still refers to something usable.
#[cfg(test)]
fn handle_live(handle: HANDLE) -> bool {
    if handle.is_null() || handle == INVALID_HANDLE_VALUE {
        return false;
    }
    unsafe {
        // GetFileType reports UNKNOWN both for genuinely unknown types and for dead
        // handles; only the last-error value tells the two apart.
        SetLastError(NO_ERROR);
        GetFileType(handle) != FILE_TYPE_UNKNOWN || GetLastError() == NO_ERROR
    }
}

/// True when the process is attached to a console.
///
/// `GetConsoleWindow` is also NULL for a windowless console. `GetConsoleCP` returns
/// 0 only when the process has no console at all, which is the service case.
#[cfg(test)]
fn has_console() -> bool {
    unsafe { GetConsoleCP() != 0 }
}

/// Snapshot the caller's console and standard slots for signaling tests.
#[cfg(test)]
#[derive(Debug, PartialEq, Eq)]
pub(crate) struct CallerConsoleState {
    pub(crate) has_console: bool,
    std_handles: [usize; 3],
    live: [bool; 3],
    members: Vec<u32>,
}

#[cfg(test)]
pub(crate) fn caller_console_state() -> CallerConsoleState {
    use windows_sys::Win32::System::Console::{GetConsoleProcessList, STD_INPUT_HANDLE};
    let handles = [STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, STD_ERROR_HANDLE]
        .map(|kind| unsafe { GetStdHandle(kind) });
    let has_console = has_console();
    let mut members = vec![0; 256];
    let count = if has_console {
        unsafe { GetConsoleProcessList(members.as_mut_ptr(), members.len() as u32) }
    } else {
        0
    };
    if count as usize > members.len() {
        members.resize(count as usize, 0);
        unsafe {
            GetConsoleProcessList(members.as_mut_ptr(), count);
        }
    }
    members.truncate(count as usize);
    members.sort_unstable();
    CallerConsoleState {
        has_console,
        std_handles: handles.map(|h| h as usize),
        live: handles.map(handle_live),
        members,
    }
}

pub fn send_force_kill(pid: u32) -> Result<()> {
    super::process::terminate_process_by_pid(pid)
}

pub fn last_signal(_status: &std::process::ExitStatus) -> Option<i32> {
    None
}

/// Terminating exception codes that `STATUS_SEVERITY_ERROR` does not cover.
///
/// The first two are warning-severity debugger events, but with no debugger
/// attached they reach the unhandled-exception filter and end the process. An
/// `int 3` from `__debugbreak()` is the usual source: V8's `CHECK` and Go's
/// `runtime.abort` both compile to one. The third is informational severity and
/// comes from the CRT's `abort()`; a UCRT on Windows 8 or later routes that
/// through `__fastfail` and lands on 0xC0000409 instead.
const TERMINATING_NON_ERROR_STATUS: [u32; 3] = [
    0x8000_0003, // STATUS_BREAKPOINT
    0x8000_0004, // STATUS_SINGLE_STEP
    0x4000_0015, // STATUS_FATAL_APP_EXIT
];

/// Whether the process died without returning a value.
///
/// Windows has no signals, so this reads the exit code instead. A process
/// terminated by an unhandled fatal exception gets the exception code as its
/// exit code ("Terminating a Process", Win32 docs). Most such codes are
/// NTSTATUS values with severity `STATUS_SEVERITY_ERROR`, i.e. the top two bits
/// set: 0xC0000005 (access violation), 0xC00000FD (stack overflow), 0xC0000374
/// (heap corruption), 0xC0000409 (stack buffer overrun). `TERMINATING_NON_ERROR_STATUS`
/// covers the terminating codes below that severity. `ExitProcess` codes carry
/// no OS-defined meaning, so they do not collide with either range in practice.
///
/// The CLI repeats this rule in `is_windows_crash_exit_code` so it can label a
/// retained exit code after a restart. Change both together.
///
/// Two accepted consequences:
///
///   - This is a heuristic. A child may call `ExitProcess(0xC0000005)`
///     deliberately and be reported as crashed; no API distinguishes the two.
///   - A `TerminateProcess` by a third party is `Failed` here but `Crashed` on
///     Unix. The killer picks the exit code, and the conventional choice (1) is
///     indistinguishable from a real failure.
///
/// Loader and DLL-init failures surface as NTSTATUS exit codes and are reported
/// as crashes even though they are closer to a spawn failure. That is accepted,
/// and arguably correct: the process image did start.
pub fn is_crash_exit(status: &std::process::ExitStatus) -> bool {
    status.code().is_some_and(|code| {
        let code = code as u32;
        code >> 30 == 0b11 || TERMINATING_NON_ERROR_STATUS.contains(&code)
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Lives in the lib target rather than `tests/e2e`, which is Linux-only, so
    /// this is the only place Windows classification gets real CI coverage.
    #[test]
    fn is_crash_exit_accepts_fatal_exception_codes() {
        use std::os::windows::process::ExitStatusExt;

        for code in [
            0xC0000005u32, // ACCESS_VIOLATION
            0xC00000FD,    // STACK_OVERFLOW
            0xC0000374,    // heap corruption
            0xC0000409,    // stack buffer overrun
            0x80000003,    // BREAKPOINT, below ERROR severity but still fatal
            0x80000004,    // SINGLE_STEP, likewise
            0x40000015,    // FATAL_APP_EXIT, from the CRT's abort()
        ] {
            let status = std::process::ExitStatus::from_raw(code);
            assert!(
                is_crash_exit(&status),
                "{code:#X} should be classified as a crash"
            );
        }
    }

    #[test]
    fn is_crash_exit_rejects_ordinary_exit_codes() {
        use std::os::windows::process::ExitStatusExt;

        for code in [0u32, 1, 2, 42, 0x7FFFFFFF] {
            let status = std::process::ExitStatus::from_raw(code);
            assert!(
                !is_crash_exit(&status),
                "{code:#X} is an ExitProcess value, not a crash"
            );
        }
    }
}
