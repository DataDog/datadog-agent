// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
use std::collections::HashMap;
use std::os::windows::ffi::OsStrExt;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::Security::{
    DuplicateTokenEx, SecurityDelegation, TOKEN_DUPLICATE, TOKEN_QUERY, TokenPrimary,
};
use windows_sys::Win32::System::Console::STD_ERROR_HANDLE;
use windows_sys::Win32::System::SystemServices::MAXIMUM_ALLOWED;
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW, CREATE_UNICODE_ENVIRONMENT,
    CreateProcessAsUserW, GetCurrentProcess, OpenProcessToken, PROCESS_INFORMATION,
    STARTF_USESTDHANDLES, STARTUPINFOW,
};

use crate::handle::ProcessHandle;
use crate::spawn_request::SpawnRequest;

use super::super::agent_credentials::AgentAccount;
use super::super::wide;
use super::logon::{TokenHandle, logon_user_credentials, logon_user_token};
use super::stdio::{map_stdio_config, map_stdio_handle_nul};

pub(super) fn spawn_as_primary_token(
    process_name: &str,
    request: &SpawnRequest,
    account: &AgentAccount,
) -> Result<ProcessHandle> {
    // Only support stdio shapes that can be mapped to explicit Win32 handles.
    // For anything else, fall back to impersonation + tokio::Command.
    let stdout_handle = map_stdio_config(
        process_name,
        &request.stdout_config,
        windows_sys::Win32::System::Console::STD_OUTPUT_HANDLE,
        account,
    )?;
    let stderr_handle = map_stdio_config(
        process_name,
        &request.stderr_config,
        STD_ERROR_HANDLE,
        account,
    )?;
    let stdin_handle = map_stdio_handle_nul()?;

    let command_line = {
        let mut cmdline = windows_crt_escape_arg(&request.command);
        for arg in &request.args {
            cmdline.push(' ');
            cmdline.push_str(&windows_crt_escape_arg(arg));
        }
        cmdline
    };

    let application_name_w = wide::null_terminated(&request.command);
    let mut command_line_w: Vec<u16> = std::ffi::OsStr::new(&command_line)
        .encode_wide()
        .chain([0])
        .collect();

    let current_dir_w = request
        .working_dir
        .as_ref()
        .map(|d| wide::null_terminated(d.to_string_lossy().as_ref()));

    // Acquire a primary token for the configured account.
    let primary_token_guard = TokenHandle::new(match account {
        AgentAccount::LocalSystem => local_system_primary_token(process_name)?,
        _ => primary_token_from_logon(process_name, account)?,
    });

    let env_block =
        env_block_from_baseline_plus_overrides(primary_token_guard.raw(), &request.env)?;
    let env_block_ptr = env_block.as_ptr() as *const std::ffi::c_void;

    let mut si: STARTUPINFOW = unsafe { std::mem::zeroed() };
    si.cb = std::mem::size_of::<STARTUPINFOW>() as u32;
    si.dwFlags = STARTF_USESTDHANDLES;
    si.hStdInput = stdin_handle.raw();
    si.hStdOutput = stdout_handle.raw();
    si.hStdError = stderr_handle.raw();

    let mut pi: PROCESS_INFORMATION = unsafe { std::mem::zeroed() };
    let dw_creation_flags = CREATE_NEW_PROCESS_GROUP
        | CREATE_NEW_CONSOLE
        | CREATE_NO_WINDOW
        | CREATE_UNICODE_ENVIRONMENT;

    let ok = unsafe {
        CreateProcessAsUserW(
            primary_token_guard.raw(),
            application_name_w.as_ptr(),
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

    Ok(ProcessHandle::from_raw(pi.dwProcessId, pi.hProcess))
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

fn primary_token_from_logon(process_name: &str, account: &AgentAccount) -> Result<HANDLE> {
    if matches!(account, AgentAccount::LocalSystem) {
        bail!("[{process_name}] internal error: LocalSystem uses supervisor token duplication")
    }
    let creds = logon_user_credentials(account);
    let logon_token = logon_user_token(process_name, &creds)?;
    duplicate_primary_token(process_name, logon_token.raw())
}

fn duplicate_primary_token(process_name: &str, token: HANDLE) -> Result<HANDLE> {
    let mut primary_token: HANDLE = std::ptr::null_mut();
    let ok = unsafe {
        DuplicateTokenEx(
            token,
            MAXIMUM_ALLOWED,
            std::ptr::null(),
            SecurityDelegation,
            TokenPrimary,
            &mut primary_token,
        )
    };
    if ok == 0 {
        bail!(
            "[{process_name}] DuplicateTokenEx failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(primary_token)
}

fn windows_crt_escape_arg(s: &str) -> String {
    // Matches the quoting rules used by the Windows CRT (inverse of
    // CommandLineToArgvW decoding).
    let mut out = String::new();
    out.push('"');
    let mut backslashes = 0usize;
    for ch in s.chars() {
        match ch {
            '\\' => backslashes += 1,
            '"' => {
                out.push_str(&"\\".repeat(backslashes * 2 + 1));
                out.push('"');
                backslashes = 0;
            }
            _ => {
                out.push_str(&"\\".repeat(backslashes));
                out.push(ch);
                backslashes = 0;
            }
        }
    }
    out.push_str(&"\\".repeat(backslashes * 2));
    out.push('"');
    out
}

fn env_block_from_baseline_plus_overrides(
    token: HANDLE,
    overrides: &[(String, String)],
) -> Result<Vec<u16>> {
    let mut vars = super::super::baseline_env_vars_from_token(token)
        .context("build child environment from spawn token")?;
    for (k, v) in overrides {
        vars.insert(k.clone(), v.clone());
    }
    Ok(env_vars_to_wide_block(&vars))
}

fn env_vars_to_wide_block(vars: &HashMap<String, String>) -> Vec<u16> {
    let mut block: Vec<u16> = Vec::new();
    for (k, v) in vars {
        let kv = format!("{k}={v}");
        block.extend(std::ffi::OsStr::new(&kv).encode_wide());
        block.push(0);
    }
    // Double NUL terminator.
    block.push(0);
    block
}
