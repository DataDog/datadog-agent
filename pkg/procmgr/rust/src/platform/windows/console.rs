// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::sync::{Mutex, OnceLock};
use std::time::{Duration, Instant};
use windows_sys::Win32::Foundation::{
    CloseHandle, DUPLICATE_SAME_ACCESS, DuplicateHandle, GetLastError, HANDLE,
    INVALID_HANDLE_VALUE, NO_ERROR, SetLastError, TRUE,
};
use windows_sys::Win32::Storage::FileSystem::{
    CreateFileW, FILE_ATTRIBUTE_NORMAL, FILE_GENERIC_READ, FILE_GENERIC_WRITE, FILE_SHARE_READ,
    FILE_SHARE_WRITE, FILE_TYPE_DISK, FILE_TYPE_PIPE, FILE_TYPE_UNKNOWN, GetFileType,
    OPEN_EXISTING,
};
use windows_sys::Win32::System::Console::{
    ATTACH_PARENT_PROCESS, AttachConsole, CTRL_BREAK_EVENT, FreeConsole, GenerateConsoleCtrlEvent,
    GetConsoleCP, GetStdHandle, STD_ERROR_HANDLE, STD_INPUT_HANDLE, STD_OUTPUT_HANDLE,
    SetConsoleCtrlHandler, SetStdHandle,
};
use windows_sys::Win32::System::Threading::GetCurrentProcess;

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

/// Where an `inherit` spawn gets the handle it hands a child, decided once at supervisor
/// startup before anything can touch the console.
///
/// Reading the std slot at spawn time instead is unsafe: a graceful stop attaches the
/// supervisor to the child's console and detaches again, which leaves closed console
/// handles in the slots (see `std_handle_live`). Windows reuses handle values, so a later
/// read can return one the OS has since given to an unrelated object, such as the named
/// pipe carrying an in-flight gRPC call. A child that inherited that as its stdout would
/// write its output straight into someone else's connection.
enum InheritSource {
    /// A private duplicate of the startup handle, which pins both the object and its
    /// value for the lifetime of the supervisor. Only for a handle console churn cannot
    /// reach, meaning a redirected file or pipe.
    Pinned(HANDLE),
    /// The startup handle was console backed, or could not be shown not to be, where
    /// there is nothing worth pinning: `FreeConsole` closes every console handle the
    /// process holds, duplicates included, so one taken at startup is dead from the first
    /// graceful stop onwards. A spawn opens `CONOUT$` instead, which names whatever
    /// console the supervisor is attached to by then, and gets nothing when it has none.
    Console,
    /// The slot held nothing usable, which is the service case.
    None,
}

impl InheritSource {
    fn capture(kind: u32) -> Self {
        if !std_handle_live(kind) {
            return Self::None;
        }
        let handle = unsafe { GetStdHandle(kind) };
        if !survives_console_churn(handle) {
            return Self::Console;
        }
        match duplicate_for_self(handle) {
            Some(duplicate) => Self::Pinned(duplicate),
            None => Self::None,
        }
    }

    /// The handle to hand the child, if any.
    ///
    /// Spawning holds the console lock (see `Process::try_spawn`), so the console arm
    /// never reads a console a graceful stop has the supervisor attached to.
    fn resolve(&self) -> Option<InheritHandle> {
        match self {
            Self::Pinned(handle) => Some(InheritHandle::Pinned(*handle)),
            Self::Console => open_console_device("CONOUT$").map(InheritHandle::Opened),
            Self::None => None,
        }
    }
}

/// A handle an `inherit` spawn may duplicate from, for as long as it is held.
pub(crate) enum InheritHandle {
    /// Belongs to `STARTUP_STDIO` and outlives every spawn.
    Pinned(HANDLE),
    /// Opened for this spawn alone.
    Opened(HANDLE),
}

impl InheritHandle {
    pub(crate) fn raw(&self) -> HANDLE {
        match self {
            Self::Pinned(handle) | Self::Opened(handle) => *handle,
        }
    }
}

impl Drop for InheritHandle {
    fn drop(&mut self) {
        if let Self::Opened(handle) = self {
            unsafe {
                CloseHandle(*handle);
            }
        }
    }
}

/// The supervisor's own stdout and stderr, as they were before anything could touch the
/// console. These are the only things an `inherit` spawn may resolve against.
struct StartupStdio {
    stdout: InheritSource,
    stderr: InheritSource,
}

