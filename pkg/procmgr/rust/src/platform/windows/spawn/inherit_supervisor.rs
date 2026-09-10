// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::mem;
use std::os::windows::ffi::OsStrExt;

use anyhow::{Result, bail};
use windows_sys::Win32::Security::{TOKEN_DUPLICATE, TOKEN_QUERY};
use windows_sys::Win32::System::Console::STD_ERROR_HANDLE;
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW, CREATE_SUSPENDED,
    CREATE_UNICODE_ENVIRONMENT, CreateProcessW, EXTENDED_STARTUPINFO_PRESENT, PROCESS_INFORMATION,
};

use crate::spawn::SpawnRequest;

use super::super::token_identity::open_current_process_token;
use super::super::wide;
use super::credential::SpawnCredential;
use super::startup_info_ex::StartupInfoEx;
use super::stdio::{map_stdio_handle_nul, map_stdio_setting};
use super::suspended::SuspendedChild;
use super::win32::{build_windows_command_line, env_block_from_baseline_plus_overrides};

/// Spawns a suspended child in the supervisor's security context (CreateProcessW).
///
/// CreateProcessAsUserW is not used here: the child matches dd-procmgrd's identity, and that
/// API requires SeIncreaseQuotaPrivilege that the installed agent account does not hold.
pub(super) fn spawn_inherit_supervisor(
    process_name: &str,
    request: &SpawnRequest,
    credential: &SpawnCredential,
) -> Result<SuspendedChild> {
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

    let stdio_handles = [stdin_handle.raw(), stdout_handle.raw(), stderr_handle.raw()];
    let mut startup_info = StartupInfoEx::with_stdio_handles(&stdio_handles)?;

    let creation_flags = CREATE_SUSPENDED
        | CREATE_NEW_PROCESS_GROUP
        | CREATE_NEW_CONSOLE
        | CREATE_NO_WINDOW
        | CREATE_UNICODE_ENVIRONMENT
        | EXTENDED_STARTUPINFO_PRESENT;

    let mut process_info: PROCESS_INFORMATION = unsafe { mem::zeroed() };
    let ok = unsafe {
        CreateProcessW(
            std::ptr::null(),
            command_line_w.as_mut_ptr(),
            std::ptr::null(),
            std::ptr::null(),
            1,
            creation_flags,
            env_block_ptr,
            current_dir_w
                .as_ref()
                .map(|w| w.as_ptr())
                .unwrap_or(std::ptr::null()),
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

    Ok(SuspendedChild::new(
        process_info.dwProcessId,
        process_info.hProcess,
        process_info.hThread,
    ))
}
