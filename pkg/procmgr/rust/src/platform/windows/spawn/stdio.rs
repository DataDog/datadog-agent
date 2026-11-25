// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
use log::warn;
use std::path::Path;
use std::ptr;
use windows_sys::Win32::Foundation::{
    CloseHandle, DUPLICATE_SAME_ACCESS, DuplicateHandle, HANDLE, HANDLE_FLAG_INHERIT,
    INVALID_HANDLE_VALUE, SetHandleInformation,
};
use windows_sys::Win32::Storage::FileSystem::{
    CreateFileW, FILE_APPEND_DATA, FILE_ATTRIBUTE_NORMAL, FILE_GENERIC_READ, FILE_GENERIC_WRITE,
    FILE_SHARE_DELETE, FILE_SHARE_READ, FILE_SHARE_WRITE, OPEN_ALWAYS, OPEN_EXISTING,
};
use windows_sys::Win32::System::Console::GetStdHandle;
use windows_sys::Win32::System::Threading::GetCurrentProcess;

use crate::spawn::StdioSetting;

use super::super::wide;
use super::credential::SpawnCredential;
use super::logon::{logon_user_credentials, logon_user_token, with_impersonated_token};

/// Map a processes.d stdio setting to an inheritable Win32 handle for the child.
///
/// On Windows, children inherit stdio via HANDLEs, not file descriptors. We impersonate the
/// spawn account when opening log paths so ACLs match that user. Falls back to inherit or NUL.
/// A log open that does not finish, or reports a timeout, fails the spawn instead of falling back.
pub(super) fn map_stdio_setting(
    process_name: &str,
    setting: &StdioSetting,
    kind: u32,
    credential: &SpawnCredential,
) -> Result<MappedStdioHandle> {
    match setting {
        StdioSetting::Null => MappedStdioHandle::nul(),
        StdioSetting::Inherit => map_stdio_inherit(kind),
        StdioSetting::File(path) => {
            let path = path.to_string_lossy();
            match open_log_file(
                process_name,
                path.as_ref(),
                credential,
                open_stdio_file_as_account,
            )? {
                Some(handle) => Ok(handle),
                None => map_stdio_inherit(kind),
            }
        }
    }
}

/// Opens a stdio log file through `open`. `None` means fall back to inherit. The spawn fails
/// when the bounded call did not finish or the open reports `TimedOut` or `Interrupted`, as on
/// Unix. Any other open error falls back.
fn open_log_file(
    process_name: &str,
    path: &str,
    credential: &SpawnCredential,
    open: OpenFn,
) -> Result<Option<MappedStdioHandle>> {
    match open_stdio_file_bounded(process_name, path, credential, open)? {
        Ok(handle) => Ok(Some(handle)),
        Err(e) if is_stalled_open(&e) => Err(e.context(format!(
            "[{process_name}] opening stdio file {path} stalled"
        ))),
        Err(e) => {
            warn!(
                "[{process_name}] failed to open stdio file {path}: {e:#}, falling back to inherit"
            );
            Ok(None)
        }
    }
}

fn is_stalled_open(error: &anyhow::Error) -> bool {
    error
        .chain()
        .filter_map(|cause| cause.downcast_ref::<std::io::Error>())
        .any(|e| {
            matches!(
                e.kind(),
                std::io::ErrorKind::TimedOut | std::io::ErrorKind::Interrupted
            )
        })
}

/// Inherit duplicates the current standard handle; a service without one
/// retains the NUL fallback.
fn map_stdio_inherit(kind: u32) -> Result<MappedStdioHandle> {
    map_stdio_source(unsafe { GetStdHandle(kind) })
}

fn map_stdio_source(source: HANDLE) -> Result<MappedStdioHandle> {
    // INVALID_HANDLE_VALUE is also a Win32 pseudo-handle for the current process.
    // Never let DuplicateHandle turn a missing standard stream into that object.
    if source.is_null() || source == INVALID_HANDLE_VALUE {
        return MappedStdioHandle::nul();
    }
    match duplicate_inheritable_handle(source) {
        Ok(handle) => Ok(MappedStdioHandle(handle)),
        Err(error) => {
            warn!("standard stream duplication failed: {error}; using NUL");
            MappedStdioHandle::nul()
        }
    }
}

pub(super) fn map_stdio_handle_nul() -> Result<MappedStdioHandle> {
    MappedStdioHandle::nul()
}

pub(crate) struct MappedStdioHandle(HANDLE);

