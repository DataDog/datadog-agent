// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Resolve the Datadog Agent Windows service account for child process spawning.
//!
//! Reads `installedDomain` / `installedUser` from the Agent MSI registry state and,
//! when needed, the agent-user password stored in LSA by the installer. Mirrors
//! `pkg/fleet/installer/packages/user/windows` and `pkg/util/filesystem/rights_windows.go`.

use anyhow::{Context, Result, bail};
use std::ptr;
use windows_sys::Win32::Security::Authentication::Identity::{
    LSA_HANDLE, LSA_OBJECT_ATTRIBUTES, LSA_UNICODE_STRING, LsaClose, LsaFreeMemory, LsaOpenPolicy,
    LsaRetrievePrivateData, POLICY_GET_PRIVATE_INFORMATION,
};
use windows_sys::Win32::Security::{
    IsWellKnownSid, LookupAccountNameW, WinLocalServiceSid, WinLocalSystemSid, WinNetworkServiceSid,
};

use super::wide;
use super::{open_datadog_agent_key, registry_nonempty_string};

const AGENT_PASSWORD_LSA_KEY: &str = "L$datadog_ddagentuser_password";
const STATUS_OBJECT_NAME_NOT_FOUND: i32 = 0xC000_0034u32 as i32;

/// Agent service account resolved from installer state.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum AgentAccount {
    /// Well-known LocalSystem account (spawn inherits supervisor when procmgr runs as SYSTEM).
    LocalSystem,
    /// Well-known LocalService account (empty-password service logon).
    LocalService,
    /// Well-known NetworkService account (empty-password service logon).
    NetworkService,
    /// Interactive/service logon with a stored password (typical `ddagentuser`).
    PasswordLogon {
        domain: String,
        user: String,
        password: String,
    },
    /// gMSA or other service account logon without a stored password.
    ServiceAccountLogon { domain: String, user: String },
}

impl AgentAccount {
    /// True when agent-profile children may inherit the LocalSystem procmgr supervisor token.
    pub(crate) fn inherits_supervisor_token(&self) -> bool {
        matches!(self, AgentAccount::LocalSystem)
    }
}

/// Resolve the agent service account from registry + LSA private data.
pub(crate) fn resolve_agent_account() -> Result<AgentAccount> {
    let key = open_datadog_agent_key().context("open HKLM\\SOFTWARE\\Datadog\\Datadog Agent")?;
    let user = registry_nonempty_string(&key, "installedUser")
        .context("read installedUser from registry")?;
    let domain = key
        .get_string("installedDomain")
        .unwrap_or_default()
        .trim()
        .to_string();

    if let Some(account) = well_known_from_names(&domain, &user) {
        return Ok(account);
    }

    let sid = lookup_account_sid(&domain, &user)
        .with_context(|| format!("lookup SID for {domain}\\{user}"))?;
    if let Some(account) = well_known_from_sid(&sid) {
        return Ok(account);
    }

    match read_agent_password_from_lsa()? {
        Some(password) if !password.is_empty() => Ok(AgentAccount::PasswordLogon {
            domain,
            user,
            password,
        }),
        _ => Ok(AgentAccount::ServiceAccountLogon { domain, user }),
    }
}

fn well_known_from_names(domain: &str, user: &str) -> Option<AgentAccount> {
    if is_local_system_name(domain, user) {
        Some(AgentAccount::LocalSystem)
    } else if is_local_service_name(domain, user) {
        Some(AgentAccount::LocalService)
    } else if is_network_service_name(domain, user) {
        Some(AgentAccount::NetworkService)
    } else {
        None
    }
}

fn well_known_from_sid(sid: &[u8]) -> Option<AgentAccount> {
    if is_local_system_sid(sid) {
        Some(AgentAccount::LocalSystem)
    } else if is_local_service_sid(sid) {
        Some(AgentAccount::LocalService)
    } else if is_network_service_sid(sid) {
        Some(AgentAccount::NetworkService)
    } else {
        None
    }
}

fn is_local_system_name(domain: &str, user: &str) -> bool {
    user.eq_ignore_ascii_case("LocalSystem")
        || (domain.eq_ignore_ascii_case("NT AUTHORITY") && user.eq_ignore_ascii_case("SYSTEM"))
}

fn is_local_service_name(domain: &str, user: &str) -> bool {
    user.eq_ignore_ascii_case("LocalService")
        || (domain.eq_ignore_ascii_case("NT AUTHORITY")
            && user.eq_ignore_ascii_case("LOCAL SERVICE"))
}

fn is_network_service_name(domain: &str, user: &str) -> bool {
    user.eq_ignore_ascii_case("NetworkService")
        || (domain.eq_ignore_ascii_case("NT AUTHORITY")
            && user.eq_ignore_ascii_case("NETWORK SERVICE"))
}

fn is_local_system_sid(sid: &[u8]) -> bool {
    is_well_known_sid(sid, WinLocalSystemSid)
}

