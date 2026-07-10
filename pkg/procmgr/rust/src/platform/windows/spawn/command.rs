// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
use log::info;
use tokio::process::Command;
use windows_sys::Win32::Security::ImpersonateLoggedOnUser;

use crate::handle::ProcessHandle;
use crate::spawn_context;
use crate::spawn_request::SpawnRequest;

use super::super::agent_credentials::{AgentAccount, resolve_agent_account};
use super::super::{apply_child_baseline_env, setup_process_group};
use super::logon::{
    ImpersonationGuard, LogonUserCredentials, TokenHandle, logon_user_credentials, logon_user_token,
};

pub(super) fn build_command(request: SpawnRequest) -> Result<(String, Command)> {
    let SpawnRequest {
        command,
        args,
        env,
        working_dir,
        stdout_config: _,
        stderr_config: _,
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

pub(super) fn spawn_as_local_system(
    process_name: &str,
    command: &str,
    cmd: &mut Command,
) -> Result<ProcessHandle> {
    // dd-procmgr-service runs as LocalSystem; privileged children inherit SYSTEM.
    exec_spawn(process_name, command, cmd)
}

pub(super) fn spawn_as_agent_user(
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
        _ => {
            let creds = logon_user_credentials(&account);
            spawn_with_impersonation(process_name, command, cmd, &creds)
        }
    }
}

fn exec_spawn(process_name: &str, command: &str, cmd: &mut Command) -> Result<ProcessHandle> {
    setup_process_group(cmd);
    let child = cmd
        .spawn()
        .with_context(|| spawn_context::failed_message(process_name, command))?;
    Ok(ProcessHandle::from_child(child))
}

fn spawn_with_token_impersonation(
    process_name: &str,
    command: &str,
    cmd: &mut Command,
    token: TokenHandle,
) -> Result<ProcessHandle> {
    unsafe {
        if ImpersonateLoggedOnUser(token.raw()) == 0 {
            bail!(
                "[{process_name}] ImpersonateLoggedOnUser failed: {}",
                std::io::Error::last_os_error()
            );
        }

        let _impersonation = ImpersonationGuard::new(token);
        exec_spawn(process_name, command, cmd)
    }
}

fn spawn_with_impersonation(
    process_name: &str,
    command: &str,
    cmd: &mut Command,
    creds: &LogonUserCredentials<'_>,
) -> Result<ProcessHandle> {
    let logon_token = logon_user_token(process_name, creds)?;
    spawn_with_token_impersonation(process_name, command, cmd, logon_token)
}
