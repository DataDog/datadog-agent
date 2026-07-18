// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#[cfg(target_os = "linux")]
mod linux;
#[cfg(target_os = "macos")]
mod macos;

use anyhow::Result;

/// Return the username owning `pid`, if the process exists and is readable.
pub(crate) fn runtime_user_for_pid(pid: u32) -> Option<String> {
    match lookup_runtime_user(pid) {
        Ok(user) => Some(user),
        Err(e) => {
            log::debug!("[pid={pid}] runtime user lookup failed: {e:#}");
            None
        }
    }
}

#[cfg(target_os = "linux")]
fn lookup_runtime_user(pid: u32) -> Result<String> {
    linux::lookup_runtime_user(pid)
}

#[cfg(target_os = "macos")]
fn lookup_runtime_user(pid: u32) -> Result<String> {
    macos::lookup_runtime_user(pid)
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
fn lookup_runtime_user(_pid: u32) -> Result<String> {
    use anyhow::bail;
    bail!("runtime user lookup is not supported on this platform")
}
