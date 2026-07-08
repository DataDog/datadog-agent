// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows child spawning by spawn profile (inherit vs agent-user impersonation).

use anyhow::{Context, Result, bail};
use log::info;
use std::ptr;
use tokio::process::{Child, Command};
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::Security::{
    ImpersonateLoggedOnUser, LogonUserW, RevertToSelf, LOGON32_LOGON_SERVICE,
    LOGON32_PROVIDER_DEFAULT,
};

use crate::spawn_context;
use crate::spawn_profile::SpawnProfile;

use super::agent_credentials::{AgentAccount, resolve_agent_account};
use super::setup_process_group;
use super::wide;

/// Spawn a managed child using the platform spawn profile for `process_name`.
///
/// Caller must hold [`super::console_lock`] on Windows (see `ManagedProcess::try_spawn`).
pub(crate) fn spawn_child(
    process_name: &str,
    command: &str,
    profile: SpawnProfile,
    cmd: &mut Command,
) -> Result<Child> {
    info!("[{process_name}] spawn profile: {profile}");
    match profile {
        // Preserve legacy `datadog-process-agent` (LocalSystem) behavior by explicitly
        // impersonating LocalSystem for the legacy SCM privilege level.
        SpawnProfile::Privileged => spawn_as_local_system(process_name, command, cmd).or_else(|e| {
            log::warn!(
                "[{process_name}] failed to spawn as LocalSystem (falling back to inherited token): {e:#}"
            );
            exec_spawn(process_name, command, cmd)
        }),
        SpawnProfile::Agent => spawn_as_agent_user(process_name, command, cmd),
    }
}

fn exec_spawn(process_name: &str, command: &str, cmd: &mut Command) -> Result<Child> {
    setup_process_group(cmd);
    cmd.spawn()
        .with_context(|| spawn_context::failed_message(process_name, command))
}

fn spawn_as_local_system(process_name: &str, command: &str, cmd: &mut Command) -> Result<Child> {
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

fn spawn_as_agent_user(process_name: &str, command: &str, cmd: &mut Command) -> Result<Child> {
    let account = resolve_agent_account().with_context(|| {
        format!("[{process_name}] resolve agent service account for spawn")
    })?;

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
        AgentAccount::ServiceAccountLogon { domain, user } => spawn_with_impersonation(
            process_name,
            command,
            cmd,
            &domain,
            &user,
            None,
        ),
    }
}

fn spawn_with_impersonation(
    process_name: &str,
    command: &str,
    cmd: &mut Command,
    domain: &str,
    user: &str,
    password: Option<&str>,
) -> Result<Child> {
    let domain_wide = wide::null_terminated(logon_domain(domain));
    let user_wide = wide::null_terminated(user);
    let password_wide = password.map(wide::null_terminated);

    unsafe {
        let mut logon_token = TokenHandle(ptr::null_mut());
        let ok = LogonUserW(
            user_wide.as_ptr(),
            domain_wide.as_ptr(),
            password_wide
                .as_ref()
                .map_or(ptr::null(), |p| p.as_ptr()),
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
}
