// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::mem;
use std::os::windows::ffi::OsStrExt;

use anyhow::{bail, Result};
use windows_sys::Win32::Foundation::{CloseHandle, ERROR_INVALID_PARAMETER};
use windows_sys::Win32::Security::{TOKEN_DUPLICATE, TOKEN_QUERY};
use windows_sys::Win32::System::Console::STD_ERROR_HANDLE;
use windows_sys::Win32::System::Threading::{CreateProcessW, PROCESS_INFORMATION};

use crate::handle::ProcessHandle;
use crate::spawn::SpawnRequest;

use super::super::job_object::current_process_in_job;
use super::super::process::terminate_process;
use super::super::token_identity::open_current_process_token;
use super::super::wide;
use super::super::JobObject;
use super::credential::SpawnCredential;
use super::startup_info_ex::StartupInfoEx;
use super::stdio::{map_stdio_handle_nul, map_stdio_setting};
use super::win32::{
    build_windows_command_line, env_block_from_baseline_plus_overrides,
    managed_process_creation_flags_for_job_list, managed_process_creation_flags_for_post_assign,
    resume_child_primary_thread,
};

/// Spawns a child in the supervisor's security context (`CreateProcessW`).
///
/// `CreateProcessAsUserW` is not used here: the child matches dd-procmgrd's identity, and that
/// API requires `SeIncreaseQuotaPrivilege` that the installed agent account does not hold.
///
/// Used for privileged spawn and for agent-profile spawn when the supervisor already
/// runs as the target account.
///
/// The child joins `job` at create time via `PROC_THREAD_ATTRIBUTE_JOB_LIST` when the
/// supervisor is not already in a foreign job object.
pub(super) fn spawn_inherit_supervisor(
    process_name: &str,
    request: &SpawnRequest,
    credential: &SpawnCredential,
    job: &JobObject,
) -> Result<ProcessHandle> {
    let stdout_handle = map_stdio_setting(
        process_name,
        request.stdout_setting(),
        windows_sys::Win32::System::Console::STD_OUTPUT_HANDLE,
        credential,
    )?;
    let stderr_handle = map_stdio_setting(
        process_name,
        request.stderr_setting(),
        STD_ERROR_HANDLE,
        credential,
    )?;
    let stdin_handle = map_stdio_handle_nul()?;

    let command_line = build_windows_command_line(request.command(), request.args());
    let mut command_line_w: Vec<u16> = std::ffi::OsStr::new(&command_line)
        .encode_wide()
        .chain([0])
        .collect();

    let current_dir_w = request
        .working_dir()
        .map(|d| wide::null_terminated(d.to_string_lossy().as_ref()));

    let supervisor_token =
        open_current_process_token(TOKEN_QUERY | TOKEN_DUPLICATE).map_err(|e| {
            anyhow::anyhow!("[{process_name}] OpenProcessToken(GetCurrentProcess()) failed: {e}")
        })?;
    let env_block = env_block_from_baseline_plus_overrides(
        process_name,
        supervisor_token.as_handle(),
        request.env(),
    )?;
    let env_block_ptr = env_block.as_ptr() as *const std::ffi::c_void;

    let stdin = stdin_handle.raw();
    let stdout = stdout_handle.raw();
    let stderr = stderr_handle.raw();
    let current_dir_ptr = current_dir_w
        .as_ref()
        .map(|w| w.as_ptr())
        .unwrap_or(std::ptr::null());

    let mut process_info: PROCESS_INFORMATION = unsafe { mem::zeroed() };

    if !current_process_in_job() {
        let mut startup_info =
            StartupInfoEx::with_stdio_and_job(stdin, stdout, stderr, job.raw_handle())?;
        let ok = unsafe {
            CreateProcessW(
                std::ptr::null(),
                command_line_w.as_mut_ptr(),
                std::ptr::null(),
                std::ptr::null(),
                1,
                managed_process_creation_flags_for_job_list(),
                env_block_ptr,
                current_dir_ptr,
                startup_info.startup_info(),
                &mut process_info,
            )
        };
        if ok != 0 {
            return finish_inherit_spawn(process_name, process_info);
        }
        let err = std::io::Error::last_os_error();
        if err.raw_os_error() != Some(ERROR_INVALID_PARAMETER as i32) {
            bail!("[{process_name}] CreateProcessW failed: {err}");
        }
    }

    spawn_post_assign_inherit_supervisor(
        process_name,
        &mut command_line_w,
        env_block_ptr,
        current_dir_ptr,
        stdin,
        stdout,
        stderr,
        job,
    )
}

fn spawn_post_assign_inherit_supervisor(
    process_name: &str,
    command_line_w: &mut [u16],
    env_block_ptr: *const std::ffi::c_void,
    current_dir_ptr: *const u16,
    stdin: windows_sys::Win32::Foundation::HANDLE,
    stdout: windows_sys::Win32::Foundation::HANDLE,
    stderr: windows_sys::Win32::Foundation::HANDLE,
    job: &JobObject,
) -> Result<ProcessHandle> {
    let mut startup_info = StartupInfoEx::with_stdio_handles(stdin, stdout, stderr)?;
    let mut process_info: PROCESS_INFORMATION = unsafe { mem::zeroed() };
    let ok = unsafe {
        CreateProcessW(
            std::ptr::null(),
            command_line_w.as_mut_ptr(),
            std::ptr::null(),
            std::ptr::null(),
            1,
            managed_process_creation_flags_for_post_assign(),
            env_block_ptr,
            current_dir_ptr,
            startup_info.startup_info(),
            &mut process_info,
        )
    };
    if ok == 0 {
        bail!(
            "[{process_name}] CreateProcessW failed: {}",
            std::io::Error::last_os_error()
        );
    }

    if let Err(e) = job.assign_process(process_info.dwProcessId) {
        abort_suspended_spawn(&process_info);
        return Err(anyhow::anyhow!(
            "[{process_name}] post-create job assignment failed: {e}"
        ));
    }

    if let Err(e) =
        resume_child_primary_thread(process_name, process_info.dwProcessId, process_info.hThread)
    {
        abort_suspended_spawn(&process_info);
        return Err(e);
    }

    finish_inherit_spawn(process_name, process_info)
}

fn finish_inherit_spawn(
    process_name: &str,
    process_info: PROCESS_INFORMATION,
) -> Result<ProcessHandle> {
    let pid = process_info.dwProcessId;
    let handle = match ProcessHandle::from_borrowed(pid, process_info.hProcess) {
        Ok(handle) => handle,
        Err(e) => {
            let _ = terminate_process(process_info.hProcess);
            unsafe {
                CloseHandle(process_info.hProcess);
                CloseHandle(process_info.hThread);
            }
            return Err(e);
        }
    };

    unsafe {
        CloseHandle(process_info.hProcess);
        CloseHandle(process_info.hThread);
    }

    Ok(handle)
}

fn abort_suspended_spawn(process_info: &PROCESS_INFORMATION) {
    let _ = terminate_process(process_info.hProcess);
    unsafe {
        CloseHandle(process_info.hProcess);
        CloseHandle(process_info.hThread);
    }
}
