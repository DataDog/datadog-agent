// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::process::ExitStatus;

#[cfg(not(windows))]
use tokio::process::Child;

#[cfg(windows)]
use std::sync::atomic::{AtomicBool, Ordering};
#[cfg(windows)]
use std::sync::{Arc, Mutex};
#[cfg(windows)]
use windows_sys::Win32::Foundation::{CloseHandle, DUPLICATE_SAME_ACCESS, DuplicateHandle, HANDLE};
#[cfg(windows)]
use windows_sys::Win32::System::Threading::GetCurrentProcess;

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

    fn close(&mut self) {
        if !self.handle.is_null() {
            unsafe {
                CloseHandle(self.handle);
            }
            self.handle = std::ptr::null_mut();
        }
    }
}

#[cfg(windows)]
impl Drop for OwnedProcessHandle {
    fn drop(&mut self) {
        self.close();
    }
}

/// Shared duplicate process handle used for exit waits. Closing it unblocks an
/// in-flight `WaitForSingleObject` so shutdown can proceed without waiting for
/// a stuck child.
#[cfg(windows)]
pub(crate) struct ProcessWaitControl {
    wait_handle: Mutex<OwnedProcessHandle>,
    cancelled: AtomicBool,
}

#[cfg(windows)]
impl ProcessWaitControl {
    fn new(process_handle: HANDLE) -> Result<Arc<Self>> {
        Ok(Arc::new(Self {
            wait_handle: Mutex::new(OwnedProcessHandle {
                handle: duplicate_process_handle(process_handle)?,
            }),
            cancelled: AtomicBool::new(false),
        }))
    }

    pub(crate) fn cancel(&self) {
        self.cancelled.store(true, Ordering::Release);
        if let Ok(mut guard) = self.wait_handle.lock() {
            guard.close();
        }
    }

    pub(crate) fn is_cancelled(&self) -> bool {
        self.cancelled.load(Ordering::Acquire)
    }

    fn wait_handle_for_blocking(&self) -> Result<HANDLE> {
        let guard = self
            .wait_handle
            .lock()
            .map_err(|_| std::io::Error::other("process wait handle lock poisoned"))?;
        let handle = guard.get();
        if handle.is_null() {
            return Err(std::io::Error::other("process wait cancelled").into());
        }
        Ok(handle)
    }
}

#[cfg(windows)]
fn duplicate_process_handle(source: HANDLE) -> Result<HANDLE> {
    let mut duplicate: HANDLE = std::ptr::null_mut();
    let ok = unsafe {
        DuplicateHandle(
            GetCurrentProcess(),
            source,
            GetCurrentProcess(),
            &mut duplicate,
            0,
            0,
            DUPLICATE_SAME_ACCESS,
        )
    };
    if ok == 0 {
        return Err(std::io::Error::last_os_error().into());
    }
    Ok(duplicate)
}

pub struct ProcessHandle {
    #[cfg(not(windows))]
    child: Child,

    #[cfg(windows)]
    pid: u32,
    #[cfg(windows)]
    process_handle: OwnedProcessHandle,
    #[cfg(windows)]
    wait_control: Arc<ProcessWaitControl>,
}

impl ProcessHandle {
    #[cfg(not(windows))]
    pub fn from_child(child: Child) -> Self {
        Self { child }
    }

    #[cfg(windows)]
    pub fn from_raw(pid: u32, process_handle: HANDLE) -> Result<Self> {
        let wait_control = ProcessWaitControl::new(process_handle)?;
        Ok(Self {
            pid,
            process_handle: OwnedProcessHandle {
                handle: process_handle,
            },
            wait_control,
        })
    }

    #[cfg(windows)]
    pub(crate) fn wait_control(&self) -> Arc<ProcessWaitControl> {
        Arc::clone(&self.wait_control)
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
            raw_wait_exit_code(Arc::clone(&self.wait_control)).await
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
async fn raw_wait_exit_code(wait_control: Arc<ProcessWaitControl>) -> Result<ExitStatus> {
    use std::os::windows::process::ExitStatusExt;
    use windows_sys::Win32::System::Threading::{
        GetExitCodeProcess, INFINITE, WaitForSingleObject,
    };

    const WAIT_OBJECT_0: u32 = 0;
    const WAIT_FAILED: u32 = 0xFFFF_FFFF;

    let wc = Arc::clone(&wait_control);
    let exit_code = tokio::task::spawn_blocking(move || -> Result<u32> {
        let process_handle = wc.wait_handle_for_blocking()?;
        let wait_result = unsafe { WaitForSingleObject(process_handle, INFINITE) };
        if wait_result == WAIT_FAILED {
            if wc.is_cancelled() {
                return Err(std::io::Error::other("process wait cancelled").into());
            }
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
