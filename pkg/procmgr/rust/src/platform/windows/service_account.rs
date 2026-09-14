// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;
use std::ffi::c_void;
use std::sync::OnceLock;
use windows_sys::Win32::Foundation::HMODULE;
use windows_sys::Win32::System::LibraryLoader::{GetProcAddress, LoadLibraryW};

use super::local_agent_account::AccountName;
use super::wide;

type NetApiStatus = u32;

type NetIsServiceAccountFn = unsafe extern "system" fn(
    servername: *const u16,
    accountname: *const u16,
    isservice: *mut i32,
) -> NetApiStatus;

struct LogonCli {
    _module: HMODULE,
    net_is_service_account: NetIsServiceAccountFn,
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
    let logoncli = logoncli_api()?;
    let account = wide::null_terminated(&AccountName::new(domain, user).display());
    let mut is_service = 0i32;
    let status = unsafe {
        (logoncli.net_is_service_account)(std::ptr::null(), account.as_ptr(), &mut is_service)
    };
    if status != 0 {
        return Err(std::io::Error::from_raw_os_error(status as i32))
            .context("NetIsServiceAccount");
    }
    Ok(is_service != 0)
}

fn logoncli_api() -> Result<&'static LogonCli> {
    static LOGONCLI: OnceLock<Result<LogonCli>> = OnceLock::new();
    LOGONCLI
        .get_or_init(load_logoncli)
        .as_ref()
        .map_err(|error| anyhow::anyhow!("{error:#}"))
}

fn load_logoncli() -> Result<LogonCli> {
    // NetIsServiceAccount has no import library; load Logoncli.dll at runtime.
    // https://learn.microsoft.com/en-us/windows/win32/api/lmaccess/nf-lmaccess-netisserviceaccount
    let dll = wide::null_terminated("Logoncli.dll");
    let module = unsafe { LoadLibraryW(dll.as_ptr()) };
    if module.is_null() {
        return Err(std::io::Error::last_os_error()).context("LoadLibraryW(Logoncli.dll)");
    }

    let symbol = c"NetIsServiceAccount";
    let proc = unsafe { GetProcAddress(module, symbol.as_ptr()) };
    let Some(proc) = proc else {
        return Err(std::io::Error::last_os_error())
            .context("GetProcAddress(NetIsServiceAccount)");
    };

    let net_is_service_account = unsafe { std::mem::transmute::<*const c_void, NetIsServiceAccountFn>(proc) };
    Ok(LogonCli {
        _module: module,
        net_is_service_account,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn logoncli_api_loads_net_is_service_account() {
        logoncli_api().expect("Logoncli.dll must load on Windows test hosts");
    }

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
