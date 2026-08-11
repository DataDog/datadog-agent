// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! DACL validation for `secret_backend_command` executables on Windows.
//!
//! Mirrors `pkg/util/filesystem/rights_windows.go` (`CheckRights`): only LocalSystem,
//! Administrators, and the Agent service account may have rights on the backend binary,
//! and the Agent account must be explicitly allowed to execute it.

use std::mem::offset_of;
use std::path::Path;
use std::ptr;

use anyhow::{Context, Result, bail};
use windows_sys::Win32::Foundation::{LocalFree, WIN32_ERROR};
use windows_sys::Win32::Security::Authorization::{
    ConvertStringSidToSidW, GetNamedSecurityInfoW, SE_FILE_OBJECT,
};
use windows_sys::Win32::Security::{
    ACCESS_ALLOWED_ACE, ACL, ACL_SIZE_INFORMATION, AclSizeInformation, AllocateAndInitializeSid,
    DACL_SECURITY_INFORMATION, EqualSid, FreeSid, GetAce, GetAclInformation, PSID,
    SECURITY_NT_AUTHORITY,
};
use windows_sys::Win32::System::SystemServices::{
    DOMAIN_ALIAS_RID_ADMINS, SECURITY_BUILTIN_DOMAIN_RID,
};

use super::sid::lookup_account_sid;
use super::wide;
use super::{open_datadog_agent_key, registry_nonempty_string};

const ACCESS_ALLOWED_ACE_TYPE: u8 = 0;
const ACCESS_DENIED_ACE_TYPE: u8 = 1;
const LOCAL_SYSTEM_SID: &str = "S-1-5-18";

/// Validate executable DACLs before running it as the Agent account.
pub(crate) fn check_secret_backend_command_rights(path: &str) -> Result<()> {
    if cfg!(test) {
        // Unit tests create backends under %TEMP% without installer ACLs; Windows E2E covers this.
        return Ok(());
    }

    if !Path::new(path).is_file() {
        bail!("secretBackendCommand '{path}' does not exist");
    }

    let local_system = SidLocalAlloc::from_string(LOCAL_SYSTEM_SID)?;
    let administrators = SidAllocated::administrators()?;
    let secret_user = SidBytes::from_registry_agent_user()?;

    let dacl = file_dacl(path)?;
    let mut acl_info = ACL_SIZE_INFORMATION {
        AceCount: 0,
        AclBytesInUse: 0,
        AclBytesFree: 0,
    };
    let ok = unsafe {
        GetAclInformation(
            dacl.0,
            &mut acl_info as *mut _ as *mut _,
            std::mem::size_of::<ACL_SIZE_INFORMATION>() as u32,
            AclSizeInformation,
        )
    };
    if ok == 0 {
        bail!(
            "could not query ACLs for '{path}': {}",
            std::io::Error::last_os_error()
        );
    }

    let secret_user_display = sid_display(secret_user.as_ptr());
    let mut secret_user_allowed = false;
    for index in 0..acl_info.AceCount {
        let mut ace = ptr::null_mut();
        let ok = unsafe { GetAce(dacl.0, index, &mut ace) };
        if ok == 0 {
            bail!(
                "could not query an ACE on '{path}': {}",
                std::io::Error::last_os_error()
            );
        }

        let ace_type = unsafe { (*(ace as *const ACCESS_ALLOWED_ACE)).Header.AceType };
        let ace_sid = ace_sid(ace as *const ACCESS_ALLOWED_ACE);
        let is_local_system = sids_equal(ace_sid, local_system.as_ptr());
        let is_administrators = sids_equal(ace_sid, administrators.as_ptr());
        let is_secret_user = sids_equal(ace_sid, secret_user.as_ptr());

        match ace_type {
            ACCESS_DENIED_ACE_TYPE if is_local_system || is_administrators || is_secret_user => {
                bail!(
                    "invalid executable '{path}': explicit deny access for LOCAL_SYSTEM, Administrators or {secret_user_display}"
                );
            }
            ACCESS_ALLOWED_ACE_TYPE => {
                if !(is_local_system || is_administrators || is_secret_user) {
                    bail!(
                        "invalid executable '{path}': other users/groups than LOCAL_SYSTEM, Administrators or {secret_user_display} have rights on it"
                    );
                }
                if is_secret_user {
                    secret_user_allowed = true;
                }
            }
            _ => {}
        }
    }

    if !secret_user_allowed {
        bail!(
            "'{secret_user_display}' user is not allowed to execute secretBackendCommand '{path}'"
        );
    }
    Ok(())
}

