// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod run_privileged;
mod wide;

pub(crate) use run_privileged::run_privileged_command;

use anyhow::Result;
use std::ffi::c_void;
use std::os::windows::ffi::OsStringExt;
use std::path::PathBuf;
use std::sync::{Mutex, OnceLock};
use tokio::sync::Notify;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE, INVALID_HANDLE_VALUE, TRUE};
use windows_sys::Win32::System::Console::{
    AttachConsole, CTRL_BREAK_EVENT, FreeConsole, GenerateConsoleCtrlEvent, GetStdHandle,
    STD_ERROR_HANDLE, STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, SetConsoleCtrlHandler, SetStdHandle,
};
use windows_sys::Win32::System::JobObjects::{
    AssignProcessToJobObject, CreateJobObjectW, JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION, JobObjectExtendedLimitInformation,
    SetInformationJobObject, TerminateJobObject,
};
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW, OpenProcess, PROCESS_SET_QUOTA,
    PROCESS_TERMINATE, TerminateProcess,
};

static SHUTDOWN_NOTIFY: OnceLock<Notify> = OnceLock::new();

/// Serialize process-global console state: attach/detach, std-handle reads for inherit, and spawn.
static CONSOLE_LOCK: Mutex<()> = Mutex::new(());

/// Hold while touching std handles or the attached console (graceful stop, inherit checks, spawn).
pub(crate) fn console_lock() -> std::sync::MutexGuard<'static, ()> {
    CONSOLE_LOCK.lock().expect("console lock poisoned")
}

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

/// True when this process has a non-null stdout/stderr handle (safe to honor `inherit` in YAML).
/// Caller must hold [`console_lock`]; cleared by [`reset_std_handles`] after [`detach_console`].
fn std_handle_inheritable(handle: u32) -> bool {
    unsafe {
        let h = GetStdHandle(handle);
        !h.is_null() && h != INVALID_HANDLE_VALUE
    }
}

pub fn stdout_inheritable() -> bool {
    std_handle_inheritable(STD_OUTPUT_HANDLE)
}

pub fn stderr_inheritable() -> bool {
    std_handle_inheritable(STD_ERROR_HANDLE)
}

/// `FreeConsole` leaves stale values in the process std-handle table; clear them so a
/// recycled handle is not mistaken for a valid inherit target.
fn reset_std_handles() {
    unsafe {
        for std_handle in [STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, STD_ERROR_HANDLE] {
            let _ = SetStdHandle(std_handle, std::ptr::null_mut());
        }
    }
}

fn detach_console() {
    unsafe {
        let _ = FreeConsole();
    }
    reset_std_handles();
}

/// Give the child its own hidden console plus a new process group so
/// `GenerateConsoleCtrlEvent` / `AttachConsole` graceful shutdown can work (null stdio alone
/// often leaves no attachable console).
pub fn setup_process_group(cmd: &mut tokio::process::Command) {
    cmd.creation_flags(CREATE_NEW_PROCESS_GROUP | CREATE_NEW_CONSOLE | CREATE_NO_WINDOW);
}

/// While injecting CTRL_BREAK for a child, treat any console control delivered to this process as
/// handled so we do not run default service shutdown logic for the same event.
unsafe extern "system" fn ignore_console_ctrl_events(_: u32) -> i32 {
    TRUE
}

