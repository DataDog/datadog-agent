// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
#[cfg(not(test))]
use log::info;
use std::ptr;
use windows_sys::Win32::System::Services::{
    CloseServiceHandle, OpenSCManagerW, OpenServiceW, QUERY_SERVICE_CONFIGW, QueryServiceConfigW,
    SC_HANDLE, SC_MANAGER_CONNECT, SERVICE_QUERY_CONFIG,
};

#[cfg(not(test))]
use super::sid::{lookup_account_sid, sid_to_string};
#[cfg(not(test))]
use super::wide;

#[cfg(not(test))]
pub(crate) const DATADOG_AGENT_SERVICE: &str = "datadogagent";

#[cfg(not(test))]
pub(crate) fn service_runs_as_agent_user(
    service_name: &str,
    domain: &str,
    user: &str,
) -> Result<bool> {
    let service_user = service_start_name(service_name)
        .with_context(|| format!("could not get {service_name} service user"))?;
    let agent_sid = lookup_installed_user_sid(domain, user)
        .with_context(|| format!("lookup SID for installed agent user {domain}\\{user}"))?;
    let service_sid = lookup_service_account_sid(&service_user)
        .with_context(|| format!("lookup SID for service account {service_user}"))?;
    let matches = sids_equal(&agent_sid, &service_sid);
    if !matches {
        info!(
            "datadogagent service account {service_user} (sid {}) does not match installed agent user {domain}\\{user} (sid {})",
            sid_to_string(&service_sid).unwrap_or_else(|_| "<unknown>".to_string()),
            sid_to_string(&agent_sid).unwrap_or_else(|_| "<unknown>".to_string()),
        );
    }
    Ok(matches)
}

#[cfg(not(test))]
pub(crate) fn lookup_installed_user_sid(domain: &str, user: &str) -> Result<Vec<u8>> {
    let mut last_err = None;
    for (candidate_domain, candidate_user) in installed_user_lookup_candidates(domain, user) {
        match lookup_account_sid(&candidate_domain, &candidate_user) {
            Ok(sid) => return Ok(sid),
            Err(err) => last_err = Some(err),
        }
    }
    Err(last_err.unwrap_or_else(|| {
        anyhow::anyhow!("no lookup candidates for installed agent user {domain}\\{user}")
    }))
}

fn installed_user_lookup_candidates(domain: &str, user: &str) -> Vec<(String, String)> {
    let mut candidates = vec![(domain.to_string(), user.to_string())];
    if !domain.is_empty() {
        candidates.push((String::new(), format!("{user}@{domain}")));
        candidates.push((String::new(), user.to_string()));
    }
    candidates
}

#[cfg(not(test))]
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
    let ok = unsafe {
        QueryServiceConfigW(
            service,
            buffer.as_mut_ptr().cast(),
            bytes_needed,
            &mut bytes_needed,
        )
    };
    if ok == 0 {
        bail!(
            "QueryServiceConfigW({service_name}): {}",
            std::io::Error::last_os_error()
        );
    }

    let config = unsafe { ptr::read_unaligned(buffer.as_ptr().cast::<QUERY_SERVICE_CONFIGW>()) };
    let start_name = wide::from_ptr(config.lpServiceStartName);
    if start_name.is_empty() {
        bail!("QueryServiceConfigW({service_name}): empty service start name");
    }
    Ok(start_name)
}

#[cfg(not(test))]
fn lookup_service_account_sid(service_user: &str) -> Result<Vec<u8>> {
    if service_user.eq_ignore_ascii_case("LocalSystem") {
        return lookup_account_sid("NT AUTHORITY", "SYSTEM")
            .or_else(|_| lookup_account_sid("", "SYSTEM"));
    }

    let mut parts = service_user.splitn(2, '\\');
    match (parts.next(), parts.next()) {
        (Some("."), Some(user)) => lookup_account_sid("", user),
        (Some(domain), Some(user)) => lookup_account_sid(domain, user),
        (Some(user), None) => lookup_account_sid("", user),
        _ => bail!("invalid service account name {service_user}"),
    }
}

#[cfg(not(test))]
fn sids_equal(left: &[u8], right: &[u8]) -> bool {
    use windows_sys::Win32::Security::EqualSid;
    unsafe { EqualSid(left.as_ptr() as *mut _, right.as_ptr() as *mut _) != 0 }
}

#[cfg(test)]
fn service_user_for_sid_lookup(user: &str) -> String {
    let mut parts = user.splitn(2, '\\');
    let first = parts.next().unwrap_or(user);
    match parts.next() {
        None => user.to_string(),
        Some(second) if first == "." => second.to_string(),
        Some(_) => user.to_string(),
    }
}

#[cfg(not(test))]
struct ServiceHandle(SC_HANDLE);

#[cfg(not(test))]
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

    #[test]
    fn installed_user_lookup_candidates_include_upn_and_default_domain() {
        let candidates = installed_user_lookup_candidates("datadogqalab.com", "TestUser");
        assert_eq!(
            candidates,
            vec![
                ("datadogqalab.com".to_string(), "TestUser".to_string()),
                (String::new(), "TestUser@datadogqalab.com".to_string()),
                (String::new(), "TestUser".to_string()),
            ]
        );
    }
}
