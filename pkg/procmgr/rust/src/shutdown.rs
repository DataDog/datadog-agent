// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::process::ManagedProcess;
use std::time::{Duration, Instant};

/// Shared time budget for ordered child shutdown.
///
/// When a deadline is set (Windows service stop), graceful, force-kill, and
/// job-drain waits all consume the same remaining time instead of each phase
/// applying its own cap serially per child.
#[derive(Clone, Copy, Debug)]
pub(crate) struct ShutdownBudget {
    signal_time: Instant,
    deadline: Option<Instant>,
}

impl ShutdownBudget {
    /// No service-wide cap; each phase uses its configured per-phase limit.
    pub(crate) fn unlimited(signal_time: Instant) -> Self {
        Self {
            signal_time,
            deadline: None,
        }
    }

    /// Prefer the SCM-anchored budget when a Windows service stop is in progress.
    pub(crate) fn prefer_service_stop(fallback: Self) -> Self {
        #[cfg(windows)]
        {
            if let Some(signal_time) = crate::platform::service_stop_signal_time() {
                return Self::service_stop(signal_time);
            }
        }
        fallback
    }

    /// Re-read the active service stop signal (for example after graceful wait).
    pub(crate) fn refresh(self) -> Self {
        Self::prefer_service_stop(self)
    }

    /// Budget for a single Stop RPC or child teardown outside ordered shutdown.
    pub(crate) fn for_single_stop() -> Self {
        Self::prefer_service_stop(Self::unlimited(Instant::now()))
    }

    /// Budget for ordered shutdown after a service stop signal.
    pub(crate) fn service_stop(signal_time: Instant) -> Self {
        #[cfg(windows)]
        {
            Self {
                signal_time,
                deadline: Some(crate::platform::service_shutdown_deadline(signal_time)),
            }
        }
        #[cfg(not(windows))]
        {
            Self::unlimited(signal_time)
        }
    }

    #[cfg(test)]
    pub(crate) fn with_deadline(signal_time: Instant, deadline: Instant) -> Self {
        Self {
            signal_time,
            deadline: Some(deadline),
        }
    }

    pub(crate) fn graceful_budget(&self, stop_timeout: Duration) -> Duration {
        let from_stop_timeout = stop_timeout.saturating_sub(self.signal_time.elapsed());
        self.remaining_cap(from_stop_timeout)
    }

    /// Remaining time for a phase, capped by `cap` and the service deadline.
    pub(crate) fn remaining_cap(&self, cap: Duration) -> Duration {
        self.deadline
            .map(|deadline| cap.min(deadline.saturating_duration_since(Instant::now())))
            .unwrap_or(cap)
    }

    pub(crate) fn is_bounded(&self) -> bool {
        self.deadline.is_some()
    }
}

/// Wait for graceful child exit, or cut short when SCM requests service shutdown.
pub(crate) async fn wait_graceful_or_shutdown<T, F: std::future::Future<Output = T>>(
    graceful_budget: Duration,
    fut: F,
) -> Result<T, tokio::time::error::Elapsed> {
    #[cfg(windows)]
    {
        tokio::select! {
            _ = crate::platform::shutdown_notify().notified() => {
                Err(tokio::time::error::Elapsed(()))
            }
            result = tokio::time::timeout(graceful_budget, fut) => result,
        }
    }
    #[cfg(not(windows))]
    {
        tokio::time::timeout(graceful_budget, fut).await
    }
}

pub async fn shutdown_ordered(processes: &mut [ManagedProcess], order: &[usize]) {
    for &idx in order {
        processes[idx].request_stop();
    }
    let signal_time = crate::platform::service_stop_signal_time().unwrap_or_else(Instant::now);
    let budget = ShutdownBudget::service_stop(signal_time);
    for &idx in order {
        processes[idx].wait_for_stop_since(budget).await;
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
    use std::time::Duration;

    fn sleep_config() -> crate::config::ProcessConfig {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        test_helpers::make_config(cmd, args)
    }

    #[test]
    fn test_prefer_service_stop_without_signal_preserves_fallback() {
        let signal_time = Instant::now();
        let fallback = ShutdownBudget::unlimited(signal_time);
        let budget = ShutdownBudget::prefer_service_stop(fallback);
        assert_eq!(
            budget.graceful_budget(Duration::from_secs(90)),
            Duration::from_secs(90)
        );
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
        let mut cfg = test_helpers::make_config(&cmd, args);
        cfg.stop_timeout = Some(1);
        let mut proc =
            ManagedProcess::new_config("stubborn".into(), test_helpers::test_uuid(), cfg);
        proc.spawn().unwrap();

        let mut procs = vec![proc];
        shutdown_all(&mut procs).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
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

    #[tokio::test]
    async fn test_shutdown_ordered_shared_graceful_budget() {
        let (cmd, args) = test_helpers::trap_term_sleep();
        let mut cfg = test_helpers::make_config(&cmd, args);
        cfg.stop_timeout = Some(2);

        let mut p1 =
            ManagedProcess::new_config("p1".into(), test_helpers::test_uuid(), cfg.clone());
        let mut p2 = ManagedProcess::new_config("p2".into(), test_helpers::test_uuid(), cfg);
        p1.spawn().unwrap();
        p2.spawn().unwrap();

        let mut procs = vec![p1, p2];
        let started = Instant::now();
        shutdown_ordered(&mut procs, &[0, 1]).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert_eq!(procs[1].state(), ProcessState::Stopped);
        assert!(
            started.elapsed() < Duration::from_secs(5),
            "shared graceful budget should not apply two full stop timeouts sequentially, took {:?}",
            started.elapsed()
        );
    }

    #[tokio::test]
    async fn test_shutdown_ordered_shared_force_kill_budget() {
        let (cmd, args) = test_helpers::trap_term_sleep();
        let mut cfg = test_helpers::make_config(&cmd, args);
        cfg.stop_timeout = Some(1);

        let mut procs: Vec<ManagedProcess> = (0..3)
            .map(|i| {
                ManagedProcess::new_config(format!("p{i}"), test_helpers::test_uuid(), cfg.clone())
            })
            .collect();
        for proc in &mut procs {
            proc.spawn().unwrap();
        }

        let signal_time = Instant::now();
        let budget =
            ShutdownBudget::with_deadline(signal_time, signal_time + Duration::from_secs(5));
        for proc in &mut procs {
            proc.request_stop();
        }

        let started = Instant::now();
        for proc in &mut procs {
            proc.wait_for_stop_since(budget).await;
        }

        for proc in &procs {
            assert_eq!(proc.state(), ProcessState::Stopped);
        }
        assert!(
            started.elapsed() < Duration::from_secs(8),
            "shared shutdown deadline should cap serial force-kill waits, took {:?}",
            started.elapsed()
        );
    }
}
