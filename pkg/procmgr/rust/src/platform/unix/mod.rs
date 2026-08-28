// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod runtime_user;
mod spawn;

pub(crate) use runtime_user::runtime_user_for_pid;
pub(crate) use spawn::spawn_managed_child;

use nix::sys::signal::{self, Signal};
use nix::unistd::Pid;
use std::os::unix::process::ExitStatusExt;
use std::path::PathBuf;
use tokio::process::Command;

use log::warn;
use nix::unistd::{User, geteuid};
#[cfg(test)]
use std::sync::Arc;
use std::sync::OnceLock;
use std::sync::atomic::{AtomicBool, Ordering};
use tokio::sync::Notify;

static SUPERVISOR_SPAWN_USER: OnceLock<String> = OnceLock::new();
static SHUTDOWN_NOTIFY: OnceLock<Notify> = OnceLock::new();
static SHUTDOWN_REQUESTED: AtomicBool = AtomicBool::new(false);

fn shutdown_notify() -> &'static Notify {
    SHUTDOWN_NOTIFY.get_or_init(Notify::new)
}

fn mark_shutdown_requested() {
    SHUTDOWN_REQUESTED.store(true, Ordering::SeqCst);
}

pub(crate) fn shutdown_requested() -> bool {
    SHUTDOWN_REQUESTED.load(Ordering::SeqCst)
}

fn resolve_supervisor_spawn_user() -> String {
    match User::from_uid(geteuid()) {
        Ok(Some(user)) => user.name,
        Ok(None) => {
            warn!("no passwd entry for supervisor uid");
            "unknown".to_string()
        }
        Err(e) => {
            warn!("could not resolve supervisor spawn user for display: {e:#}");
            "unknown".to_string()
        }
    }
}

/// Return the passwd name for procmgr's effective user (the account Unix children inherit).
/// The result is cached after the first lookup, including failures (stored as `unknown`).
pub(crate) fn spawn_user_for_supervisor() -> String {
    SUPERVISOR_SPAWN_USER
        .get_or_init(resolve_supervisor_spawn_user)
        .clone()
}

/// Place the child in its own process group so signals don't propagate
/// to the daemon itself and SIGTERM can target all descendants.
pub fn setup_process_group(cmd: &mut Command) {
    cmd.process_group(0);
}

/// Negate a PID to produce the process group ID for `kill(2)`.
/// Sending a signal to `-pgid` targets every process in the group.
pub(crate) fn process_group_id(pid: u32) -> Result<Pid, anyhow::Error> {
    use anyhow::Context;
    let raw = i32::try_from(pid).context("PID overflows i32")?;
    Ok(Pid::from_raw(-raw))
}

/// Send SIGTERM to the entire process group (graceful stop).
pub fn send_graceful_stop(pid: u32) -> Result<(), anyhow::Error> {
    use anyhow::Context;
    signal::kill(process_group_id(pid)?, Signal::SIGTERM)
        .with_context(|| format!("failed to send SIGTERM to pgid {pid}"))?;
    Ok(())
}

/// Send SIGKILL to the entire process group (force kill).
pub fn send_force_kill(pid: u32) -> Result<(), anyhow::Error> {
    use anyhow::Context;
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

pub(crate) async fn wait_for_shutdown() {
    let notified = shutdown_notify().notified();
    tokio::pin!(notified);
    notified.as_mut().enable();
    if shutdown_requested() {
        return;
    }
    use tokio::signal::unix::{SignalKind, signal};
    let mut sigterm = signal(SignalKind::terminate()).expect("failed to register SIGTERM handler");
    let mut sigint = signal(SignalKind::interrupt()).expect("failed to register SIGINT handler");
    tokio::select! {
        _ = sigterm.recv() => {
            mark_shutdown_requested();
            log::info!("received SIGTERM");
        }
        _ = sigint.recv() => {
            mark_shutdown_requested();
            log::info!("received SIGINT");
        }
        _ = notified => {
            mark_shutdown_requested();
        }
    }
}

pub async fn shutdown_signal() {
    wait_for_shutdown().await;
}

#[cfg(test)]
pub(crate) fn signal_shutdown_for_test() {
    mark_shutdown_requested();
    shutdown_notify().notify_waiters();
}

#[cfg(test)]
fn drain_shutdown_notify() {
    struct Noop;
    impl std::task::Wake for Noop {
        fn wake(self: Arc<Self>) {}
        fn wake_by_ref(self: &Arc<Self>) {}
    }
    let waker = std::task::Waker::from(Arc::new(Noop));
    let mut cx = std::task::Context::from_waker(&waker);
    let mut fut = std::pin::pin!(shutdown_notify().notified());
    let _ = fut.as_mut().poll(&mut cx);
}

#[cfg(test)]
pub(crate) fn reset_shutdown_state_for_test() {
    SHUTDOWN_REQUESTED.store(false, Ordering::SeqCst);
    drain_shutdown_notify();
}

#[cfg(test)]
pub(crate) async fn test_shutdown_lock() -> tokio::sync::MutexGuard<'static, ()> {
    static SHUTDOWN_TEST_LOCK: OnceLock<tokio::sync::Mutex<()>> = OnceLock::new();
    SHUTDOWN_TEST_LOCK
        .get_or_init(|| tokio::sync::Mutex::new(()))
        .lock()
        .await
}

pub(crate) fn service_stop_signal_time() -> Option<std::time::Instant> {
    None
}
