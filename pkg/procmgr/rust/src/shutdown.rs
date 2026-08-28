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

    pub(crate) fn prefer_service_stop(fallback: Self) -> Self {
        #[cfg(windows)]
        {
            if let Some(signal_time) = crate::platform::service_stop_signal_time() {
                return Self::service_stop(signal_time);
            }
        }
        fallback
    }

    pub(crate) fn refresh(self) -> Self {
        Self::prefer_service_stop(self)
    }

    pub(crate) fn for_single_stop() -> Self {
        Self::prefer_service_stop(Self::unlimited(Instant::now()))
    }

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

    pub(crate) fn remaining_cap(&self, cap: Duration) -> Duration {
        self.deadline
            .map(|deadline| cap.min(deadline.saturating_duration_since(Instant::now())))
            .unwrap_or(cap)
    }

    pub(crate) fn is_bounded(&self) -> bool {
        self.deadline.is_some()
    }

    /// Remaining SCM shutdown budget for a caller-defined cap, if a service stop is in progress.
    #[cfg(windows)]
    pub(crate) fn remaining_service_stop_cap(cap: Duration) -> Option<Duration> {
        crate::platform::service_stop_signal_time()
            .map(|signal_time| Self::service_stop(signal_time).remaining_cap(cap))
    }
}

pub(crate) async fn wait_graceful_or_shutdown<T, F: std::future::Future<Output = T>>(
    graceful_budget: Duration,
    fut: F,
) -> Result<T, tokio::time::error::Elapsed> {
    #[cfg(windows)]
    {
        let notified = crate::platform::shutdown_notify().notified();
        tokio::pin!(notified);
        notified.as_mut().enable();
        let budget = if crate::platform::shutdown_requested() {
            ShutdownBudget::remaining_service_stop_cap(graceful_budget).unwrap_or(graceful_budget)
        } else {
            graceful_budget
        };
        if budget.is_zero() {
            return tokio::time::timeout(Duration::ZERO, std::future::pending::<T>()).await;
        }
        tokio::select! {
            biased;
            _ = notified => {
                tokio::time::timeout(Duration::ZERO, std::future::pending::<T>()).await
            }
            result = tokio::time::timeout(budget, fut) => result,
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

    #[tokio::test]
    async fn test_already_requested_shutdown_still_waits_graceful_budget() {
        let _guard = crate::platform::test_shutdown_lock().await;
        crate::platform::reset_shutdown_state_for_test();
        crate::platform::signal_shutdown_for_test();

        let started = Instant::now();
        let result =
            wait_graceful_or_shutdown(Duration::from_millis(150), std::future::pending::<()>())
                .await;
        crate::platform::reset_shutdown_state_for_test();

        assert!(result.is_err(), "pending future should time out");
        let elapsed = started.elapsed();
        assert!(
            elapsed >= Duration::from_millis(100),
            "already-requested shutdown must not skip the graceful budget, elapsed {elapsed:?}"
        );
    }

    #[test]
    fn test_prefer_service_stop_without_signal_preserves_fallback() {
        let signal_time = Instant::now();
        let fallback = ShutdownBudget::unlimited(signal_time);
        let budget = ShutdownBudget::prefer_service_stop(fallback);
        let graceful = budget.graceful_budget(Duration::from_secs(90));
        assert!(
            graceful >= Duration::from_secs(89) && graceful <= Duration::from_secs(90),
            "expected ~90s graceful budget without SCM signal, got {graceful:?}"
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
