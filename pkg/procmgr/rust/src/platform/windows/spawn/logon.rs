// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use std::ptr;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::Security::{
    ImpersonateLoggedOnUser, LOGON32_LOGON_SERVICE, LOGON32_PROVIDER_DEFAULT, LogonUserW,
    RevertToSelf,
};

use super::super::local_agent_account::AgentAccount;
use super::super::wide;

pub(super) struct LogonUserCredentials<'a> {
    domain: &'a str,
    username: &'a str,
    password: Option<&'a str>,
}

pub(crate) fn logon_user_credentials(account: &AgentAccount) -> LogonUserCredentials<'_> {
    match account {
        AgentAccount::LocalSystem => LogonUserCredentials {
            domain: "NT AUTHORITY",
            username: "SYSTEM",
            password: Some(""),
        },
        AgentAccount::LocalService => LogonUserCredentials {
            domain: "NT AUTHORITY",
            username: "LocalService",
            password: None,
        },
        AgentAccount::NetworkService => LogonUserCredentials {
            domain: "NT AUTHORITY",
            username: "NetworkService",
            password: None,
        },
        AgentAccount::PasswordLogon {
            domain,
            user,
            password,
        } => LogonUserCredentials {
            domain: domain.as_str(),
            username: user.as_str(),
            password: Some(password.as_str()),
        },
    }
}

pub(crate) fn logon_user_token(
    process_name: &str,
    creds: &LogonUserCredentials<'_>,
) -> Result<TokenHandle> {
    let domain_w = wide::null_terminated(logon_domain(creds.domain));
    let user_w = wide::null_terminated(creds.username);
    let password_w = creds.password.map(wide::null_terminated);

    let mut logon_token: HANDLE = ptr::null_mut();
    let ok = unsafe {
        LogonUserW(
            user_w.as_ptr(),
            domain_w.as_ptr(),
            password_w.as_ref().map_or(ptr::null(), |p| p.as_ptr()),
            LOGON32_LOGON_SERVICE,
            LOGON32_PROVIDER_DEFAULT,
            &mut logon_token,
        )
    };
    if ok == 0 {
        bail!(
            "[{process_name}] LogonUserW({}\\{}) failed: {}",
            logon_domain(creds.domain),
            creds.username,
            std::io::Error::last_os_error()
        );
    }
    Ok(TokenHandle::new(logon_token))
}

fn logon_domain(domain: &str) -> &str {
    if domain.is_empty() { "." } else { domain }
}

pub(crate) struct TokenHandle(HANDLE);

unsafe impl Send for TokenHandle {}
unsafe impl Sync for TokenHandle {}

impl TokenHandle {
    pub(crate) fn new(handle: HANDLE) -> Self {
        Self(handle)
    }

    pub(crate) fn raw(&self) -> HANDLE {
        self.0
    }
}

impl Drop for TokenHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}

pub(super) fn with_impersonated_token<T>(
    process_name: &str,
    token: HANDLE,
    f: impl FnOnce() -> Result<T>,
) -> Result<T> {
    unsafe {
        if ImpersonateLoggedOnUser(token) == 0 {
            bail!(
                "[{process_name}] ImpersonateLoggedOnUser failed: {}",
                std::io::Error::last_os_error()
            );
        }
    }

    struct RevertGuard;
    impl Drop for RevertGuard {
        fn drop(&mut self) {
            unsafe {
                if RevertToSelf() == 0 {
                    log::warn!(
                        "RevertToSelf failed after impersonation: {}",
                        std::io::Error::last_os_error()
                    );
                }
            }
        }
    }

    let _revert = RevertGuard;
    f()
}

#[cfg(test)]
mod tests {
    use super::super::super::local_agent_account::AgentAccount;
    use super::*;

    #[test]
    fn logon_domain_uses_dot_for_local_accounts() {
        assert_eq!(logon_domain(""), ".");
        assert_eq!(logon_domain("WIN-HOST"), "WIN-HOST");
    }

    #[test]
    fn logon_user_credentials_normalize_empty_domain_for_logon() {
        let acct = AgentAccount::PasswordLogon {
            domain: String::new(),
            user: "ddagentuser".to_string(),
            password: "secret".to_string(),
        };
        let creds = logon_user_credentials(&acct);
        assert_eq!(logon_domain(creds.domain), ".");
        assert_eq!(creds.username, "ddagentuser");
        assert_eq!(creds.password, Some("secret"));
    }

    #[test]
    fn logon_user_credentials_map_account_kinds() {
        let ls = AgentAccount::LocalSystem;
        let creds = logon_user_credentials(&ls);
        assert_eq!(creds.password, Some(""));

        let local_service = AgentAccount::LocalService;
        let creds = logon_user_credentials(&local_service);
        assert_eq!(creds.domain, "NT AUTHORITY");
        assert_eq!(creds.username, "LocalService");
        assert!(creds.password.is_none());

        let network_service = AgentAccount::NetworkService;
        let creds = logon_user_credentials(&network_service);
        assert_eq!(creds.username, "NetworkService");
        assert!(creds.password.is_none());
    }
}