impl MappedStdioHandle {
    pub(crate) fn raw(&self) -> HANDLE {
        self.0
    }

    fn nul() -> Result<Self> {
        Ok(Self(open_nul_handle(
            FILE_GENERIC_READ | FILE_GENERIC_WRITE,
        )?))
    }
}

impl Drop for MappedStdioHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}

/// Carries the open result off the helper thread. `HANDLE` is not `Send` on its own.
struct OpenOutcome(Result<MappedStdioHandle>);

// SAFETY: the handle is owned solely by this value.
unsafe impl Send for OpenOutcome {}

/// Takes the credential by value, so the open can drop it before it blocks.
type OpenFn = fn(&str, &str, SpawnCredential) -> Result<MappedStdioHandle>;

/// Outer error: the open did not finish. Impersonation is per thread, so it runs inside the call.
fn open_stdio_file_bounded(
    process_name: &str,
    path: &str,
    credential: &SpawnCredential,
    open: OpenFn,
) -> Result<Result<MappedStdioHandle>> {
    let process = process_name.to_owned();
    let owned_path = path.to_owned();
    let credential = credential.clone();
    let outcome = crate::stop::bounded(Path::new(path), move || {
        OpenOutcome(open(&process, &owned_path, credential))
    })
    .with_context(|| format!("[{process_name}] opening stdio file {path} did not finish"))?;
    Ok(outcome.0)
}

fn open_stdio_file_as_account(
    process_name: &str,
    path: &str,
    credential: SpawnCredential,
) -> Result<MappedStdioHandle> {
    if credential.reuses_supervisor_token() {
        return Ok(MappedStdioHandle(open_append_file(path)?));
    }
    let token = {
        let creds = logon_user_credentials(credential.account());
        logon_user_token(process_name, &creds)?
    };
    // Zeroes a logon password before the open, which can block for good on a dead share.
    drop(credential);
    with_impersonated_token(process_name, token.raw(), || {
        Ok(MappedStdioHandle(open_append_file(path)?))
    })
}

fn open_append_file(path: &str) -> Result<HANDLE> {
    let path_w = wide::null_terminated(path);
    let h = unsafe {
        CreateFileW(
            path_w.as_ptr(),
            FILE_APPEND_DATA,
            FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE,
            ptr::null(),
            OPEN_ALWAYS,
            FILE_ATTRIBUTE_NORMAL,
            ptr::null_mut(),
        )
    };
    if h == INVALID_HANDLE_VALUE || h.is_null() {
        // Typed, so `is_stalled_open` can see a timeout kind.
        let error = std::io::Error::last_os_error();
        return Err(anyhow::Error::new(error).context(format!("CreateFileW({path}) failed")));
    }
    if let Err(e) = set_handle_inheritable(h) {
        unsafe {
            CloseHandle(h);
        }
        return Err(e);
    }
    Ok(h)
}

