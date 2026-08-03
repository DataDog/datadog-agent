// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Resolve the core Agent service account SID from SCM.
//!
//! Mirrors `pkg/util/winutil/users.go` (`GetDDAgentUserSID` / `GetServiceUserSID`).

use anyhow::{Context, Result, bail};
use std::ptr;
use windows_sys::Win32::System::Services::{
    CloseServiceHandle, OpenSCManagerW, OpenServiceW, QueryServiceConfigW, SC_HANDLE,
    SC_MANAGER_CONNECT, SERVICE_QUERY_CONFIG,
};

use super::sid::{lookup_account_sid, sid_to_string};
use super::wide;

/// SCM service name for the core Datadog Agent (`winutil.GetDDAgentUserSID`).
const DATADOG_AGENT_SERVICE: &str = "datadogagent";
/// Manually mapped alias that SCM uses for LocalSystem (`winutil.GetServiceUserSID`).
const LOCAL_SYSTEM_SID: &str = "S-1-5-18";

/// Returns the SID string for the `datadogagent` service account.
pub(crate) fn datadog_agent_user_sid_string() -> Result<String> {
    let service_user = service_start_name(DATADOG_AGENT_SERVICE)
        .with_context(|| format!("could not get {DATADOG_AGENT_SERVICE} service user"))?;
    let lookup_name = service_user_for_sid_lookup(&service_user);

    if lookup_name.eq_ignore_ascii_case("LocalSystem") {
        return Ok(LOCAL_SYSTEM_SID.to_string());
    }

    let sid = lookup_account_sid("", &lookup_name)
        .with_context(|| format!("lookup SID for {lookup_name}"))?;
    sid_to_string(&sid)
}

fn service_start_name(service_name: &str) -> Result<String> {
    let manager = unsafe { OpenSCManagerW(ptr::null(), ptr::null(), SC_MANAGER_CONNECT) };
    if manager.is_null() {
        bail!("OpenSCManagerW: {}", std::io::Error::last_os_error());
    }
    let _manager = ServiceHandle(manager);

    let service_name_w = wide::null_terminated(service_name);
    let service = unsafe { OpenServiceW(manager, service_name_w.as_ptr(), SERVICE_QUERY_CONFIG) };
    if service.is_null() {
        bail!(
            "OpenServiceW({service_name}): {}",
            std::io::Error::last_os_error()
        );
    }
    let _service = ServiceHandle(service);

    let mut bytes_needed = 0u32;
    unsafe {
        QueryServiceConfigW(service, ptr::null_mut(), 0, &mut bytes_needed);
    }
    if bytes_needed == 0 {
        bail!("QueryServiceConfigW({service_name}): zero buffer size");
    }

    let mut buffer = vec![0u8; bytes_needed as usize];
    let config =
        buffer.as_mut_ptr() as *mut windows_sys::Win32::System::Services::QUERY_SERVICE_CONFIGW;
    let ok = unsafe { QueryServiceConfigW(service, config, bytes_needed, &mut bytes_needed) };
    if ok == 0 {
        bail!(
            "QueryServiceConfigW({service_name}): {}",
            std::io::Error::last_os_error()
        );
    }

    let start_name = wide::from_ptr(unsafe { (*config).lpServiceStartName });
    if start_name.is_empty() {
        bail!("QueryServiceConfigW({service_name}): empty service start name");
    }
    Ok(start_name)
}

/// Mirrors `winutil.getUserFromServiceUser`.
fn service_user_for_sid_lookup(user: &str) -> String {
    let mut parts = user.splitn(2, '\\');
    let first = parts.next().unwrap_or(user);
    match parts.next() {
        None => user.to_string(),
        Some(second) if first == "." => second.to_string(),
        Some(_) => user.to_string(),
    }
}

struct ServiceHandle(SC_HANDLE);

impl Drop for ServiceHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseServiceHandle(self.0);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn service_user_for_sid_lookup_matches_winutil() {
        assert_eq!(service_user_for_sid_lookup("ddagentuser"), "ddagentuser");
        assert_eq!(service_user_for_sid_lookup(r".\ddagentuser"), "ddagentuser");
        assert_eq!(
            service_user_for_sid_lookup(r"NT AUTHORITY\LocalService"),
            "NT AUTHORITY\\LocalService"
        );
        assert_eq!(service_user_for_sid_lookup("LocalSystem"), "LocalSystem");
    }
}
