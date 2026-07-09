// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows child spawning by spawn profile (inherit vs agent-user impersonation).

use anyhow::{Context, Result, bail};
use log::info;
use std::collections::HashMap;
use std::os::windows::ffi::OsStrExt;
use std::process::Stdio;
use std::ptr;
use tokio::process::Command;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE, INVALID_HANDLE_VALUE};
use windows_sys::Win32::Security::{
    DuplicateTokenEx, ImpersonateLoggedOnUser, LOGON32_LOGON_SERVICE, LOGON32_PROVIDER_DEFAULT,
    LogonUserW, RevertToSelf, SecurityDelegation, TokenPrimary,
};
use windows_sys::Win32::Storage::FileSystem::{
    CreateFileW, FILE_ATTRIBUTE_NORMAL, FILE_GENERIC_READ, FILE_GENERIC_WRITE, FILE_SHARE_READ,
    FILE_SHARE_WRITE, OPEN_EXISTING,
};
use windows_sys::Win32::System::Console::{
    GetStdHandle, STD_ERROR_HANDLE, STD_INPUT_HANDLE, STD_OUTPUT_HANDLE,
};
use windows_sys::Win32::System::SystemServices::MAXIMUM_ALLOWED;
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW, CREATE_UNICODE_ENVIRONMENT,
    CreateProcessAsUserW, PROCESS_INFORMATION, STARTF_USESTDHANDLES, STARTUPINFOW,
};

use crate::handle::ProcessHandle;
use crate::spawn_context;
use crate::spawn_profile::SpawnProfile;
use crate::spawn_request::SpawnRequest;

use super::agent_credentials::{AgentAccount, resolve_agent_account};
use super::apply_child_baseline_env;
use super::setup_process_group;
use super::wide;
use super::{install_root, program_data_root};

/// Spawn a managed child using the platform spawn profile for `process_name`.
///
/// Caller must hold [`super::console_lock`] on Windows (see `ManagedProcess::try_spawn`).
pub(crate) fn spawn_child(
    process_name: &str,
    request: SpawnRequest,
    profile: SpawnProfile,
) -> Result<ProcessHandle> {
    info!("[{process_name}] spawn profile: {profile}");

    if matches!(profile, SpawnProfile::Privileged) {
        validate_privileged_process_request(process_name, &request)?;
    }

    // Prefer primary-token spawning for privileged profiles; if that fails, try
    // impersonation-based LocalSystem spawn. Never fall back to the supervisor token.
    match profile {
        SpawnProfile::Privileged => {
            match spawn_as_primary_token(process_name, &request, &AgentAccount::LocalSystem) {
                Ok(handle) => return Ok(handle),
                Err(e) => {
                    log::warn!(
                        "[{process_name}] primary-token LocalSystem spawn failed (trying impersonation): {e:#}"
                    );
                }
            }
        }
        SpawnProfile::Agent => {
            // Primary-token spawn as the resolved agent account (passwordless supported).
            if let Ok(account) = resolve_agent_account() {
                if let Ok(handle) = spawn_as_primary_token(process_name, &request, &account) {
                    return Ok(handle);
                }
            }
        }
    }

    let (command, mut cmd) = build_command(request)?;
    match profile {
        SpawnProfile::Privileged => spawn_as_local_system(process_name, &command, &mut cmd)
            .with_context(|| {
                format!("[{process_name}] privileged spawn failed: could not run as LocalSystem")
            }),
        SpawnProfile::Agent => spawn_as_agent_user(process_name, &command, &mut cmd),
    }
}

/// Reject privileged spawn requests that don't exactly match our embedded
/// privileged process catalog spec.
fn validate_privileged_process_request(process_name: &str, request: &SpawnRequest) -> Result<()> {
    let install_root = install_root();
    let etc_root = program_data_root();

    let spec = privileged_process_spec(process_name, &install_root, &etc_root)?;

    validate_privileged_stdio(process_name, request)?;
    validate_privileged_working_dir(process_name, &spec, request)?;
    validate_privileged_command_args(process_name, &spec, request)?;
    validate_privileged_env(process_name, &spec, request)?;

    Ok(())
}

fn validate_privileged_stdio(process_name: &str, request: &SpawnRequest) -> Result<()> {
    // Privileged spawn only allows inherit/null stdio.
    let allow = |s: &std::process::Stdio| {
        matches!(s, std::process::Stdio::Inherit | std::process::Stdio::Null)
    };
    if !allow(&request.stdout) || !allow(&request.stderr) {
        bail!("[{process_name}] refusing privileged spawn: stdout/stderr must be inherit or null");
    }
    Ok(())
}

