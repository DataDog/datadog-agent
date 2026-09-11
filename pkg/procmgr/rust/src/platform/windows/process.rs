// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::time::Duration;

use anyhow::Result;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::System::Threading::{
    GetExitCodeProcess, OpenProcess, PROCESS_TERMINATE, TerminateProcess, WaitForSingleObject,
};

const WAIT_OBJECT_0: u32 = 0;
const WAIT_FAILED: u32 = 0xFFFF_FFFF;
const WAIT_TIMEOUT: u32 = 0x0000_0102;
pub(crate) const WAIT_INFINITE: u32 = 0xFFFF_FFFF;

pub(crate) enum ProcessWaitOutcome {
    Exited(u32),
    TimedOut,
}

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

pub(crate) fn wait_for_process_exit(
    handle: HANDLE,
    timeout: Duration,
) -> Result<ProcessWaitOutcome> {
    let timeout_ms = u32::try_from(timeout.as_millis()).unwrap_or(WAIT_INFINITE);
    wait_for_process_exit_ms(handle, timeout_ms)
}

pub(crate) fn wait_for_process_exit_ms(
    handle: HANDLE,
    timeout_ms: u32,
) -> Result<ProcessWaitOutcome> {
    let wait_result = unsafe { WaitForSingleObject(handle, timeout_ms) };
    if wait_result == WAIT_FAILED {
        return Err(std::io::Error::last_os_error().into());
    }
    if wait_result == WAIT_TIMEOUT {
        return Ok(ProcessWaitOutcome::TimedOut);
    }
    if wait_result != WAIT_OBJECT_0 {
        return Err(std::io::Error::other(format!(
            "WaitForSingleObject returned unexpected status: {wait_result}"
        ))
        .into());
    }

    let mut exit_code: u32 = 0;
    let ok = unsafe { GetExitCodeProcess(handle, &mut exit_code) };
    if ok == 0 {
        return Err(std::io::Error::last_os_error().into());
    }
    Ok(ProcessWaitOutcome::Exited(exit_code))
}
