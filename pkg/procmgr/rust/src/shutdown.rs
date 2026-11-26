// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::process::ManagedProcess;
#[cfg(test)]
use crate::process::test_exit_channel;

/// Signals each process in `order` and starts its wait right after its own signal, then
/// runs all the waits concurrently. Each process is force-killed at its own deadline, so
/// a dependency with a short `stop_timeout` can be killed while its dependents are still
/// shutting down.
pub async fn shutdown_ordered(processes: &mut [ManagedProcess], order: &[usize]) {
    let mut waits = Vec::new();
    for &idx in order {
        processes[idx].request_stop();
        if let Some(wait) = processes[idx].take_stop_wait() {
            waits.push((idx, tokio::spawn(wait.run())));
        }
    }
    let mut failed = Vec::new();
    for (idx, wait) in waits {
        if let Err(error) = wait.await {
            log::warn!(
                "[{}] stop wait failed: {error}; leaving its state unfinalized",
                processes[idx].name()
            );
            failed.push(idx);
        }
    }
    for &idx in order {
        if !failed.contains(&idx) {
            processes[idx].finish_stop();
        }
    }
}

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
    #[cfg(unix)]
    use tokio::time::Duration;

    fn sleep_config() -> crate::config::ProcessConfig {
        test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS)
    }

    #[tokio::test]
    async fn test_shutdown_all_graceful() {
        let cfg1 = sleep_config();
        let cfg2 = sleep_config();

        let mut p1 = ManagedProcess::new_config("p1".into(), test_helpers::test_uuid(), cfg1);
        let mut p2 = ManagedProcess::new_config("p2".into(), test_helpers::test_uuid(), cfg2);
        p1.spawn(test_exit_channel().0).unwrap();
        p2.spawn(test_exit_channel().0).unwrap();

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
    async fn test_shutdown_graceful_stop_signal() {
        use std::time::Instant;

        let mut proc = ManagedProcess::new_config(
            "graceful".into(),
            test_helpers::test_uuid(),
            test_helpers::graceful_stop_test_config(),
        );
        let (tx, exit_rx) = test_exit_channel();
        #[cfg(windows)]
        let mut exit_rx = exit_rx;
        proc.spawn(tx).unwrap();
        assert!(proc.is_running());

        // Only the detached helper attaches to the workload console. Caller
        // membership and all standard slots must remain exactly unchanged.
        #[cfg(windows)]
        let console_before = crate::platform::caller_console_state();

        let started = Instant::now();
        proc.request_stop();
        proc.wait_for_stop().await;

        #[cfg(windows)]
        assert_eq!(
            crate::platform::caller_console_state(),
            console_before,
            "graceful stop must leave the caller console as it found it"
        );

        assert_eq!(proc.state(), ProcessState::Stopped);
        #[cfg(windows)]
        {
            let status = exit_rx.recv().await.expect("graceful workload exit").status;
            // A CTRL_BREAK beating the workload's handler ends it through the default handler.
            assert!(
                status.success()
                    || status.code() == Some(windows_sys::Win32::Foundation::STATUS_CONTROL_C_EXIT),
                "CTRL_BREAK should be delivered, not escalated to job force-kill: {status:?}"
            );
        }
        #[cfg(not(windows))]
        let _ = exit_rx;
        assert!(
            started.elapsed().as_secs() < 2,
            "graceful stop should not wait for stop_timeout force-kill (took {:?})",
            started.elapsed()
        );
    }

    #[tokio::test]
    async fn test_shutdown_all_sigkill_on_timeout() {
        // Use ping (via sleep_config) instead of powershell: ping ignores graceful
        // stop on Windows, and powershell.exe is not always on PATH in CI containers.
        let mut proc = ManagedProcess::new_config(
            "stubborn".into(),
            test_helpers::test_uuid(),
            sleep_config(),
        );
        proc.spawn(test_exit_channel().0).unwrap();

        let mut procs = vec![proc];
        shutdown_all(&mut procs).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
    }

    /// Two SIGTERM-ignoring children share one grace period, so shutdown takes about one grace.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_shutdown_stubborn_children_share_one_grace() {
        use std::time::Instant;

        let dir = tempfile::tempdir().unwrap();
        let mut procs = Vec::new();
        for name in ["stubborn-a", "stubborn-b"] {
            let ready = dir.path().join(format!("{name}.ready"));
            let (cmd, args) = test_helpers::trap_term_sleep_ready(&ready);
            let mut config = test_helpers::make_config(cmd, args);
            config.stop_timeout = Some(2);
            let mut proc =
                ManagedProcess::new_config(name.into(), test_helpers::test_uuid(), config);
            proc.spawn(test_exit_channel().0).unwrap();
            test_helpers::wait_for_file(&ready, Duration::from_secs(10)).await;
            procs.push(proc);
        }

        let started = Instant::now();
        shutdown_all(&mut procs).await;
        let elapsed = started.elapsed();

        for proc in &procs {
            assert_eq!(proc.state(), ProcessState::Stopped);
        }
        assert!(
            elapsed < Duration::from_millis(3500),
            "two 2 s stops ran one after another: shutdown took {elapsed:?}"
        );
        assert!(
            elapsed >= Duration::from_millis(1900),
            "shutdown ended before the 2 s grace ran out: took {elapsed:?}"
        );
    }

    /// A short-timeout child is force-killed on its own deadline, not the longer child's.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_shutdown_short_timeout_killed_on_its_own_deadline() {
        use std::os::unix::process::ExitStatusExt;
        use std::time::Instant;

        let dir = tempfile::tempdir().unwrap();
        let mut procs = Vec::new();
        let mut exits = Vec::new();
        // The long-timeout child is listed first, so a sequential wait would block on it
        // before the short child's deadline is checked.
        for (name, stop_timeout) in [("slow-stop", 3), ("fast-stop", 1)] {
            let ready = dir.path().join(format!("{name}.ready"));
            let (cmd, args) = test_helpers::trap_term_sleep_ready(&ready);
            let mut config = test_helpers::make_config(cmd, args);
            config.stop_timeout = Some(stop_timeout);
            let mut proc =
                ManagedProcess::new_config(name.into(), test_helpers::test_uuid(), config);
            let (tx, rx) = test_exit_channel();
            proc.spawn(tx).unwrap();
            test_helpers::wait_for_file(&ready, Duration::from_secs(10)).await;
            procs.push(proc);
            exits.push(rx);
        }

        // Record when each child actually exits, and how.
        let exit_tasks: Vec<_> = exits
            .into_iter()
            .map(|mut rx| {
                tokio::spawn(async move {
                    let event = rx.recv().await.expect("exit event");
                    (Instant::now(), event.status)
                })
            })
            .collect();

        let started = Instant::now();
        shutdown_all(&mut procs).await;

        for proc in &procs {
            assert_eq!(proc.state(), ProcessState::Stopped);
        }
        let mut results = Vec::new();
        for task in exit_tasks {
            let (at, status) = task.await.unwrap();
            results.push((at.saturating_duration_since(started), status));
        }
        let (slow_exit, slow_status) = &results[0];
        let (fast_exit, fast_status) = &results[1];

        assert!(
            *fast_exit >= Duration::from_millis(1000) && *fast_exit < Duration::from_millis(2000),
            "fast-stop should be force-killed at its 1 s deadline: exited at {fast_exit:?}"
        );
        assert!(
            *slow_exit >= Duration::from_millis(2500) && *slow_exit < Duration::from_millis(4500),
            "slow-stop should be force-killed at its 3 s deadline: exited at {slow_exit:?}"
        );
        for status in [slow_status, fast_status] {
            assert_eq!(
                status.signal(),
                Some(nix::sys::signal::Signal::SIGKILL as i32),
                "child should have been killed by SIGKILL: {status:?}"
            );
        }
    }

    #[tokio::test]
    async fn test_shutdown_all_after_spawn_watcher_owns_handle() {
        let mut proc =
            ManagedProcess::new_config("t".into(), test_helpers::test_uuid(), sleep_config());
        proc.spawn(test_exit_channel().0).unwrap();
        assert!(proc.is_running(), "state should still be Running");
        let mut procs = vec![proc];
        shutdown_all(&mut procs).await;
        assert_eq!(
            procs[0].state(),
            ProcessState::Stopped,
            "shutdown should transition to Stopped via exit watcher"
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
        p1.spawn(test_exit_channel().0).unwrap();
        p2.spawn(test_exit_channel().0).unwrap();
        p3.spawn(test_exit_channel().0).unwrap();

        let mut procs = vec![p1, p2, p3];
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
