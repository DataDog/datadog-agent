// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;

use super::local_agent_account::AccountName;
use super::wide;

type NetApiStatus = u32;

#[link(name = "logoncli")]
extern "system" {
    fn NetIsServiceAccount(
        servername: *const u16,
        accountname: *const u16,
        isservice: *mut i32,
    ) -> NetApiStatus;
}

/// Returns whether the installed Agent user is a managed service account (gMSA/sMSA).
///
/// Matches the MSI installer's `IsServiceAccount` check. Well-known SCM accounts are
/// handled earlier and should not reach this helper.
pub(crate) fn is_managed_service_account(domain: &str, user: &str) -> bool {
    let display = AccountName::new(domain, user).display();
    match net_is_service_account(domain, user) {
        Ok(is_service) => is_service,
        Err(error) => {
            info!("could not check managed service account for {display}: {error:#}");
            false
        }
    }
}

fn net_is_service_account(domain: &str, user: &str) -> Result<bool> {
    let account = wide::null_terminated(&AccountName::new(domain, user).display());
    let mut is_service = 0i32;
    let status =
        unsafe { NetIsServiceAccount(std::ptr::null(), account.as_ptr(), &mut is_service) };
    if status != 0 {
        return Err(std::io::Error::from_raw_os_error(status as i32))
            .context("NetIsServiceAccount");
    }
    Ok(is_service != 0)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn net_is_service_account_rejects_well_known_local_system_name() {
        let err = net_is_service_account("NT AUTHORITY", "SYSTEM")
            .expect_err("NetIsServiceAccount rejects NT AUTHORITY\\SYSTEM");
        assert!(
            format!("{err:#}").contains("NetIsServiceAccount"),
            "expected NetIsServiceAccount failure, got {err:#}"
        );
    }
}
