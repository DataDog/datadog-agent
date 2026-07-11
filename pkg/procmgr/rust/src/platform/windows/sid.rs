// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows SID lookup helpers shared by spawn and pipe ACL code.

use anyhow::{Result, bail};
use std::ptr;
use windows_sys::Win32::Security::Authorization::ConvertSidToStringSidW;
use windows_sys::Win32::Security::LookupAccountNameW;

use super::wide;

pub(crate) fn lookup_account_sid(domain: &str, user: &str) -> Result<Vec<u8>> {
    let account = if domain.is_empty() {
        user.to_string()
    } else {
        format!("{domain}\\{user}")
    };
    let system_w = wide::null_terminated("");
    let account_w = wide::null_terminated(&account);

    unsafe {
        let mut sid_size = 0u32;
        let mut domain_size = 0u32;
        let mut sid_type = 0i32;

        let _ = LookupAccountNameW(
            system_w.as_ptr(),
            account_w.as_ptr(),
            ptr::null_mut(),
            &mut sid_size,
            ptr::null_mut(),
            &mut domain_size,
            &mut sid_type,
        );

        let mut sid = vec![0u8; sid_size as usize];
        let mut _domain_buf = vec![0u16; domain_size as usize];
        let ok = LookupAccountNameW(
            system_w.as_ptr(),
            account_w.as_ptr(),
            sid.as_mut_ptr() as *mut _,
            &mut sid_size,
            _domain_buf.as_mut_ptr(),
            &mut domain_size,
            &mut sid_type,
        );
        if ok == 0 {
            bail!(
                "LookupAccountNameW({account}): {}",
                std::io::Error::last_os_error()
            );
        }
        sid.truncate(sid_size as usize);
        Ok(sid)
    }
}

pub(crate) fn sid_to_string(sid: &[u8]) -> Result<String> {
    unsafe {
        let mut sid_string: *mut u16 = ptr::null_mut();
        if ConvertSidToStringSidW(sid.as_ptr() as *mut _, &mut sid_string) == 0 {
            bail!(
                "ConvertSidToStringSidW: {}",
                std::io::Error::last_os_error()
            );
        }
        let sid_str = wide::from_ptr(sid_string);
        windows_sys::Win32::Foundation::LocalFree(sid_string as _);
        Ok(sid_str)
    }
}
