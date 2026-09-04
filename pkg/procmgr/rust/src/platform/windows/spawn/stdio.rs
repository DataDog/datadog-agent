// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use log::warn;
use std::path::Path;
use std::process::Stdio;
use std::ptr;
use windows_sys::Win32::Foundation::{
    CloseHandle, DUPLICATE_SAME_ACCESS, DuplicateHandle, HANDLE, HANDLE_FLAG_INHERIT,
    INVALID_HANDLE_VALUE, SetHandleInformation,
};
use windows_sys::Win32::Storage::FileSystem::{
    CreateFileW, FILE_APPEND_DATA, FILE_ATTRIBUTE_NORMAL, FILE_GENERIC_READ, FILE_GENERIC_WRITE,
    FILE_SHARE_DELETE, FILE_SHARE_READ, FILE_SHARE_WRITE, OPEN_ALWAYS, OPEN_EXISTING,
};
use windows_sys::Win32::System::Console::{GetStdHandle, STD_ERROR_HANDLE, STD_OUTPUT_HANDLE};
use windows_sys::Win32::System::Threading::GetCurrentProcess;

use crate::spawn::StdioSetting;

use super::super::wide;
use super::credential::SpawnCredential;
use super::logon::{logon_user_credentials, logon_user_token, with_impersonated_token};

pub(crate) fn to_command_stdio(setting: &StdioSetting, inheritable: bool) -> Stdio {
    match setting {
        StdioSetting::Null => Stdio::null(),
        StdioSetting::Inherit => {
            if inheritable {
                Stdio::inherit()
            } else {
                Stdio::null()
            }
        }
        StdioSetting::File(path) => file_to_stdio(path),
    }
}

fn file_to_stdio(path: &Path) -> Stdio {
    match std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)
    {
        Ok(f) => f.into(),
        Err(e) => {
            warn!(
                "failed to open stdio file {}: {e}, falling back to inherit",
                path.display()
            );
            Stdio::inherit()
        }
    }
}

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
            match open_stdio_file_as_account(process_name, path.as_ref(), credential) {
                Ok(handle) => Ok(handle),
                Err(e) => {
                    warn!(
                        "[{process_name}] failed to open stdio file {path}: {e:#}, falling back to inherit"
                    );
                    map_stdio_inherit(kind)
                }
            }
        }
    }
}

fn map_stdio_inherit(kind: u32) -> Result<MappedStdioHandle> {
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

pub(super) fn map_stdio_handle_nul() -> Result<MappedStdioHandle> {
    MappedStdioHandle::nul()
}

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
    credential: &SpawnCredential,
) -> Result<MappedStdioHandle> {
    let account = credential.account();
    if account.spawns_with_supervisor_token().unwrap_or_else(|e| {
        warn!("[{process_name}] compare spawn account to supervisor token: {e:#}");
        false
    }) {
        return Ok(MappedStdioHandle(open_append_file(path)?));
    }
    let creds = logon_user_credentials(account);
    let token = logon_user_token(process_name, &creds)?;
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
        bail!(
            "CreateFileW({path}) failed: {}",
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

    #[test]
    fn stdio_setting_inherit_or_null() {
        assert!(StdioSetting::Inherit.is_inherit_or_null());
        assert!(StdioSetting::Null.is_inherit_or_null());
        assert!(!StdioSetting::File(PathBuf::from(r"C:\logs\trace.log")).is_inherit_or_null());
    }

    #[test]
    fn unopenable_file_path_falls_back_to_inherit() {
        let bad_path = StdioSetting::File(PathBuf::from(r"C:\nonexistent_pmgr_stdio_dir\out.log"));
        let credential = SpawnCredential::from_account(
            crate::platform::windows::local_agent_account::AgentAccount::LocalSystem,
        );
        let handle = map_stdio_setting("test-proc", &bad_path, STD_OUTPUT_HANDLE, &credential)
            .expect("map_stdio_setting should fall back instead of failing spawn");
        assert!(!handle.raw().is_null());
    }
}
