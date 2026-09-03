// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ptr;
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::Security::{
    GetLengthSid, GetTokenInformation, IsWellKnownSid, TOKEN_USER, TokenUser, WinLocalSystemSid,
};
use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};

use super::win_handle::WinHandle;

#[cfg(test)]
use windows_sys::Win32::Security::LookupAccountSidW;

#[cfg(test)]
use super::account_name::AccountName;
#[cfg(test)]
use super::local_account::is_local_account;
#[cfg(test)]
use super::wide;

pub(crate) fn token_user_is_local_system(token: HANDLE) -> std::io::Result<bool> {
    if token.is_null() {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidInput,
            "null token handle",
        ));
    }

    let mut size = 0u32;
    let _ = unsafe { GetTokenInformation(token, TokenUser, ptr::null_mut(), 0, &mut size) };
    if size == 0 {
        return Err(std::io::Error::last_os_error());
    }

    let mut buffer = vec![0u8; size as usize];
    let ok = unsafe {
        GetTokenInformation(
            token,
            TokenUser,
            buffer.as_mut_ptr().cast(),
            size,
            &mut size,
        )
    };
    if ok == 0 {
        return Err(std::io::Error::last_os_error());
    }

    let token_user = unsafe { ptr::read_unaligned(buffer.as_ptr().cast::<TOKEN_USER>()) };
    let sid = token_user.User.Sid;
    if sid.is_null() {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            "TokenUser SID is null",
        ));
    }

    Ok(unsafe { IsWellKnownSid(sid, WinLocalSystemSid) != 0 })
}

#[cfg(test)]
pub(crate) fn current_process_account_display() -> std::io::Result<String> {
    let token = open_current_process_token(windows_sys::Win32::Security::TOKEN_QUERY)?;
    let sid = token_user_sid_bytes(token.as_handle())?;
    Ok(lookup_account_display(&sid)?.display())
}

pub(crate) fn token_user_sid_bytes(token: HANDLE) -> std::io::Result<Vec<u8>> {
    if token.is_null() {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidInput,
            "null token handle",
        ));
    }

    let mut size = 0u32;
    let _ = unsafe { GetTokenInformation(token, TokenUser, ptr::null_mut(), 0, &mut size) };
    if size == 0 {
        return Err(std::io::Error::last_os_error());
    }

    let mut buffer = vec![0u8; size as usize];
    let ok = unsafe {
        GetTokenInformation(
            token,
            TokenUser,
            buffer.as_mut_ptr().cast(),
            size,
            &mut size,
        )
    };
    if ok == 0 {
        return Err(std::io::Error::last_os_error());
    }

    let token_user = unsafe { ptr::read_unaligned(buffer.as_ptr().cast::<TOKEN_USER>()) };
    let sid_ptr = token_user.User.Sid;
    if sid_ptr.is_null() {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            "TokenUser SID is null",
        ));
    }

    let sid_len = unsafe { GetLengthSid(sid_ptr) };
    if sid_len == 0 {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            "GetLengthSid returned 0",
        ));
    }
    let mut sid = vec![0u8; sid_len as usize];
    unsafe {
        std::ptr::copy_nonoverlapping(sid_ptr as *const u8, sid.as_mut_ptr(), sid_len as usize);
    }
    Ok(sid)
}

#[cfg(test)]
fn lookup_account_display(sid: &[u8]) -> std::io::Result<AccountName> {
    unsafe {
        let sid_ptr = sid.as_ptr().cast_mut().cast();
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
            return Err(std::io::Error::last_os_error());
        }

        name.truncate(name_size as usize);
        domain.truncate(domain_size as usize);
        let user = wide::from_slice(&name);
        let domain = wide::from_slice(&domain);
        let domain = if is_local_account(sid).unwrap_or(false) {
            String::new()
        } else {
            domain
        };
        Ok(AccountName::new(domain, user))
    }
}

pub(crate) fn open_current_process_token(access: u32) -> std::io::Result<WinHandle> {
    let mut token: HANDLE = ptr::null_mut();
    let ok = unsafe { OpenProcessToken(GetCurrentProcess(), access, &mut token) };
    if ok == 0 {
        return Err(std::io::Error::last_os_error());
    }
    Ok(WinHandle::new(token))
}

#[cfg(test)]
mod tests {
    use super::*;
    use windows_sys::Win32::Security::TOKEN_QUERY;

    #[test]
    fn current_process_token_identity_is_queryable() {
        let token = open_current_process_token(TOKEN_QUERY)
            .expect("OpenProcessToken on current process should succeed");
        token_user_is_local_system(token.as_handle())
            .expect("GetTokenInformation(TokenUser) should succeed");
    }
}
