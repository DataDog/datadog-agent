// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Read SCM-stored service account passwords from the LSA secret store.
//!
//! Windows stores service passwords under `_SC_<ServiceName>`. Even LocalSystem
//! receives `STATUS_ACCESS_DENIED` from `LsaRetrievePrivateData` on that prefix,
//! so copy the secret to a temporary non-`_SC_` name first (same approach as
//! common service-credential tooling).

use std::ptr;

use anyhow::{Context, Result, bail};
use uuid::Uuid;
use windows_sys::Win32::Foundation::ERROR_SUCCESS;
use windows_sys::Win32::Security::Authentication::Identity::{
    LSA_HANDLE, LSA_OBJECT_ATTRIBUTES, LSA_UNICODE_STRING, LsaClose, LsaFreeMemory, LsaOpenPolicy,
    LsaRetrievePrivateData, POLICY_GET_PRIVATE_INFORMATION,
};
use windows_sys::Win32::System::Registry::{
    HKEY, HKEY_LOCAL_MACHINE, KEY_READ, KEY_WRITE, REG_OPTION_NON_VOLATILE, RegCloseKey,
    RegCopyTreeW, RegCreateKeyExW, RegDeleteTreeW, RegOpenKeyExW,
};

use super::wide;

const LSA_SECRETS_KEY: &str = r"SECURITY\Policy\Secrets";
const SCM_SECRET_PREFIX: &str = "_SC_";
const STATUS_OBJECT_NAME_NOT_FOUND: i32 = 0xC000_0034u32 as i32;

struct RegistryKey(HKEY);

impl RegistryKey {
    fn open(local_machine: HKEY, subkey: &str, access: u32) -> Result<Self> {
        let subkey_w = wide::null_terminated(subkey);
        let mut handle = ptr::null_mut();
        let status =
            unsafe { RegOpenKeyExW(local_machine, subkey_w.as_ptr(), 0, access, &mut handle) };
        if status != ERROR_SUCCESS {
            bail!("RegOpenKeyExW({subkey}): win32 {status}");
        }
        Ok(Self(handle))
    }

    fn create(local_machine: HKEY, subkey: &str) -> Result<Self> {
        let subkey_w = wide::null_terminated(subkey);
        let mut handle = ptr::null_mut();
        let mut disposition = 0u32;
        let status = unsafe {
            RegCreateKeyExW(
                local_machine,
                subkey_w.as_ptr(),
                0,
                ptr::null(),
                REG_OPTION_NON_VOLATILE,
                KEY_WRITE,
                ptr::null(),
                &mut handle,
                &mut disposition,
            )
        };
        if status != ERROR_SUCCESS {
            bail!("RegCreateKeyExW({subkey}): win32 {status}");
        }
        Ok(Self(handle))
    }

    fn copy_tree(&self, destination: &Self) -> Result<()> {
        let status = unsafe { RegCopyTreeW(self.0, ptr::null(), destination.0) };
        if status != ERROR_SUCCESS {
            bail!("RegCopyTreeW: win32 {status}");
        }
        Ok(())
    }

    fn delete_tree(local_machine: HKEY, subkey: &str) -> Result<()> {
        let subkey_w = wide::null_terminated(subkey);
        let status = unsafe { RegDeleteTreeW(local_machine, subkey_w.as_ptr()) };
        if status != ERROR_SUCCESS {
            bail!("RegDeleteTreeW({subkey}): win32 {status}");
        }
        Ok(())
    }
}

impl Drop for RegistryKey {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                RegCloseKey(self.0);
            }
        }
    }
}

struct TempLsaSecret {
    registry_subkey: String,
}

impl TempLsaSecret {
    fn remove(&self) {
        if let Err(err) = RegistryKey::delete_tree(HKEY_LOCAL_MACHINE, &self.registry_subkey) {
            log::warn!(
                "failed to delete temporary LSA secret {}: {err:#}",
                self.registry_subkey
            );
        }
    }
}

impl Drop for TempLsaSecret {
    fn drop(&mut self) {
        self.remove();
    }
}

fn scm_secret_registry_subkey(service_name: &str) -> String {
    format!("{LSA_SECRETS_KEY}\\{SCM_SECRET_PREFIX}{service_name}")
}

fn temp_secret_registry_subkey() -> String {
    let id = Uuid::new_v4().simple().to_string();
    format!("{LSA_SECRETS_KEY}\\datadogprocmgr{id}")
}

fn temp_secret_lsa_name(registry_subkey: &str) -> Result<String> {
    registry_subkey
        .rsplit('\\')
        .next()
        .map(str::to_string)
        .context("temporary LSA secret name")
}

/// Read the SCM-stored password for `service_name`, if present.
pub(crate) fn read_scm_service_password(service_name: &str) -> Result<Option<String>> {
    let source_subkey = scm_secret_registry_subkey(service_name);
    let source = match RegistryKey::open(HKEY_LOCAL_MACHINE, &source_subkey, KEY_READ) {
        Ok(key) => key,
        Err(_) => return Ok(None),
    };

    let temp_subkey = temp_secret_registry_subkey();
    let temp_lsa_name = temp_secret_lsa_name(&temp_subkey)?;
    let _temp_guard = TempLsaSecret {
        registry_subkey: temp_subkey.clone(),
    };

    let destination = RegistryKey::create(HKEY_LOCAL_MACHINE, &temp_subkey)?;
    source
        .copy_tree(&destination)
        .with_context(|| format!("copy SCM LSA secret for {service_name}"))?;

    let password = read_lsa_private_data(&temp_lsa_name)?;
    Ok(match password {
        Some(value) if value.is_empty() => None,
        other => other,
    })
}

fn read_lsa_private_data(key: &str) -> Result<Option<String>> {
    let mut key_w = wide::null_terminated(key);
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
    fn scm_secret_registry_subkey_uses_sc_prefix() {
        assert_eq!(
            scm_secret_registry_subkey("datadogagent"),
            r"SECURITY\Policy\Secrets\_SC_datadogagent"
        );
    }

    #[test]
    fn temp_secret_lsa_name_uses_leaf_key() {
        let subkey = r"SECURITY\Policy\Secrets\datadogprocmgrabc123";
        assert_eq!(
            temp_secret_lsa_name(subkey).expect("leaf"),
            "datadogprocmgrabc123"
        );
    }
}
