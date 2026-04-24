// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::process::ManagedProcess;

/// Shut down processes in the given index order (typically reverse startup order).
/// Sends SIGTERM to all first, then waits for each in order.
pub async fn shutdown_ordered(processes: &mut [ManagedProcess], order: &[usize]) {
    for &idx in order {
        processes[idx].request_stop();
    }
    for &idx in order {
        processes[idx].wait_for_stop().await;
    }
}

/// Convenience wrapper: shut down all processes in forward index order.
#[cfg(test)]
pub async fn shutdown_all(processes: &mut [ManagedProcess]) {
    let order: Vec<usize> = (0..processes.len()).collect();
    shutdown_ordered(processes, &order).await;
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::state::ProcessState;
    use crate::test_helpers;

    fn sleep_config() -> crate::config::ProcessConfig {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        test_helpers::make_config(cmd, args)
    }

    #[tokio::test]
    async fn test_shutdown_all_graceful() {
        let cfg1 = sleep_config();
        let cfg2 = sleep_config();

        let mut p1 = ManagedProcess::new_config("p1".into(), test_helpers::test_uuid(), cfg1);
        let mut p2 = ManagedProcess::new_config("p2".into(), test_helpers::test_uuid(), cfg2);
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
        let (cmd, args) = test_helpers::trap_term_sleep();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.stop_timeout = Some(1);
        let mut proc =
            ManagedProcess::new_config("stubborn".into(), test_helpers::test_uuid(), cfg);
        proc.spawn().unwrap();

        let mut procs = vec![proc];
        shutdown_all(&mut procs).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
    }

    #[tokio::test]
    async fn test_shutdown_all_after_take_child() {
        let mut proc =
            ManagedProcess::new_config("t".into(), test_helpers::test_uuid(), sleep_config());
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

    #[tokio::test]
    async fn test_shutdown_ordered_reverse() {
        let mut p1 =
            ManagedProcess::new_config("p1".into(), test_helpers::test_uuid(), sleep_config());
        let mut p2 =
            ManagedProcess::new_config("p2".into(), test_helpers::test_uuid(), sleep_config());
        let mut p3 =
            ManagedProcess::new_config("p3".into(), test_helpers::test_uuid(), sleep_config());
        p1.spawn().unwrap();
        p2.spawn().unwrap();
        p3.spawn().unwrap();

        let mut procs = vec![p1, p2, p3];
        // Reverse order: p3, p2, p1
        shutdown_ordered(&mut procs, &[2, 1, 0]).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert_eq!(procs[1].state(), ProcessState::Stopped);
        assert_eq!(procs[2].state(), ProcessState::Stopped);
    }

    #[tokio::test]
    async fn test_shutdown_ordered_empty() {
        let mut procs: Vec<ManagedProcess> = vec![];
        shutdown_ordered(&mut procs, &[]).await;
    }
}
