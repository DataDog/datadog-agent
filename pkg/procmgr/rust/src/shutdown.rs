// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::process::ManagedProcess;
use log::{info, warn};
use nix::sys::signal::Signal;
use tokio::time::{Duration, timeout};

const SIGKILL_TIMEOUT: Duration = Duration::from_secs(10);

/// Send SIGTERM to all running processes, wait per-process `stop_timeout`, then SIGKILL stragglers.
pub async fn shutdown_all(processes: &mut [ManagedProcess]) {
    for proc in processes.iter() {
        if proc.is_running() {
            info!("[{}] sending SIGTERM", proc.name);
            proc.send_signal(Signal::SIGTERM);
        }
    }

    for proc in processes.iter_mut() {
        if !proc.is_running() {
            continue;
        }
        if !proc.has_child_handle() {
            // Child handle is with the watcher task; SIGTERM was sent by PID above.
            proc.mark_stopped();
            continue;
        }
        let stop = proc.stop_timeout();
        if timeout(stop, proc.wait()).await.is_err() {
            warn!(
                "[{}] stop timeout ({}s) reached, sending SIGKILL",
                proc.name,
                stop.as_secs()
            );
            proc.send_signal(Signal::SIGKILL);
            if timeout(SIGKILL_TIMEOUT, proc.wait()).await.is_err() {
                warn!("[{}] still running after SIGKILL, giving up", proc.name);
            }
        }
        proc.mark_stopped();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::process::tests::make_config;
    use crate::state::ProcessState;

    #[tokio::test]
    async fn test_shutdown_all_graceful() {
        let cfg1 = make_config("/bin/sleep", vec!["60"]);
        let cfg2 = make_config("/bin/sleep", vec!["60"]);

        let mut p1 = ManagedProcess::new("p1".into(), cfg1);
        let mut p2 = ManagedProcess::new("p2".into(), cfg2);
        p1.spawn().unwrap();
        p2.spawn().unwrap();

        let mut procs = vec![p1, p2];
        shutdown_all(&mut procs).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert_eq!(procs[1].state(), ProcessState::Stopped);
        assert!(!procs[0].is_running());
        assert!(!procs[1].is_running());
    }

    #[tokio::test]
    async fn test_shutdown_all_empty() {
        let mut procs: Vec<ManagedProcess> = vec![];
        shutdown_all(&mut procs).await;
    }

    #[tokio::test]
    async fn test_shutdown_all_sigkill_on_timeout() {
        let mut cfg = make_config("/bin/sh", vec!["-c", "trap '' TERM; sleep 60"]);
        cfg.stop_timeout = Some(1);
        let mut proc = ManagedProcess::new("stubborn".into(), cfg);
        proc.spawn().unwrap();

        let mut procs = vec![proc];
        shutdown_all(&mut procs).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
    }

    #[tokio::test]
    async fn test_shutdown_all_after_take_child() {
        let mut proc = ManagedProcess::new("t".into(), make_config("/bin/sleep", vec!["60"]));
        proc.spawn().unwrap();
        let _child = proc.take_child();

        assert!(proc.is_running(), "state should still be Running");
        let mut procs = vec![proc];
        shutdown_all(&mut procs).await;
        assert_eq!(
            procs[0].state(),
            ProcessState::Stopped,
            "shutdown should transition to Stopped even without child handle"
        );
    }
}
