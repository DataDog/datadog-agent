// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ffi::OsStr;
use std::io;
use std::ptr;
use std::sync::OnceLock;
use std::time::Duration;

use anyhow::{Context, Result, bail};
use tokio::net::windows::named_pipe::{NamedPipeServer, ServerOptions};
use windows_sys::Win32::Foundation::{HLOCAL, LocalFree};
use windows_sys::Win32::Security::Authorization::{
    ConvertStringSecurityDescriptorToSecurityDescriptorW, SDDL_REVISION_1,
};
use windows_sys::Win32::Security::SECURITY_ATTRIBUTES;
use windows_sys::Win32::Storage::FileSystem::{FILE_GENERIC_READ, FILE_WRITE_DATA};

use super::agent_service_sid::installed_agent_user_sid_string;
use super::wide;

const AGENT_PIPE_CLIENT_ACCESS_MASK: u32 = FILE_GENERIC_READ | FILE_WRITE_DATA;
const EVERYONE_SID: &str = "S-1-1-0";
const SID_LOOKUP_MAX_ATTEMPTS: u32 = 5;
const SID_LOOKUP_RETRY_DELAY: Duration = Duration::from_millis(500);

static CACHED_PIPE_SDDL: OnceLock<Result<String, String>> = OnceLock::new();

pub(crate) fn create_pipe_server(
    options: &ServerOptions,
    pipe_name: &OsStr,
) -> io::Result<NamedPipeServer> {
    let sddl = cached_pipe_sddl().map_err(|e| io::Error::other(e.to_string()))?;
    with_security_attributes(sddl, |attrs| unsafe {
        options.create_with_security_attributes_raw(pipe_name, attrs)
    })
}

fn cached_pipe_sddl() -> Result<&'static str> {
    match CACHED_PIPE_SDDL
        .get_or_init(|| setup_security_descriptor_with_retries().map_err(|err| format!("{err:#}")))
    {
        Ok(sddl) => Ok(sddl.as_str()),
        Err(msg) => Err(anyhow::anyhow!("{msg}")),
    }
}

fn setup_security_descriptor_with_retries() -> Result<String> {
    let mut last_err = None;
    for attempt in 1..=SID_LOOKUP_MAX_ATTEMPTS {
        match setup_security_descriptor_once() {
            Ok(sd) => return Ok(sd),
            Err(err) => {
                if attempt < SID_LOOKUP_MAX_ATTEMPTS {
                    log::warn!(
                        "installed agent user SID lookup failed (attempt {attempt}/{SID_LOOKUP_MAX_ATTEMPTS}): {err:#}; retrying"
                    );
                    std::thread::sleep(SID_LOOKUP_RETRY_DELAY);
                }
                last_err = Some(err);
            }
        }
    }
    Err(last_err.unwrap())
}

fn setup_security_descriptor_once() -> Result<String> {
    let sid =
        installed_agent_user_sid_string().context("failed to get SID for installed agent user")?;

    if sid.is_empty() {
        bail!("failed to get SID string for installed agent user");
    }
    if sid.eq_ignore_ascii_case(EVERYONE_SID) {
        bail!("installed agent user as Everyone is not supported");
    }

    format_security_descriptor_with_sid(&sid)
}

fn format_security_descriptor_with_sid(sid: &str) -> Result<String> {
    if !sid.starts_with("S-") {
        bail!("invalid SID {sid}");
    }
    Ok(format!(
        "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)(A;NP;{:#x};;;{sid})",
        AGENT_PIPE_CLIENT_ACCESS_MASK,
    ))
}

fn with_security_attributes<F>(sddl: &str, f: F) -> io::Result<NamedPipeServer>
where
    F: FnOnce(*mut std::ffi::c_void) -> io::Result<NamedPipeServer>,
{
    let mut sd = ptr::null_mut();
    let mut sd_size = 0u32;
    let sddl_w = wide::null_terminated(sddl);

    let ok = unsafe {
        ConvertStringSecurityDescriptorToSecurityDescriptorW(
            sddl_w.as_ptr(),
            SDDL_REVISION_1,
            &mut sd,
            &mut sd_size,
        )
    };
    if ok == 0 {
        return Err(io::Error::last_os_error());
    }

    let mut attrs = SECURITY_ATTRIBUTES {
        nLength: std::mem::size_of::<SECURITY_ATTRIBUTES>() as u32,
        lpSecurityDescriptor: sd,
        bInheritHandle: 0,
    };

    let result = f(&mut attrs as *mut SECURITY_ATTRIBUTES as *mut std::ffi::c_void);
    unsafe {
        LocalFree(sd as HLOCAL);
    }
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn format_security_descriptor_with_sid_rejects_invalid_sid() {
        assert!(format_security_descriptor_with_sid("not-a-sid").is_err());
    }

    #[test]
    fn format_security_descriptor_with_sid_includes_narrowed_agent_ace() {
        let sid = "S-1-5-21-0-0-0-1000";
        let sd = format_security_descriptor_with_sid(sid).unwrap();
        assert_eq!(
            sd,
            format!(
                "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)(A;NP;{:#x};;;S-1-5-21-0-0-0-1000)",
                AGENT_PIPE_CLIENT_ACCESS_MASK,
            )
        );
        assert!(
            !sd.contains("FRFW"),
            "must not grant generic write to agent SID"
        );
    }

    #[test]
    fn agent_pipe_client_access_mask_excludes_create_pipe_instance() {
        use windows_sys::Win32::Storage::FileSystem::{
            FILE_CREATE_PIPE_INSTANCE, FILE_GENERIC_WRITE,
        };

        assert_eq!(
            AGENT_PIPE_CLIENT_ACCESS_MASK & FILE_CREATE_PIPE_INSTANCE,
            0,
            "agent ACE must not allow creating pipe instances"
        );
        assert_ne!(
            FILE_GENERIC_WRITE & FILE_CREATE_PIPE_INSTANCE,
            0,
            "sanity: generic write is what we are avoiding"
        );
    }
}