fn is_local_service_sid(sid: &[u8]) -> bool {
    is_well_known_sid(sid, WinLocalServiceSid)
}

fn is_network_service_sid(sid: &[u8]) -> bool {
    is_well_known_sid(sid, WinNetworkServiceSid)
}

fn is_well_known_sid(
    sid: &[u8],
    well_known: windows_sys::Win32::Security::WELL_KNOWN_SID_TYPE,
) -> bool {
    unsafe { IsWellKnownSid(sid.as_ptr() as *mut _, well_known) != 0 }
}

fn lookup_account_sid(domain: &str, user: &str) -> Result<Vec<u8>> {
    let account = if domain.is_empty() {
        user.to_string()
    } else {
        format!("{domain}\\{user}")
    };
    let system_w = wide::null_terminated("");
    let account_w = wide::null_terminated(&account);

    unsafe {
        let mut sid_size = 0u32;
        let mut domain_size = 0u32;
        let mut sid_type = 0i32;

        let _ = LookupAccountNameW(
            system_w.as_ptr(),
            account_w.as_ptr(),
            ptr::null_mut(),
            &mut sid_size,
            ptr::null_mut(),
            &mut domain_size,
            &mut sid_type,
        );

        let mut sid = vec![0u8; sid_size as usize];
        let mut _domain_buf = vec![0u16; domain_size as usize];
        let ok = LookupAccountNameW(
            system_w.as_ptr(),
            account_w.as_ptr(),
            sid.as_mut_ptr() as *mut _,
            &mut sid_size,
            _domain_buf.as_mut_ptr(),
            &mut domain_size,
            &mut sid_type,
        );
        if ok == 0 {
            bail!(
                "LookupAccountNameW({account}): {}",
                std::io::Error::last_os_error()
            );
        }
        sid.truncate(sid_size as usize);
        Ok(sid)
    }
}

fn read_agent_password_from_lsa() -> Result<Option<String>> {
    let mut key_w = wide::null_terminated(AGENT_PASSWORD_LSA_KEY);
    let key_len = key_w.len().saturating_sub(1);
    let mut key_name = LSA_UNICODE_STRING {
        Length: (key_len * 2) as u16,
        MaximumLength: (key_w.len() * 2) as u16,
        Buffer: key_w.as_mut_ptr(),
    };

    unsafe {
        let mut object_attributes: LSA_OBJECT_ATTRIBUTES = std::mem::zeroed();
        let mut policy_handle: LSA_HANDLE = 0;

        let status = LsaOpenPolicy(
            ptr::null(),
            &mut object_attributes,
            POLICY_GET_PRIVATE_INFORMATION as u32,
            &mut policy_handle,
        );
        if status != 0 {
            bail!("LsaOpenPolicy: NTSTATUS {status:#010x}");
        }

        let policy = PolicyHandle(policy_handle);
        let mut secret: *mut LSA_UNICODE_STRING = ptr::null_mut();
        let status = LsaRetrievePrivateData(policy.0, &mut key_name, &mut secret);

        if status == STATUS_OBJECT_NAME_NOT_FOUND {
            return Ok(None);
        }
        if status != 0 {
            bail!("LsaRetrievePrivateData: NTSTATUS {status:#010x}");
        }
        if secret.is_null() {
            return Ok(None);
        }

        let secret_ref = &*secret;
        let char_count = secret_ref.Length as usize / 2;
        let password = if char_count == 0 {
            String::new()
        } else {
            let slice = std::slice::from_raw_parts(secret_ref.Buffer, char_count);
            String::from_utf16_lossy(slice)
        };

        LsaFreeMemory(secret as _);
        Ok(Some(password))
    }
}

struct PolicyHandle(LSA_HANDLE);

impl Drop for PolicyHandle {
    fn drop(&mut self) {
        if self.0 != 0 {
            unsafe {
                LsaClose(self.0);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn local_system_name_detection() {
        assert!(is_local_system_name("", "LocalSystem"));
        assert!(is_local_system_name("NT AUTHORITY", "SYSTEM"));
        assert!(!is_local_system_name("WIN-HOST", "ddagentuser"));
    }

    #[test]
    fn builtin_service_name_detection() {
        assert!(is_local_service_name("", "LocalService"));
        assert!(is_local_service_name("NT AUTHORITY", "LOCAL SERVICE"));
        assert!(is_network_service_name("", "NetworkService"));
        assert!(is_network_service_name("NT AUTHORITY", "NETWORK SERVICE"));
        assert!(!is_local_service_name("WIN-HOST", "ddagentuser"));
    }

    #[test]
    fn well_known_from_names_maps_builtin_accounts() {
        assert_eq!(
            well_known_from_names("", "LocalService"),
            Some(AgentAccount::LocalService)
        );
        assert_eq!(
            well_known_from_names("", "NetworkService"),
            Some(AgentAccount::NetworkService)
        );
        assert_eq!(well_known_from_names("CORP", "gmsa$"), None);
    }
}
