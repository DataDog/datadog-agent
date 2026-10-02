// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#[cfg(windows)]
use crate::platform::{
    ProcessWaitOutcome, WAIT_INFINITE, terminate_process, wait_for_process_exit_ms,
};

use anyhow::Result;
use std::process::ExitStatus;

#[cfg(not(windows))]
use tokio::process::Child;

#[cfg(windows)]
use std::sync::Arc;
#[cfg(windows)]
use windows_sys::Win32::Foundation::{CloseHandle, DUPLICATE_SAME_ACCESS, DuplicateHandle, HANDLE};
#[cfg(windows)]
use windows_sys::Win32::System::Threading::{
    GetCurrentProcess, PROCESS_QUERY_LIMITED_INFORMATION, PROCESS_SYNCHRONIZE,
};

#[cfg(windows)]
pub(crate) struct OwnedProcessHandle {
    handle: HANDLE,
}

#[cfg(windows)]
unsafe impl Send for OwnedProcessHandle {}

#[cfg(windows)]
unsafe impl Sync for OwnedProcessHandle {}

#[cfg(windows)]
impl OwnedProcessHandle {
    /// Takes ownership of a valid kernel handle.
    pub(crate) unsafe fn from_raw(handle: HANDLE) -> Self {
        Self { handle }
    }

    pub(crate) fn get(&self) -> HANDLE {
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

#[cfg(windows)]
struct ProcessWaitHandle(OwnedProcessHandle);

#[cfg(windows)]
impl ProcessWaitHandle {
    fn new(process_handle: HANDLE) -> Result<Arc<Self>> {
        let wait_handle = match duplicate_process_handle(process_handle) {
            Ok(handle) => handle,
            Err(e) => {
                unsafe {
                    CloseHandle(process_handle);
                }
                return Err(e);
            }
        };
        Ok(Arc::new(Self(OwnedProcessHandle {
            handle: wait_handle,
        })))
    }

    fn raw(&self) -> HANDLE {
        self.0.get()
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

/// Shutdown's reference to the original kernel process object, independent of
/// the exit watcher. It must never be reconstructed from a (reusable) PID.
#[cfg(windows)]
#[derive(Clone)]
pub(crate) struct RetainedProcessHandle(Arc<OwnedProcessHandle>);

#[cfg(windows)]
impl RetainedProcessHandle {
    pub(crate) fn raw(&self) -> HANDLE {
        self.0.get()
    }

    /// Only this restricted duplicate is allowed to cross into the helper.
    pub(crate) fn duplicate_for_helper(&self) -> Result<OwnedProcessHandle> {
        let mut handle = std::ptr::null_mut();
        let ok = unsafe {
            DuplicateHandle(
                GetCurrentProcess(),
                self.raw(),
                GetCurrentProcess(),
                &mut handle,
                PROCESS_SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION,
                1,
                0,
            )
        };
        if ok == 0 {
            return Err(std::io::Error::last_os_error().into());
        }
        Ok(OwnedProcessHandle { handle })
    }

    pub(crate) fn terminate(&self) -> Result<()> {
        terminate_process(self.raw())
    }
}

pub(crate) struct ProcessHandle {
    #[cfg(not(windows))]
    child: Child,

    #[cfg(windows)]
    pid: u32,
    #[cfg(windows)]
    process_handle: OwnedProcessHandle,
    #[cfg(windows)]
    wait_handle: Arc<ProcessWaitHandle>,
}

impl ProcessHandle {
    #[cfg(not(windows))]
    pub(crate) fn from_child(child: Child) -> Self {
        Self { child }
    }

    #[cfg(windows)]
    pub(crate) fn from_borrowed(pid: u32, source: HANDLE) -> Result<Self> {
        let process_handle = duplicate_process_handle(source)?;
        let wait_handle = ProcessWaitHandle::new(process_handle)?;
        Ok(Self {
            pid,
            process_handle: OwnedProcessHandle {
                handle: process_handle,
            },
            wait_handle,
        })
    }

    #[cfg(windows)]
    pub(crate) fn retain_for_shutdown(&self) -> Result<RetainedProcessHandle> {
        Ok(RetainedProcessHandle(Arc::new(OwnedProcessHandle {
            handle: duplicate_process_handle(self.process_handle.get())?,
        })))
    }

    pub(crate) fn id(&self) -> Option<u32> {
        #[cfg(not(windows))]
        {
            self.child.id()
        }
        #[cfg(windows)]
        {
            Some(self.pid)
        }
    }

    pub(crate) async fn wait(&mut self) -> Result<ExitStatus> {
        #[cfg(not(windows))]
        {
            Ok(self.child.wait().await?)
        }
        #[cfg(windows)]
        {
            raw_wait_exit_code(Arc::clone(&self.wait_handle)).await
        }
    }

    pub(crate) async fn kill(&mut self) -> Result<()> {
        #[cfg(not(windows))]
        {
            self.child.kill().await?;
            Ok(())
        }
        #[cfg(windows)]
        {
            terminate_process(self.process_handle.get())
        }
    }
}

#[cfg(windows)]
async fn raw_wait_exit_code(wait_handle: Arc<ProcessWaitHandle>) -> Result<ExitStatus> {
    use std::os::windows::process::ExitStatusExt;

    let exit_code = tokio::task::spawn_blocking(move || -> Result<u32> {
        match wait_for_process_exit_ms(wait_handle.raw(), WAIT_INFINITE) {
            Ok(ProcessWaitOutcome::Exited(code)) => Ok(code),
            Ok(ProcessWaitOutcome::TimedOut) => {
                Err(std::io::Error::other("WaitForSingleObject(INFINITE) timed out").into())
            }
            Err(e) => Err(e),
        }
    })
    .await??;

    Ok(ExitStatus::from_raw(exit_code))
}

#[cfg(all(test, windows))]
mod tests {
    use super::*;
    use std::os::windows::io::AsRawHandle;
    use windows_sys::Win32::System::Threading::GetProcessId;

    #[tokio::test]
    async fn retained_identity_survives_exit_watcher() {
        let mut child = std::process::Command::new("cmd.exe")
            .args(["/C", "exit 0"])
            .spawn().unwrap();
        let pid = child.id();
        let mut watcher = ProcessHandle::from_borrowed(pid, child.as_raw_handle()).unwrap();
        let retained = watcher.retain_for_shutdown().unwrap();
        watcher.wait().await.unwrap();
        drop(watcher);
        child.wait().unwrap();
        drop(child);
        assert_eq!(unsafe { GetProcessId(retained.raw()) }, pid);
        assert!(matches!(wait_for_process_exit_ms(retained.raw(), 0).unwrap(),
            ProcessWaitOutcome::Exited(0)));
        let restricted = retained.duplicate_for_helper().unwrap();
        assert_eq!(unsafe { GetProcessId(restricted.get()) }, pid);
        // The helper cannot terminate the original process through its handle.
        assert!(terminate_process(restricted.get()).is_err());
    }
}
