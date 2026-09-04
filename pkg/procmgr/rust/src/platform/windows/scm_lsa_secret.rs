// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ptr;

use anyhow::{Context, Result, bail};
use log::warn;
use uuid::Uuid;
use windows_sys::Win32::Foundation::{ERROR_NO_MORE_ITEMS, ERROR_SUCCESS};
use windows_sys::Win32::Security::Authentication::Identity::{
    LSA_HANDLE, LSA_OBJECT_ATTRIBUTES, LSA_UNICODE_STRING, LsaClose, LsaFreeMemory, LsaOpenPolicy,
    LsaRetrievePrivateData, POLICY_GET_PRIVATE_INFORMATION,
};
use windows_sys::Win32::Security::{
    AdjustTokenPrivileges, LUID_AND_ATTRIBUTES, LookupPrivilegeValueW, SE_PRIVILEGE_ENABLED,
    TOKEN_ADJUST_PRIVILEGES, TOKEN_PRIVILEGES, TOKEN_QUERY,
};
use windows_sys::Win32::System::Registry::{
    HKEY, HKEY_LOCAL_MACHINE, KEY_ALL_ACCESS, REG_OPTION_VOLATILE, RegCloseKey, RegCopyTreeW,
    RegCreateKeyExW, RegDeleteTreeW, RegEnumKeyExW, RegOpenKeyExW,
};
use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};

use super::wide;

const LSA_SECRETS_KEY: &str = r"SECURITY\Policy\Secrets";
const SCM_SECRET_PREFIX: &str = "_SC_";
const STATUS_OBJECT_NAME_NOT_FOUND: i32 = 0xC000_0034u32 as i32;
const SE_BACKUP_PRIVILEGE: &str = "SeBackupPrivilege";
const SE_RESTORE_PRIVILEGE: &str = "SeRestorePrivilege";

struct RegistryKey(HKEY);

