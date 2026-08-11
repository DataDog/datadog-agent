// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use std::os::windows::ffi::OsStrExt;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::Security::{TOKEN_DUPLICATE, TOKEN_QUERY};
use windows_sys::Win32::System::Console::STD_ERROR_HANDLE;
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW, CREATE_UNICODE_ENVIRONMENT,
    CreateProcessAsUserW, GetCurrentProcess, OpenProcessToken, PROCESS_INFORMATION,
    STARTF_USESTDHANDLES, STARTUPINFOW,
};

use crate::handle::ProcessHandle;
use crate::spawn::SpawnRequest;

use super::super::agent_credentials::AgentAccount;
use super::super::wide;
use super::logon::{TokenHandle, logon_user_credentials, logon_user_token};
use super::stdio::{map_stdio_handle_nul, map_stdio_setting};
use super::user_profile::UserProfileGuard;
use super::win32::{
    build_windows_command_line, duplicate_primary_token, env_block_from_baseline_plus_overrides,
};

pub(super) fn spawn_as_primary_token(
    process_name: &str,
    request: &SpawnRequest,
    account: &AgentAccount,
) -> Result<(ProcessHandle, Option<UserProfileGuard>)> {
    // Map stdio to explicit Win32 handles for CreateProcessAsUserW.
    let stdout_handle = map_stdio_setting(
        process_name,
        &request.stdout_setting,
        windows_sys::Win32::System::Console::STD_OUTPUT_HANDLE,
        account,
    )?;
    let stderr_handle = map_stdio_setting(
        process_name,
        &request.stderr_setting,
        STD_ERROR_HANDLE,
        account,
    )?;
    let stdin_handle = map_stdio_handle_nul()?;

    let command_line = build_windows_command_line(&request.command, &request.args);

    let mut command_line_w: Vec<u16> = std::ffi::OsStr::new(&command_line)
        .encode_wide()
        .chain([0])
        .collect();

    let current_dir_w = request
        .working_dir
        .as_ref()
        .map(|d| wide::null_terminated(d.to_string_lossy().as_ref()));

    let primary_token_guard = TokenHandle::new(match account {
        AgentAccount::LocalSystem => local_system_primary_token(process_name)?,
        _ => duplicate_primary_token(
            process_name,
            logon_user_token(process_name, &logon_user_credentials(account))?.raw(),
        )?,
    });

    let profile_guard = if account.inherits_supervisor_token() {
        None
    } else {
        Some(UserProfileGuard::load(
            process_name,
            primary_token_guard.raw(),
            account,
        )?)
    };

    let env_block = env_block_from_baseline_plus_overrides(
        process_name,
        primary_token_guard.raw(),
        &request.env,
    )?;
    let env_block_ptr = env_block.as_ptr() as *const std::ffi::c_void;

    let mut si: STARTUPINFOW = unsafe { std::mem::zeroed() };
    si.cb = std::mem::size_of::<STARTUPINFOW>() as u32;
    si.dwFlags = STARTF_USESTDHANDLES;
    si.hStdInput = stdin_handle.raw();
    si.hStdOutput = stdout_handle.raw();
    si.hStdError = stderr_handle.raw();

    let dw_creation_flags = CREATE_NEW_PROCESS_GROUP
        | CREATE_NEW_CONSOLE
        | CREATE_NO_WINDOW
        | CREATE_UNICODE_ENVIRONMENT;

    // Run as the supervisor (LocalSystem): pass the target primary token via `hToken`.
    // Do not impersonate here — a job object handle created by LocalSystem is not valid
    // in an impersonated thread context (CreateProcessAsUserW returns ERROR_INVALID_HANDLE).
    // Job assignment is done post-spawn by the caller.
    let mut pi: PROCESS_INFORMATION = unsafe { std::mem::zeroed() };
    let ok = unsafe {
        CreateProcessAsUserW(
            primary_token_guard.raw(),
            // Null application name: resolve the image from the command line (including PATH),
            // matching `Command::new` / legacy spawn behavior for bare names like powershell.exe.
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
            &si,
            &mut pi,
        )
    };
    if ok == 0 {
        bail!(
            "[{process_name}] CreateProcessAsUserW failed: {}",
            std::io::Error::last_os_error()
        );
    }

    unsafe {
        let _ = CloseHandle(pi.hThread);
    }

    Ok((
        ProcessHandle::from_raw(pi.dwProcessId, pi.hProcess),
        profile_guard,
    ))
}

fn local_system_primary_token(process_name: &str) -> Result<HANDLE> {
    let mut process_token: HANDLE = std::ptr::null_mut();
    let ok = unsafe {
        OpenProcessToken(
            GetCurrentProcess(),
            TOKEN_QUERY | TOKEN_DUPLICATE,
            &mut process_token,
        )
    };
    if ok == 0 {
        bail!(
            "[{process_name}] OpenProcessToken(GetCurrentProcess()) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    let process_token_guard = TokenHandle::new(process_token);
    duplicate_primary_token(process_name, process_token_guard.raw())
}

#[cfg(test)]
mod tests {
    use super::super::win32::{build_windows_command_line, env_vars_to_wide_block};
    use std::collections::HashMap;

    #[test]
    fn command_line_preserves_args_without_spaces() {
        let line = build_windows_command_line(
            "ping.exe",
            &["-n".to_string(), "61".to_string(), "127.0.0.1".to_string()],
        );
        assert_eq!(line, "ping.exe -n 61 127.0.0.1");
    }

    #[test]
    fn env_block_is_sorted_case_insensitively() {
        let mut vars = HashMap::new();
        vars.insert("ZZZ".to_string(), "1".to_string());
        vars.insert("aaa".to_string(), "2".to_string());
        vars.insert("BBB".to_string(), "3".to_string());

        let block = env_vars_to_wide_block(&vars);
        let entries = wide_block_entries(&block);
        assert_eq!(entries, ["aaa=2", "BBB=3", "ZZZ=1"]);
    }

    fn wide_block_entries(block: &[u16]) -> Vec<String> {
        let mut entries = Vec::new();
        let mut start = 0usize;
        for (i, &unit) in block.iter().enumerate() {
            if unit == 0 {
                if i == start {
                    break;
                }
                entries.push(String::from_utf16_lossy(&block[start..i]));
                start = i + 1;
            }
        }
        entries
    }
}
