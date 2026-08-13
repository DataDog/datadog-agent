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
    FILE_SHARE_READ, FILE_SHARE_WRITE, OPEN_ALWAYS, OPEN_EXISTING,
};
use windows_sys::Win32::System::Console::{GetStdHandle, STD_ERROR_HANDLE, STD_OUTPUT_HANDLE};
use windows_sys::Win32::System::Threading::GetCurrentProcess;

use crate::spawn::StdioSetting;

use super::super::agent_credentials::AgentAccount;
use super::super::wide;
use super::logon::{logon_user_credentials, logon_user_token, with_impersonated_token};

/// Resolve portable stdio settings for `tokio::process::Command` fallback spawns.
pub(super) fn to_command_stdio(setting: &StdioSetting, inheritable: bool) -> Stdio {
    match setting {
        StdioSetting::Null => Stdio::null(),
        StdioSetting::Inherit if inheritable => Stdio::inherit(),
        StdioSetting::Inherit => Stdio::null(),
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
    account: &AgentAccount,
) -> Result<MappedStdioHandle> {
    match setting {
        StdioSetting::Null => MappedStdioHandle::nul(),
        StdioSetting::Inherit => {
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
        StdioSetting::File(path) => {
            open_stdio_file_as_account(process_name, path.to_string_lossy().as_ref(), account)
        }
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
    with_impersonated_token(process_name, token.raw(), || {
        Ok(MappedStdioHandle(open_append_file(path)?))
    })
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
            std::ptr::null_mut(),
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
    use crate::test_helpers;
    use std::path::PathBuf;

    #[test]
    fn stdio_setting_inherit_or_null() {
        assert!(StdioSetting::Inherit.is_inherit_or_null());
        assert!(StdioSetting::Null.is_inherit_or_null());
        assert!(!StdioSetting::File(PathBuf::from(r"C:\logs\trace.log")).is_inherit_or_null());
    }

    fn command_stdio(yaml: &str) -> Stdio {
        let setting = match yaml {
            "null" => StdioSetting::Null,
            "inherit" | "" => StdioSetting::Inherit,
            path => StdioSetting::File(path.into()),
        };
        to_command_stdio(&setting, crate::platform::stdout_inheritable())
    }

    #[test]
    fn null_discards_child_stdout() {
        let (sh, flag) = test_helpers::shell_cmd();
        let out = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo hello")
            .stdout(command_stdio("null"))
            .output()
            .unwrap();
        assert!(out.stdout.is_empty());
    }

    #[test]
    fn writable_path_redirect() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("pmgr_stdio_redirect.log");
        let path_str = path.to_str().unwrap();
        let (sh, flag) = test_helpers::shell_cmd();
        let status = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo fileline")
            .stdout(command_stdio(path_str))
            .status()
            .unwrap();
        assert!(status.success());
        let contents = std::fs::read_to_string(&path).unwrap();
        assert!(contents.contains("fileline"), "got {contents:?}");
    }
}
