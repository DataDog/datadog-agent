// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Named-pipe client identity for gRPC authorization.

use windows_sys::Win32::Foundation::{HANDLE, TRUE};
use windows_sys::Win32::Security::{
    AllocateAndInitializeSid, CheckTokenMembership, FreeSid, GetTokenInformation, IsWellKnownSid,
    RevertToSelf, SECURITY_NT_AUTHORITY, TOKEN_QUERY, TokenUser, WinLocalSystemSid,
};
use windows_sys::Win32::System::Pipes::ImpersonateNamedPipeClient;
use windows_sys::Win32::System::SystemServices::{
    DOMAIN_ALIAS_RID_ADMINS, SECURITY_BUILTIN_DOMAIN_RID,
};
use windows_sys::Win32::System::Threading::{GetCurrentThread, OpenThreadToken};

/// Returns whether the connected pipe client may invoke mutating gRPC methods (`Create`).
///
/// Must be called only after at least one message has been read from the pipe: the named
/// pipe filesystem impersonates the security context of the last message read
/// ([`ImpersonateNamedPipeClient`](https://learn.microsoft.com/en-us/windows/win32/api/namedpipeapi/nf-namedpipeapi-impersonatenamedpipeclient)).
/// Clients must connect with at least `SECURITY_IDENTIFICATION` (tokio's default; COAT
/// uses go-winio identification).
///
/// A `false` result does **not** drop the connection: tonic keeps serving the pipe and
/// non-privileged RPCs (`Start`/`Stop`/`ReloadConfig`/reads) continue to work. Only
/// handlers that call [`crate::grpc::caller_auth::require_privileged_pipe_client`] return
/// `PERMISSION_DENIED`.
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
    match impersonated_client_may_mutate() {
        Some(may_mutate) => may_mutate,
        None => false,
    }
}

/// Win32 error codes for clients that connect or impersonate without a usable token.
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
