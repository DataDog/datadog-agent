// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ptr;

use anyhow::Result;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::Security::{
    GetLengthSid, GetTokenInformation, LookupAccountSidW, TOKEN_USER, TokenUser,
};
use windows_sys::Win32::System::Threading::{
    OpenProcess, OpenProcessToken, PROCESS_QUERY_LIMITED_INFORMATION,
};

use super::wide;

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
    let process = open_process_for_query(pid)?;
    let token = open_process_token(&process)?;
    let mut sid = token_user_sid(&token)?;
    lookup_account_display(&mut sid)
}

fn open_process_for_query(pid: u32) -> Result<QueryProcessHandle> {
    unsafe {
        let handle = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid);
        if handle.is_null() {
            anyhow::bail!("OpenProcess: {}", std::io::Error::last_os_error());
        }
        Ok(QueryProcessHandle(handle))
    }
}

fn open_process_token(process: &QueryProcessHandle) -> Result<QueryTokenHandle> {
    unsafe {
        let mut token = std::ptr::null_mut();
        if OpenProcessToken(
            process.0,
            windows_sys::Win32::Security::TOKEN_QUERY,
            &mut token,
        ) == 0
        {
            anyhow::bail!("OpenProcessToken: {}", std::io::Error::last_os_error());
        }
        Ok(QueryTokenHandle(token))
    }
}

fn token_user_sid(token: &QueryTokenHandle) -> Result<Vec<u8>> {
    unsafe {
        let mut needed = 0u32;
        let _ = GetTokenInformation(token.0, TokenUser, ptr::null_mut(), 0, &mut needed);
        if needed == 0 {
            anyhow::bail!("GetTokenInformation size query failed");
        }

        let mut buffer = vec![0u8; needed as usize];
        if GetTokenInformation(
            token.0,
            TokenUser,
            buffer.as_mut_ptr().cast(),
            needed,
            &mut needed,
        ) == 0
        {
            anyhow::bail!("GetTokenInformation: {}", std::io::Error::last_os_error());
        }

        let token_user = ptr::read_unaligned(buffer.as_ptr().cast::<TOKEN_USER>());
        let sid_ptr = token_user.User.Sid;
        if sid_ptr.is_null() {
            anyhow::bail!("TokenUser SID is null");
        }

        let sid_len = GetLengthSid(sid_ptr);
        if sid_len == 0 {
            anyhow::bail!("GetLengthSid returned 0");
        }
        let mut sid = vec![0u8; sid_len as usize];
        std::ptr::copy_nonoverlapping(sid_ptr.cast(), sid.as_mut_ptr(), sid_len as usize);
        Ok(sid)
    }
}

fn lookup_account_display(sid: &mut [u8]) -> Result<String> {
    unsafe {
        let sid_ptr = sid.as_mut_ptr().cast();
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
            anyhow::bail!("LookupAccountSidW: {}", std::io::Error::last_os_error());
        }

        let user = wide::trim_wide_nul(&name);
        let domain = wide::trim_wide_nul(&domain);
        if domain.is_empty() {
            Ok(user)
        } else {
            Ok(format!("{domain}\\{user}"))
        }
    }
}

struct QueryProcessHandle(HANDLE);

impl Drop for QueryProcessHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}

struct QueryTokenHandle(HANDLE);

impl Drop for QueryTokenHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}
