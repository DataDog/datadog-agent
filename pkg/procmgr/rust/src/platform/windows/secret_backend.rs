// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Run `secret_backend_command` under the core Agent service account on Windows.
//!
//! Secret resolution must match `datadogagent`, not the dd-procmgr-service supervisor
//! (LocalSystem) or a [`SpawnProfile::Privileged`] child identity.

use std::collections::HashMap;
use std::io::Write;
use std::os::windows::ffi::OsStrExt;
use std::os::windows::io::FromRawHandle;
use std::ptr;
use std::time::{Duration, Instant};

use anyhow::{Context, Result, bail};
use windows_sys::Win32::Foundation::{
    CloseHandle, HANDLE, HANDLE_FLAG_INHERIT, INVALID_HANDLE_VALUE, SetHandleInformation,
    WAIT_OBJECT_0, WAIT_TIMEOUT,
};
use windows_sys::Win32::Storage::FileSystem::{
    CreateFileW, FILE_ATTRIBUTE_NORMAL, FILE_GENERIC_WRITE, FILE_SHARE_READ, FILE_SHARE_WRITE,
    OPEN_EXISTING,
};
use windows_sys::Win32::System::Pipes::CreatePipe;
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW, CREATE_UNICODE_ENVIRONMENT,
    CreateProcessAsUserW, GetExitCodeProcess, PROCESS_INFORMATION, STARTF_USESTDHANDLES,
    STARTUPINFOW, TerminateProcess, WaitForSingleObject,
};

use crate::secret_backend_exec::{BackendRun, exec_inherited_token, wait_with_stdout_drain};

use super::agent_credentials::{AgentAccount, resolve_agent_account};
use super::baseline_env_vars_from_token;
use super::legacy_scm_env::build_secret_backend_env_vars;
use super::resolve_executable::resolve_executable_in_env;
use super::secret_backend_rights;
use super::spawn::logon::{TokenHandle, logon_user_credentials, logon_user_token};
use super::spawn::user_profile::UserProfileGuard;
use super::spawn::win32::{
    build_windows_command_line, duplicate_primary_token, env_block_for_secret_backend,
};
use super::wide;

const PROCESS_NAME: &str = "secret-backend";

pub(crate) fn exec_secret_backend(
    command: &str,
    arguments: &[String],
    payload: &str,
    timeout: Duration,
    max_output_bytes: usize,
    skip_acl_check: bool,
) -> Result<String> {
    let account =
        resolve_agent_account().context("resolve agent service account for secret backend")?;
    if supervisor_runs_as_agent_account(&account) {
        let env = secret_backend_resolution_env(&account, None)?;
        let resolved_command = resolve_executable_in_env(command, &env)
            .with_context(|| format!("resolve secret backend executable {command}"))?;
        validate_secret_backend_command(&resolved_command, skip_acl_check)?;
        let run = BackendRun {
            command: &resolved_command,
            arguments,
            payload,
            timeout,
            max_output_bytes,
        };
        return exec_inherited_token(&run);
    }

    let identity = AgentIdentity::load(&account)?;
    let env = secret_backend_resolution_env(&account, Some(&identity))?;
    let resolved_command = resolve_executable_in_env(command, &env)
        .with_context(|| format!("resolve secret backend executable {command}"))?;
    validate_secret_backend_command(&resolved_command, skip_acl_check)?;
    let run = BackendRun {
        command: &resolved_command,
        arguments,
        payload,
        timeout,
        max_output_bytes,
    };
    exec_as_agent_account(&identity, &run)
}

fn validate_secret_backend_command(resolved_command: &str, skip_acl_check: bool) -> Result<()> {
    if skip_acl_check {
        return Ok(());
    }
    secret_backend_rights::check_secret_backend_command_rights(resolved_command)
        .with_context(|| format!("validate secret backend executable {resolved_command}"))
}

fn secret_backend_resolution_env(
    account: &AgentAccount,
    identity: Option<&AgentIdentity>,
) -> Result<HashMap<String, String>> {
    let baseline = if account.inherits_supervisor_token() {
        std::env::vars().collect()
    } else {
        let token = identity
            .map(|loaded| loaded.token.raw())
            .context("agent identity required for secret backend PATH resolution")?;
        baseline_env_vars_from_token(token)?
    };
    Ok(build_secret_backend_env_vars(baseline))
}

