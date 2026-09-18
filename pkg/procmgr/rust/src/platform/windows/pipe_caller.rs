// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::sync::OnceLock;

use super::agent_service_sid::installed_agent_user_sid_bytes;
use super::token_identity::{sids_equal, token_user_sid_bytes};
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE, TRUE};
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
    may_mutate(pipe, installed_agent_user_sid())
}

/// Decide whether the client on `pipe` may call a mutating RPC.
///
/// `agent_sid` arrives already resolved, and must stay that way: clients open the pipe at
/// `SECURITY_IDENTIFICATION`, so the thread impersonating one of them cannot pass an
/// access check. Resolving the SID here would fail the registry read and the LSA lookup
/// with `ERROR_BAD_IMPERSONATION_LEVEL`, denying every Agent-user caller.
fn may_mutate(pipe: HANDLE, agent_sid: Option<&[u8]>) -> bool {
    let allowed =
        with_client_token(pipe, |token| token_may_mutate(token, agent_sid)).unwrap_or(false);
    if !allowed {
        log::info!(
            "denied a mutating RPC: the pipe client is not LocalSystem, a local Administrator, or the installed Agent user"
        );
    }
    allowed
}

/// Run `f` against the client's access token, then drop the impersonation.
fn with_client_token<T>(pipe: HANDLE, f: impl FnOnce(HANDLE) -> Option<T>) -> Option<T> {
    if unsafe { ImpersonateNamedPipeClient(pipe) } == 0 {
        let err = std::io::Error::last_os_error();
        log_impersonation_failure("ImpersonateNamedPipeClient", &err);
        return None;
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

    let mut token: HANDLE = std::ptr::null_mut();
    // OpenAsSelf: the access check runs against the process token, not the client's.
    let ok = unsafe { OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, TRUE, &mut token) };
    if ok == 0 {
        let err = std::io::Error::last_os_error();
        log_impersonation_failure("OpenThreadToken after pipe impersonation", &err);
        return None;
    }

    let result = f(token);
    unsafe {
        CloseHandle(token);
    }
    result
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

fn token_may_mutate(token: HANDLE, agent_sid: Option<&[u8]>) -> Option<bool> {
    if query_token_is_local_system(token)? {
        return Some(true);
    }
    if token_is_builtin_admin(token)? {
        return Some(true);
    }
    token_is_installed_agent_user(token, agent_sid)
}

static INSTALLED_AGENT_USER_SID: OnceLock<Vec<u8>> = OnceLock::new();

/// Resolve the installed Agent user SID. Reads the registry and queries the LSA, so it
/// must never run while the calling thread impersonates a pipe client.
fn installed_agent_user_sid() -> Option<&'static [u8]> {
    if let Some(sid) = INSTALLED_AGENT_USER_SID.get() {
        return Some(sid.as_slice());
    }
    match installed_agent_user_sid_bytes() {
        Ok(sid) => {
            let _ = INSTALLED_AGENT_USER_SID.set(sid);
            INSTALLED_AGENT_USER_SID.get().map(Vec::as_slice)
        }
        Err(err) => {
            log::warn!(
                "installed agent user SID lookup failed: {err:#}; Agent-user pipe clients cannot call mutating RPCs"
            );
            None
        }
    }
}

