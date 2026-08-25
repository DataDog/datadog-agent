// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod account_name;
mod agent_credentials;
mod agent_service_sid;
mod legacy_scm_env;
mod local_account;
mod managed_service_account;
mod pipe_caller;
mod pipe_security;
mod runtime_user;
#[cfg(not(test))]
mod scm_lsa_secret;
mod scm_service;
mod sid;
mod spawn;
mod token_identity;
mod wide;
mod win_handle;

#[cfg(any(test, feature = "test-helpers"))]
pub(crate) use agent_credentials::spawn_user_for_profile;
pub(crate) use pipe_caller::pipe_client_may_mutate;
pub(crate) use pipe_security::create_pipe_server;
pub(crate) use runtime_user::runtime_user_for_pid;
pub use scm_service::run_as_service;
pub(crate) use scm_service::service_shutdown_deadline;
pub(crate) use spawn::spawn_child_handle;
pub(crate) use spawn::user_profile::UserProfileGuard;

use anyhow::Result;
use std::collections::HashMap;
use std::ffi::c_void;
use std::os::windows::ffi::OsStringExt;
use std::path::PathBuf;
use std::sync::{Mutex, OnceLock};
use std::time::Instant;
use tokio::sync::Notify;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE, INVALID_HANDLE_VALUE, TRUE};
use windows_sys::Win32::System::Console::{
    AttachConsole, CTRL_BREAK_EVENT, FreeConsole, GenerateConsoleCtrlEvent, GetStdHandle,
    STD_ERROR_HANDLE, STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, SetConsoleCtrlHandler, SetStdHandle,
};
use windows_sys::Win32::System::JobObjects::{
    AssignProcessToJobObject, CreateJobObjectW, JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
    JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, JOBOBJECT_EXTENDED_LIMIT_INFORMATION,
    JobObjectBasicAccountingInformation, JobObjectExtendedLimitInformation,
    QueryInformationJobObject, SetInformationJobObject, TerminateJobObject,
};
use windows_sys::Win32::System::Threading::{
    OpenProcess, PROCESS_SET_QUOTA, PROCESS_TERMINATE, TerminateProcess,
};

static SHUTDOWN_NOTIFY: OnceLock<Notify> = OnceLock::new();
static SERVICE_STOP_SIGNAL_TIME: OnceLock<Instant> = OnceLock::new();

static CONSOLE_LOCK: Mutex<()> = Mutex::new(());

pub(crate) fn console_lock() -> std::sync::MutexGuard<'static, ()> {
    CONSOLE_LOCK.lock().expect("console lock poisoned")
}

pub fn shutdown_notify() -> &'static Notify {
    SHUTDOWN_NOTIFY.get_or_init(Notify::new)
}

/// Record when SCM delivered STOP/SHUTDOWN/PRESHUTDOWN (before async teardown).
pub(crate) fn record_service_stop_signal() {
    let _ = SERVICE_STOP_SIGNAL_TIME.set(Instant::now());
}

/// Time SCM stop was signaled, if this process is stopping as a Windows service.
pub(crate) fn service_stop_signal_time() -> Option<Instant> {
    SERVICE_STOP_SIGNAL_TIME.get().copied()
}

/// Win32 job object with kill-on-close (supervises child process trees).
pub struct JobObject {
    handle: HANDLE,
}

unsafe impl Send for JobObject {}
unsafe impl Sync for JobObject {}

impl JobObject {
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

    pub fn active_process_count(&self) -> Result<u32> {
        unsafe {
            let mut info: JOBOBJECT_BASIC_ACCOUNTING_INFORMATION = std::mem::zeroed();
            let ok = QueryInformationJobObject(
                self.handle,
                JobObjectBasicAccountingInformation,
                &mut info as *mut _ as *mut _,
                std::mem::size_of::<JOBOBJECT_BASIC_ACCOUNTING_INFORMATION>() as u32,
                std::ptr::null_mut(),
            );
            if ok == 0 {
                anyhow::bail!(
                    "QueryInformationJobObject failed: {}",
                    std::io::Error::last_os_error()
                );
            }
            Ok(info.ActiveProcesses)
        }
    }

    pub(crate) fn may_have_active_members(&self) -> bool {
        !matches!(self.active_process_count(), Ok(0))
    }

