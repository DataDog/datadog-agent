// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ptr;
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::Security::{
    GetTokenInformation, IsWellKnownSid, TOKEN_USER, TokenUser, WinLocalSystemSid,
};
use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};

use super::win_handle::WinHandle;

/// Returns whether `token`'s user SID is the built-in LocalSystem account.
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

/// Opens the current process token with the requested access rights.
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
