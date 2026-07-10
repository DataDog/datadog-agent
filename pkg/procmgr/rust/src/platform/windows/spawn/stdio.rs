// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use std::ptr;
use windows_sys::Win32::Foundation::{
    CloseHandle, DUPLICATE_SAME_ACCESS, DuplicateHandle, HANDLE, HANDLE_FLAG_INHERIT,
    INVALID_HANDLE_VALUE, SetHandleInformation,
};
use windows_sys::Win32::Security::ImpersonateLoggedOnUser;
use windows_sys::Win32::Storage::FileSystem::{
    CreateFileW, FILE_APPEND_DATA, FILE_ATTRIBUTE_NORMAL, FILE_GENERIC_READ, FILE_GENERIC_WRITE,
    FILE_SHARE_READ, FILE_SHARE_WRITE, OPEN_ALWAYS, OPEN_EXISTING,
};
use windows_sys::Win32::System::Console::{GetStdHandle, STD_ERROR_HANDLE, STD_OUTPUT_HANDLE};
use windows_sys::Win32::System::Threading::GetCurrentProcess;

use super::super::agent_credentials::AgentAccount;
use super::super::wide;
use super::logon::{ImpersonationGuard, logon_user_credentials, logon_user_token};

pub(super) fn is_stdio_config_inherit_or_null(config: &str) -> bool {
    matches!(config, "inherit" | "" | "null")
}

pub(super) fn map_stdio_config(
    process_name: &str,
    config: &str,
    kind: u32,
    account: &AgentAccount,
) -> Result<MappedStdioHandle> {
    match config {
        "null" => MappedStdioHandle::nul(),
        "inherit" | "" => {
            let inheritable = match kind {
                STD_OUTPUT_HANDLE => super::super::stdout_inheritable(),
                STD_ERROR_HANDLE => super::super::stderr_inheritable(),
                _ => false,
            };
            if !inheritable {
                return MappedStdioHandle::nul();
            }
            let source = unsafe {
                let h = GetStdHandle(kind);
                if h == INVALID_HANDLE_VALUE || h.is_null() {
                    bail!("GetStdHandle({kind}) returned invalid");
                }
                h
            };
            Ok(MappedStdioHandle(duplicate_inheritable_handle(source)?))
        }
        path => open_stdio_file_as_account(process_name, path, account),
    }
}

pub(super) fn map_stdio_handle_nul() -> Result<MappedStdioHandle> {
    MappedStdioHandle::nul()
}

/// Owned stdio handle for CreateProcessAsUserW (never the process-wide GetStdHandle value).
pub(super) struct MappedStdioHandle(HANDLE);

impl MappedStdioHandle {
    pub(super) fn raw(&self) -> HANDLE {
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

fn open_stdio_file_as_account(
    process_name: &str,
    path: &str,
    account: &AgentAccount,
) -> Result<MappedStdioHandle> {
    if matches!(account, AgentAccount::LocalSystem) {
        return Ok(MappedStdioHandle(open_append_file(path)?));
    }
    let creds = logon_user_credentials(account);
    let token = logon_user_token(process_name, &creds)?;
    unsafe {
        if ImpersonateLoggedOnUser(token.raw()) == 0 {
            bail!(
                "[{process_name}] ImpersonateLoggedOnUser failed: {}",
                std::io::Error::last_os_error()
            );
        }
        let _impersonation = ImpersonationGuard::new(token);
        Ok(MappedStdioHandle(open_append_file(path)?))
    }
}

fn open_append_file(path: &str) -> Result<HANDLE> {
    let path_w = wide::null_terminated(path);
    let h = unsafe {
        CreateFileW(
            path_w.as_ptr(),
            FILE_GENERIC_WRITE | FILE_APPEND_DATA,
            FILE_SHARE_READ | FILE_SHARE_WRITE,
            ptr::null(),
            OPEN_ALWAYS,
            FILE_ATTRIBUTE_NORMAL,
            ptr::null_mut(),
        )
    };
    if h == INVALID_HANDLE_VALUE || h.is_null() {
        bail!(
            "CreateFileW({path}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    set_handle_inheritable(h)?;
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
            ptr::null_mut(),
        )
    };
    if h == INVALID_HANDLE_VALUE || h.is_null() {
        bail!(
            "CreateFileW(NUL) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    set_handle_inheritable(h)?;
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
    set_handle_inheritable(dup)?;
    Ok(dup)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn is_stdio_config_inherit_or_null_accepts_inherit_and_null() {
        assert!(is_stdio_config_inherit_or_null("inherit"));
        assert!(is_stdio_config_inherit_or_null(""));
        assert!(is_stdio_config_inherit_or_null("null"));
    }

    #[test]
    fn is_stdio_config_inherit_or_null_rejects_file_paths() {
        assert!(!is_stdio_config_inherit_or_null(r"C:\logs\trace.log"));
    }
}
