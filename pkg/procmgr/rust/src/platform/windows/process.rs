// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::System::Threading::{OpenProcess, PROCESS_TERMINATE, TerminateProcess};

pub(crate) fn terminate_process(handle: HANDLE) -> Result<()> {
    let ok = unsafe { TerminateProcess(handle, 1) };
    if ok == 0 {
        Err(std::io::Error::last_os_error().into())
    } else {
        Ok(())
    }
}

pub(crate) fn terminate_process_by_pid(pid: u32) -> Result<()> {
    unsafe {
        let handle = OpenProcess(PROCESS_TERMINATE, 0, pid);
        if handle.is_null() {
            anyhow::bail!(
                "OpenProcess(TERMINATE, {pid}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        let result = terminate_process(handle);
        CloseHandle(handle);
        if result.is_err() {
            anyhow::bail!(
                "TerminateProcess({pid}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
    }
    Ok(())
}