    pub fn wait_until_empty(&self, timeout: std::time::Duration) -> bool {
        const POLL_INTERVAL: std::time::Duration = std::time::Duration::from_millis(100);
        let deadline = std::time::Instant::now() + timeout;
        while std::time::Instant::now() < deadline {
            match self.active_process_count() {
                Ok(0) => return true,
                Ok(_) => std::thread::sleep(
                    POLL_INTERVAL
                        .min(deadline.saturating_duration_since(std::time::Instant::now())),
                ),
                Err(_) => return false,
            }
        }
        matches!(self.active_process_count(), Ok(0))
    }
}

impl Drop for JobObject {
    fn drop(&mut self) {
        unsafe {
            CloseHandle(self.handle);
        }
    }
}

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

unsafe extern "system" fn ignore_console_ctrl_events(_: u32) -> i32 {
    TRUE
}

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

pub(crate) fn open_datadog_agent_key() -> Option<windows_registry::Key> {
    use windows_registry::LOCAL_MACHINE;
    use windows_sys::Win32::System::Registry::KEY_WOW64_64KEY;

    LOCAL_MACHINE
        .options()
        .read()
        .access(KEY_WOW64_64KEY)
        .open(r"SOFTWARE\Datadog\Datadog Agent")
        .ok()
}

pub(crate) fn registry_nonempty_string(key: &windows_registry::Key, name: &str) -> Option<String> {
    let value: String = key.get_string(name).ok()?;
    if value.is_empty() { None } else { Some(value) }
}

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
    let root = install_root_from_registry().unwrap_or_else(default_install_root);
    resolve_install_root_symlinks(root)
}

fn resolve_install_root_symlinks(path: PathBuf) -> PathBuf {
    match std::fs::canonicalize(&path) {
        Ok(resolved) => strip_verbatim_path_prefix(resolved),
        Err(_) => path,
    }
}

fn strip_verbatim_path_prefix(path: PathBuf) -> PathBuf {
    let s = path.to_string_lossy();
    if let Some(stripped) = s.strip_prefix(r"\\?\UNC\") {
        return PathBuf::from(format!(r"\\{stripped}"));
    }
    if let Some(stripped) = s.strip_prefix(r"\\?\") {
        return PathBuf::from(stripped);
    }
    path
}

pub fn default_config_dir() -> PathBuf {
    install_root().join("processes.d")
}

pub fn fleet_policies_dir_fallback() -> Option<PathBuf> {
    fleet_policies_dir_from_registry()
        .map(PathBuf::from)
        .or_else(default_stable_fleet_policies_dir)
}

pub fn resolve_fleet_policies_dir() -> Option<PathBuf> {
    if let Ok(dir) = std::env::var("DD_FLEET_POLICIES_DIR")
        && !dir.is_empty()
    {
        return Some(PathBuf::from(dir));
    }
    fleet_policies_dir_fallback()
}

fn fleet_policies_dir_from_registry() -> Option<String> {
    open_datadog_agent_key().and_then(|k| registry_nonempty_string(&k, "fleet_policies_dir"))
}

fn default_stable_fleet_policies_dir() -> Option<PathBuf> {
    Some(
        program_data_root()
            .join("Installer")
            .join("managed")
            .join("datadog-agent")
            .join("stable"),
    )
}

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
// Spawn token environment block (`CreateProcessAsUserW`)
// ---------------------------------------------------------------------------

/// Keys copied from the supervisor process when `CreateEnvironmentBlock` fails.
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

pub(crate) fn baseline_env_vars_for_spawn(
    process_name: &str,
    token: HANDLE,
) -> HashMap<String, String> {
    match baseline_env_vars_from_token(token) {
        Ok(vars) => vars,
        Err(e) => {
            log::warn!(
                "[{process_name}] CreateEnvironmentBlock failed ({e:#}); using allowlisted process-env fallback"
            );
            fallback_process_env_vars()
        }
    }
}

pub(crate) fn baseline_env_vars_from_token(token: HANDLE) -> Result<HashMap<String, String>> {
    if token.is_null() {
        anyhow::bail!("baseline_env_vars_from_token: null token handle");
    }

    use windows_sys::Win32::System::Environment::{
        CreateEnvironmentBlock, DestroyEnvironmentBlock,
    };

    let mut env_block: *mut c_void = std::ptr::null_mut();
    let ok = unsafe { CreateEnvironmentBlock(&mut env_block, token, 0) };
    if ok == 0 {
        anyhow::bail!(
            "CreateEnvironmentBlock: {}",
            std::io::Error::last_os_error()
        );
    }

    let vars = wide_env_block_to_map(env_block as *const u16);

    unsafe {
        let _ = DestroyEnvironmentBlock(env_block as *const c_void);
    }
    Ok(vars)
}