/// Send CTRL_BREAK to the child's process group (`pid` is the group root from `CREATE_NEW_PROCESS_GROUP`).
/// Services have no console, so we `AttachConsole(pid)` before `GenerateConsoleCtrlEvent`; then detach.
pub fn send_graceful_stop(pid: u32) -> Result<()> {
    let _guard = console_lock();

    unsafe {
        detach_console();
        if AttachConsole(pid) == 0 {
            anyhow::bail!(
                "AttachConsole({pid}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        struct DetachOnDrop;
        impl Drop for DetachOnDrop {
            fn drop(&mut self) {
                detach_console();
            }
        }
        let _detach = DetachOnDrop;

        if SetConsoleCtrlHandler(Some(ignore_console_ctrl_events), 1) == 0 {
            anyhow::bail!("SetConsoleCtrlHandler: {}", std::io::Error::last_os_error());
        }
        let ok = GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid);
        if SetConsoleCtrlHandler(Some(ignore_console_ctrl_events), 0) == 0 {
            log::warn!(
                "SetConsoleCtrlHandler(remove console ctrl ignore handler) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        if ok == 0 {
            anyhow::bail!(
                "GenerateConsoleCtrlEvent(CTRL_BREAK, {pid}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
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

fn open_datadog_agent_key() -> Option<windows_registry::Key> {
    use windows_registry::LOCAL_MACHINE;
    use windows_sys::Win32::System::Registry::KEY_WOW64_64KEY;

    LOCAL_MACHINE
        .options()
        .read()
        .access(KEY_WOW64_64KEY)
        .open(r"SOFTWARE\Datadog\Datadog Agent")
        .ok()
}

fn registry_nonempty_string(key: &windows_registry::Key, name: &str) -> Option<String> {
    let value: String = key.get_string(name).ok()?;
    if value.is_empty() { None } else { Some(value) }
}

/// Root directory for agent program data on Windows (logs, etc.).
///
/// Mirrors `pkg/util/winutil.GetProgramDataDir` in Go:
/// `HKLM\SOFTWARE\Datadog\Datadog Agent\ConfigRoot`, else `%ProgramData%\Datadog`.
pub fn program_data_root() -> PathBuf {
    open_datadog_agent_key()
        .and_then(|k| registry_nonempty_string(&k, "ConfigRoot"))
        .map(PathBuf::from)
        .unwrap_or_else(default_program_data_dir)
}

fn default_program_data_dir() -> PathBuf {
    let base = std::env::var("ProgramData").unwrap_or_else(|_| r"C:\ProgramData".to_string());
    PathBuf::from(base).join("Datadog")
}

fn install_root_from_registry() -> Option<PathBuf> {
    open_datadog_agent_key()
        .and_then(|k| registry_nonempty_string(&k, "InstallPath"))
        .map(PathBuf::from)
}

fn default_install_root() -> PathBuf {
    let program_files =
        std::env::var("ProgramFiles").unwrap_or_else(|_| r"C:\Program Files".to_string());
    PathBuf::from(program_files)
        .join("Datadog")
        .join("Datadog Agent")
}

fn install_root() -> PathBuf {
    install_root_from_registry().unwrap_or_else(default_install_root)
}

/// Agent install root from registry (`InstallPath`) or the Windows default layout.
pub(crate) fn agent_install_root() -> PathBuf {
    install_root()
}

/// Default directory for process-manager YAML (`*.yaml`), same layout as Linux
/// (`/opt/datadog-agent/processes.d`) and omnibus. Resolves the install root like
/// `pkg/util/winutil.GetProgramFilesDirForProduct` in Go (`InstallPath` registry value,
/// else `%ProgramFiles%\Datadog\Datadog Agent`), then appends `processes.d`.
pub fn default_config_dir() -> PathBuf {
    install_root().join("processes.d")
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

// ---------------------------------------------------------------------------
// Child process baseline environment (after `env_clear`)
// ---------------------------------------------------------------------------

/// Keys copied from the **current** process when `CreateEnvironmentBlock` is unavailable.
/// Matches the former installer-side snapshot list (minus disk); values come from
/// dd-procmgrd at spawn time so PATH / profile vars reflect the service account.
const FALLBACK_ENV_KEYS: &[&str] = &[
    "SystemRoot",
    "WINDIR",
    "SystemDrive",
    "ProgramData",
    "ProgramFiles",
    "ProgramFiles(x86)",
    "ProgramW6432",
    "CommonProgramFiles",
    "CommonProgramFiles(x86)",
    "CommonProgramW6432",
    "PUBLIC",
    "TEMP",
    "TMP",
    "Path",
    "PATHEXT",
    "LOCALAPPDATA",
    "APPDATA",
    "USERPROFILE",
    "ComSpec",
];

/// After [`tokio::process::Command::env_clear`], merge a Windows-appropriate baseline so
/// managed children (e.g. otel-agent) see PATH, profile directories, and system roots for
/// the **dd-procmgr** process token — not the fleet installer's environment.
pub fn apply_child_baseline_env(cmd: &mut tokio::process::Command) {
    if let Err(e) = try_apply_create_environment_block(cmd) {
        log::warn!("CreateEnvironmentBlock baseline failed ({e:#}); using process-env fallback");
        apply_fallback_process_env(cmd);
    }
}

fn try_apply_create_environment_block(cmd: &mut tokio::process::Command) -> Result<()> {
    use windows_sys::Win32::Security::{TOKEN_DUPLICATE, TOKEN_QUERY};
    use windows_sys::Win32::System::Environment::{
        CreateEnvironmentBlock, DestroyEnvironmentBlock,
    };
    use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};

    let mut token: HANDLE = std::ptr::null_mut();
    let ok = unsafe {
        OpenProcessToken(
            GetCurrentProcess(),
            TOKEN_QUERY | TOKEN_DUPLICATE,
            &mut token,
        )
    };
    if ok == 0 {
        anyhow::bail!("OpenProcessToken: {}", std::io::Error::last_os_error());
    }

    let mut env_block: *mut c_void = std::ptr::null_mut();
    let ok = unsafe { CreateEnvironmentBlock(&mut env_block, token, 0) };
    if ok == 0 {
        unsafe {
            CloseHandle(token);
        }
        anyhow::bail!(
            "CreateEnvironmentBlock: {}",
            std::io::Error::last_os_error()
        );
    }

    merge_wide_env_block_into_cmd(cmd, env_block as *const u16);

    unsafe {
        let _ = DestroyEnvironmentBlock(env_block as *const c_void);
        CloseHandle(token);
    }
    Ok(())
}

fn merge_wide_env_block_into_cmd(cmd: &mut tokio::process::Command, block: *const u16) {
    if block.is_null() {
        return;
    }
    let mut p = block;
    loop {
        // SAFETY: `block` must point at a valid NUL-terminated Windows environment block from
        // `CreateEnvironmentBlock` until `DestroyEnvironmentBlock` is called (caller guarantees).
        unsafe {
            if *p == 0 {
                break;
            }
            let entry_start = p;
            while *p != 0 {
                p = p.add(1);
            }
            let len = (p as usize - entry_start as usize) / std::mem::size_of::<u16>();
            let slice = std::slice::from_raw_parts(entry_start, len);
            p = p.add(1);
            if let Some((k, v)) = split_env_entry_wide(slice) {
                cmd.env(k, v);
            }
        }
    }
}

fn split_env_entry_wide(wide: &[u16]) -> Option<(std::ffi::OsString, std::ffi::OsString)> {
    let eq = wide.iter().position(|&c| c == u16::from(b'='))?;
    let (k, v) = wide.split_at(eq);
    let v = &v[1..];
    if k.is_empty() {
        return None;
    }
    Some((
        std::ffi::OsString::from_wide(k),
        std::ffi::OsString::from_wide(v),
    ))
}

fn apply_fallback_process_env(cmd: &mut tokio::process::Command) {
    for &key in FALLBACK_ENV_KEYS {
        if let Ok(val) = std::env::var(key)
            && !val.is_empty()
        {
            cmd.env(key, val);
        }
    }
}
