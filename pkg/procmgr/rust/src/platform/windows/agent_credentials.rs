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
use std::ffi::OsStr;
use std::os::windows::ffi::OsStrExt;
use std::ptr;

use super::{open_datadog_agent_key, registry_nonempty_string};

/// Agent service account resolved from installer state.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum AgentAccount {
    /// Well-known LocalSystem account (spawn inherits supervisor when needed).
    LocalSystem,
    /// Interactive/service logon with a stored password (typical `ddagentuser`).
    PasswordLogon {
        domain: String,
        user: String,
        password: String,
    },
    /// gMSA or other service account logon without a stored password.
    ServiceAccountLogon { domain: String, user: String },
}

/// Resolve the agent service account from registry + LSA private data.
pub(crate) fn resolve_agent_account() -> Result<AgentAccount> {
    let key =
        open_datadog_agent_key().context("open HKLM\\SOFTWARE\\Datadog\\Datadog Agent")?;
    let user = registry_nonempty_string(&key, "installedUser")
        .context("read installedUser from registry")?;
    let domain = key
        .get_string("installedDomain")
        .unwrap_or_default()
        .trim()
        .to_string();

    if is_local_system_name(&domain, &user) {
        return Ok(AgentAccount::LocalSystem);
    }

    let sid = lookup_account_sid(&domain, &user).with_context(|| {
        format!("lookup SID for {domain}\\{user}")
    })?;
    if is_local_system_sid(&sid) {
        return Ok(AgentAccount::LocalSystem);
    }

    if let Some(password) = read_agent_password_from_lsa()? {
        if !password.is_empty() {
            return Ok(AgentAccount::PasswordLogon {
                domain,
                user,
                password,
            });
        }
    }

    Ok(AgentAccount::ServiceAccountLogon { domain, user })
}

fn is_local_system_name(domain: &str, user: &str) -> bool {
    user.eq_ignore_ascii_case("LocalSystem")
        || (domain.eq_ignore_ascii_case("NT AUTHORITY") && user.eq_ignore_ascii_case("SYSTEM"))
}

fn is_local_system_sid(sid: &[u8]) -> bool {
    unsafe {
        windows_sys::Win32::Security::IsWellKnownSid(
            sid.as_ptr() as *mut _,
            windows_sys::Win32::Security::WinLocalSystemSid,
        ) != 0
    }
}

fn lookup_account_sid(domain: &str, user: &str) -> Result<Vec<u8>> {
    let account = if domain.is_empty() {
        user.to_string()
    } else {
        format!("{domain}\\{user}")
    };
    let system_wide = wide_nullterminated("");
    let account_wide = wide_nullterminated(&account);

    unsafe {
        let mut sid_size = 0u32;
        let mut domain_size = 0u32;
        let mut sid_type = 0i32;

        let _ = windows_sys::Win32::Security::LookupAccountNameW(
            system_wide.as_ptr(),
            account_wide.as_ptr(),
            ptr::null_mut(),
            &mut sid_size,
            ptr::null_mut(),
            &mut domain_size,
            &mut sid_type,
        );

        let mut sid = vec![0u8; sid_size as usize];
        let mut domain_buf = vec![0u16; domain_size as usize];
        let ok = windows_sys::Win32::Security::LookupAccountNameW(
            system_wide.as_ptr(),
            account_wide.as_ptr(),
            sid.as_mut_ptr() as *mut _,
            &mut sid_size,
            domain_buf.as_mut_ptr(),
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

/// LSA secret name written by the MSI (`ConfigureUserCustomActions`).
fn agent_password_lsa_key() -> &'static str {
    "L$datadog_ddagentuser_password"
}

fn read_agent_password_from_lsa() -> Result<Option<String>> {
    const STATUS_OBJECT_NAME_NOT_FOUND: i32 = 0xC000_0034u32 as i32;

    let mut key_wide = wide_nullterminated(agent_password_lsa_key());
    let key_len = key_wide.len().saturating_sub(1);
    let mut key_name = windows_sys::Win32::Security::Authentication::Identity::LSA_UNICODE_STRING {
        Length: (key_len * 2) as u16,
        MaximumLength: (key_wide.len() * 2) as u16,
        Buffer: key_wide.as_mut_ptr(),
    };

    unsafe {
        let mut object_attributes: windows_sys::Win32::Security::Authentication::Identity::LSA_OBJECT_ATTRIBUTES =
            std::mem::zeroed();
        let mut policy_handle: windows_sys::Win32::Security::Authentication::Identity::LSA_HANDLE = 0;

        let status = windows_sys::Win32::Security::Authentication::Identity::LsaOpenPolicy(
            ptr::null(),
            &mut object_attributes,
            windows_sys::Win32::Security::Authentication::Identity::POLICY_GET_PRIVATE_INFORMATION
                as u32,
            &mut policy_handle,
        );
        if status != 0 {
            bail!(
                "LsaOpenPolicy: NTSTATUS {status:#010x}",
            );
        }

        let policy = PolicyHandle(policy_handle);
        let mut secret: *mut windows_sys::Win32::Security::Authentication::Identity::LSA_UNICODE_STRING =
            ptr::null_mut();
        let status =
            windows_sys::Win32::Security::Authentication::Identity::LsaRetrievePrivateData(
                policy.0,
                &mut key_name,
                &mut secret,
            );

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

        windows_sys::Win32::Security::Authentication::Identity::LsaFreeMemory(secret as _);
        Ok(Some(password))
    }
}

struct PolicyHandle(windows_sys::Win32::Security::Authentication::Identity::LSA_HANDLE);

impl Drop for PolicyHandle {
    fn drop(&mut self) {
        if self.0 != 0 {
            unsafe {
                windows_sys::Win32::Security::Authentication::Identity::LsaClose(self.0);
            }
        }
    }
}

fn wide_nullterminated(value: &str) -> Vec<u16> {
    OsStr::new(value).encode_wide().chain([0]).collect()
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
    fn lsa_key_matches_msi_secret_name() {
        assert_eq!(
            agent_password_lsa_key(),
            "L$datadog_ddagentuser_password"
        );
    }
}
