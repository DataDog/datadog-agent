// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::sync::OnceLock;
use windows_sys::Win32::Foundation::{
    CloseHandle, DUPLICATE_SAME_ACCESS, DuplicateHandle, GetLastError, HANDLE,
    INVALID_HANDLE_VALUE, NO_ERROR, SetLastError,
};
use windows_sys::Win32::Storage::FileSystem::{FILE_TYPE_UNKNOWN, GetFileType};
#[cfg(test)]
use windows_sys::Win32::System::Console::GetConsoleCP;
use windows_sys::Win32::System::Console::{GetStdHandle, STD_ERROR_HANDLE, STD_OUTPUT_HANDLE};
use windows_sys::Win32::System::Threading::GetCurrentProcess;

// Test fixtures change process-global console state; production signaling does not.
#[cfg(test)]
static CONSOLE_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());
#[cfg(test)]
pub(crate) fn console_lock() -> std::sync::MutexGuard<'static, ()> {
    CONSOLE_LOCK.lock().unwrap_or_else(|e| e.into_inner())
}

/// True when the handle still refers to something usable.
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

struct OwnedStdHandle(HANDLE);
// SAFETY: this immutable owner only duplicates a kernel handle and closes it on drop.
unsafe impl Send for OwnedStdHandle {}
unsafe impl Sync for OwnedStdHandle {}
impl Drop for OwnedStdHandle {
    fn drop(&mut self) {
        unsafe {
            CloseHandle(self.0);
        }
    }
}

/// Pin every usable startup object, including write-only console and character
/// devices. The daemon never detaches, so console handles remain usable too.
struct InheritSource(Option<OwnedStdHandle>);
impl InheritSource {
    fn capture(kind: u32) -> Self {
        let source = unsafe { GetStdHandle(kind) };
        if !handle_live(source) {
            return Self(None);
        }
        let mut duplicate = std::ptr::null_mut();
        let ok = unsafe {
            DuplicateHandle(
                GetCurrentProcess(),
                source,
                GetCurrentProcess(),
                &mut duplicate,
                0,
                0,
                DUPLICATE_SAME_ACCESS,
            )
        };
        if ok == 0 {
            log::warn!(
                "DuplicateHandle(startup stdio) failed: {}; using NUL",
                std::io::Error::last_os_error()
            );
            return Self(None);
        }
        Self(Some(OwnedStdHandle(duplicate)))
    }

    fn resolve(&self) -> Option<InheritHandle<'_>> {
        self.0.as_ref().map(InheritHandle)
    }
}

/// A handle an `inherit` spawn may duplicate from, for as long as it is held.
pub(crate) struct InheritHandle<'a>(&'a OwnedStdHandle);
impl InheritHandle<'_> {
    pub(crate) fn raw(&self) -> HANDLE {
        self.0.0
    }
}

/// The supervisor's own stdout and stderr, as they were before anything could touch the
/// console. These are the only things an `inherit` spawn may resolve against.
struct StartupStdio {
    stdout: InheritSource,
    stderr: InheritSource,
}
static STARTUP_STDIO: OnceLock<StartupStdio> = OnceLock::new();

/// Settles how `inherit` resolves for the rest of the process lifetime. Idempotent,
/// and the first call is the one that counts. Called before service or interactive
/// execution.
pub fn capture_startup_stdio() {
    let _ = startup_stdio();
}

fn startup_stdio() -> &'static StartupStdio {
    STARTUP_STDIO.get_or_init(|| StartupStdio {
        stdout: InheritSource::capture(STD_OUTPUT_HANDLE),
        stderr: InheritSource::capture(STD_ERROR_HANDLE),
    })
}