fn validate_privileged_working_dir(
    process_name: &str,
    spec: &PrivilegedProcessSpec,
    request: &SpawnRequest,
) -> Result<()> {
    if spec.disallow_working_dir && request.working_dir.is_some() {
        bail!("[{process_name}] refusing privileged spawn: working_dir is not allowed");
    }
    Ok(())
}

fn validate_privileged_command_args(
    process_name: &str,
    spec: &PrivilegedProcessSpec,
    request: &SpawnRequest,
) -> Result<()> {
    // Validate command/args match the embedded privileged catalog spec.
    let norm_cmd = normalize_win_path(&request.command);
    let expected_cmd = normalize_win_path(spec.expected_command.as_str());
    if norm_cmd != expected_cmd {
        bail!(
            "[{process_name}] refusing privileged spawn: unexpected command (got {}, expected {})",
            request.command,
            spec.expected_command
        );
    }

    let norm_args: Vec<_> = request.args.iter().map(|a| normalize_win_path(a)).collect();
    let expected_args: Vec<_> = spec
        .expected_args
        .iter()
        .map(|a| normalize_win_path(a.as_str()))
        .collect();

    if norm_args != expected_args {
        bail!(
            "[{process_name}] refusing privileged spawn: unexpected args {:?} (expected {:?})",
            request.args,
            spec.expected_args
        );
    }
    Ok(())
}

fn validate_privileged_env(
    process_name: &str,
    spec: &PrivilegedProcessSpec,
    request: &SpawnRequest,
) -> Result<()> {
    // Env allowlist: only variables required by the embedded template.
    // Anything else could be used to alter privileged behavior.
    for (k, v) in &request.env {
        if !spec.allowed_env.contains(&k.as_str()) {
            bail!(
                "[{process_name}] refusing privileged spawn: disallowed env var for privileged process: {k}"
            );
        }
        if spec.non_empty_env.contains(&k.as_str()) && v.trim().is_empty() {
            bail!("[{process_name}] refusing privileged spawn: {k} must be non-empty");
        }
    }
    Ok(())
}

struct PrivilegedProcessSpec {
    expected_command: String,
    expected_args: Vec<String>,
    allowed_env: &'static [&'static str],
    non_empty_env: &'static [&'static str],
    disallow_working_dir: bool,
}

fn privileged_process_spec(
    process_name: &str,
    install_root: &std::path::PathBuf,
    etc_root: &std::path::PathBuf,
) -> Result<PrivilegedProcessSpec> {
    use crate::spawn_profile::DATADOG_AGENT_PROCESS;

    match process_name {
        DATADOG_AGENT_PROCESS => Ok(PrivilegedProcessSpec {
            expected_command: install_root
                .join(r"bin\agent\process-agent.exe")
                .to_string_lossy()
                .into_owned(),
            expected_args: vec![
                "--cfgpath".to_string(),
                etc_root.join("datadog.yaml").to_string_lossy().into_owned(),
                "--sysprobe-config".to_string(),
                etc_root
                    .join("system-probe.yaml")
                    .to_string_lossy()
                    .into_owned(),
                "--pid".to_string(),
                install_root
                    .join(r"run\process-agent.pid")
                    .to_string_lossy()
                    .into_owned(),
            ],
            allowed_env: &["DD_FLEET_POLICIES_DIR"],
            non_empty_env: &["DD_FLEET_POLICIES_DIR"],
            disallow_working_dir: true,
        }),
        other => bail!(
            "[{other}] refusing privileged spawn: no privileged catalog template (internal error?)"
        ),
    }
}

fn normalize_win_path(s: &str) -> String {
    // Normalize to reduce differences between the embedded template (uses `/` via
    // `filepath.ToSlash`) and paths created/returned via Rust (`\`).
    s.replace('/', "\\").to_ascii_lowercase()
}

fn exec_spawn(process_name: &str, command: &str, cmd: &mut Command) -> Result<ProcessHandle> {
    setup_process_group(cmd);
    let child = cmd
        .spawn()
        .with_context(|| spawn_context::failed_message(process_name, command))?;
    Ok(ProcessHandle::from_child(child))
}

fn spawn_as_local_system(
    process_name: &str,
    command: &str,
    cmd: &mut Command,
) -> Result<ProcessHandle> {
    // LogonUserW + impersonation is required because dd-procmgr-service may not run as
    // LocalSystem; we explicitly impersonate LocalSystem so the privileged behavior matches
    // the legacy SCM service.
    //
    // For LocalSystem, the well-known identity is `NT AUTHORITY\\SYSTEM` with an empty password.
    spawn_with_impersonation(
        process_name,
        command,
        cmd,
        "NT AUTHORITY",
        "SYSTEM",
        Some(""),
    )
}

