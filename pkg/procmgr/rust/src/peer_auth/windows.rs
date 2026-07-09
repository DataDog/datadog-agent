// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use std::path::{Path, PathBuf};
use windows_sys::Win32::Foundation::CloseHandle;
use windows_sys::Win32::System::Threading::{
    OpenProcess, PROCESS_QUERY_LIMITED_INFORMATION, QueryFullProcessImageNameW,
};

pub const PAR_EXE_BASENAME: &str = "privateactionrunner.exe";
pub const PROCMGR_CLI_EXE_BASENAME: &str = "dd-procmgr.exe";

fn cli_peer_auth_enabled() -> bool {
    std::env::var("DD_PM_PRIVILEGED_COMMANDS_ALLOW_CLI")
        .is_ok_and(|v| matches!(v.as_str(), "1" | "true" | "TRUE"))
}

/// Return true when `client_pid` is an authorized RunPrivilegedCommand caller.
pub fn authorize_par_caller(client_pid: u32) -> bool {
    if client_pid == 0 {
        return false;
    }

    #[cfg(test)]
    if std::env::var("DD_PM_PRIVILEGED_COMMANDS_TEST_SKIP_PEER_AUTH").is_ok_and(|v| v == "1") {
        return true;
    }

    match process_exe_path(client_pid) {
        Ok(image_path) => {
            if image_path_matches_par_install(&image_path) {
                return true;
            }
            cli_peer_auth_enabled()
                && path_basename(&image_path)
                    .is_some_and(|basename| basename.eq_ignore_ascii_case(PROCMGR_CLI_EXE_BASENAME))
        }
        Err(e) => {
            log::warn!("peer auth: failed to resolve pid {client_pid}: {e:#}");
            false
        }
    }
}

fn image_path_matches_par_install(image_path: &Path) -> bool {
    if !path_basename(image_path)
        .is_some_and(|basename| basename.eq_ignore_ascii_case(PAR_EXE_BASENAME))
    {
        return false;
    }

    let normalized = normalize_windows_path(image_path);
    expected_par_exe_paths()
        .into_iter()
        .any(|expected| normalize_windows_path(&expected) == normalized)
}

fn expected_par_exe_paths() -> Vec<PathBuf> {
    let install_root = crate::platform::agent_install_root();
    vec![
        install_root
            .join("bin")
            .join("agent")
            .join(PAR_EXE_BASENAME),
        install_root
            .join("embedded")
            .join("bin")
            .join(PAR_EXE_BASENAME),
    ]
}

fn normalize_windows_path(path: &Path) -> String {
    path.to_string_lossy().replace('/', "\\").to_ascii_lowercase()
}

fn path_basename(path: &Path) -> Option<String> {
    path.file_name()
        .and_then(|s| s.to_str())
        .map(str::to_string)
}

fn process_exe_path(pid: u32) -> Result<PathBuf> {
    let handle = unsafe { OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid) };
    if handle.is_null() {
        anyhow::bail!(
            "OpenProcess({pid}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    let _guard = HandleGuard(handle);

    let mut buffer = vec![0u16; 32_768];
    let mut size = buffer.len() as u32;
    let ok = unsafe { QueryFullProcessImageNameW(handle, 0, buffer.as_mut_ptr(), &mut size) };
    if ok == 0 {
        anyhow::bail!(
            "QueryFullProcessImageNameW({pid}) failed: {}",
            std::io::Error::last_os_error()
        );
    }

    let image_path = String::from_utf16_lossy(&buffer[..size as usize]);
    Ok(PathBuf::from(image_path))
}

struct HandleGuard(windows_sys::Win32::Foundation::HANDLE);

impl Drop for HandleGuard {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn non_par_process_is_rejected() {
        assert!(!authorize_par_caller(std::process::id()));
    }

    #[test]
    fn par_basename_spoof_from_wrong_directory_is_rejected() {
        assert!(!image_path_matches_par_install(Path::new(
            r"C:\Temp\privateactionrunner.exe"
        )));
    }

    #[test]
    fn par_install_path_is_accepted() {
        for expected in expected_par_exe_paths() {
            assert!(
                image_path_matches_par_install(&expected),
                "expected trusted PAR path {expected:?}"
            );
        }
    }

    #[test]
    fn dd_procmgr_cli_allowed_when_opted_in() {
        unsafe {
            std::env::set_var("DD_PM_PRIVILEGED_COMMANDS_ALLOW_CLI", "1");
        }
        let image_path = process_exe_path(std::process::id()).unwrap();
        if path_basename(&image_path)
            .is_some_and(|basename| basename.eq_ignore_ascii_case(PROCMGR_CLI_EXE_BASENAME))
        {
            assert!(authorize_par_caller(std::process::id()));
        }
        unsafe {
            std::env::remove_var("DD_PM_PRIVILEGED_COMMANDS_ALLOW_CLI");
        }
    }
}
