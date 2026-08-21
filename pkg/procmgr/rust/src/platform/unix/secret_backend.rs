// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Run `secret_backend_command` under the core Agent service account on Unix.
//!
//! Secret resolution must match `datadog-agent`, not the procmgr supervisor identity.
//! Today procmgr runs as `dd-agent`, so the inherited token is sufficient. When Linux
//! [`SpawnProfile::Privileged`] children run under a host-privileged supervisor, the
//! backend is spawned with `setuid`/`setgid` to `dd-agent`.

use std::os::unix::process::CommandExt;

use anyhow::{Context, Result, bail};
use nix::unistd::{Gid, Uid, User};

use crate::secret_backend_exec::{BackendRun, exec_inherited_token, spawn_and_capture};

/// Agent package user; matches systemd `User=` for `datadog-agent.service`.
const AGENT_USER: &str = "dd-agent";
const PROCESS_NAME: &str = "secret-backend";

pub(crate) fn exec_secret_backend(
    command: &str,
    arguments: &[String],
    payload: &str,
    timeout: std::time::Duration,
    max_output_bytes: usize,
    _skip_acl_check: bool,
) -> Result<String> {
    let run = BackendRun {
        command,
        arguments,
        payload,
        timeout,
        max_output_bytes,
    };
    if supervisor_runs_as_agent_user()? {
        return exec_inherited_token(&run);
    }
    if nix::unistd::getuid().is_root() {
        return exec_as_agent_user(&run);
    }
    // Dev/CI: neither dd-agent nor root — best-effort inherited token.
    log::debug!(
        "[{PROCESS_NAME}] procmgr supervisor is not {AGENT_USER}; using inherited identity for secret backend"
    );
    exec_inherited_token(&run)
}

fn supervisor_runs_as_agent_user() -> Result<bool> {
    let Some(agent) = User::from_name(AGENT_USER).context("lookup agent service user")? else {
        return Ok(false);
    };
    Ok(nix::unistd::getuid() == agent.uid)
}

fn exec_as_agent_user(run: &BackendRun<'_>) -> Result<String> {
    let Some(agent) = User::from_name(AGENT_USER).context("lookup agent service user")? else {
        bail!("agent service user {AGENT_USER} not found");
    };
    let uid = agent.uid;
    let gid = agent.gid;
    spawn_and_capture(run, |command| {
        unsafe {
            command.pre_exec(move || drop_to_agent_user(uid, gid));
        }
        Ok(())
    })
}

unsafe fn drop_to_agent_user(uid: Uid, gid: Gid) -> std::io::Result<()> {
    nix::unistd::setgid(gid).map_err(io_error)?;
    nix::unistd::setuid(uid).map_err(io_error)?;
    Ok(())
}

fn io_error(err: nix::errno::Errno) -> std::io::Error {
    std::io::Error::new(
        std::io::ErrorKind::PermissionDenied,
        format!("[{PROCESS_NAME}] {err}"),
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn supervisor_identity_matches_agent_user_when_present() {
        let is_agent = User::from_name(AGENT_USER)
            .expect("lookup dd-agent")
            .map(|agent| nix::unistd::getuid() == agent.uid)
            .unwrap_or(false);
        assert_eq!(
            supervisor_runs_as_agent_user().expect("supervisor check"),
            is_agent
        );
    }
}