fn spawn_as_agent_user(
    process_name: &str,
    command: &str,
    cmd: &mut Command,
) -> Result<ProcessHandle> {
    let account = resolve_agent_account()
        .with_context(|| format!("[{process_name}] resolve agent service account for spawn"))?;

    match account {
        AgentAccount::LocalSystem => {
            info!("[{process_name}] agent account is LocalSystem; inheriting supervisor token");
            exec_spawn(process_name, command, cmd)
        }
        AgentAccount::PasswordLogon {
            domain,
            user,
            password,
        } => spawn_with_impersonation(
            process_name,
            command,
            cmd,
            &domain,
            &user,
            Some(password.as_str()),
        ),
        AgentAccount::ServiceAccountLogon { domain, user } => {
            spawn_with_impersonation(process_name, command, cmd, &domain, &user, None)
        }
    }
}

fn spawn_with_impersonation(
    process_name: &str,
    command: &str,
    cmd: &mut Command,
    domain: &str,
    user: &str,
    password: Option<&str>,
) -> Result<ProcessHandle> {
    let domain_wide = wide::null_terminated(logon_domain(domain));
    let user_wide = wide::null_terminated(user);
    let password_wide = password.map(wide::null_terminated);

    unsafe {
        let mut logon_token = TokenHandle(ptr::null_mut());
        let ok = LogonUserW(
            user_wide.as_ptr(),
            domain_wide.as_ptr(),
            password_wide.as_ref().map_or(ptr::null(), |p| p.as_ptr()),
            LOGON32_LOGON_SERVICE,
            LOGON32_PROVIDER_DEFAULT,
            &mut logon_token.0,
        );
        if ok == 0 {
            bail!(
                "[{process_name}] LogonUserW failed: {}",
                std::io::Error::last_os_error()
            );
        }

        if ImpersonateLoggedOnUser(logon_token.0) == 0 {
            bail!(
                "[{process_name}] ImpersonateLoggedOnUser failed: {}",
                std::io::Error::last_os_error()
            );
        }

        let _impersonation = ImpersonationGuard {
            _token: logon_token,
        };
        exec_spawn(process_name, command, cmd)
    }
}