fn open_nul_handle(access: u32) -> Result<HANDLE> {
    let nul = wide::null_terminated("NUL");
    let h = unsafe {
        CreateFileW(
            nul.as_ptr(),
            access,
            FILE_SHARE_READ | FILE_SHARE_WRITE,
            std::ptr::null(),
            OPEN_EXISTING,
            FILE_ATTRIBUTE_NORMAL,
            std::ptr::null_mut(),
        )
    };
    if h == INVALID_HANDLE_VALUE || h.is_null() {
        bail!(
            "CreateFileW(NUL) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    if let Err(e) = set_handle_inheritable(h) {
        unsafe {
            CloseHandle(h);
        }
        return Err(e);
    }
    Ok(h)
}

fn set_handle_inheritable(handle: HANDLE) -> Result<()> {
    if handle.is_null() || handle == INVALID_HANDLE_VALUE {
        bail!("cannot mark invalid handle inheritable");
    }
    let ok = unsafe { SetHandleInformation(handle, HANDLE_FLAG_INHERIT, HANDLE_FLAG_INHERIT) };
    if ok == 0 {
        bail!(
            "SetHandleInformation(HANDLE_FLAG_INHERIT) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(())
}

fn duplicate_inheritable_handle(source: HANDLE) -> Result<HANDLE> {
    let mut dup: HANDLE = std::ptr::null_mut();
    let ok = unsafe {
        DuplicateHandle(
            GetCurrentProcess(),
            source,
            GetCurrentProcess(),
            &mut dup,
            0,
            1,
            DUPLICATE_SAME_ACCESS,
        )
    };
    if ok == 0 {
        bail!(
            "DuplicateHandle failed: {}",
            std::io::Error::last_os_error()
        );
    }
    if let Err(e) = set_handle_inheritable(dup) {
        unsafe {
            CloseHandle(dup);
        }
        return Err(e);
    }
    Ok(dup)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    use std::sync::{Mutex, mpsc};
    use windows_sys::Win32::System::Console::STD_OUTPUT_HANDLE;

    fn system_credential() -> SpawnCredential {
        SpawnCredential::from_account(
            crate::platform::windows::local_agent_account::AgentAccount::LocalSystem,
        )
        .expect("build credential")
    }

    /// Released by the stall test. An opener is a plain function, so this is a static.
    static OPEN_RELEASE: Mutex<Option<mpsc::Receiver<()>>> = Mutex::new(None);

    #[test]
    fn a_stalled_stdio_open_fails_the_spawn() {
        fn stalled_open(_: &str, _: &str, _: SpawnCredential) -> Result<MappedStdioHandle> {
            let release = OPEN_RELEASE.lock().unwrap().take().unwrap();
            let _ = release.recv();
            bail!("opener returned after the deadline")
        }
        let (release_tx, release_rx) = mpsc::channel::<()>();
        *OPEN_RELEASE.lock().unwrap() = Some(release_rx);
        let path = r"C:\unused\stalled-stdio.log";
        let credential = system_credential();
        let err = match open_log_file("test-proc", path, &credential, stalled_open) {
            Ok(_) => panic!("a stalled open must fail the spawn"),
            Err(err) => err,
        };
        assert!(format!("{err:#}").contains("stalled-stdio.log"), "{err:#}");

        // A refusal fails the spawn too.
        let refused = open_log_file("test-proc", path, &credential, |_, _, _| {
            bail!("never reached")
        });
        assert!(refused.is_err(), "a refused open must fail the spawn");
        let _ = release_tx.send(());
    }

    #[test]
    fn an_open_that_times_out_fails_the_spawn() {
        let credential = system_credential();
        let timed_out = open_log_file(
            "test-proc",
            r"C:\unused\timed-out-stdio.log",
            &credential,
            |_, _, _| {
                Err(
                    anyhow::Error::new(std::io::Error::from(std::io::ErrorKind::TimedOut))
                        .context("CreateFileW failed"),
                )
            },
        );
        assert!(timed_out.is_err(), "a timed out open must fail the spawn");
        let interrupted = open_log_file(
            "test-proc",
            r"C:\unused\interrupted-stdio.log",
            &credential,
            |_, _, _| {
                Err(
                    anyhow::Error::new(std::io::Error::from(std::io::ErrorKind::Interrupted))
                        .context("CreateFileW failed"),
                )
            },
        );
        assert!(
            interrupted.is_err(),
            "an interrupted open must fail the spawn"
        );
    }

    #[test]
    fn an_open_that_fails_for_another_reason_falls_back_to_inherit() {
        let credential = system_credential();
        let denied = open_log_file(
            "test-proc",
            r"C:\unused\denied-stdio.log",
            &credential,
            |_, _, _| {
                Err(
                    anyhow::Error::new(std::io::Error::from(std::io::ErrorKind::PermissionDenied))
                        .context("CreateFileW failed"),
                )
            },
        );
        assert!(matches!(denied, Ok(None)), "an access error falls back");
        let untyped = open_log_file(
            "test-proc",
            r"C:\unused\untyped-stdio.log",
            &credential,
            |_, _, _| bail!("logon failed"),
        );
        assert!(matches!(untyped, Ok(None)), "an untyped error falls back");
    }

    #[test]
    fn unusable_inheritance_source_falls_back_to_nul() {
        assert!(
            !map_stdio_source(std::ptr::null_mut())
                .unwrap()
                .raw()
                .is_null()
        );
        assert!(
            !map_stdio_source(INVALID_HANDLE_VALUE)
                .unwrap()
                .raw()
                .is_null()
        );
    }

    #[test]
    fn unopenable_file_path_falls_back_to_inherit() {
        let bad_path = StdioSetting::File(PathBuf::from(r"C:\nonexistent_pmgr_stdio_dir\out.log"));
        let credential = SpawnCredential::from_account(
            crate::platform::windows::local_agent_account::AgentAccount::LocalSystem,
        )
        .expect("build credential");
        let handle = map_stdio_setting("test-proc", &bad_path, STD_OUTPUT_HANDLE, &credential)
            .expect("map_stdio_setting should fall back instead of failing spawn");
        assert!(!handle.raw().is_null());
    }
}
