// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use windows_sys::Win32::Foundation::{HANDLE, TRUE};
use windows_sys::Win32::Security::{
    AllocateAndInitializeSid, EqualSid, FreeSid, GetTokenInformation, IsWellKnownSid, RevertToSelf,
    SECURITY_NT_AUTHORITY, TOKEN_GROUPS, TOKEN_QUERY, TokenGroups, TokenUser, WinLocalSystemSid,
};
use windows_sys::Win32::System::Pipes::ImpersonateNamedPipeClient;
use windows_sys::Win32::System::SystemServices::{
    DOMAIN_ALIAS_RID_ADMINS, SE_GROUP_ENABLED, SECURITY_BUILTIN_DOMAIN_RID,
};
use windows_sys::Win32::System::Threading::{GetCurrentThread, OpenThreadToken};

/// True when the pipe client may call mutating gRPC methods (`Create`). Call after reading a message.
pub(crate) fn pipe_client_may_mutate(pipe: HANDLE) -> bool {
    if unsafe { ImpersonateNamedPipeClient(pipe) } == 0 {
        let err = std::io::Error::last_os_error();
        log_impersonation_failure("ImpersonateNamedPipeClient", &err);
        return false;
    }

    struct RevertGuard;
    impl Drop for RevertGuard {
        fn drop(&mut self) {
            unsafe {
                if RevertToSelf() == 0 {
                    log::warn!(
                        "RevertToSelf failed after pipe client check: {}",
                        std::io::Error::last_os_error()
                    );
                }
            }
        }
    }
    let _revert = RevertGuard;

    // Unidentified clients (e.g. SecurityAnonymous) are denied Create, same as a known
    // non-privileged identity — but the pipe connection itself stays open.
    impersonated_client_may_mutate().unwrap_or_default()
}

const ERROR_NO_TOKEN: i32 = 1008;
const ERROR_CANT_OPEN_ANONYMOUS: i32 = 1347;

fn log_impersonation_failure(context: &str, err: &std::io::Error) {
    let level = match err.raw_os_error() {
        Some(ERROR_NO_TOKEN) | Some(ERROR_CANT_OPEN_ANONYMOUS) => log::Level::Debug,
        _ => log::Level::Warn,
    };
    log::log!(level, "{context} failed: {err}");
}

fn impersonated_client_may_mutate() -> Option<bool> {
    let mut token: HANDLE = std::ptr::null_mut();
    let ok = unsafe { OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, TRUE, &mut token) };
    if ok == 0 {
        let err = std::io::Error::last_os_error();
        log_impersonation_failure("OpenThreadToken after pipe impersonation", &err);
        return None;
    }

    let result = token_may_mutate(token);
    unsafe {
        windows_sys::Win32::Foundation::CloseHandle(token);
    }
    result
}

fn token_may_mutate(token: HANDLE) -> Option<bool> {
    if token_is_local_system(token)? {
        return Some(true);
    }
    token_is_builtin_admin(token)
}

fn token_is_local_system(token: HANDLE) -> Option<bool> {
    let mut size = 0u32;
    let _ = unsafe { GetTokenInformation(token, TokenUser, std::ptr::null_mut(), 0, &mut size) };
    if size == 0 {
        return None;
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
        log::warn!(
            "GetTokenInformation(TokenUser) failed: {}",
            std::io::Error::last_os_error()
        );
        return None;
    }
    let user = buffer
        .as_ptr()
        .cast::<windows_sys::Win32::Security::TOKEN_USER>();
    Some(unsafe { IsWellKnownSid((*user).User.Sid, WinLocalSystemSid) != 0 })
}

fn token_is_builtin_admin(token: HANDLE) -> Option<bool> {
    let admin_sid = allocate_builtin_administrators_sid()?;
    let is_admin = token_has_enabled_group_sid(token, admin_sid);
    unsafe {
        FreeSid(admin_sid);
    }
    is_admin
}

fn allocate_builtin_administrators_sid() -> Option<*mut std::ffi::c_void> {
    let mut admin_sid = std::ptr::null_mut();
    let ok = unsafe {
        AllocateAndInitializeSid(
            &SECURITY_NT_AUTHORITY,
            2,
            SECURITY_BUILTIN_DOMAIN_RID.try_into().unwrap(),
            DOMAIN_ALIAS_RID_ADMINS.try_into().unwrap(),
            0,
            0,
            0,
            0,
            0,
            0,
            &mut admin_sid,
        )
    };
    if ok == 0 {
        log::warn!(
            "AllocateAndInitializeSid(Administrators) failed: {}",
            std::io::Error::last_os_error()
        );
        return None;
    }
    Some(admin_sid)
}

fn token_has_enabled_group_sid(token: HANDLE, target_sid: *mut std::ffi::c_void) -> Option<bool> {
    let mut size = 0u32;
    let _ = unsafe { GetTokenInformation(token, TokenGroups, std::ptr::null_mut(), 0, &mut size) };
    if size == 0 {
        return None;
    }
    let mut buffer = vec![0u8; size as usize];
    let ok = unsafe {
        GetTokenInformation(
            token,
            TokenGroups,
            buffer.as_mut_ptr().cast(),
            size,
            &mut size,
        )
    };
    if ok == 0 {
        log::warn!(
            "GetTokenInformation(TokenGroups) failed: {}",
            std::io::Error::last_os_error()
        );
        return None;
    }

    let groups = buffer.as_ptr().cast::<TOKEN_GROUPS>();
    let count = unsafe { (*groups).GroupCount } as usize;
    let first_group = unsafe { (*groups).Groups.as_ptr() };
    for i in 0..count {
        let group = unsafe { &*first_group.add(i) };
        if group.Attributes & SE_GROUP_ENABLED as u32 == 0 {
            continue;
        }
        if unsafe { EqualSid(group.Sid, target_sid) != 0 } {
            return Some(true);
        }
    }
    Some(false)
}
