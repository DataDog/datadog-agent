// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Named-pipe DACL for dd-procmgrd when the service runs as LocalSystem.
//!
//! Mirrors `pkg/system-probe/api/server/listener_windows.go`: grant Administrators and
//! LocalSystem full access, plus read/write to the core Agent service account SID
//! (`winutil.GetDDAgentUserSID`) so the core Agent and `dd-procmgr` CLI can connect.

use std::ffi::OsStr;
use std::io;
use std::ptr;

use anyhow::{Context, Result, bail};
use log::warn;
use tokio::net::windows::named_pipe::{NamedPipeServer, ServerOptions};
use windows_sys::Win32::Foundation::{HLOCAL, LocalFree};
use windows_sys::Win32::Security::Authorization::{
    ConvertStringSecurityDescriptorToSecurityDescriptorW, SDDL_REVISION_1,
};
use windows_sys::Win32::Security::SECURITY_ATTRIBUTES;

use super::agent_service_sid;
use super::wide;

const NAMED_PIPE_SECURITY_DESCRIPTOR_TEMPLATE: &str =
    "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)(A;NP;FRFW;;;%s)";
const NAMED_PIPE_DEFAULT_SECURITY_DESCRIPTOR: &str = "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)";
const EVERYONE_SID: &str = "S-1-1-0";

/// Create a named pipe server with an explicit DACL for agent-user clients.
pub(crate) fn create_pipe_server(
    options: &ServerOptions,
    pipe_name: &OsStr,
) -> io::Result<NamedPipeServer> {
    let sddl = match setup_security_descriptor() {
        Ok(sd) => sd,
        Err(e) => {
            warn!("failed to setup security descriptor, ddagentuser is denied: {e:#}");
            NAMED_PIPE_DEFAULT_SECURITY_DESCRIPTOR.to_string()
        }
    };
    with_security_attributes(&sddl, |attrs| unsafe {
        options.create_with_security_attributes_raw(pipe_name, attrs)
    })
}

fn setup_security_descriptor() -> Result<String> {
    let sid = agent_service_sid::datadog_agent_user_sid_string()
        .context("failed to get SID for ddagentuser")?;

    if sid.is_empty() {
        bail!("failed to get SID string from ddagentuser");
    }
    if sid.eq_ignore_ascii_case(EVERYONE_SID) {
        bail!("ddagentuser as Everyone is not supported");
    }

    format_security_descriptor_with_sid(&sid)
}

fn format_security_descriptor_with_sid(sid: &str) -> Result<String> {
    if !sid.starts_with("S-") {
        bail!("invalid SID {sid}");
    }
    Ok(NAMED_PIPE_SECURITY_DESCRIPTOR_TEMPLATE.replace("%s", sid))
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
    fn format_security_descriptor_with_sid_uses_system_probe_template() {
        let sid = "S-1-5-21-0-0-0-1000";
        let sd = format_security_descriptor_with_sid(sid).unwrap();
        assert_eq!(
            sd,
            "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)(A;NP;FRFW;;;S-1-5-21-0-0-0-1000)"
        );
    }
}