fn token_is_installed_agent_user(token: HANDLE, agent_sid: Option<&[u8]>) -> Option<bool> {
    let Some(expected) = agent_sid else {
        return Some(false);
    };
    let token_sid = token_user_sid_bytes(token).ok()?;
    sids_equal(&token_sid, expected).ok()
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

#[cfg(test)]
mod tests {
    use super::super::sid::create_well_known_sid;
    use super::super::token_identity::open_current_process_token;
    use super::*;
    use std::ffi::OsStr;
    use std::fs::File;
    use std::io::{Read, Write};
    use std::os::windows::ffi::OsStrExt;
    use std::os::windows::io::{AsRawHandle, FromRawHandle, RawHandle};
    use std::sync::atomic::{AtomicU32, Ordering};
    use windows_sys::Win32::Foundation::INVALID_HANDLE_VALUE;
    use windows_sys::Win32::Security::WinNullSid;
    use windows_sys::Win32::Storage::FileSystem::{
        CreateFileW, FILE_GENERIC_READ, FILE_WRITE_DATA, OPEN_EXISTING, PIPE_ACCESS_DUPLEX,
        SECURITY_IDENTIFICATION, SECURITY_SQOS_PRESENT,
    };
    use windows_sys::Win32::System::Pipes::{
        CreateNamedPipeW, PIPE_READMODE_BYTE, PIPE_TYPE_BYTE, PIPE_WAIT,
    };

    /// Both ends of a connected pipe. The server end has already read a byte, which is
    /// what `ImpersonateNamedPipeClient` derives the client's security context from.
    struct PipePair {
        server: File,
        _client: File,
    }

    impl PipePair {
        fn server(&self) -> HANDLE {
            self.server.as_raw_handle() as HANDLE
        }
    }

    fn wide(value: &str) -> Vec<u16> {
        OsStr::new(value).encode_wide().chain(Some(0)).collect()
    }

    /// Opens the client end the way the shipped clients do, at `SECURITY_IDENTIFICATION`.
    /// That level is the point of these tests: it lets the server read the client's
    /// identity but not use the token to open securable objects.
    fn connected_pipe_pair() -> PipePair {
        static COUNTER: AtomicU32 = AtomicU32::new(0);
        let name = format!(
            r"\\.\pipe\dd-procmgrd-pipe-caller-test-{}-{}",
            std::process::id(),
            COUNTER.fetch_add(1, Ordering::Relaxed)
        );
        let name_w = wide(&name);

        let server = unsafe {
            CreateNamedPipeW(
                name_w.as_ptr(),
                PIPE_ACCESS_DUPLEX,
                PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT,
                1,
                4096,
                4096,
                0,
                std::ptr::null(),
            )
        };
        assert!(
            server != INVALID_HANDLE_VALUE,
            "CreateNamedPipeW({name}): {}",
            std::io::Error::last_os_error()
        );
        let mut server = unsafe { File::from_raw_handle(server as RawHandle) };

        // Connecting before the server waits is what makes ConnectNamedPipe unnecessary:
        // the client's open completes the connection on its own.
        let client = unsafe {
            CreateFileW(
                name_w.as_ptr(),
                FILE_GENERIC_READ | FILE_WRITE_DATA,
                0,
                std::ptr::null(),
                OPEN_EXISTING,
                SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION,
                std::ptr::null_mut(),
            )
        };
        assert!(
            client != INVALID_HANDLE_VALUE,
            "CreateFileW({name}): {}",
            std::io::Error::last_os_error()
        );
        let mut client = unsafe { File::from_raw_handle(client as RawHandle) };

        client.write_all(&[0u8]).expect("client write");
        let mut received = [0u8; 1];
        server.read_exact(&mut received).expect("server read");

        PipePair {
            server,
            _client: client,
        }
    }

    fn current_process_sid() -> Vec<u8> {
        let token = open_current_process_token(TOKEN_QUERY)
            .expect("OpenProcessToken on current process should succeed");
        token_user_sid_bytes(token.as_handle()).expect("current process should have a user SID")
    }

    #[test]
    fn identification_level_client_is_matched_by_user_sid() {
        let pair = connected_pipe_pair();
        let expected = current_process_sid();

        let matched = with_client_token(pair.server(), |token| {
            token_is_installed_agent_user(token, Some(&expected))
        });

        assert_eq!(
            matched,
            Some(true),
            "an identification-level client must still be recognizable by its user SID"
        );
    }

    #[test]
    fn identification_level_client_is_rejected_by_another_sid() {
        let pair = connected_pipe_pair();
        // S-1-0-0, which no token can ever have as its user.
        let foreign = create_well_known_sid(WinNullSid).expect("null SID");

        let matched = with_client_token(pair.server(), |token| {
            token_is_installed_agent_user(token, Some(&foreign))
        });

        assert_eq!(matched, Some(false));
    }

    #[test]
    fn unresolved_agent_sid_does_not_match_any_client() {
        let pair = connected_pipe_pair();

        let matched = with_client_token(pair.server(), |token| {
            token_is_installed_agent_user(token, None)
        });

        assert_eq!(matched, Some(false));
    }

    #[test]
    fn may_mutate_accepts_a_client_whose_sid_is_the_agent_sid() {
        let pair = connected_pipe_pair();
        let agent_sid = current_process_sid();

        assert!(may_mutate(pair.server(), Some(&agent_sid)));
    }
}
