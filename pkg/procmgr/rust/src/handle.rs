// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::process::ExitStatus;

#[cfg(not(windows))]
use tokio::process::Child;

#[cfg(windows)]
use tokio::process::Child as TokioChild;

#[cfg(windows)]
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};

/// Platform-agnostic process handle for procmgr supervision.
///
/// On Unix we wrap `tokio::process::Child`.
/// On Windows we also support a raw process handle path for token-based
/// spawning (e.g. `CreateProcessAsUserW`), while preserving the same
/// `wait`/`kill` interface for the supervisor.
pub struct ProcessHandle {
    #[cfg(not(windows))]
    child: Child,

    #[cfg(windows)]
    inner: ProcessHandleInner,
}

#[cfg(windows)]
enum ProcessHandleInner {
    Tokio(TokioChild),
    Raw { pid: u32, process_handle: HANDLE },
}

#[cfg(windows)]
impl Drop for ProcessHandleInner {
    fn drop(&mut self) {
        if let ProcessHandleInner::Raw { process_handle, .. } = self {
            if !process_handle.is_null() {
                unsafe {
                    CloseHandle(*process_handle);
                }
            }
        }
    }
}

impl ProcessHandle {
    #[cfg(not(windows))]
    pub fn from_child(child: Child) -> Self {
        Self { child }
    }

    #[cfg(windows)]
    pub fn from_child(child: TokioChild) -> Self {
        Self {
            inner: ProcessHandleInner::Tokio(child),
        }
    }

    #[cfg(windows)]
    pub fn from_raw(pid: u32, process_handle: HANDLE) -> Self {
        Self {
            inner: ProcessHandleInner::Raw {
                pid,
                process_handle,
            },
        }
    }

    pub fn id(&self) -> Option<u32> {
        #[cfg(not(windows))]
        {
            self.child.id()
        }
        #[cfg(windows)]
        match &self.inner {
            ProcessHandleInner::Tokio(child) => child.id(),
            ProcessHandleInner::Raw { pid, .. } => Some(*pid),
        }
    }

    pub async fn wait(&mut self) -> Result<ExitStatus> {
        #[cfg(not(windows))]
        {
            Ok(self.child.wait().await?)
        }
        #[cfg(windows)]
        {
            match &mut self.inner {
                ProcessHandleInner::Tokio(child) => Ok(child.wait().await?),
                ProcessHandleInner::Raw { process_handle, .. } => {
                    raw_wait_exit_code(*process_handle).await
                }
            }
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
            match &mut self.inner {
                ProcessHandleInner::Tokio(child) => {
                    child.kill().await?;
                    Ok(())
                }
                ProcessHandleInner::Raw { process_handle, .. } => {
                    raw_terminate_process(*process_handle)
                }
            }
        }
    }
}

#[cfg(windows)]
async fn raw_wait_exit_code(process_handle: HANDLE) -> Result<ExitStatus> {
    use std::ffi::c_void;

    use tokio::sync::oneshot;
    use windows_sys::Win32::Foundation::INVALID_HANDLE_VALUE;
    use windows_sys::Win32::System::Threading::GetExitCodeProcess;
    use windows_sys::Win32::System::Threading::{
        INFINITE, RegisterWaitForSingleObject, UnregisterWaitEx, WT_EXECUTEINWAITTHREAD,
        WT_EXECUTEONLYONCE,
    };

    use std::os::windows::process::ExitStatusExt;

    struct WaitData {
        tx: Option<oneshot::Sender<u32>>,
        process_handle: HANDLE,
    }

    unsafe extern "system" fn callback(ptr: *mut c_void, _timed_out: bool) {
        let data = &mut *(ptr as *mut WaitData);
        if let Some(tx) = data.tx.take() {
            let mut exit_code: u32 = 0;
            let ok = GetExitCodeProcess(data.process_handle, &mut exit_code);
            // Even if GetExitCodeProcess fails, propagate a sentinel code.
            let _ = tx.send(if ok != 0 { exit_code } else { u32::MAX });
        }
    }

    let (tx, rx) = oneshot::channel::<u32>();
    let mut wait_object: HANDLE = std::ptr::null_mut();
    let boxed = Box::new(WaitData {
        tx: Some(tx),
        process_handle,
    });
    let wait_data_ptr = Box::into_raw(boxed);

    let ok = unsafe {
        RegisterWaitForSingleObject(
            &mut wait_object,
            process_handle,
            Some(callback),
            wait_data_ptr as *mut c_void,
            INFINITE,
            WT_EXECUTEINWAITTHREAD | WT_EXECUTEONLYONCE,
        )
    };
    if ok == 0 {
        unsafe {
            drop(Box::from_raw(wait_data_ptr));
        }
        return Err(std::io::Error::last_os_error().into());
    }

    struct Guard {
        wait_object: HANDLE,
        ptr: *mut WaitData,
    }
    impl Drop for Guard {
        fn drop(&mut self) {
            unsafe {
                // Best-effort: prevent callback from running after cancellation.
                if !self.wait_object.is_null() {
                    let _ = UnregisterWaitEx(self.wait_object, INVALID_HANDLE_VALUE);
                }
                drop(Box::from_raw(self.ptr));
            }
        }
    }
    let mut guard = Guard {
        wait_object,
        ptr: wait_data_ptr as *mut WaitData,
    };

    let exit_code = rx
        .await
        .map_err(|e| anyhow::anyhow!("wait cancelled: {e}"))?;
    drop(guard);
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