/// The handle an `inherit` spawn should duplicate for `kind`, if any.
pub(crate) fn inherit_std_handle(kind: u32) -> Option<InheritHandle<'static>> {
    match kind {
        STD_OUTPUT_HANDLE => startup_stdio().stdout.resolve(),
        STD_ERROR_HANDLE => startup_stdio().stderr.resolve(),
        _ => None,
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
pub(crate) struct StdSlotsGuard([HANDLE; 3]);
#[cfg(test)]
impl StdSlotsGuard {
    pub(crate) fn capture() -> Self {
        use windows_sys::Win32::System::Console::STD_INPUT_HANDLE;
        Self(
            [STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, STD_ERROR_HANDLE]
                .map(|kind| unsafe { GetStdHandle(kind) }),
        )
    }
}
#[cfg(test)]
impl Drop for StdSlotsGuard {
    fn drop(&mut self) {
        use windows_sys::Win32::System::Console::{STD_INPUT_HANDLE, SetStdHandle};
        for (kind, handle) in [STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, STD_ERROR_HANDLE]
            .into_iter()
            .zip(self.0)
        {
            assert_ne!(unsafe { SetStdHandle(kind, handle) }, 0);
        }
    }
}

#[cfg(test)]
pub(crate) struct TestConsole {
    allocated: bool,
    slots: Option<StdSlotsGuard>,
}
#[cfg(test)]
impl TestConsole {
    pub(crate) fn acquire() -> Self {
        use windows_sys::Win32::System::Console::{AllocConsole, GetConsoleCP};
        let slots = StdSlotsGuard::capture();
        let allocated = unsafe { GetConsoleCP() } == 0;
        if allocated {
            assert_ne!(unsafe { AllocConsole() }, 0);
        }
        Self {
            allocated,
            slots: Some(slots),
        }
    }
}
#[cfg(test)]
impl Drop for TestConsole {
    fn drop(&mut self) {
        if self.allocated {
            unsafe {
                windows_sys::Win32::System::Console::FreeConsole();
            }
        }
        drop(self.slots.take());
    }
}

#[cfg(test)]
mod tests {
    use super::super::wide;
    use super::*;
    use windows_sys::Win32::Foundation::CompareObjectHandles;
    use windows_sys::Win32::Storage::FileSystem::{
        CreateFileW, FILE_ATTRIBUTE_NORMAL, FILE_GENERIC_READ, FILE_GENERIC_WRITE, FILE_SHARE_READ,
        FILE_SHARE_WRITE, OPEN_EXISTING,
    };
    use windows_sys::Win32::System::Console::SetStdHandle;

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

    fn open_device(device: &str, access: u32) -> OwnedStdHandle {
        let name = wide::null_terminated(device);
        let handle = unsafe {
            CreateFileW(
                name.as_ptr(),
                access,
                FILE_SHARE_READ | FILE_SHARE_WRITE,
                std::ptr::null(),
                OPEN_EXISTING,
                FILE_ATTRIBUTE_NORMAL,
                std::ptr::null_mut(),
            )
        };
        assert!(handle_live(handle), "{}", std::io::Error::last_os_error());
        OwnedStdHandle(handle)
    }

    fn assert_pinned(handle: HANDLE) {
        let slots = StdSlotsGuard::capture();
        assert_ne!(unsafe { SetStdHandle(STD_OUTPUT_HANDLE, handle) }, 0);
        let source = InheritSource::capture(STD_OUTPUT_HANDLE);
        let scratch = open_device("NUL", FILE_GENERIC_WRITE);
        assert_ne!(unsafe { SetStdHandle(STD_OUTPUT_HANDLE, scratch.0) }, 0);
        let resolved = source.resolve().unwrap();
        assert_ne!(unsafe { CompareObjectHandles(resolved.raw(), handle) }, 0);
        assert_eq!(
            unsafe { CompareObjectHandles(resolved.raw(), scratch.0) },
            0
        );
        drop(slots);
    }

    #[test]
    fn console_and_write_only_console_inherit_startup_object() {
        let _lock = console_lock();
        let _console = TestConsole::acquire();
        for access in [FILE_GENERIC_READ | FILE_GENERIC_WRITE, FILE_GENERIC_WRITE] {
            let handle = open_device("CONOUT$", access);
            assert_pinned(handle.0);
        }
    }

    #[test]
    fn redirected_pipe_and_file_keep_startup_object() {
        use std::os::windows::io::AsRawHandle;
        let _lock = console_lock();
        let file = tempfile::tempfile().unwrap();
        assert_pinned(file.as_raw_handle());
        let mut reader = std::ptr::null_mut();
        let mut writer = std::ptr::null_mut();
        assert_ne!(
            unsafe {
                windows_sys::Win32::System::Pipes::CreatePipe(
                    &mut reader,
                    &mut writer,
                    std::ptr::null(),
                    0,
                )
            },
            0
        );
        let _reader = OwnedStdHandle(reader);
        let writer = OwnedStdHandle(writer);
        assert_pinned(writer.0);
    }

    #[test]
    fn write_only_character_device_is_pinned() {
        let _lock = console_lock();
        let handle = open_device("NUL", FILE_GENERIC_WRITE);
        assert_pinned(handle.0);
    }

    #[test]
    fn missing_stream_has_no_inheritance_source() {
        let _lock = console_lock();
        let _slots = StdSlotsGuard::capture();
        for handle in [std::ptr::null_mut(), INVALID_HANDLE_VALUE] {
            assert_ne!(unsafe { SetStdHandle(STD_OUTPUT_HANDLE, handle) }, 0);
            assert!(
                InheritSource::capture(STD_OUTPUT_HANDLE)
                    .resolve()
                    .is_none()
            );
        }
    }
}