impl RegistryKey {
    fn open(local_machine: HKEY, subkey: &str) -> Result<Self> {
        let subkey_w = wide::null_terminated(subkey);
        let mut handle = ptr::null_mut();
        let status = unsafe {
            RegOpenKeyExW(
                local_machine,
                subkey_w.as_ptr(),
                0,
                KEY_ALL_ACCESS,
                &mut handle,
            )
        };
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
                REG_OPTION_VOLATILE,
                KEY_ALL_ACCESS,
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

    fn open_child(&self, name: &str) -> Result<Self> {
        let name_w = wide::null_terminated(name);
        let mut handle = ptr::null_mut();
        let status =
            unsafe { RegOpenKeyExW(self.0, name_w.as_ptr(), 0, KEY_ALL_ACCESS, &mut handle) };
        if status != ERROR_SUCCESS {
            bail!("RegOpenKeyExW({name}): win32 {status}");
        }
        Ok(Self(handle))
    }

    fn create_child(&self, name: &str) -> Result<Self> {
        let name_w = wide::null_terminated(name);
        let mut handle = ptr::null_mut();
        let mut disposition = 0u32;
        let status = unsafe {
            RegCreateKeyExW(
                self.0,
                name_w.as_ptr(),
                0,
                ptr::null(),
                REG_OPTION_VOLATILE,
                KEY_ALL_ACCESS,
                ptr::null(),
                &mut handle,
                &mut disposition,
            )
        };
        if status != ERROR_SUCCESS {
            bail!("RegCreateKeyExW({name}): win32 {status}");
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
            warn!(
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

struct RegistryPrivilegeGuard {
    token: windows_sys::Win32::Foundation::HANDLE,
    previous_states: Vec<Vec<u8>>,
}

impl RegistryPrivilegeGuard {
    fn enable_for_lsa_secret_copy() -> Self {
        let mut token = ptr::null_mut();
        let ok = unsafe {
            OpenProcessToken(
                GetCurrentProcess(),
                TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY,
                &mut token,
            )
        };
        if ok == 0 {
            warn!(
                "could not open process token for LSA secret copy: {}",
                std::io::Error::last_os_error()
            );
            return Self {
                token: ptr::null_mut(),
                previous_states: Vec::new(),
            };
        }

        let mut previous_states = Vec::new();
        for privilege in [SE_BACKUP_PRIVILEGE, SE_RESTORE_PRIVILEGE] {
            match enable_privilege(token, privilege) {
                Ok(Some(previous)) => previous_states.push(previous),
                Ok(None) => {}
                Err(err) => {
                    warn!("could not enable {privilege} for LSA secret copy: {err:#}");
                }
            }
        }

        Self {
            token,
            previous_states,
        }
    }
}

impl Drop for RegistryPrivilegeGuard {
    fn drop(&mut self) {
        if self.token.is_null() {
            return;
        }

        for previous in self.previous_states.iter().rev() {
            let previous_tp = previous.as_ptr().cast::<TOKEN_PRIVILEGES>();
            unsafe {
                AdjustTokenPrivileges(
                    self.token,
                    0,
                    previous_tp,
                    0,
                    ptr::null_mut(),
                    ptr::null_mut(),
                );
            }
        }

        unsafe {
            windows_sys::Win32::Foundation::CloseHandle(self.token);
        }
        self.token = ptr::null_mut();
    }
}

fn enable_privilege(
    token: windows_sys::Win32::Foundation::HANDLE,
    name: &str,
) -> Result<Option<Vec<u8>>> {
    let mut luid = windows_sys::Win32::Foundation::LUID {
        LowPart: 0,
        HighPart: 0,
    };
    let name_w = wide::null_terminated(name);
    let ok = unsafe { LookupPrivilegeValueW(ptr::null(), name_w.as_ptr(), &mut luid) };
    if ok == 0 {
        bail!(
            "LookupPrivilegeValueW({name}): {}",
            std::io::Error::last_os_error()
        );
    }

    let new_state = TOKEN_PRIVILEGES {
        PrivilegeCount: 1,
        Privileges: [LUID_AND_ATTRIBUTES {
            Luid: luid,
            Attributes: SE_PRIVILEGE_ENABLED,
        }],
    };

    let mut previous = TOKEN_PRIVILEGES {
        PrivilegeCount: 0,
        Privileges: [LUID_AND_ATTRIBUTES {
            Luid: luid,
            Attributes: 0,
        }],
    };
    let mut return_length = 0u32;
    let buffer_length = std::mem::size_of::<TOKEN_PRIVILEGES>() as u32;
    let ok = unsafe {
        AdjustTokenPrivileges(
            token,
            0,
            &new_state,
            buffer_length,
            &mut previous,
            &mut return_length,
        )
    };
    if ok == 0 {
        bail!(
            "AdjustTokenPrivileges({name}): {}",
            std::io::Error::last_os_error()
        );
    }
    let err = unsafe { windows_sys::Win32::Foundation::GetLastError() };
    if err == windows_sys::Win32::Foundation::ERROR_NOT_ALL_ASSIGNED {
        return Ok(None);
    }
    if previous.PrivilegeCount == 0 {
        return Ok(None);
    }

    Ok(Some(token_privileges_bytes(&previous)))
}

fn token_privileges_bytes(tp: &TOKEN_PRIVILEGES) -> Vec<u8> {
    unsafe {
        std::slice::from_raw_parts(
            (tp as *const TOKEN_PRIVILEGES).cast::<u8>(),
            std::mem::size_of::<TOKEN_PRIVILEGES>(),
        )
    }
    .to_vec()
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

fn copy_lsa_secret_children(source: &RegistryKey, destination: &RegistryKey) -> Result<()> {
    let mut index = 0u32;
    loop {
        let mut name = vec![0u16; 256];
        let mut name_len = (name.len() - 1) as u32;
        let status = unsafe {
            RegEnumKeyExW(
                source.0,
                index,
                name.as_mut_ptr(),
                &mut name_len,
                ptr::null_mut(),
                ptr::null_mut(),
                ptr::null_mut(),
                ptr::null_mut(),
            )
        };
        if status == ERROR_NO_MORE_ITEMS {
            break;
        }
        if status != ERROR_SUCCESS {
            bail!("RegEnumKeyExW: win32 {status}");
        }

        let child_name = wide::from_ptr(name.as_ptr());
        if child_name.is_empty() {
            index += 1;
            continue;
        }

        let source_child = source
            .open_child(&child_name)
            .with_context(|| format!("open SCM LSA secret child {child_name}"))?;
        let dest_child = destination
            .create_child(&child_name)
            .with_context(|| format!("create temporary LSA secret child {child_name}"))?;
        source_child
            .copy_tree(&dest_child)
            .with_context(|| format!("copy SCM LSA secret child {child_name}"))?;
        index += 1;
    }
    Ok(())
}

pub(crate) fn read_scm_service_password(service_name: &str) -> Result<Option<String>> {
    let _privileges = RegistryPrivilegeGuard::enable_for_lsa_secret_copy();

    let source_subkey = scm_secret_registry_subkey(service_name);
    let source = match RegistryKey::open(HKEY_LOCAL_MACHINE, &source_subkey) {
        Ok(key) => key,
        Err(_) => return Ok(None),
    };

    let temp_subkey = temp_secret_registry_subkey();
    let temp_lsa_name = temp_secret_lsa_name(&temp_subkey)?;
    let _temp_guard = TempLsaSecret {
        registry_subkey: temp_subkey.clone(),
    };

    let destination = RegistryKey::create(HKEY_LOCAL_MACHINE, &temp_subkey)?;
    copy_lsa_secret_children(&source, &destination)
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