fn supervisor_runs_as_agent_account(account: &AgentAccount) -> bool {
    // dd-procmgr-service is LocalSystem; when datadogagent is too, inherited token matches.
    account.inherits_supervisor_token()
}

fn exec_as_agent_account(identity: &AgentIdentity, run: &BackendRun<'_>) -> Result<String> {
    let child = spawn_with_pipes(identity, run)?;
    child.finish(run)
}

struct AgentIdentity {
    token: TokenHandle,
    _profile: UserProfileGuard,
}

impl AgentIdentity {
    fn load(account: &AgentAccount) -> Result<Self> {
        let creds = logon_user_credentials(account);
        let logon_token = logon_user_token(PROCESS_NAME, &creds)?;
        let primary = duplicate_primary_token(PROCESS_NAME, logon_token.raw())?;
        let token = TokenHandle::new(primary);
        let profile = UserProfileGuard::load(PROCESS_NAME, token.raw(), account)?;
        Ok(Self {
            token,
            _profile: profile,
        })
    }
}

struct CapturedChild {
    process: WinHandle,
    stdin: std::fs::File,
    stdout: std::fs::File,
}

impl CapturedChild {
    fn finish(mut self, run: &BackendRun<'_>) -> Result<String> {
        self.stdin
            .write_all(run.payload.as_bytes())
            .context("write secret backend payload")?;
        drop(self.stdin);

        let process = SendProcessHandle(self.process.as_handle());
        let stdout = self.stdout;
        let command = run.command;
        let timeout = run.timeout;

        wait_with_stdout_drain(
            stdout,
            run.max_output_bytes,
            {
                let process = process;
                move || terminate_process(process.0)
            },
            move || {
                let exit_code = wait_for_exit(process.0, timeout, command)?;
                if exit_code != 0 {
                    bail!("secret backend {command} exited with code {exit_code}");
                }
                Ok(())
            },
        )
    }
}

fn spawn_with_pipes(identity: &AgentIdentity, run: &BackendRun<'_>) -> Result<CapturedChild> {
    let stdin = Pipe::parent_writes()?;
    let stdout = Pipe::parent_reads()?;
    let stderr = nul_stderr_handle()?;

    let command_line = build_windows_command_line(run.command, run.arguments);
    let mut command_line_w: Vec<u16> = std::ffi::OsStr::new(&command_line)
        .encode_wide()
        .chain([0])
        .collect();

    let env_block = env_block_for_secret_backend(identity.token.raw())?;
    let env_block_ptr = env_block.as_ptr() as *const std::ffi::c_void;

    let mut si: STARTUPINFOW = unsafe { std::mem::zeroed() };
    si.cb = std::mem::size_of::<STARTUPINFOW>() as u32;
    si.dwFlags = STARTF_USESTDHANDLES;
    si.hStdInput = stdin.child.raw();
    si.hStdOutput = stdout.child.raw();
    si.hStdError = stderr;

    let mut pi: PROCESS_INFORMATION = unsafe { std::mem::zeroed() };
    let ok = unsafe {
        CreateProcessAsUserW(
            identity.token.raw(),
            std::ptr::null(),
            command_line_w.as_mut_ptr(),
            std::ptr::null(),
            std::ptr::null(),
            1,
            CREATE_NEW_PROCESS_GROUP
                | CREATE_NEW_CONSOLE
                | CREATE_NO_WINDOW
                | CREATE_UNICODE_ENVIRONMENT,
            env_block_ptr,
            std::ptr::null(),
            &si,
            &mut pi,
        )
    };

    // Child ends are inherited by the process; close our copies.
    drop(stdin.child);
    drop(stdout.child);
    drop(WinHandle(stderr));

    if ok == 0 {
        bail!(
            "[{PROCESS_NAME}] CreateProcessAsUserW({}) failed: {}",
            run.command,
            std::io::Error::last_os_error()
        );
    }

    unsafe {
        CloseHandle(pi.hThread);
    }

    Ok(CapturedChild {
        process: WinHandle(pi.hProcess),
        stdin: stdin.parent,
        stdout: stdout.parent,
    })
}

