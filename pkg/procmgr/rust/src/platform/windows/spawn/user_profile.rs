// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Load a service-account user profile so `CreateEnvironmentBlock` includes profile env vars.

use anyhow::{Context, Result, bail};
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::UI::Shell::{LoadUserProfileW, PROFILEINFOW, UnloadUserProfile};

use super::super::agent_credentials::AgentAccount;
use super::super::local_account::computer_name;
use super::super::wide;

/// Keeps a user profile loaded for the lifetime of the guard.
pub(crate) struct UserProfileGuard {
    token: HANDLE,
    profile_handle: HANDLE,
    _username_wide: Vec<u16>,
}

impl UserProfileGuard {
    pub(super) fn load(process_name: &str, token: HANDLE, account: &AgentAccount) -> Result<Self> {
        if account.inherits_supervisor_token() {
            bail!("[{process_name}] internal error: LocalSystem profile load not required");
        }

        let username = profile_load_user_name(account)
            .with_context(|| format!("[{process_name}] profile user name for LoadUserProfileW"))?;
        let mut username_wide = wide::null_terminated(&username);

        let mut profile_info: PROFILEINFOW = unsafe { std::mem::zeroed() };
        profile_info.dwSize = std::mem::size_of::<PROFILEINFOW>() as u32;
        profile_info.lpUserName = username_wide.as_mut_ptr();

        let ok = unsafe { LoadUserProfileW(token, &mut profile_info) };
        if ok == 0 {
            bail!(
                "[{process_name}] LoadUserProfileW({username}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        if profile_info.hProfile.is_null() {
            bail!("[{process_name}] LoadUserProfileW({username}) returned a null profile handle");
        }

        Ok(Self {
            token,
            profile_handle: profile_info.hProfile,
            _username_wide: username_wide,
        })
    }
}

impl Drop for UserProfileGuard {
    fn drop(&mut self) {
        if self.profile_handle.is_null() {
            return;
        }
        let ok = unsafe { UnloadUserProfile(self.token, self.profile_handle) };
        if ok == 0 {
            log::warn!(
                "UnloadUserProfile failed: {}",
                std::io::Error::last_os_error()
            );
        }
    }
}

fn profile_load_user_name(account: &AgentAccount) -> Result<String> {
    match account {
        AgentAccount::LocalSystem => {
            bail!("LocalSystem does not require LoadUserProfileW");
        }
        AgentAccount::LocalService => Ok(r"NT AUTHORITY\LocalService".to_string()),
        AgentAccount::NetworkService => Ok(r"NT AUTHORITY\NetworkService".to_string()),
        AgentAccount::PasswordLogon { domain, user, .. }
        | AgentAccount::ServiceAccountLogon { domain, user } => {
            let computer = if domain.is_empty() {
                computer_name()?
            } else {
                domain.clone()
            };
            Ok(profile_load_user_name_from_parts(&computer, user))
        }
    }
}

fn profile_load_user_name_from_parts(domain: &str, user: &str) -> String {
    format!(r"{domain}\{user}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn profile_load_user_name_from_parts_formats_domain_user() {
        assert_eq!(
            profile_load_user_name_from_parts("CORP", "ddagentuser"),
            r"CORP\ddagentuser"
        );
        assert_eq!(
            profile_load_user_name_from_parts("WIN-HOST", "ddagentuser"),
            r"WIN-HOST\ddagentuser"
        );
    }

    #[test]
    fn profile_load_user_name_maps_well_known_service_accounts() {
        let gmsa = AgentAccount::ServiceAccountLogon {
            domain: "CORP".to_string(),
            user: "gmsa$".to_string(),
        };
        assert_eq!(profile_load_user_name(&gmsa).unwrap(), r"CORP\gmsa$");

        let local_service = AgentAccount::LocalService;
        assert_eq!(
            profile_load_user_name(&local_service).unwrap(),
            r"NT AUTHORITY\LocalService"
        );
    }
}
