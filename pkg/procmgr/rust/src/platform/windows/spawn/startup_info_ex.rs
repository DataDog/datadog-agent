// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::mem;
use std::ptr;

use anyhow::{Result, bail};
use windows_sys::Win32::Foundation::{ERROR_INSUFFICIENT_BUFFER, HANDLE};
use windows_sys::Win32::System::Threading::{
    DeleteProcThreadAttributeList, InitializeProcThreadAttributeList,
    PROC_THREAD_ATTRIBUTE_HANDLE_LIST, PROC_THREAD_ATTRIBUTE_JOB_LIST, STARTF_USESTDHANDLES,
    STARTUPINFOEXW, STARTUPINFOW, UpdateProcThreadAttribute,
};

/// Extended `STARTUPINFO` used by `CreateProcessW` / `CreateProcessAsUserW`.
///
/// If you are not familiar with Windows: `STARTUPINFOEX` carries extra create-time attributes
/// (which stdio HANDLEs to inherit, which job object to join). Attribute pointers must remain
/// valid until `CreateProcess*` returns, so this type owns those arrays.
pub(crate) struct StartupInfoEx {
    siex: STARTUPINFOEXW,
    attribute_list_storage: Vec<u8>,
    stdio_handles: [HANDLE; 3],
    job_handles: [HANDLE; 1],
}

impl StartupInfoEx {
    /// Builds startup info with stdio inheritance and job membership at create time.
    pub(crate) fn with_stdio_and_job(
        stdin: HANDLE,
        stdout: HANDLE,
        stderr: HANDLE,
        job: HANDLE,
    ) -> Result<Self> {
        const ATTRIBUTE_COUNT: u32 = 2;
        let mut attribute_list_size = 0usize;
        unsafe {
            InitializeProcThreadAttributeList(
                ptr::null_mut(),
                ATTRIBUTE_COUNT,
                0,
                &mut attribute_list_size,
            );
        }
        // Sizing probe: this call is expected to fail with ERROR_INSUFFICIENT_BUFFER.
        if std::io::Error::last_os_error().raw_os_error() != Some(ERROR_INSUFFICIENT_BUFFER as i32)
        {
            bail!(
                "InitializeProcThreadAttributeList sizing failed: {}",
                std::io::Error::last_os_error()
            );
        }

        let mut attribute_list_storage = vec![0u8; attribute_list_size];
        let attribute_list = attribute_list_storage.as_mut_ptr() as *mut std::ffi::c_void;
        let ok = unsafe {
            InitializeProcThreadAttributeList(
                attribute_list,
                ATTRIBUTE_COUNT,
                0,
                &mut attribute_list_size,
            )
        };
        if ok == 0 {
            bail!(
                "InitializeProcThreadAttributeList failed: {}",
                std::io::Error::last_os_error()
            );
        }

        let mut startup = Self {
            siex: new_siex(stdin, stdout, stderr, attribute_list),
            attribute_list_storage,
            stdio_handles: [stdin, stdout, stderr],
            job_handles: [job],
        };
        // JOB_LIST before HANDLE_LIST: assign supervision job before restricting inheritance.
        startup.attach_job_list()?;
        startup.attach_stdio_handle_list()?;
        Ok(startup)
    }

    fn attach_stdio_handle_list(&mut self) -> Result<()> {
        update_attribute(
            self.siex.lpAttributeList,
            PROC_THREAD_ATTRIBUTE_HANDLE_LIST as usize,
            self.stdio_handles.as_ptr().cast(),
            self.stdio_handles.len() * mem::size_of::<HANDLE>(),
            "HANDLE_LIST",
        )
    }

    fn attach_job_list(&mut self) -> Result<()> {
        update_attribute(
            self.siex.lpAttributeList,
            PROC_THREAD_ATTRIBUTE_JOB_LIST as usize,
            self.job_handles.as_ptr().cast(),
            self.job_handles.len() * mem::size_of::<HANDLE>(),
            "JOB_LIST",
        )
    }

    pub(crate) fn startup_info(&mut self) -> &mut STARTUPINFOW {
        &mut self.siex.StartupInfo
    }
}

fn new_siex(
    stdin: HANDLE,
    stdout: HANDLE,
    stderr: HANDLE,
    attribute_list: *mut std::ffi::c_void,
) -> STARTUPINFOEXW {
    let mut siex: STARTUPINFOEXW = unsafe { mem::zeroed() };
    siex.StartupInfo.cb = mem::size_of::<STARTUPINFOEXW>() as u32;
    siex.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
    siex.StartupInfo.hStdInput = stdin;
    siex.StartupInfo.hStdOutput = stdout;
    siex.StartupInfo.hStdError = stderr;
    siex.lpAttributeList = attribute_list;
    siex
}

fn update_attribute(
    attribute_list: *mut std::ffi::c_void,
    attribute: usize,
    value: *const std::ffi::c_void,
    size: usize,
    label: &str,
) -> Result<()> {
    let ok = unsafe {
        UpdateProcThreadAttribute(
            attribute_list,
            0,
            attribute,
            value,
            size,
            ptr::null_mut(),
            ptr::null_mut(),
        )
    };
    if ok == 0 {
        bail!(
            "UpdateProcThreadAttribute({label}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(())
}

impl Drop for StartupInfoEx {
    fn drop(&mut self) {
        if !self.siex.lpAttributeList.is_null() {
            // attribute_list_storage owns the buffer lpAttributeList points at
            let _ = &self.attribute_list_storage;
            unsafe {
                DeleteProcThreadAttributeList(self.siex.lpAttributeList);
            }
            self.siex.lpAttributeList = ptr::null_mut();
        }
    }
}