pub(crate) fn merge_env_overrides(
    vars: &mut HashMap<String, String>,
    overrides: &[(String, String)],
) {
    for (key, value) in overrides {
        vars.retain(|existing, _| !existing.eq_ignore_ascii_case(key));
        vars.insert(key.clone(), value.clone());
    }
}

fn wide_env_block_to_map(block: *const u16) -> HashMap<String, String> {
    if block.is_null() {
        return HashMap::new();
    }
    let mut vars = HashMap::new();
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
                vars.insert(
                    k.to_string_lossy().into_owned(),
                    v.to_string_lossy().into_owned(),
                );
            }
        }
    }
    vars
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

fn fallback_process_env_vars() -> HashMap<String, String> {
    let mut vars = HashMap::new();
    for &key in FALLBACK_ENV_KEYS {
        if let Ok(val) = std::env::var(key)
            && !val.is_empty()
        {
            vars.insert(key.to_string(), val);
        }
    }
    vars
}

#[cfg(test)]
mod env_override_tests {
    use super::*;
    use std::collections::HashMap;
    use windows_sys::Win32::Security::TOKEN_QUERY;

    use super::token_identity::open_current_process_token;

    #[test]
    fn supervisor_machine_env_vars_only_includes_allowlisted_keys() {
        let vars = supervisor_machine_env_vars();
        for key in vars.keys() {
            assert!(
                FALLBACK_ENV_KEYS
                    .iter()
                    .any(|allowed| allowed.eq_ignore_ascii_case(key)),
                "unexpected machine fallback env key: {key}"
            );
        }
        for key in ["LOCALAPPDATA", "APPDATA", "USERPROFILE"] {
            assert!(
                !vars.contains_key(key),
                "machine fallback must not copy supervisor profile env: {key}"
            );
        }
    }

    #[test]
    fn token_profile_env_vars_match_token_directory() {
        let token = open_current_process_token(TOKEN_QUERY)
            .expect("OpenProcessToken on current process should succeed");
        let profile_dir = user_profile_directory_for_token(token.as_handle())
            .expect("GetUserProfileDirectoryW should not error")
            .expect("current process token should have a profile directory");
        let vars = token_profile_env_vars(token.as_handle());
        assert_eq!(vars.get("USERPROFILE").expect("USERPROFILE"), &profile_dir);
        assert!(
            vars.get("LOCALAPPDATA")
                .is_some_and(|path| !path.is_empty()),
            "LOCALAPPDATA should expand for current user token"
        );
        assert!(
            vars.get("APPDATA").is_some_and(|path| !path.is_empty()),
            "APPDATA should expand for current user token"
        );
    }

    #[test]
    fn fallback_env_vars_for_spawn_merges_machine_and_token_profile_vars() {
        let token = open_current_process_token(TOKEN_QUERY)
            .expect("OpenProcessToken on current process should succeed");
        let vars = fallback_env_vars_for_spawn(token.as_handle());
        let machine = supervisor_machine_env_vars();
        for (key, value) in &machine {
            assert_eq!(vars.get(key).expect("machine fallback key"), value);
        }
        let profile_dir = user_profile_directory_for_token(token.as_handle())
            .expect("GetUserProfileDirectoryW should not error")
            .expect("current process token should have a profile directory");
        assert_eq!(vars.get("USERPROFILE").expect("USERPROFILE"), &profile_dir);
    }

    #[test]
    fn fallback_process_env_vars_only_includes_allowlisted_keys() {
        let vars = fallback_process_env_vars();
        for key in vars.keys() {
            assert!(
                FALLBACK_ENV_KEYS
                    .iter()
                    .any(|allowed| allowed.eq_ignore_ascii_case(key)),
                "unexpected fallback env key: {key}"
            );
        }
    }

    #[test]
    fn merge_env_overrides_replaces_case_insensitive_baseline_key() {
        let mut vars = HashMap::from([("Path".to_string(), "baseline".to_string())]);
        merge_env_overrides(&mut vars, &[("PATH".to_string(), "override".to_string())]);
        assert_eq!(vars.len(), 1);
        assert_eq!(vars.get("PATH").unwrap(), "override");
    }

    #[test]
    fn strip_verbatim_path_prefix_removes_extended_prefix() {
        assert_eq!(
            strip_verbatim_path_prefix(PathBuf::from(
                r"\\?\C:\Program Files\Datadog\Datadog Agent"
            )),
            PathBuf::from(r"C:\Program Files\Datadog\Datadog Agent")
        );
    }
}
