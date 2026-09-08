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
    PROC_THREAD_ATTRIBUTE_HANDLE_LIST, STARTF_USESTDHANDLES, STARTUPINFOEXW, STARTUPINFOW,
    UpdateProcThreadAttribute,
};

pub(crate) struct StartupInfoEx {
    siex: STARTUPINFOEXW,
    attribute_list_storage: Vec<u8>,
}

impl StartupInfoEx {
    pub(crate) fn with_stdio_handles(
        stdin: HANDLE,
        stdout: HANDLE,
        stderr: HANDLE,
    ) -> Result<Self> {
        let mut attribute_list_size = 0usize;
        unsafe {
            InitializeProcThreadAttributeList(
                ptr::null_mut(),
                1,
                0,
                &mut attribute_list_size,
            );
        }
        if std::io::Error::last_os_error().raw_os_error()
            != Some(ERROR_INSUFFICIENT_BUFFER as i32)
        {
            bail!(
                "InitializeProcThreadAttributeList sizing failed: {}",
                std::io::Error::last_os_error()
            );
        }

        let mut attribute_list_storage = vec![0u8; attribute_list_size];
        let attribute_list = attribute_list_storage.as_mut_ptr() as *mut std::ffi::c_void;
        let ok = unsafe {
            InitializeProcThreadAttributeList(attribute_list, 1, 0, &mut attribute_list_size)
        };
        if ok == 0 {
            bail!(
                "InitializeProcThreadAttributeList failed: {}",
                std::io::Error::last_os_error()
            );
        }

        let stdio_handles = [stdin, stdout, stderr];
        let ok = unsafe {
            UpdateProcThreadAttribute(
                attribute_list,
                0,
                PROC_THREAD_ATTRIBUTE_HANDLE_LIST as usize,
                stdio_handles.as_ptr().cast(),
                stdio_handles.len() * mem::size_of::<HANDLE>(),
                ptr::null_mut(),
                ptr::null_mut(),
            )
        };
        if ok == 0 {
            unsafe {
                DeleteProcThreadAttributeList(attribute_list);
            }
            bail!(
                "UpdateProcThreadAttribute(HANDLE_LIST) failed: {}",
                std::io::Error::last_os_error()
            );
        }

        let mut siex: STARTUPINFOEXW = unsafe { mem::zeroed() };
        siex.StartupInfo.cb = mem::size_of::<STARTUPINFOEXW>() as u32;
        siex.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
        siex.StartupInfo.hStdInput = stdin;
        siex.StartupInfo.hStdOutput = stdout;
        siex.StartupInfo.hStdError = stderr;
        siex.lpAttributeList = attribute_list;

        Ok(Self {
            siex,
            attribute_list_storage,
        })
    }

    pub(crate) fn startup_info(&mut self) -> &mut STARTUPINFOW {
        &mut self.siex.StartupInfo
    }
}

impl Drop for StartupInfoEx {
    fn drop(&mut self) {
        if !self.siex.lpAttributeList.is_null() {
            unsafe {
                DeleteProcThreadAttributeList(self.siex.lpAttributeList);
            }
            self.siex.lpAttributeList = ptr::null_mut();
        }
    }
}
