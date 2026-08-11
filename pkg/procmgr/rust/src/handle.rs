// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::process::ExitStatus;

#[cfg(not(windows))]
use tokio::process::Child;

#[cfg(windows)]
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};

#[cfg(windows)]
struct OwnedProcessHandle {
    handle: HANDLE,
}

#[cfg(windows)]
// SAFETY: The Win32 HANDLE is a kernel handle safe to send across threads.
// The kernel serialises concurrent operations on the same handle.
unsafe impl Send for OwnedProcessHandle {}

#[cfg(windows)]
unsafe impl Sync for OwnedProcessHandle {}

#[cfg(windows)]
impl OwnedProcessHandle {
    fn get(&self) -> HANDLE {
        self.handle
    }
}

#[cfg(windows)]
impl Drop for OwnedProcessHandle {
    fn drop(&mut self) {
        if !self.handle.is_null() {
            unsafe {
                CloseHandle(self.handle);
            }
        }
    }
}

/// Platform-agnostic process handle for procmgr supervision.
///
/// On Unix we wrap `tokio::process::Child`.
/// On Windows we wrap a raw process handle from `CreateProcessAsUserW`.
pub struct ProcessHandle {
    #[cfg(not(windows))]
    child: Child,

    #[cfg(windows)]
    pid: u32,
    #[cfg(windows)]
    process_handle: OwnedProcessHandle,
}

impl ProcessHandle {
    #[cfg(not(windows))]
    pub fn from_child(child: Child) -> Self {
        Self { child }
    }

    #[cfg(windows)]
    pub fn from_raw(pid: u32, process_handle: HANDLE) -> Self {
        Self {
            pid,
            process_handle: OwnedProcessHandle {
                handle: process_handle,
            },
        }
    }

    pub fn id(&self) -> Option<u32> {
        #[cfg(not(windows))]
        {
            self.child.id()
        }
        #[cfg(windows)]
        {
            Some(self.pid)
        }
    }

    pub async fn wait(&mut self) -> Result<ExitStatus> {
        #[cfg(not(windows))]
        {
            Ok(self.child.wait().await?)
        }
        #[cfg(windows)]
        {
            raw_wait_exit_code(self.process_handle.get() as usize).await
        }
    }

    pub async fn kill(&mut self) -> Result<()> {
        #[cfg(not(windows))]
        {
            self.child.kill().await?;
            Ok(())
        }
        #[cfg(windows)]
        {
            raw_terminate_process(self.process_handle.get())
        }
    }
}

#[cfg(windows)]
async fn raw_wait_exit_code(process_handle: usize) -> Result<ExitStatus> {
    use std::os::windows::process::ExitStatusExt;
    use windows_sys::Win32::System::Threading::{
        GetExitCodeProcess, INFINITE, WaitForSingleObject,
    };

    const WAIT_OBJECT_0: u32 = 0;
    const WAIT_FAILED: u32 = 0xFFFF_FFFF;

    // Wait synchronously on the blocking pool so the future stays Send (required by
    // supervisor tasks spawned via tokio::spawn). The process HANDLE remains owned by
    // ProcessHandle for the duration of this wait.
    let exit_code = tokio::task::spawn_blocking(move || -> Result<u32> {
        let process_handle = process_handle as HANDLE;
        let wait_result = unsafe { WaitForSingleObject(process_handle, INFINITE) };
        if wait_result == WAIT_FAILED {
            return Err(std::io::Error::last_os_error().into());
        }
        if wait_result != WAIT_OBJECT_0 {
            return Err(std::io::Error::other(format!(
                "WaitForSingleObject returned unexpected status: {wait_result}"
            ))
            .into());
        }
        let mut exit_code: u32 = 0;
        let ok = unsafe { GetExitCodeProcess(process_handle, &mut exit_code) };
        if ok == 0 {
            return Err(std::io::Error::last_os_error().into());
        }
        Ok(exit_code)
    })
    .await??;

    Ok(ExitStatus::from_raw(exit_code))
}

#[cfg(windows)]
fn raw_terminate_process(process_handle: HANDLE) -> Result<()> {
    use windows_sys::Win32::System::Threading::TerminateProcess;
    let ok = unsafe { TerminateProcess(process_handle, 1) };
    if ok == 0 {
        Err(std::io::Error::last_os_error().into())
    } else {
        Ok(())
    }
}
