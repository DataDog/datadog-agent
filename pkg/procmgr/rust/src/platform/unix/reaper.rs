// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Orphan reaping for the container-init role.
//!
//! Peeks with `WNOWAIT` so supervised catalog children stay for Tokio's
//! `Child.wait`. [`spawn_registered`] and the reap pass share one lock so a
//! just-spawned pid cannot be reaped before it is registered. Linux only;
//! other Unix builds keep the registry for uniform spawn code.

use std::collections::BTreeSet;
use std::io;
use std::sync::{Mutex, MutexGuard, PoisonError};

use tokio::process::Child;

static SUPERVISED: Mutex<BTreeSet<u32>> = Mutex::new(BTreeSet::new());

fn supervised() -> MutexGuard<'static, BTreeSet<u32>> {
    SUPERVISED.lock().unwrap_or_else(PoisonError::into_inner)
}

/// Holds a catalog pid in [`SUPERVISED`] until dropped (after Tokio reaps it).
#[derive(Debug)]
pub(crate) struct SupervisedPid(u32);

impl Drop for SupervisedPid {
    fn drop(&mut self) {
        supervised().remove(&self.0);
    }
}

/// Spawn under the supervised lock so the reaper cannot see the pid first.
pub(crate) fn spawn_registered(
    spawn: impl FnOnce() -> io::Result<Child>,
) -> io::Result<(Child, Option<SupervisedPid>)> {
    let mut set = supervised();
    let child = spawn()?;
    let guard = child.id().map(|pid| {
        set.insert(pid);
        SupervisedPid(pid)
    });
    Ok((child, guard))
}

#[cfg(test)]
fn is_supervised(pid: u32) -> bool {
    supervised().contains(&pid)
}

#[must_use = "dropping the reaper stops it"]
pub struct Reaper {
    #[cfg(target_os = "linux")]
    task: Option<tokio::task::JoinHandle<()>>,
}

#[cfg(target_os = "linux")]
impl Drop for Reaper {
    fn drop(&mut self) {
        if let Some(task) = self.task.take() {
            task.abort();
        }
    }
}

/// Become a child subreaper and start collecting orphans. No-op off Linux.
#[cfg(target_os = "linux")]
pub fn start() -> Reaper {
    use nix::sys::prctl::set_child_subreaper;

    match set_child_subreaper(true) {
        Ok(()) => log::info!("registered as child subreaper"),
        Err(e) => log::warn!("PR_SET_CHILD_SUBREAPER failed, orphans will not be adopted: {e}"),
    }
    Reaper {
        task: Some(tokio::spawn(reap_loop())),
    }
}

#[cfg(not(target_os = "linux"))]
pub fn start() -> Reaper {
    Reaper {}
}

/// Poll interval when SIGCHLD is quiet (Tokio also consumes SIGCHLD).
#[cfg(target_os = "linux")]
const IDLE_POLL: std::time::Duration = std::time::Duration::from_millis(500);

/// Backoff while a supervised zombie is at the head of the waitid queue.
#[cfg(target_os = "linux")]
const KNOWN_PENDING_RETRY: std::time::Duration = std::time::Duration::from_millis(20);

#[cfg(target_os = "linux")]
async fn reap_loop() {
    use tokio::signal::unix::{SignalKind, signal};

    let mut sigchld = match signal(SignalKind::child()) {
        Ok(s) => Some(s),
        Err(e) => {
            log::warn!("cannot subscribe to SIGCHLD, falling back to polling: {e}");
            None
        }
    };
    loop {
        let delay = match reap_pass() {
            PassOutcome::Drained => IDLE_POLL,
            PassOutcome::KnownPending => KNOWN_PENDING_RETRY,
        };
        let woken = async {
            match sigchld.as_mut() {
                Some(s) => {
                    s.recv().await;
                }
                None => std::future::pending::<()>().await,
            }
        };
        tokio::select! {
            () = woken => {}
            () = tokio::time::sleep(delay) => {}
        }
    }
}

#[cfg(target_os = "linux")]
#[derive(Debug, PartialEq, Eq)]
enum PassOutcome {
    Drained,
    KnownPending,
}

#[cfg(target_os = "linux")]
fn reap_pass() -> PassOutcome {
    use nix::errno::Errno;
    use nix::sys::wait::{Id, WaitPidFlag, WaitStatus, waitid, waitpid};

    let known = supervised();
    loop {
        let peek = waitid(
            Id::All,
            WaitPidFlag::WEXITED | WaitPidFlag::WNOHANG | WaitPidFlag::WNOWAIT,
        );
        let status = match peek {
            Ok(WaitStatus::StillAlive) => return PassOutcome::Drained,
            Ok(status) => status,
            Err(Errno::EINTR) => continue,
            Err(Errno::ECHILD) => return PassOutcome::Drained,
            Err(e) => {
                log::warn!("waitid failed: {e}");
                return PassOutcome::Drained;
            }
        };
        let Some(pid) = status.pid() else {
            return PassOutcome::Drained;
        };
        if u32::try_from(pid.as_raw()).is_ok_and(|raw| known.contains(&raw)) {
            return PassOutcome::KnownPending;
        }
        match waitpid(pid, Some(WaitPidFlag::WNOHANG)) {
            Ok(reaped) => log::info!("reaped orphaned child pid={pid}: {reaped:?}"),
            Err(e) => log::warn!("failed to reap orphaned child pid={pid}: {e}"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Above typical pid_max so unit tests never collide with real pids.
    const FAKE_PID_A: u32 = 4_100_000_001;
    const FAKE_PID_B: u32 = 4_100_000_002;
    const FAKE_PID_C: u32 = 4_100_000_003;

    #[test]
    fn guard_registers_until_dropped() {
        supervised().insert(FAKE_PID_A);
        let guard = SupervisedPid(FAKE_PID_A);
        assert!(is_supervised(FAKE_PID_A));
        drop(guard);
        assert!(!is_supervised(FAKE_PID_A));
    }

    #[test]
    fn guards_are_independent_per_pid() {
        supervised().insert(FAKE_PID_B);
        supervised().insert(FAKE_PID_C);
        let guard_b = SupervisedPid(FAKE_PID_B);
        let guard_c = SupervisedPid(FAKE_PID_C);
        drop(guard_c);
        assert!(is_supervised(FAKE_PID_B));
        assert!(!is_supervised(FAKE_PID_C));
        drop(guard_b);
    }

    #[tokio::test]
    async fn spawn_registered_tracks_the_child_pid() {
        let (child, guard) = spawn_registered(|| {
            let (cmd, args) = crate::test_helpers::true_cmd();
            tokio::process::Command::new(cmd).args(args).spawn()
        })
        .expect("spawn");
        let pid = child.id().expect("pid");
        assert!(is_supervised(pid));
        let mut child = child;
        child.wait().await.expect("wait");
        drop(guard);
        assert!(!is_supervised(pid));
    }

    #[test]
    fn spawn_registered_failure_registers_nothing() {
        let result = spawn_registered(|| Err(io::Error::other("boom")));
        assert!(result.is_err());
    }
}
