// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::System::JobObjects::{
    AssignProcessToJobObject, CreateJobObjectW, JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION, JobObjectExtendedLimitInformation,
    SetInformationJobObject, TerminateJobObject,
};
use windows_sys::Win32::System::Threading::{OpenProcess, PROCESS_SET_QUOTA, PROCESS_TERMINATE};

pub struct JobObject {
    handle: HANDLE,
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

            Ok(Self { handle })
        }
    }

    pub fn assign_process(&self, pid: u32) -> Result<()> {
        unsafe {
            let proc_handle = OpenProcess(PROCESS_SET_QUOTA | PROCESS_TERMINATE, 0, pid);
            if proc_handle.is_null() {
                anyhow::bail!(
                    "OpenProcess({pid}) for job assignment failed: {}",
                    std::io::Error::last_os_error()
                );
            }
            let ok = AssignProcessToJobObject(self.handle, proc_handle);
            CloseHandle(proc_handle);
            if ok == 0 {
                anyhow::bail!(
                    "AssignProcessToJobObject({pid}) failed: {}",
                    std::io::Error::last_os_error()
                );
            }
        }
        Ok(())
    }

    pub fn terminate(&self) -> Result<()> {
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

impl Drop for JobObject {
    fn drop(&mut self) {
        unsafe {
            CloseHandle(self.handle);
        }
    }
}