// SAFETY: a pinned handle is owned for the process lifetime and only duplicated from.
unsafe impl Send for StartupStdio {}
unsafe impl Sync for StartupStdio {}

static STARTUP_STDIO: OnceLock<StartupStdio> = OnceLock::new();

/// Settles how `inherit` resolves for the rest of the process lifetime. Idempotent, and
/// the first call is the one that counts, so every path that manipulates the console
/// calls it before doing so.
pub fn capture_startup_stdio() {
    let _ = startup_stdio();
}

fn startup_stdio() -> &'static StartupStdio {
    STARTUP_STDIO.get_or_init(|| StartupStdio {
        stdout: InheritSource::capture(STD_OUTPUT_HANDLE),
        stderr: InheritSource::capture(STD_ERROR_HANDLE),
    })
}

/// True when a duplicate of `handle` is worth keeping, meaning console churn cannot
/// invalidate it. Only a disk file or a pipe qualifies.
///
/// Everything else is treated as console backed, including the character devices that are
/// not consoles, because the two mistakes do not cost the same: routing a child's output
/// to the console when it should have gone elsewhere is a misroute, while pinning a
/// console handle is the dead or recycled handle this module exists to avoid.
///
/// `GetFileType` is the only test that holds whatever access the handle carries.
/// `GetConsoleMode` needs `GENERIC_READ`, and a parent is free to hand its child a
/// write-only `CONOUT$`.
fn survives_console_churn(handle: HANDLE) -> bool {
    matches!(
        unsafe { GetFileType(handle) },
        FILE_TYPE_DISK | FILE_TYPE_PIPE
    )
}

/// A private duplicate of `handle`, or `None` when it cannot be taken.
fn duplicate_for_self(handle: HANDLE) -> Option<HANDLE> {
    let mut dup: HANDLE = std::ptr::null_mut();
    let ok = unsafe {
        DuplicateHandle(
            GetCurrentProcess(),
            handle,
            GetCurrentProcess(),
            &mut dup,
            0,
            0,
            DUPLICATE_SAME_ACCESS,
        )
    };
    if ok == 0 {
        log::warn!(
            "DuplicateHandle(std handle) failed: {}, spawns will not inherit it",
            std::io::Error::last_os_error()
        );
        return None;
    }
    Some(dup)
}

/// The handle an `inherit` spawn should duplicate for `kind`, if any.
pub(crate) fn inherit_std_handle(kind: u32) -> Option<InheritHandle> {
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
    /// Windows has reassigned that value to something else, which is exactly the state a
    /// graceful stop must not leave behind. The values are not an invariant for a process
    /// that does have a console: reattaching to the parent reopens `CONOUT$`, so fresh
    /// handles there are correct rather than a regression.
    std_handles: Option<[usize; 3]>,
}

