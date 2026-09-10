// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use std::mem;
use std::os::windows::ffi::OsStrExt;
use windows_sys::Win32::Foundation::CloseHandle;
use windows_sys::Win32::System::Console::{STD_ERROR_HANDLE, STD_OUTPUT_HANDLE};
use windows_sys::Win32::System::Threading::{CreateProcessAsUserW, PROCESS_INFORMATION};

use crate::handle::ProcessHandle;
use crate::spawn::SpawnRequest;

use super::super::JobObject;
use super::super::process::terminate_process;
use super::super::wide;
use super::credential::SpawnCredential;
use super::logon::TokenHandle;
use super::startup_info_ex::StartupInfoEx;
use super::stdio::{map_stdio_handle_nul, map_stdio_setting};
use super::user_profile::UserProfileGuard;
use super::win32::{
    build_windows_command_line, env_block_from_baseline_plus_overrides,
    managed_process_creation_flags,
};

/// Spawn a child with `CreateProcessAsUserW` using a **primary access token**.
///
/// If you are not familiar with Windows: a primary token is the security context the new
/// process runs under (user, groups, privileges). This path is agent-profile `LogonUser`
/// only. Privileged and same-account agent spawns use `CreateProcessW` in
/// `inherit_supervisor`.
///
/// The child is created already assigned to `job` via `PROC_THREAD_ATTRIBUTE_JOB_LIST`.
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

    // Job object: Windows kernel construct to group processes for kill-on-stop (see managed.rs).
    let mut startup_info = StartupInfoEx::with_stdio_and_job(
        stdin_handle.raw(),
        stdout_handle.raw(),
        stderr_handle.raw(),
        job.raw_handle(),
    )?;

    let dw_creation_flags = managed_process_creation_flags();

    let mut pi: PROCESS_INFORMATION = unsafe { mem::zeroed() };
    // bInheritHandles=1 plus PROC_THREAD_ATTRIBUTE_HANDLE_LIST: only listed handles are inherited.
    let ok = unsafe {
        CreateProcessAsUserW(
            primary_token_guard.raw(),
            std::ptr::null(),
            command_line_w.as_mut_ptr(),
            std::ptr::null(),
            std::ptr::null(),
            1,
            dw_creation_flags,
            env_block_ptr,
            current_dir_w
                .as_ref()
                .map(|w| w.as_ptr())
                .unwrap_or(std::ptr::null()),
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

    let pid = pi.dwProcessId;
    // Duplicate into ProcessHandle, then close the kernel handles from PROCESS_INFORMATION.
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
