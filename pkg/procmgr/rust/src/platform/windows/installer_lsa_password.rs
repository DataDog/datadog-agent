// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ptr;

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
    read_lsa_private_data(INSTALLER_AGENT_PASSWORD_LSA_KEY)
}

fn read_lsa_private_data(key: &str) -> Result<Option<String>> {
    let mut key_w = super::wide::null_terminated(key);
    let key_len = key_w.len().saturating_sub(1);
    let key_name = LSA_UNICODE_STRING {
        Length: (key_len * 2) as u16,
        MaximumLength: (key_w.len() * 2) as u16,
        Buffer: key_w.as_mut_ptr(),
    };

    unsafe {
        let object_attributes: LSA_OBJECT_ATTRIBUTES = std::mem::zeroed();
        let mut policy_handle: LSA_HANDLE = 0;

        let status = LsaOpenPolicy(
            ptr::null(),
            &object_attributes,
            POLICY_GET_PRIVATE_INFORMATION as u32,
            &mut policy_handle,
        );
        if status != 0 {
            bail!("LsaOpenPolicy: NTSTATUS {status:#010x}");
        }

        let policy = PolicyHandle(policy_handle);
        let mut secret: *mut LSA_UNICODE_STRING = ptr::null_mut();
        let status = LsaRetrievePrivateData(policy.0, &key_name, &mut secret);

        if status == STATUS_OBJECT_NAME_NOT_FOUND {
            return Ok(None);
        }
        if status != 0 {
            bail!("LsaRetrievePrivateData({key}): NTSTATUS {status:#010x}");
        }
        if secret.is_null() {
            return Ok(None);
        }

        let secret_ref = &*secret;
        let char_count = secret_ref.Length as usize / 2;
        let password = if char_count == 0 {
            String::new()
        } else {
            let slice = std::slice::from_raw_parts(secret_ref.Buffer, char_count);
            String::from_utf16_lossy(slice)
        };

        LsaFreeMemory(secret as _);
        Ok(Some(password))
    }
}

struct PolicyHandle(LSA_HANDLE);

impl Drop for PolicyHandle {
    fn drop(&mut self) {
        if self.0 != 0 {
            unsafe {
                LsaClose(self.0);
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
