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
use crate::spawn::SpawnRequest;

use super::super::agent_credentials::AgentAccount;
use super::super::wide;
use super::logon::{TokenHandle, logon_user_credentials, logon_user_token};
use super::stdio::{map_stdio_handle_nul, map_stdio_setting};

pub(super) fn spawn_as_primary_token(
    process_name: &str,
    request: &SpawnRequest,
    account: &AgentAccount,
) -> Result<ProcessHandle> {
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
            &mut si,
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
    if account.inherits_supervisor_token() {
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
            // Agent children may need delegated auth to network resources (file shares, SQL, etc.).
            // TODO: make optional — probably only the core Agent needs delegation.
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

fn build_windows_command_line(command: &str, args: &[String]) -> String {
    let mut cmdline = windows_command_line_arg(command);
    for arg in args {
        cmdline.push(' ');
        cmdline.push_str(&windows_command_line_arg(arg));
    }
    cmdline
}

fn windows_command_line_arg(s: &str) -> String {
    // Matches MSVC / CommandLineToArgvW: quote only when the token contains
    // whitespace or quotes. Unconditional quoting breaks cmd.exe /C because it
    // parses the raw command line after the switch, not argv[2] alone.
    if s.is_empty() {
        return "\"\"".to_string();
    }
    if !s.chars().any(|ch| ch.is_whitespace() || ch == '"') {
        return s.to_string();
    }

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
    super::super::merge_env_overrides(&mut vars, overrides);
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn command_line_quotes_only_when_needed_for_cmd_c() {
        let line = build_windows_command_line("cmd.exe", &["/C".to_string(), "exit 1".to_string()]);
        assert_eq!(line, r#"cmd.exe /C "exit 1""#);
    }

    #[test]
    fn command_line_preserves_args_without_spaces() {
        let line = build_windows_command_line(
            "ping.exe",
            &["-n".to_string(), "61".to_string(), "127.0.0.1".to_string()],
        );
        assert_eq!(line, "ping.exe -n 61 127.0.0.1");
    }

    #[test]
    fn command_line_quotes_paths_with_spaces() {
        let line = build_windows_command_line(r"C:\Program Files\app.exe", &["--flag".to_string()]);
        assert_eq!(line, r#""C:\Program Files\app.exe" --flag"#);
    }
}
