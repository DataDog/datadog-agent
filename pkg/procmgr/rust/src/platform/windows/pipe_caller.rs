// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use windows_sys::Win32::Foundation::{HANDLE, TRUE};
use windows_sys::Win32::Security::{
    AllocateAndInitializeSid, CheckTokenMembership, FreeSid, RevertToSelf, SECURITY_NT_AUTHORITY,
    TOKEN_QUERY,
};
use windows_sys::Win32::System::Pipes::ImpersonateNamedPipeClient;
use windows_sys::Win32::System::SystemServices::{
    DOMAIN_ALIAS_RID_ADMINS, SECURITY_BUILTIN_DOMAIN_RID,
};
use windows_sys::Win32::System::Threading::{GetCurrentThread, OpenThreadToken};

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
    if query_token_is_local_system(token)? {
        return Some(true);
    }
    token_is_builtin_admin(token)
}

fn query_token_is_local_system(token: HANDLE) -> Option<bool> {
    match super::token_identity::token_user_is_local_system(token) {
        Ok(is_local_system) => Some(is_local_system),
        Err(err) => {
            log::warn!("token_user_is_local_system failed: {err}");
            None
        }
    }
}

fn token_is_builtin_admin(token: HANDLE) -> Option<bool> {
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

    let mut is_member = 0i32;
    let ok = unsafe { CheckTokenMembership(token, admin_sid, &mut is_member) };
    unsafe {
        FreeSid(admin_sid);
    }
    if ok == 0 {
        log::warn!(
            "CheckTokenMembership(Administrators) failed: {}",
            std::io::Error::last_os_error()
        );
        return None;
    }
    Some(is_member != 0)
}