fn file_dacl(path: &str) -> Result<FileDacl> {
    let path_w = wide::null_terminated(path);
    let mut dacl = ptr::null_mut::<ACL>();
    let status: WIN32_ERROR = unsafe {
        GetNamedSecurityInfoW(
            path_w.as_ptr(),
            SE_FILE_OBJECT,
            DACL_SECURITY_INFORMATION,
            ptr::null_mut(),
            ptr::null_mut(),
            &mut dacl,
            ptr::null_mut(),
            ptr::null_mut(),
        )
    };
    if status != 0 {
        bail!(
            "could not query ACLs for '{path}': {}",
            std::io::Error::from_raw_os_error(status as i32)
        );
    }
    if dacl.is_null() {
        bail!("could not query ACLs for '{path}': missing DACL");
    }
    Ok(FileDacl(dacl))
}

struct FileDacl(*mut ACL);

fn ace_sid(ace: *const ACCESS_ALLOWED_ACE) -> PSID {
    unsafe { (ace as *const u8).add(offset_of!(ACCESS_ALLOWED_ACE, SidStart)) as PSID }
}

fn sids_equal(left: PSID, right: PSID) -> bool {
    unsafe { EqualSid(left, right) != 0 }
}

fn sid_display(sid: PSID) -> String {
    super::sid::sid_to_string(unsafe {
        std::slice::from_raw_parts(sid as *const u8, sid_length(sid))
    })
    .unwrap_or_else(|_| "<unknown SID>".to_string())
}

fn sid_length(sid: PSID) -> usize {
    unsafe {
        let sub_authority_count = *(sid as *const u8).add(1) as usize;
        8 + sub_authority_count * 4
    }
}

struct SidBytes(Vec<u8>);

impl SidBytes {
    fn from_registry_agent_user() -> Result<Self> {
        Ok(Self(registry_agent_user_sid()?))
    }

    fn as_ptr(&self) -> PSID {
        self.0.as_ptr() as PSID
    }
}

struct SidLocalAlloc(PSID);

impl SidLocalAlloc {
    fn from_string(text: &str) -> Result<Self> {
        let text_w = wide::null_terminated(text);
        let mut sid = ptr::null_mut();
        if unsafe { ConvertStringSidToSidW(text_w.as_ptr(), &mut sid) } == 0 {
            bail!(
                "ConvertStringSidToSidW({text}): {}",
                std::io::Error::last_os_error()
            );
        }
        Ok(Self(sid))
    }

    fn as_ptr(&self) -> PSID {
        self.0
    }
}

impl Drop for SidLocalAlloc {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                LocalFree(self.0 as _);
            }
        }
    }
}

struct SidAllocated(PSID);

impl SidAllocated {
    fn administrators() -> Result<Self> {
        let mut sid = ptr::null_mut();
        let ok = unsafe {
            AllocateAndInitializeSid(
                &SECURITY_NT_AUTHORITY,
                2,
                SECURITY_BUILTIN_DOMAIN_RID,
                DOMAIN_ALIAS_RID_ADMINS,
                0,
                0,
                0,
                0,
                0,
                0,
                &mut sid,
            )
        };
        if ok == 0 {
            bail!(
                "AllocateAndInitializeSid(Administrators): {}",
                std::io::Error::last_os_error()
            );
        }
        Ok(Self(sid))
    }

    fn as_ptr(&self) -> PSID {
        self.0
    }
}

impl Drop for SidAllocated {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                FreeSid(self.0);
            }
        }
    }
}

fn registry_agent_user_sid() -> Result<Vec<u8>> {
    let key = open_datadog_agent_key().context("open HKLM\\SOFTWARE\\Datadog\\Datadog Agent")?;
    let user = registry_nonempty_string(&key, "installedUser")
        .context("read installedUser from registry")?;
    let domain = key
        .get_string("installedDomain")
        .unwrap_or_default()
        .trim()
        .to_string();
    lookup_account_sid(&domain, &user).with_context(|| format!("lookup SID for {domain}\\{user}"))
}