fn spawn_as_primary_token(
    process_name: &str,
    request: &SpawnRequest,
    account: &AgentAccount,
) -> Result<ProcessHandle> {
    // Only support stdio shapes that can be mapped to explicit Win32 handles.
    // For anything else, fall back to impersonation + tokio::Command.
    let stdout_handle = map_stdio_handle(&request.stdout, STD_OUTPUT_HANDLE)?;
    let stderr_handle = map_stdio_handle(&request.stderr, STD_ERROR_HANDLE)?;
    let stdin_handle = open_nul_handle(FILE_GENERIC_READ | FILE_GENERIC_WRITE)?;

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

    let env_block = env_block_from_current_plus_overrides(&request.env)?;
    let env_block_ptr = env_block.as_ptr() as *const std::ffi::c_void;

    let current_dir_w = request
        .working_dir
        .as_ref()
        .map(|d| wide::null_terminated(d.to_string_lossy().as_ref()));

    // Acquire a token for the configured account.
    let (domain, user, password) = primary_token_logon_credentials(account);
    let domain_w = wide::null_terminated(domain);
    let user_w = wide::null_terminated(user);
    let password_w = password.map(wide::null_terminated);

    let mut logon_token: HANDLE = std::ptr::null_mut();
    let ok = unsafe {
        LogonUserW(
            user_w.as_ptr(),
            domain_w.as_ptr(),
            password_w.as_ref().map_or(std::ptr::null(), |p| p.as_ptr()),
            LOGON32_LOGON_SERVICE,
            LOGON32_PROVIDER_DEFAULT,
            &mut logon_token,
        )
    };
    if ok == 0 {
        bail!(
            "[{process_name}] LogonUserW({domain}\\{user}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    let logon_token_guard = TokenHandle(logon_token);

    // Ensure we have a primary token suitable for CreateProcessAsUserW.
    let mut primary_token: HANDLE = std::ptr::null_mut();
    let ok = unsafe {
        DuplicateTokenEx(
            logon_token_guard.0,
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
    let primary_token_guard = TokenHandle(primary_token);

    let mut si: STARTUPINFOW = unsafe { std::mem::zeroed() };
    si.cb = std::mem::size_of::<STARTUPINFOW>() as u32;
    si.dwFlags = STARTF_USESTDHANDLES;
    si.hStdInput = stdin_handle;
    si.hStdOutput = stdout_handle;
    si.hStdError = stderr_handle;

    let mut pi: PROCESS_INFORMATION = unsafe { std::mem::zeroed() };
    let dw_creation_flags = CREATE_NEW_PROCESS_GROUP
        | CREATE_NEW_CONSOLE
        | CREATE_NO_WINDOW
        | CREATE_UNICODE_ENVIRONMENT;

    let ok = unsafe {
        CreateProcessAsUserW(
            primary_token_guard.0,
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

    // Close local stdio handles: the child has its own copies by the time
    // CreateProcessAsUserW returns.
    unsafe {
        let _ = CloseHandle(stdin_handle);
        let _ = CloseHandle(stdout_handle);
        let _ = CloseHandle(stderr_handle);
    }

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

fn primary_token_logon_credentials(account: &AgentAccount) -> (&str, &str, Option<&str>) {
    match account {
        AgentAccount::LocalSystem => ("NT AUTHORITY", "SYSTEM", Some("")),
        AgentAccount::PasswordLogon {
            domain,
            user,
            password,
        } => (domain.as_str(), user.as_str(), Some(password.as_str())),
        AgentAccount::ServiceAccountLogon { domain, user } => {
            (domain.as_str(), user.as_str(), None)
        }
    }
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

fn env_block_from_current_plus_overrides(overrides: &[(String, String)]) -> Result<Vec<u16>> {
    let mut vars: HashMap<String, String> = std::env::vars().collect();
    for (k, v) in overrides {
        vars.insert(k.clone(), v.clone());
    }

    let mut block: Vec<u16> = Vec::new();
    for (k, v) in vars {
        let kv = format!("{k}={v}");
        block.extend(std::ffi::OsStr::new(&kv).encode_wide());
        block.push(0);
    }
    // Double NUL terminator.
    block.push(0);
    Ok(block)
}

fn open_nul_handle(access: u32) -> Result<HANDLE> {
    let nul = wide::null_terminated("NUL");
    let h = unsafe {
        CreateFileW(
            nul.as_ptr(),
            access,
            FILE_SHARE_READ | FILE_SHARE_WRITE,
            std::ptr::null(),
            OPEN_EXISTING,
            FILE_ATTRIBUTE_NORMAL,
            0,
        )
    };
    if h == INVALID_HANDLE_VALUE || h.is_null() {
        bail!(
            "CreateFileW(NUL) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(h)
}

fn map_stdio_handle(stdio: &Stdio, kind: u32) -> Result<HANDLE> {
    match stdio {
        Stdio::Inherit => unsafe {
            let h = GetStdHandle(kind);
            if h == INVALID_HANDLE_VALUE || h.is_null() {
                bail!("GetStdHandle({kind}) returned invalid");
            }
            Ok(h)
        },
        Stdio::Null => open_nul_handle(FILE_GENERIC_READ | FILE_GENERIC_WRITE),
        other => bail!("primary-token spawn only supports inherit/null stdio, got {other:?}"),
    }
}

fn build_command(request: SpawnRequest) -> Result<(String, Command)> {
    let SpawnRequest {
        command,
        args,
        env,
        working_dir,
        stdout,
        stderr,
    } = request;

    let mut cmd = Command::new(&command);
    cmd.args(&args);
    // Ensure children don't see fleet installer environment.
    cmd.env_clear();
    apply_child_baseline_env(&mut cmd);
    for (k, v) in env {
        cmd.env(k, v);
    }
    if let Some(dir) = working_dir {
        cmd.current_dir(dir);
    }

    // Don't inherit stdin: invalid after AttachConsole/FreeConsole on stop.
    cmd.stdin(std::process::Stdio::null());
    cmd.stdout(stdout);
    cmd.stderr(stderr);

    Ok((command, cmd))
}

/// Local account logon expects `"."` when the registry domain is empty.
fn logon_domain(domain: &str) -> &str {
    if domain.is_empty() { "." } else { domain }
}

struct TokenHandle(HANDLE);

impl Drop for TokenHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}

struct ImpersonationGuard {
    _token: TokenHandle,
}

impl Drop for ImpersonationGuard {
    fn drop(&mut self) {
        unsafe {
            if RevertToSelf() == 0 {
                log::warn!(
                    "RevertToSelf failed after impersonated spawn: {}",
                    std::io::Error::last_os_error()
                );
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn logon_domain_uses_dot_for_local_accounts() {
        assert_eq!(logon_domain(""), ".");
        assert_eq!(logon_domain("WIN-HOST"), "WIN-HOST");
    }

    #[test]
    fn primary_token_logon_credentials_handle_passwordless_accounts() {
        let svc = AgentAccount::ServiceAccountLogon {
            domain: "CORP".to_string(),
            user: "gmsa$".to_string(),
        };
        let (domain, user, password) = primary_token_logon_credentials(&svc);
        assert_eq!(domain, "CORP");
        assert_eq!(user, "gmsa$");
        assert!(password.is_none());

        let ls = AgentAccount::LocalSystem;
        let (_domain, _user, password) = primary_token_logon_credentials(&ls);
        assert_eq!(password, Some(""));
    }
}
