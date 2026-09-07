// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ptr;
use std::sync::atomic::{Ordering, compiler_fence};

use anyhow::{Result, bail};
use windows_sys::Win32::Security::Authentication::Identity::{
    LSA_HANDLE, LSA_OBJECT_ATTRIBUTES, LSA_UNICODE_STRING, LsaClose, LsaFreeMemory, LsaOpenPolicy,
    LsaRetrievePrivateData, POLICY_GET_PRIVATE_INFORMATION,
};

/// Keep in sync with MSI `ConfigureUserCustomActions.AgentPasswordPrivateDataKey`
/// and fleet `agentPasswordPrivateDataKey`.
pub(crate) const INSTALLER_AGENT_PASSWORD_LSA_KEY: &str = "L$datadog_ddagentuser_password";

const STATUS_OBJECT_NAME_NOT_FOUND: i32 = 0xC000_0034u32 as i32;

/// Read the ddagentuser password stored by the 7.66+ installer in LSA.
///
/// Requires `POLICY_GET_PRIVATE_INFORMATION` (LocalSystem / administrators). Not
/// available to ddagentuser; callers on the supervisor-inherit path must not use this.
pub(crate) fn read_installer_agent_password() -> Result<Option<String>> {
    let mut key_w = super::wide::null_terminated(INSTALLER_AGENT_PASSWORD_LSA_KEY);
    let key_name = lsa_unicode_string(&mut key_w);

    let mut object_attributes: LSA_OBJECT_ATTRIBUTES = unsafe { std::mem::zeroed() };
    let mut policy_handle: LSA_HANDLE = 0;
    let status = unsafe {
        LsaOpenPolicy(
            ptr::null(),
            &mut object_attributes,
            POLICY_GET_PRIVATE_INFORMATION as u32,
            &mut policy_handle,
        )
    };
    if status != 0 {
        bail!("LsaOpenPolicy: NTSTATUS {status:#010x}");
    }
    let policy = PolicyHandle {
        handle: policy_handle,
    };

    let mut secret = ptr::null_mut();
    let status = unsafe { LsaRetrievePrivateData(policy.handle, &key_name, &mut secret) };
    if status == STATUS_OBJECT_NAME_NOT_FOUND {
        return Ok(None);
    }
    if status != 0 {
        bail!(
            "LsaRetrievePrivateData({INSTALLER_AGENT_PASSWORD_LSA_KEY}): NTSTATUS {status:#010x}"
        );
    }

    Ok(LsaSecret { data: secret }.into_password())
}

/// `Length` / `MaximumLength` are byte counts, not UTF-16 units.
fn lsa_unicode_string(wide: &mut [u16]) -> LSA_UNICODE_STRING {
    let char_count = wide.len().saturating_sub(1);
    LSA_UNICODE_STRING {
        Length: (char_count * 2) as u16,
        MaximumLength: (wide.len() * 2) as u16,
        Buffer: wide.as_mut_ptr(),
    }
}

/// LSA-owned private data. Drop zeros the password bytes, then `LsaFreeMemory`.
///
/// Matches fleet `retrieve_private_data` cleanup in
/// `pkg/fleet/installer/packages/user/windows/lsa.c`.
struct LsaSecret {
    data: *mut LSA_UNICODE_STRING,
}

impl LsaSecret {
    fn into_password(self) -> Option<String> {
        if self.data.is_null() {
            return None;
        }
        unsafe {
            let secret = &*self.data;
            if secret.Buffer.is_null() || secret.Length == 0 {
                return None;
            }
            let char_count = secret.Length as usize / 2;
            let slice = std::slice::from_raw_parts(secret.Buffer, char_count);
            Some(String::from_utf16_lossy(slice))
        }
    }
}

impl Drop for LsaSecret {
    fn drop(&mut self) {
        if self.data.is_null() {
            return;
        }
        unsafe {
            let secret = &*self.data;
            if !secret.Buffer.is_null() && secret.Length > 0 {
                secure_zero(secret.Buffer.cast(), secret.Length as usize);
            }
            LsaFreeMemory(self.data.cast());
            self.data = ptr::null_mut();
        }
    }
}

fn secure_zero(ptr: *mut u8, len: usize) {
    for i in 0..len {
        unsafe {
            ptr::write_volatile(ptr.add(i), 0);
        }
    }
    compiler_fence(Ordering::SeqCst);
}

struct PolicyHandle {
    handle: LSA_HANDLE,
}

impl Drop for PolicyHandle {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                LsaClose(self.handle);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn installer_agent_password_lsa_key_matches_msi() {
        assert_eq!(
            INSTALLER_AGENT_PASSWORD_LSA_KEY,
            "L$datadog_ddagentuser_password"
        );
    }
}
