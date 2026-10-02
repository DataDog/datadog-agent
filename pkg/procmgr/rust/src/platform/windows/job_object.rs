// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::System::JobObjects::{
    CreateJobObjectW, JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, JOBOBJECT_EXTENDED_LIMIT_INFORMATION,
    JobObjectExtendedLimitInformation, SetInformationJobObject, TerminateJobObject,
};

#[cfg(test)]
use windows_sys::Win32::System::JobObjects::{
    JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, JobObjectBasicAccountingInformation,
    QueryInformationJobObject,
};

pub struct JobObject {
    handle: HANDLE,
    #[cfg(test)]
    pub(crate) faults: std::sync::Arc<TestFaults>,
}

#[cfg(test)]
pub(crate) struct TestFaults {
    pub(crate) query_error: std::sync::atomic::AtomicBool,
    pub(crate) terminate_error: std::sync::atomic::AtomicBool,
    pub(crate) terminate_calls: std::sync::atomic::AtomicUsize,
    pub(crate) active_count: std::sync::atomic::AtomicI64,
}

#[cfg(test)]
impl Default for TestFaults {
    fn default() -> Self {
        Self { query_error: false.into(), terminate_error: false.into(), terminate_calls: 0.into(), active_count: (-1).into() }
    }
}

// SAFETY: Win32 HANDLE is a plain pointer-sized value; the kernel serialises use per handle.
unsafe impl Send for JobObject {}
unsafe impl Sync for JobObject {}

impl JobObject {
    pub fn new() -> Result<Self> {
        unsafe {
            let handle = CreateJobObjectW(std::ptr::null(), std::ptr::null());
            if handle.is_null() {
                anyhow::bail!(
                    "CreateJobObjectW failed: {}",
                    std::io::Error::last_os_error()
                );
            }

            if let Err(err) = configure_kill_on_close(handle) {
                CloseHandle(handle);
                return Err(err);
            }

            Ok(Self { handle, #[cfg(test)] faults: TestFaults::default().into() })
        }
    }

    pub(crate) fn raw_handle(&self) -> HANDLE {
        self.handle
    }

    /// Accounting can lag process exit; the main-process watcher is not proof
    /// that the workload (including descendants) has drained.
    #[cfg(test)]
    pub(crate) fn active_process_count(&self) -> Result<u32> {
        #[cfg(test)]
        {
            use std::sync::atomic::Ordering;
            anyhow::ensure!(!self.faults.query_error.load(Ordering::SeqCst), "injected job query failure");
            let count = self.faults.active_count.load(Ordering::SeqCst);
            if count >= 0 { return Ok(count as u32); }
        }
        let mut info: JOBOBJECT_BASIC_ACCOUNTING_INFORMATION = unsafe { std::mem::zeroed() };
        let ok = unsafe {
            QueryInformationJobObject(
                self.handle,
                JobObjectBasicAccountingInformation,
                (&mut info as *mut JOBOBJECT_BASIC_ACCOUNTING_INFORMATION).cast(),
                std::mem::size_of_val(&info) as u32,
                std::ptr::null_mut(),
            )
        };
        if ok == 0 {
            anyhow::bail!("QueryInformationJobObject failed: {}", std::io::Error::last_os_error());
        }
        Ok(info.ActiveProcesses)
    }

    #[cfg(test)]
    pub(crate) async fn wait_for_drain(&self, deadline: tokio::time::Instant) -> Result<bool> {
        loop {
            if self.active_process_count()? == 0 {
                return Ok(true);
            }
            if tokio::time::Instant::now() >= deadline {
                return Ok(false);
            }
            tokio::time::sleep_until(
                (tokio::time::Instant::now() + std::time::Duration::from_millis(10)).min(deadline),
            ).await;
        }
    }

    pub fn terminate(&self) -> Result<()> {
        #[cfg(test)]
        self.faults.terminate_calls.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
        #[cfg(test)]
        anyhow::ensure!(!self.faults.terminate_error.load(std::sync::atomic::Ordering::SeqCst),
            "injected job termination failure");
        unsafe {
            let ok = TerminateJobObject(self.handle, 1);
            if ok == 0 {
                anyhow::bail!(
                    "TerminateJobObject failed: {}",
                    std::io::Error::last_os_error()
                );
            }
        }
        Ok(())
    }
}

fn configure_kill_on_close(handle: HANDLE) -> Result<()> {
    unsafe {
        let mut info: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = std::mem::zeroed();
        info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;

        let info_ptr = &info as *const JOBOBJECT_EXTENDED_LIMIT_INFORMATION;
        let ok = SetInformationJobObject(
            handle,
            JobObjectExtendedLimitInformation,
            info_ptr as *const std::ffi::c_void,
            std::mem::size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
        );
        if ok == 0 {
            anyhow::bail!(
                "SetInformationJobObject failed: {}",
                std::io::Error::last_os_error()
            );
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::os::windows::io::AsRawHandle;
    use std::time::Duration;
    use tokio::time::Instant;
    use windows_sys::Win32::System::JobObjects::AssignProcessToJobObject;

    #[tokio::test]
    async fn drainage_includes_descendants() {
        let job = JobObject::new().unwrap();
        // The shell creates a long-lived ordinary descendant after assignment.
        let mut child = std::process::Command::new("cmd.exe")
            .args(["/C", "ping -n 3 127.0.0.1 >nul & start /b ping -n 301 127.0.0.1 >nul"])
            .spawn().unwrap();
        assert_ne!(unsafe { AssignProcessToJobObject(job.raw_handle(), child.as_raw_handle()) }, 0);
        let deadline = Instant::now() + Duration::from_secs(5);
        while job.active_process_count().unwrap() < 2 {
            assert!(Instant::now() < deadline, "descendant did not join job");
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
        child.wait().unwrap();
        assert!(job.active_process_count().unwrap() > 0);
        assert!(!job.wait_for_drain(Instant::now() + Duration::from_millis(30)).await.unwrap());
        job.terminate().unwrap();
        assert!(job.wait_for_drain(Instant::now() + Duration::from_secs(5)).await.unwrap());
    }

    #[test]
    fn query_failure_is_not_empty() {
        // INVALID_HANDLE_VALUE, unlike NULL, does not query the calling process's job.
        let job = JobObject { handle: windows_sys::Win32::Foundation::INVALID_HANDLE_VALUE, faults: TestFaults::default().into() };
        assert!(job.active_process_count().is_err());
    }
}

impl Drop for JobObject {
    fn drop(&mut self) {
        unsafe {
            CloseHandle(self.handle);
        }
    }
}