#[cfg(test)]
pub(crate) fn caller_console_state() -> CallerConsoleState {
    let has_console = has_console();
    CallerConsoleState {
        has_console,
        stdout: std_handle_live(STD_OUTPUT_HANDLE),
        stderr: std_handle_live(STD_ERROR_HANDLE),
        std_handles: (!has_console).then(|| {
            StdHandleSlots::capture()
                .0
                .map(|(_, handle)| handle as usize)
        }),
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

/// The std handle slots as they were before attaching to another process's console.
struct StdHandleSlots([(u32, HANDLE); 3]);

impl StdHandleSlots {
    fn capture() -> Self {
        Self(CONSOLE_STD_HANDLES.map(|(kind, _)| (kind, unsafe { GetStdHandle(kind) })))
    }

    fn restore(&self) {
        for (kind, handle) in self.0 {
            if unsafe { SetStdHandle(kind, handle) } == 0 {
                log::warn!(
                    "SetStdHandle({kind}) failed: {}, the slot keeps a stale console handle",
                    std::io::Error::last_os_error()
                );
            }
        }
    }
}

/// Reattaches the caller to its own console once signaling is done.
///
/// Signaling requires leaving that console (see `ChildConsoleGuard`). A supervisor started
/// from a terminal logs to stdout, so staying detached would silence its log for the rest
/// of the process lifetime.
struct CallerConsoleGuard {
    had_console: bool,
    std_handles: StdHandleSlots,
}

impl CallerConsoleGuard {
    fn capture() -> Self {
        Self {
            had_console: has_console(),
            std_handles: StdHandleSlots::capture(),
        }
    }
}

impl Drop for CallerConsoleGuard {
    fn drop(&mut self) {
        if !self.had_console {
            // A supervisor with no console of its own still gets the child console's
            // handles written into its std slots by AttachConsole, and the matching
            // FreeConsole then closes them. Putting the original values back keeps a
            // later GetStdHandle from returning one Windows has since reassigned.
            self.std_handles.restore();
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

/// Opens one of the console devices, or `None` when the process has no console.
fn open_console_device(device: &str) -> Option<HANDLE> {
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
        return None;
    }
    Some(handle)
}

/// Point a std handle back at the console, which `AttachConsole` leaves closed.
fn rebind_std_handle(kind: u32, device: &str) {
    let Some(handle) = open_console_device(device) else {
        return;
    };
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
    // Pin stdio before the console churn below can replace the std handles.
    capture_startup_stdio();
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
    use windows_sys::Win32::Foundation::CompareObjectHandles;
    use windows_sys::Win32::System::Console::{AllocConsole, GetConsoleMode};
    use windows_sys::Win32::System::Pipes::CreatePipe;

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

    fn slot_values(slots: &StdHandleSlots) -> [usize; 3] {
        slots.0.map(|(_, handle)| handle as usize)
    }

    fn set_std_handle(kind: u32, handle: HANDLE) {
        assert_ne!(
            unsafe { SetStdHandle(kind, handle) },
            0,
            "SetStdHandle({kind}) failed: {}",
            std::io::Error::last_os_error()
        );
    }

    fn close(handle: HANDLE) {
        unsafe { CloseHandle(handle) };
    }

    /// The write end of an anonymous pipe, a second object that is not the `NUL` device.
    fn open_pipe() -> (HANDLE, HANDLE) {
        let mut read: HANDLE = std::ptr::null_mut();
        let mut write: HANDLE = std::ptr::null_mut();
        assert_ne!(
            unsafe { CreatePipe(&mut read, &mut write, std::ptr::null(), 0) },
            0,
            "CreatePipe failed: {}",
            std::io::Error::last_os_error()
        );
        (read, write)
    }

    /// Takes a console for this process and points stdout at it, which is the shape a
    /// supervisor launched from a terminal has.
    ///
    /// The second half is not optional: `AllocConsole` leaves a std handle the parent
    /// redirected alone, and a test runner that hands this process a pipe for stdout, as
    /// Bazel does, would otherwise keep that pipe in the slot.
    fn attach_fresh_console() {
        assert_ne!(
            unsafe { AllocConsole() },
            0,
            "AllocConsole failed: {}",
            std::io::Error::last_os_error()
        );
        let console_out = open_console_out(FILE_GENERIC_READ | FILE_GENERIC_WRITE);
        set_std_handle(STD_OUTPUT_HANDLE, console_out);
    }

    /// `CONOUT$` with exactly the access asked for, so a test can hold the write-only
    /// handle a parent is free to pass down as a child's stdout.
    fn open_console_out(access: u32) -> HANDLE {
        let name = wide::null_terminated("CONOUT$");
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
        assert!(
            handle != INVALID_HANDLE_VALUE && !handle.is_null(),
            "CreateFileW(CONOUT$) failed: {}",
            std::io::Error::last_os_error()
        );
        handle
    }

    /// Whether a handle this test opened with read access is a console that is still
    /// alive. Production code cannot ask this way, since `GetConsoleMode` answers only
    /// for a handle carrying `GENERIC_READ`.
    fn is_live_console(handle: HANDLE) -> bool {
        let mut mode = 0u32;
        unsafe { GetConsoleMode(handle, &mut mode) != 0 }
    }

    /// Redirected stdio is pinned: a file or a pipe survives console churn, so the
    /// duplicate taken at startup stays the right answer no matter what later writes to
    /// the std slot it came from.
    #[test]
    fn redirected_inherit_keeps_the_startup_object() {
        let _lock = console_lock();
        let slots = StdHandleSlots::capture();

        let (startup_read, startup_write) = open_pipe();
        set_std_handle(STD_OUTPUT_HANDLE, startup_write);
        let source = InheritSource::capture(STD_OUTPUT_HANDLE);

        let (other_read, other_write) = open_pipe();
        set_std_handle(STD_OUTPUT_HANDLE, other_write);

        let resolved = source.resolve().expect("a pinned source always resolves");
        let kept_the_startup_object =
            unsafe { CompareObjectHandles(resolved.raw(), startup_write) } != 0;
        let followed_the_slot = unsafe { CompareObjectHandles(resolved.raw(), other_write) } != 0;

        drop(resolved);
        slots.restore();
        if let InheritSource::Pinned(duplicate) = source {
            close(duplicate);
        }
        for handle in [startup_write, startup_read, other_write, other_read] {
            close(handle);
        }

        assert!(
            kept_the_startup_object,
            "inherit must hand the child the object the supervisor started with"
        );
        assert!(
            !followed_the_slot,
            "inherit must not hand the child whatever the std slot points at now"
        );
    }

    /// Console-backed stdio cannot be pinned. `FreeConsole` closes every console handle
    /// the process holds, duplicates included, so a handle captured at startup is dead
    /// from the first graceful stop onwards. Taking the console away and giving a
    /// different one back is the state a stop leaves behind for a supervisor started from
    /// a terminal, and `inherit` has to resolve against the new one.
    #[test]
    fn console_inherit_follows_the_console_across_a_detach() {
        let _lock = console_lock();
        let restore = CallerConsoleGuard::capture();

        // Whatever the harness gave this process, run against a console the test owns.
        leave_console();
        attach_fresh_console();

        let source = InheritSource::capture(STD_OUTPUT_HANDLE);
        let pinned_a_console_handle = matches!(source, InheritSource::Pinned(_));
        let before = source.resolve();
        let resolved_before = before.as_ref().is_some_and(|h| is_live_console(h.raw()));
        drop(before);

        leave_console();
        attach_fresh_console();

        let after = source.resolve();
        let resolved_after = after.as_ref().is_some_and(|h| is_live_console(h.raw()));
        drop(after);

        leave_console();
        drop(restore);

        assert!(
            !pinned_a_console_handle,
            "a console handle must not be pinned, FreeConsole invalidates the duplicate"
        );
        assert!(resolved_before, "inherit resolves while attached");
        assert!(
            resolved_after,
            "inherit handed the child a handle that is no longer a console"
        );
    }

    /// A console handle the supervisor cannot read from is still a console handle, and
    /// `FreeConsole` closes it like any other. Classifying with `GetConsoleMode`, which
    /// needs `GENERIC_READ`, would call this one redirected and pin a duplicate that the
    /// first graceful stop invalidates.
    #[test]
    fn write_only_console_handle_is_not_pinned() {
        let _lock = console_lock();
        let restore = CallerConsoleGuard::capture();

        leave_console();
        attach_fresh_console();
        // Closed by FreeConsole below along with every other handle to this console.
        set_std_handle(STD_OUTPUT_HANDLE, open_console_out(FILE_GENERIC_WRITE));

        let source = InheritSource::capture(STD_OUTPUT_HANDLE);
        let pinned = matches!(source, InheritSource::Pinned(_));

        leave_console();
        drop(restore);

        assert!(
            !pinned,
            "a console handle the supervisor cannot read from must not be pinned"
        );
    }

    /// The service case: the SCM starts dd-procmgrd with nothing in its std slots, so
    /// `inherit` has nothing to give a child and the spawn path falls back to NUL.
    #[test]
    fn empty_std_slot_has_nothing_to_inherit() {
        let _lock = console_lock();
        let slots = StdHandleSlots::capture();

        set_std_handle(STD_OUTPUT_HANDLE, std::ptr::null_mut());
        let source = InheritSource::capture(STD_OUTPUT_HANDLE);
        let resolved_nothing = source.resolve().is_none();

        slots.restore();

        assert!(
            resolved_nothing,
            "an empty std slot must not resolve to a handle"
        );
    }

    /// The no-console arm of `CallerConsoleGuard` is this round trip, and it is the only
    /// thing keeping a closed console handle out of a std slot after a graceful stop.
    #[test]
    fn std_handle_slots_restore_the_captured_values() {
        let _guard = console_lock();
        let captured = StdHandleSlots::capture();

        let scratch = open_nul();
        for (kind, _) in CONSOLE_STD_HANDLES {
            assert_ne!(
                unsafe { SetStdHandle(kind, scratch) },
                0,
                "SetStdHandle({kind}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        captured.restore();
        let restored = StdHandleSlots::capture();
        unsafe { CloseHandle(scratch) };

        assert_eq!(slot_values(&captured), slot_values(&restored));
    }
}
