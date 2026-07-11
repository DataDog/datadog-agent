// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Named-pipe DACL for dd-procmgrd when the service runs as LocalSystem.
//!
//! Mirrors `pkg/system-probe/api/server/listener_windows.go`: grant Administrators and
//! LocalSystem full access, plus read/write to the installed agent service account
//! (`ddagentuser` by default) so the core Agent and `dd-procmgr` CLI can connect.

use std::ffi::OsStr;
use std::io;
use std::ptr;

use log::warn;
use tokio::net::windows::named_pipe::{NamedPipeServer, ServerOptions};
use windows_sys::Win32::Foundation::{HLOCAL, LocalFree};
use windows_sys::Win32::Security::Authorization::{
    ConvertStringSecurityDescriptorToSecurityDescriptorW, SDDL_REVISION_1,
};
use windows_sys::Win32::Security::SECURITY_ATTRIBUTES;

use super::agent_credentials;
use super::wide;

const PIPE_SD_DEFAULT: &str = "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)";
const EVERYONE_SID: &str = "S-1-1-0";

/// Create a named pipe server with an explicit DACL for agent-user clients.
pub(crate) fn create_pipe_server(
    options: &ServerOptions,
    pipe_name: &OsStr,
) -> io::Result<NamedPipeServer> {
    let sddl = pipe_security_descriptor_sddl();
    with_security_attributes(&sddl, |attrs| unsafe {
        options.create_with_security_attributes_raw(pipe_name, attrs)
    })
}

fn pipe_security_descriptor_sddl() -> String {
    match agent_credentials::installed_agent_account_sid_string() {
        Ok(sid) if is_usable_agent_sid(&sid) => {
            format!("D:PAI(A;;FA;;;BA)(A;;FA;;;SY)(A;NP;FRFW;;;{sid})")
        }
        Ok(_) => {
            warn!("ddagentuser resolved to Everyone; using default procmgr pipe ACL");
            PIPE_SD_DEFAULT.to_string()
        }
        Err(e) => {
            warn!(
                "failed to resolve ddagentuser SID for procmgr pipe ACL: {e:#}; \
                 agent-user clients may be denied"
            );
            PIPE_SD_DEFAULT.to_string()
        }
    }
}

fn is_usable_agent_sid(sid: &str) -> bool {
    sid.starts_with("S-") && !sid.eq_ignore_ascii_case(EVERYONE_SID)
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
    fn rejects_everyone_sid_for_pipe_acl() {
        assert!(!is_usable_agent_sid(EVERYONE_SID));
        assert!(!is_usable_agent_sid("s-1-1-0"));
    }

    #[test]
    fn accepts_well_formed_sid() {
        assert!(is_usable_agent_sid("S-1-5-21-0-0-0-1000"));
    }
}
