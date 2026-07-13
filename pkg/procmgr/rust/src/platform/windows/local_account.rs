// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Local-vs-domain account detection for installer-configured agent users.
//!
//! Mirrors `pkg/fleet/installer/packages/user/windows.IsLocalAccount`.

use anyhow::{Context, Result, bail};
use std::io::ErrorKind;
use std::ptr;
use windows_sys::Win32::Foundation::{ERROR_INSUFFICIENT_BUFFER, WIN32_ERROR};
use windows_sys::Win32::Security::Authorization::GetWindowsAccountDomainSid;
use windows_sys::Win32::Security::EqualSid;
use windows_sys::Win32::System::SystemInformation::GetComputerNameW;

use super::sid::lookup_account_sid;
use super::wide;

const ERROR_NON_ACCOUNT_SID: WIN32_ERROR = 1257;
const ERROR_NON_DOMAIN_SID: WIN32_ERROR = 1260;

/// Returns true when `sid` belongs to the local machine account database.
pub(crate) fn is_local_account(sid: &[u8]) -> Result<bool> {
    let account_domain_sid = match account_domain_sid(sid) {
        Ok(sid) => sid,
        Err(err) if is_non_account_or_domain_sid(&err) => return Ok(false),
        Err(err) => return Err(err),
    };
    let host_sid = computer_sid()?;
    Ok(sids_equal(&account_domain_sid, &host_sid))
}

fn account_domain_sid(sid: &[u8]) -> Result<Vec<u8>> {
    unsafe {
        let mut domain_sid_size = 0u32;
        let ok = GetWindowsAccountDomainSid(
            sid.as_ptr() as *mut _,
            ptr::null_mut(),
            &mut domain_sid_size,
        );
        if ok == 0 {
            let err = std::io::Error::last_os_error();
            if err.raw_os_error() != Some(ERROR_INSUFFICIENT_BUFFER as i32) {
                return Err(err).context("GetWindowsAccountDomainSid(size)");
            }
        }

        let mut domain_sid = vec![0u8; domain_sid_size as usize];
        let ok = GetWindowsAccountDomainSid(
            sid.as_ptr() as *mut _,
            domain_sid.as_mut_ptr() as *mut _,
            &mut domain_sid_size,
        );
        if ok == 0 {
            return Err(std::io::Error::last_os_error()).context("GetWindowsAccountDomainSid");
        }
        domain_sid.truncate(domain_sid_size as usize);
        Ok(domain_sid)
    }
}

fn computer_sid() -> Result<Vec<u8>> {
    let computer_name = computer_name()?;
    lookup_account_sid("", &computer_name)
        .with_context(|| format!("lookup SID for {computer_name}"))
}

fn computer_name() -> Result<String> {
    let mut buffer = [0u16; 16];
    let mut size = buffer.len() as u32;
    let ok = unsafe { GetComputerNameW(buffer.as_mut_ptr(), &mut size) };
    if ok == 0 {
        bail!("GetComputerNameW: {}", std::io::Error::last_os_error());
    }
    Ok(wide::from_ptr(buffer.as_ptr()))
}

fn sids_equal(left: &[u8], right: &[u8]) -> bool {
    unsafe { EqualSid(left.as_ptr() as *mut _, right.as_ptr() as *mut _) != 0 }
}

fn is_non_account_or_domain_sid(err: &anyhow::Error) -> bool {
    err.chain().any(|cause| {
        cause
            .downcast_ref::<std::io::Error>()
            .and_then(|io_err| io_err.raw_os_error())
            .is_some_and(|code| {
                code == ERROR_NON_ACCOUNT_SID as i32 || code == ERROR_NON_DOMAIN_SID as i32
            })
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn non_account_sid_errors_are_recognized() {
        let err = std::io::Error::from_raw_os_error(ERROR_NON_ACCOUNT_SID as i32);
        assert!(is_non_account_or_domain_sid(
            &anyhow::Error::new(err).context("GetWindowsAccountDomainSid")
        ));
    }

    #[test]
    fn unrelated_io_errors_are_not_recognized() {
        let err = std::io::Error::new(ErrorKind::Other, "nope");
        assert!(!is_non_account_or_domain_sid(&anyhow::Error::new(err)));
    }
}