struct Pipe {
    parent: std::fs::File,
    child: WinHandle,
}

fn create_pipe() -> Result<(HANDLE, HANDLE)> {
    let mut read: HANDLE = ptr::null_mut();
    let mut write: HANDLE = ptr::null_mut();
    let ok = unsafe { CreatePipe(&mut read, &mut write, ptr::null(), 0) };
    if ok == 0 {
        bail!("CreatePipe failed: {}", std::io::Error::last_os_error());
    }
    clear_inheritable(read)?;
    clear_inheritable(write)?;
    Ok((read, write))
}

impl Pipe {
    fn parent_writes() -> Result<Self> {
        let (child_read, parent_write) = create_pipe()?;
        set_inheritable(child_read)?;
        Ok(Self {
            parent: unsafe { std::fs::File::from_raw_handle(parent_write) },
            child: WinHandle(child_read),
        })
    }

    fn parent_reads() -> Result<Self> {
        let (parent_read, child_write) = create_pipe()?;
        set_inheritable(child_write)?;
        Ok(Self {
            parent: unsafe { std::fs::File::from_raw_handle(parent_read) },
            child: WinHandle(child_write),
        })
    }
}

fn nul_stderr_handle() -> Result<HANDLE> {
    let nul = wide::null_terminated("NUL");
    let handle = unsafe {
        CreateFileW(
            nul.as_ptr(),
            FILE_GENERIC_WRITE,
            FILE_SHARE_READ | FILE_SHARE_WRITE,
            ptr::null(),
            OPEN_EXISTING,
            FILE_ATTRIBUTE_NORMAL,
            ptr::null_mut(),
        )
    };
    if handle == INVALID_HANDLE_VALUE || handle.is_null() {
        bail!(
            "CreateFileW(NUL) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    set_inheritable(handle)?;
    Ok(handle)
}

fn set_inheritable(handle: HANDLE) -> Result<()> {
    let ok = unsafe { SetHandleInformation(handle, HANDLE_FLAG_INHERIT, HANDLE_FLAG_INHERIT) };
    if ok == 0 {
        bail!(
            "SetHandleInformation(HANDLE_FLAG_INHERIT) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(())
}

fn clear_inheritable(handle: HANDLE) -> Result<()> {
    let ok = unsafe { SetHandleInformation(handle, HANDLE_FLAG_INHERIT, 0) };
    if ok == 0 {
        bail!(
            "SetHandleInformation(clear HANDLE_FLAG_INHERIT) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(())
}

struct WinHandle(HANDLE);

// SAFETY: Win32 process handles are kernel objects safe to use from a worker thread.
unsafe impl Send for WinHandle {}

struct SendProcessHandle(HANDLE);

// SAFETY: Win32 process handles are kernel objects safe to send to a worker thread.
unsafe impl Send for SendProcessHandle {}

impl WinHandle {
    fn as_handle(&self) -> HANDLE {
        self.0
    }

    fn raw(self) -> HANDLE {
        let handle = self.0;
        std::mem::forget(self);
        handle
    }
}

impl Drop for WinHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}

fn terminate_process(process: HANDLE) {
    unsafe {
        TerminateProcess(process, 1);
    }
}

fn wait_for_exit(process: HANDLE, timeout: Duration, command: &str) -> Result<u32> {
    let deadline = Instant::now() + timeout;
    loop {
        let remaining = deadline.saturating_duration_since(Instant::now());
        if remaining.is_zero() {
            terminate_process(process);
            bail!(
                "secret backend {command} timed out after {} seconds",
                timeout.as_secs()
            );
        }
        let ms = remaining.as_millis().min(u32::MAX as u128) as u32;
        match unsafe { WaitForSingleObject(process, ms) } {
            WAIT_OBJECT_0 => {
                let mut code = 0u32;
                let ok = unsafe { GetExitCodeProcess(process, &mut code) };
                if ok == 0 {
                    bail!(
                        "GetExitCodeProcess failed: {}",
                        std::io::Error::last_os_error()
                    );
                }
                return Ok(code);
            }
            WAIT_TIMEOUT => {}
            _ => {
                bail!(
                    "WaitForSingleObject failed: {}",
                    std::io::Error::last_os_error()
                );
            }
        }
    }
}
