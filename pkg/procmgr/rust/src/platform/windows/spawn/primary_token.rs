// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{bail, Result};
use std::mem;
use std::os::windows::ffi::OsStrExt;
use windows_sys::Win32::Foundation::{CloseHandle, ERROR_INVALID_PARAMETER};
use windows_sys::Win32::System::Console::{STD_ERROR_HANDLE, STD_OUTPUT_HANDLE};
use windows_sys::Win32::System::Threading::{CreateProcessAsUserW, PROCESS_INFORMATION};

use crate::handle::ProcessHandle;
use crate::spawn::SpawnRequest;

use super::super::job_object::current_process_in_job;
use super::super::process::terminate_process;
use super::super::wide;
use super::super::JobObject;
use super::credential::SpawnCredential;
use super::logon::TokenHandle;
use super::startup_info_ex::StartupInfoEx;
use super::stdio::{map_stdio_handle_nul, map_stdio_setting};
use super::user_profile::UserProfileGuard;
use super::win32::{
    build_windows_command_line, env_block_from_baseline_plus_overrides,
    managed_process_creation_flags_for_job_list, managed_process_creation_flags_for_post_assign,
    resume_child_primary_thread,
};

/// Spawn a child with `CreateProcessAsUserW` using a **primary access token**.
///
/// If you are not familiar with Windows: a primary token is the security context the new
/// process runs under (user, groups, privileges). This path is agent-profile `LogonUser`
/// only. Privileged and same-account agent spawns use `CreateProcessW` in
/// `inherit_supervisor`.
///
/// The child is created already assigned to `job` via `PROC_THREAD_ATTRIBUTE_JOB_LIST` when
/// the supervisor is not already in a foreign job object.
///
/// Returns `(ProcessHandle, UserProfileGuard)`. The profile guard must stay alive
/// for the child's lifetime.
pub(super) fn spawn_as_primary_token(
    process_name: &str,
    request: &SpawnRequest,
    credential: &SpawnCredential,
    job: &JobObject,
) -> Result<(ProcessHandle, UserProfileGuard)> {
    // Resolve inheritable stdout/stderr for the spawn token (may differ from supervisor).
    let stdout_handle = map_stdio_setting(
        process_name,
        request.stdout_setting(),
        STD_OUTPUT_HANDLE,
        credential,
    )?;
    let stderr_handle = map_stdio_setting(
        process_name,
        request.stderr_setting(),
        STD_ERROR_HANDLE,
        credential,
    )?;
    // On Windows, stdio is passed as inheritable HANDLEs; NUL stdin avoids tying the child
    // to procmgrd's console.
    let stdin_handle = map_stdio_handle_nul()?;

    // Win32 wants argv as one wchar string here (lpApplicationName is null below).
    let command_line = build_windows_command_line(request.command(), request.args());

    let mut command_line_w: Vec<u16> = std::ffi::OsStr::new(&command_line)
        .encode_wide()
        .chain([0])
        .collect();

    let current_dir_w = request
        .working_dir()
        .map(|d| wide::null_terminated(d.to_string_lossy().as_ref()));

    // Primary token for CreateProcessAsUserW, from LogonUser.
    let primary_token_guard = TokenHandle::new(credential.duplicate_primary_token(process_name)?);

    // LoadUserProfileW mounts the user's HKCU hive (per-user registry).
    let profile_guard = UserProfileGuard::load(
        process_name,
        primary_token_guard.raw(),
        credential.account(),
    )?;

    // Env: CreateEnvironmentBlock(token) -> legacy SCM registry -> processes.d overrides.
    let env_block = env_block_from_baseline_plus_overrides(
        process_name,
        primary_token_guard.raw(),
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

    let mut pi: PROCESS_INFORMATION = unsafe { mem::zeroed() };

    if !current_process_in_job() {
        let mut startup_info =
            StartupInfoEx::with_stdio_and_job(stdin, stdout, stderr, job.raw_handle())?;
        let ok = unsafe {
            CreateProcessAsUserW(
                primary_token_guard.raw(),
                std::ptr::null(),
                command_line_w.as_mut_ptr(),
                std::ptr::null(),
                std::ptr::null(),
                1,
                managed_process_creation_flags_for_job_list(),
                env_block_ptr,
                current_dir_ptr,
                startup_info.startup_info(),
                &mut pi,
            )
        };
        if ok != 0 {
            return finish_primary_spawn(process_name, pi, profile_guard);
        }
        let err = std::io::Error::last_os_error();
        if err.raw_os_error() != Some(ERROR_INVALID_PARAMETER as i32) {
            bail!("[{process_name}] CreateProcessAsUserW failed: {err}");
        }
    }

    spawn_post_assign_primary_token(
        process_name,
        primary_token_guard.raw(),
        &mut command_line_w,
        env_block_ptr,
        current_dir_ptr,
        stdin,
        stdout,
        stderr,
        job,
        profile_guard,
    )
}

fn spawn_post_assign_primary_token(
    process_name: &str,
    primary_token: windows_sys::Win32::Foundation::HANDLE,
    command_line_w: &mut [u16],
    env_block_ptr: *const std::ffi::c_void,
    current_dir_ptr: *const u16,
    stdin: windows_sys::Win32::Foundation::HANDLE,
    stdout: windows_sys::Win32::Foundation::HANDLE,
    stderr: windows_sys::Win32::Foundation::HANDLE,
    job: &JobObject,
    profile_guard: UserProfileGuard,
) -> Result<(ProcessHandle, UserProfileGuard)> {
    let mut startup_info = StartupInfoEx::with_stdio_handles(stdin, stdout, stderr)?;
    let mut pi: PROCESS_INFORMATION = unsafe { mem::zeroed() };
    let ok = unsafe {
        CreateProcessAsUserW(
            primary_token,
            std::ptr::null(),
            command_line_w.as_mut_ptr(),
            std::ptr::null(),
            std::ptr::null(),
            1,
            managed_process_creation_flags_for_post_assign(),
            env_block_ptr,
            current_dir_ptr,
            startup_info.startup_info(),
            &mut pi,
        )
    };
    if ok == 0 {
        bail!(
            "[{process_name}] CreateProcessAsUserW failed: {}",
            std::io::Error::last_os_error()
        );
    }

    if let Err(e) = job.assign_process(pi.dwProcessId) {
        abort_suspended_spawn(&pi);
        return Err(anyhow::anyhow!(
            "[{process_name}] post-create job assignment failed: {e}"
        ));
    }

    if let Err(e) = resume_child_primary_thread(process_name, pi.dwProcessId, pi.hThread) {
        abort_suspended_spawn(&pi);
        return Err(e);
    }

    finish_primary_spawn(process_name, pi, profile_guard)
}

fn finish_primary_spawn(
    process_name: &str,
    pi: PROCESS_INFORMATION,
    profile_guard: UserProfileGuard,
) -> Result<(ProcessHandle, UserProfileGuard)> {
    let pid = pi.dwProcessId;
    let handle = match ProcessHandle::from_borrowed(pid, pi.hProcess) {
        Ok(handle) => handle,
        Err(e) => {
            let _ = terminate_process(pi.hProcess);
            unsafe {
                CloseHandle(pi.hProcess);
                CloseHandle(pi.hThread);
            }
            return Err(e);
        }
    };

    unsafe {
        CloseHandle(pi.hProcess);
        CloseHandle(pi.hThread);
    }

    Ok((handle, profile_guard))
}

fn abort_suspended_spawn(process_info: &PROCESS_INFORMATION) {
    let _ = terminate_process(process_info.hProcess);
    unsafe {
        CloseHandle(process_info.hProcess);
        CloseHandle(process_info.hThread);
    }
}

#[cfg(test)]
mod tests {
    use crate::platform::windows::spawn::win32::build_windows_command_line;

    #[test]
    fn command_line_preserves_args_without_spaces() {
        let line = build_windows_command_line(
            "ping.exe",
            &["-n".to_string(), "61".to_string(), "127.0.0.1".to_string()],
        );
        assert_eq!(line, "ping.exe -n 61 127.0.0.1");
    }
}
