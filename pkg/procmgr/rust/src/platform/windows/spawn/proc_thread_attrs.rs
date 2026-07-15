// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! RAII wrapper for `PROC_THREAD_ATTRIBUTE_LIST` used with `STARTUPINFOEXW`.

use anyhow::{Result, bail};
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::System::Threading::{
    DeleteProcThreadAttributeList, InitializeProcThreadAttributeList, LPPROC_THREAD_ATTRIBUTE_LIST,
    PROC_THREAD_ATTRIBUTE_JOB_LIST, UpdateProcThreadAttribute,
};

/// Owned proc-thread attribute list with a single `PROC_THREAD_ATTRIBUTE_JOB_LIST` entry.
pub(super) struct ProcThreadAttributeList {
    // Backing storage for `list`; must outlive the attribute list pointer.
    #[allow(dead_code)]
    buffer: Vec<u8>,
    list: LPPROC_THREAD_ATTRIBUTE_LIST,
}

impl ProcThreadAttributeList {
    /// Build an attribute list that assigns `job` to the child at creation time.
    pub(super) fn with_job(job: HANDLE) -> Result<Self> {
        let mut size = 0usize;
        // First call returns FALSE and sets `size` to the required buffer length.
        let _ = unsafe { InitializeProcThreadAttributeList(std::ptr::null_mut(), 1, 0, &mut size) };
        if size == 0 {
            bail!(
                "InitializeProcThreadAttributeList(size query) failed: {}",
                std::io::Error::last_os_error()
            );
        }

        let mut buffer = vec![0u8; size];
        let list = buffer.as_mut_ptr().cast();
        let ok = unsafe { InitializeProcThreadAttributeList(list, 1, 0, &mut size) };
        if ok == 0 {
            bail!(
                "InitializeProcThreadAttributeList failed: {}",
                std::io::Error::last_os_error()
            );
        }

        let job_list = [job];
        let ok = unsafe {
            UpdateProcThreadAttribute(
                list,
                0,
                PROC_THREAD_ATTRIBUTE_JOB_LIST as usize,
                job_list.as_ptr().cast(),
                std::mem::size_of_val(&job_list),
                std::ptr::null_mut(),
                std::ptr::null(),
            )
        };
        if ok == 0 {
            unsafe {
                DeleteProcThreadAttributeList(list);
            }
            bail!(
                "UpdateProcThreadAttribute(PROC_THREAD_ATTRIBUTE_JOB_LIST) failed: {}",
                std::io::Error::last_os_error()
            );
        }

        Ok(Self { buffer, list })
    }

    pub(super) fn as_ptr(&self) -> LPPROC_THREAD_ATTRIBUTE_LIST {
        self.list
    }
}

impl Drop for ProcThreadAttributeList {
    fn drop(&mut self) {
        unsafe {
            DeleteProcThreadAttributeList(self.list);
        }
    }
}
