// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! gMSA detection for installer-configured agent users without LSA passwords.
//!
//! Mirrors `pkg/fleet/installer/packages/user/windows.IsServiceAccount`.

use anyhow::{Context, Result, bail};
use std::ptr;

use super::wide;

const MSA_INFO_NOT_EXIST: u32 = 1;
const MSA_INFO_NOT_SERVICE: u32 = 2;
const MSA_INFO_CANNOT_INSTALL: u32 = 3;
const MSA_INFO_CAN_INSTALL: u32 = 4;
const MSA_INFO_INSTALLED: u32 = 5;

const STATUS_OPEN_FAILED: u32 = 0xC000_0136;
const STATUS_INVALID_ACCOUNT_NAME: u32 = 0xC000_0064;
const STATUS_NAME_TOO_LONG: u32 = 0xC000_0106;

#[link(name = "logoncli")]
extern "system" {
    fn NetQueryServiceAccount(
        servername: *const u16,
        accountname: *const u16,
        infolevel: u32,
        buffer: *mut *mut u8,
    ) -> u32;
}

#[link(name = "netapi32")]
extern "system" {
    fn NetApiBufferFree(buffer: *mut u8) -> u32;
}

/// Result of querying whether an account is an installed gMSA.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum ManagedServiceAccountState {
    Installed,
    NotService,
    NotExist,
    CannotInstall,
    CanInstall,
    /// Ambiguous DC lookup; treat as a regular domain account.
    AssumeRegularDomainAccount,
}

/// Query whether `domain\user` is an installed gMSA.
pub(crate) fn query_managed_service_account(
    domain: &str,
    user: &str,
) -> Result<ManagedServiceAccountState> {
    let account = qualified_account_name(domain, user);
    let account_w = wide::null_terminated(&account);

    unsafe {
        let mut info: *mut u8 = ptr::null_mut();
        let status = NetQueryServiceAccount(ptr::null(), account_w.as_ptr(), 0, &mut info);
        if status != 0 {
            return map_query_status(status);
        }
        if info.is_null() {
            bail!("NetQueryServiceAccount({account}) returned a null buffer");
        }
        let state = *(info as *const u32);
        let _ = NetApiBufferFree(info);
        map_msa_info_state(state)
    }
    .with_context(|| format!("NetQueryServiceAccount({account})"))
}

fn qualified_account_name(domain: &str, user: &str) -> String {
    if domain.is_empty() {
        user.to_string()
    } else {
        format!("{domain}\\{user}")
    }
}

fn map_query_status(status: u32) -> Result<ManagedServiceAccountState> {
    match status {
        STATUS_INVALID_ACCOUNT_NAME | STATUS_NAME_TOO_LONG => {
            Ok(ManagedServiceAccountState::AssumeRegularDomainAccount)
        }
        STATUS_OPEN_FAILED => bail!(
            "error 0x{STATUS_OPEN_FAILED:X}: ensure the netlogon service is running, \
             the domain controller is available, and this process can authenticate to it"
        ),
        _ => bail!("NetQueryServiceAccount failed with status 0x{status:X}"),
    }
}

fn map_msa_info_state(state: u32) -> Result<ManagedServiceAccountState> {
    match state {
        MSA_INFO_INSTALLED => Ok(ManagedServiceAccountState::Installed),
        MSA_INFO_NOT_SERVICE => Ok(ManagedServiceAccountState::NotService),
        MSA_INFO_NOT_EXIST => Ok(ManagedServiceAccountState::NotExist),
        MSA_INFO_CANNOT_INSTALL => Ok(ManagedServiceAccountState::CannotInstall),
        MSA_INFO_CAN_INSTALL => Ok(ManagedServiceAccountState::CanInstall),
        other => bail!("unknown MSA_INFO_STATE value: {other}"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn qualified_account_name_formats_domain_users() {
        assert_eq!(qualified_account_name("CORP", "gmsa$"), "CORP\\gmsa$");
        assert_eq!(qualified_account_name("", "ddagentuser"), "ddagentuser");
    }

    #[test]
    fn map_query_status_treats_ambiguous_dc_errors_as_regular_domain_accounts() {
        assert_eq!(
            map_query_status(STATUS_INVALID_ACCOUNT_NAME).unwrap(),
            ManagedServiceAccountState::AssumeRegularDomainAccount
        );
        assert_eq!(
            map_query_status(STATUS_NAME_TOO_LONG).unwrap(),
            ManagedServiceAccountState::AssumeRegularDomainAccount
        );
    }

    #[test]
    fn map_msa_info_state_maps_installed_gmsa() {
        assert_eq!(
            map_msa_info_state(MSA_INFO_INSTALLED).unwrap(),
            ManagedServiceAccountState::Installed
        );
    }
}
