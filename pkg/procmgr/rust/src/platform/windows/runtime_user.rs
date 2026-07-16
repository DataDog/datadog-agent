// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use std::ptr;
use windows_sys::Win32::Foundation::CloseHandle;
use windows_sys::Win32::Security::{
    GetLengthSid, GetTokenInformation, LookupAccountSidW, TOKEN_QUERY, TOKEN_USER, TokenUser,
};
use windows_sys::Win32::System::Threading::{
    OpenProcess, OpenProcessToken, PROCESS_QUERY_LIMITED_INFORMATION,
};

use super::account_name::AccountName;
use super::agent_credentials::canonical_account_name_for_well_known_sid;
use super::local_account::is_local_account;
use super::wide;

/// Return `DOMAIN\user` for the primary token of `pid`, if readable.
pub(crate) fn runtime_user_for_pid(pid: u32) -> Option<String> {
    match lookup_runtime_user(pid) {
        Ok(user) => Some(user),
        Err(e) => {
            log::debug!("[pid={pid}] runtime user lookup failed: {e:#}");
            None
        }
    }
}

fn lookup_runtime_user(pid: u32) -> Result<String> {
    let process = open_process(pid)?;
    let token = open_process_token(&process)?;
    let sid = token_user_sid(&token)?;
    Ok(lookup_account_name(&sid)?.display())
}

fn open_process(pid: u32) -> Result<ProcessHandle> {
    unsafe {
        let handle = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid);
        if handle.is_null() {
            bail!("OpenProcess: {}", std::io::Error::last_os_error());
        }
        Ok(ProcessHandle(handle))
    }
}

fn open_process_token(process: &ProcessHandle) -> Result<TokenHandle> {
    unsafe {
        let mut token = ptr::null_mut();
        if OpenProcessToken(process.0, TOKEN_QUERY, &mut token) == 0 {
            bail!("OpenProcessToken: {}", std::io::Error::last_os_error());
        }
        Ok(TokenHandle(token))
    }
}

fn token_user_sid(token: &TokenHandle) -> Result<Vec<u8>> {
    unsafe {
        let mut needed = 0u32;
        let _ = GetTokenInformation(token.0, TokenUser, ptr::null_mut(), 0, &mut needed);
        if needed == 0 {
            bail!("GetTokenInformation size query failed");
        }

        let mut buffer = vec![0u8; needed as usize];
        if GetTokenInformation(
            token.0,
            TokenUser,
            buffer.as_mut_ptr() as *mut _,
            needed,
            &mut needed,
        ) == 0
        {
            bail!("GetTokenInformation: {}", std::io::Error::last_os_error());
        }

        // GetTokenInformation writes into an arbitrary byte buffer; read_unaligned
        // avoids UB from casting Vec<u8> to &TOKEN_USER.
        let token_user = ptr::read_unaligned(buffer.as_ptr().cast::<TOKEN_USER>());
        let sid_ptr = token_user.User.Sid;
        if sid_ptr.is_null() {
            bail!("TokenUser SID is null");
        }

        let sid_len = GetLengthSid(sid_ptr);
        if sid_len == 0 {
            bail!("GetLengthSid returned 0");
        }
        let mut sid = vec![0u8; sid_len as usize];
        std::ptr::copy_nonoverlapping(sid_ptr as *const u8, sid.as_mut_ptr(), sid_len as usize);
        Ok(sid)
    }
}

fn lookup_account_name(sid: &[u8]) -> Result<AccountName> {
    unsafe {
        let sid_ptr = sid.as_ptr() as *mut std::ffi::c_void;
        let mut name_size = 0u32;
        let mut domain_size = 0u32;
        let mut sid_type = 0i32;
        let _ = LookupAccountSidW(
            ptr::null(),
            sid_ptr,
            ptr::null_mut(),
            &mut name_size,
            ptr::null_mut(),
            &mut domain_size,
            &mut sid_type,
        );

        let mut name = vec![0u16; name_size as usize];
        let mut domain = vec![0u16; domain_size as usize];
        if LookupAccountSidW(
            ptr::null(),
            sid_ptr,
            name.as_mut_ptr(),
            &mut name_size,
            domain.as_mut_ptr(),
            &mut domain_size,
            &mut sid_type,
        ) == 0
        {
            bail!("LookupAccountSidW: {}", std::io::Error::last_os_error());
        }

        name.truncate(name_size as usize);
        domain.truncate(domain_size as usize);
        let user = wide::from_ptr(name.as_ptr());
        let domain = wide::from_ptr(domain.as_ptr());
        account_name_from_sid_lookup(sid, domain, user)
    }
}

/// Match registry-backed `AccountName` display for local SAM accounts.
///
/// `LookupAccountSidW` returns the computer name as the domain for local users;
/// installer state stores an empty domain and displays `.\user` instead.
fn account_name_from_sid_lookup(sid: &[u8], domain: String, user: String) -> Result<AccountName> {
    if let Some(account) = canonical_account_name_for_well_known_sid(sid) {
        return Ok(account);
    }
    let domain = if is_local_account(sid).unwrap_or(false) {
        String::new()
    } else {
        domain
    };
    Ok(AccountName::new(domain, user))
}

struct ProcessHandle(windows_sys::Win32::Foundation::HANDLE);

impl Drop for ProcessHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}

struct TokenHandle(windows_sys::Win32::Foundation::HANDLE);

impl Drop for TokenHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::super::account_name::AccountName;
    use super::super::sid::lookup_account_sid;
    use super::*;

    #[test]
    fn local_account_lookup_normalizes_computer_domain() {
        // Use a built-in local SAM account so this stays deterministic on
        // domain-joined CI hosts (where USERNAME is a domain principal).
        let username = "Administrator";
        let computer_domain = std::env::var("COMPUTERNAME").expect("COMPUTERNAME");
        let sid = lookup_account_sid(&computer_domain, username)
            .or_else(|_| lookup_account_sid("", username))
            .expect("Administrator SID");

        let account = account_name_from_sid_lookup(&sid, computer_domain, username.to_string())
            .expect("account");
        assert_eq!(
            account.display(),
            AccountName::new("", username).display(),
            "runtime lookup should match registry-style local account display"
        );
    }

    #[test]
    fn well_known_account_lookup_keeps_nt_authority_domain() {
        let sid = lookup_account_sid("NT AUTHORITY", "SYSTEM").expect("SYSTEM SID");
        let account =
            account_name_from_sid_lookup(&sid, "NT AUTHORITY".to_string(), "SYSTEM".to_string())
                .expect("account");
        assert_eq!(account.display(), r"NT AUTHORITY\SYSTEM");
    }

    #[test]
    fn well_known_service_account_lookup_normalizes_spaced_names() {
        let sid = lookup_account_sid("NT AUTHORITY", "LOCAL SERVICE").expect("LocalService SID");
        let account = account_name_from_sid_lookup(
            &sid,
            "NT AUTHORITY".to_string(),
            "LOCAL SERVICE".to_string(),
        )
        .expect("account");
        assert_eq!(account.display(), r"NT AUTHORITY\LocalService");

        let sid =
            lookup_account_sid("NT AUTHORITY", "NETWORK SERVICE").expect("NetworkService SID");
        let account = account_name_from_sid_lookup(
            &sid,
            "NT AUTHORITY".to_string(),
            "NETWORK SERVICE".to_string(),
        )
        .expect("account");
        assert_eq!(account.display(), r"NT AUTHORITY\NetworkService");
    }
}
