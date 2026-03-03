// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::process::ManagedProcess;

/// Send SIGTERM to all running processes, wait per-process `stop_timeout`, then SIGKILL stragglers.
pub async fn shutdown_all(processes: &mut [ManagedProcess]) {
    for proc in processes.iter() {
        proc.request_stop();
    }
    for proc in processes.iter_mut() {
        proc.wait_for_stop().await;
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
