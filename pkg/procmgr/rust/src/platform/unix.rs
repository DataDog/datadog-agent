// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use nix::sys::signal::{self, Signal};
use nix::unistd::Pid;
use std::os::unix::process::ExitStatusExt;
use std::path::PathBuf;

/// Place the child in its own process group so signals don't propagate
/// to the daemon itself and SIGTERM can target all descendants.
pub fn setup_process_group(cmd: &mut tokio::process::Command) {
    cmd.process_group(0);
}

/// Negate a PID to produce the process group ID for `kill(2)`.
/// Sending a signal to `-pgid` targets every process in the group.
pub(crate) fn process_group_id(pid: u32) -> Result<Pid> {
    let raw = i32::try_from(pid).context("PID overflows i32")?;
    Ok(Pid::from_raw(-raw))
}

/// Send SIGTERM to the entire process group (graceful stop).
pub fn send_graceful_stop(pid: u32) -> Result<()> {
    signal::kill(process_group_id(pid)?, Signal::SIGTERM)
        .with_context(|| format!("failed to send SIGTERM to pgid {pid}"))?;
    Ok(())
}

/// Send SIGKILL to the entire process group (force kill).
pub fn send_force_kill(pid: u32) -> Result<()> {
    signal::kill(process_group_id(pid)?, Signal::SIGKILL)
        .with_context(|| format!("failed to send SIGKILL to pgid {pid}"))?;
    Ok(())
}

/// Extract the signal number from an exit status, if the process was
/// terminated by a signal.
pub fn last_signal(status: &std::process::ExitStatus) -> Option<i32> {
    status.signal()
}

pub fn default_config_dir() -> PathBuf {
    PathBuf::from("/opt/datadog-agent/processes.d")
}

pub fn stdout_inheritable() -> bool {
    true
}

pub fn stderr_inheritable() -> bool {
    true
}

/// Wait for a shutdown trigger (SIGTERM or SIGINT).
pub async fn shutdown_signal() {
    use tokio::signal::unix::{SignalKind, signal};
    let mut sigterm = signal(SignalKind::terminate()).expect("failed to register SIGTERM handler");
    let mut sigint = signal(SignalKind::interrupt()).expect("failed to register SIGINT handler");
    tokio::select! {
        _ = sigterm.recv() => { log::info!("received SIGTERM"); }
        _ = sigint.recv() => { log::info!("received SIGINT"); }
    }
}

pub(crate) fn intended_spawn_user(
    _process_name: &str,
    _profile: crate::spawn::SpawnProfile,
) -> String {
    match supervisor_user() {
        Ok(user) => user,
        Err(e) => {
            log::warn!("intended spawn user lookup failed: {e:#}");
            "unknown".to_string()
        }
    }
}

fn supervisor_user() -> Result<String> {
    passwd_name_for_uid(nix::unistd::Uid::effective())
}

fn passwd_name_for_uid(uid: nix::unistd::Uid) -> Result<String> {
    use nix::unistd::User;

    User::from_uid(uid)
        .context("getpwuid")?
        .map(|u| u.name)
        .context("no passwd entry for uid")
}

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
    use nix::unistd::Uid;
    use std::fs::File;
    use std::io::BufReader;

    let status_path = format!("/proc/{pid}/status");
    let file = File::open(&status_path).context("open process status")?;
    let uid = parse_effective_uid(BufReader::new(file)).context("parse process uid")?;
    passwd_name_for_uid(Uid::from_raw(uid))
}

#[cfg(target_os = "linux")]
fn parse_effective_uid<R: std::io::BufRead>(reader: R) -> Result<u32> {
    for line in reader.lines() {
        let line = line?;
        if let Some(rest) = line.strip_prefix("Uid:") {
            let effective = rest
                .split_whitespace()
                .nth(1)
                .context("parse effective uid from Uid line")?;
            return effective.parse().context("parse uid");
        }
    }
    anyhow::bail!("Uid not found in process status")
}

#[cfg(target_os = "macos")]
fn lookup_runtime_user(pid: u32) -> Result<String> {
    use libc::{PROC_PIDTBSDINFO, c_int, proc_bsdinfo, proc_pidinfo};
    use nix::unistd::Uid;

    let mut info = unsafe { std::mem::zeroed::<proc_bsdinfo>() };
    let size = std::mem::size_of::<proc_bsdinfo>() as c_int;
    let result = unsafe {
        proc_pidinfo(
            pid as c_int,
            PROC_PIDTBSDINFO,
            0,
            (&raw mut info).cast(),
            size,
        )
    };
    if result != size {
        anyhow::bail!("proc_pidinfo: {}", std::io::Error::last_os_error());
    }

    passwd_name_for_uid(Uid::from_raw(info.pbi_uid))
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
fn lookup_runtime_user(_pid: u32) -> Result<String> {
    anyhow::bail!("runtime user lookup is not supported on this platform")
}

#[cfg(test)]
#[cfg(target_os = "linux")]
mod runtime_user_tests {
    use super::parse_effective_uid;
    use std::io::Cursor;

    #[test]
    fn parse_effective_uid_reads_second_field() {
        let status = "Name:\tsleep\nUid:\t1000\t0\t0\t0\n";
        let reader = Cursor::new(status.as_bytes());
        assert_eq!(parse_effective_uid(reader).expect("effective uid"), 0);
    }
}

#[cfg(test)]
mod intended_spawn_user_tests {
    use super::*;
    use crate::spawn::SpawnProfile;

    #[test]
    fn agent_profile_uses_supervisor_user() {
        let expected = supervisor_user().expect("current user");
        assert_eq!(
            intended_spawn_user("datadog-agent-trace", SpawnProfile::Agent),
            expected
        );
    }
}
