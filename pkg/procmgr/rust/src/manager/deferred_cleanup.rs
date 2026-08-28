// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::shutdown::ShutdownBudget;
use log::warn;
use std::future::Future;
use std::sync::Mutex;
use std::thread::{self, JoinHandle as ThreadJoinHandle};
use std::time::{Duration, Instant};
use tokio::task::JoinHandle;

static DEFERRED_SPAWN_JOINS: Mutex<Vec<JoinHandle<()>>> = Mutex::new(Vec::new());
static OFFLOADED_CLEANUP_THREADS: Mutex<Vec<ThreadJoinHandle<()>>> = Mutex::new(Vec::new());

pub(in crate::manager) fn register_deferred_spawn_join(handle: JoinHandle<()>) {
    DEFERRED_SPAWN_JOINS.lock().unwrap().push(handle);
}

/// Join spawn tasks deferred during bounded supervisor teardown.
///
/// Runs on the service runtime before `shutdown_timeout` so in-flight
/// `spawn_process` work can still commit or abort while the runtime is alive.
#[cfg_attr(not(windows), allow(dead_code))]
pub(crate) async fn join_deferred_spawn_tasks(budget: ShutdownBudget) {
    let handles = std::mem::take(&mut *DEFERRED_SPAWN_JOINS.lock().unwrap());
    for handle in handles {
        join_deferred_spawn_task(handle, &budget).await;
    }
}

async fn join_deferred_spawn_task(handle: JoinHandle<()>, budget: &ShutdownBudget) {
    let cap = budget.remaining_cap(Duration::MAX);
    if budget.is_bounded() && cap.is_zero() {
        warn!(
            "deferred spawn cleanup budget exhausted before runtime shutdown; offloading cleanup thread"
        );
        offload_spawn_join(handle);
        return;
    }

    if !budget.is_bounded() {
        log_join_result(handle.await);
        return;
    }

    let mut monitor = Some(tokio::spawn(handle));
    tokio::select! {
        result = async {
            monitor
                .take()
                .expect("deferred spawn monitor should be present")
                .await
        } => log_monitor_result(result),
        _ = tokio::time::sleep(cap) => {
            warn!(
                "timed out waiting for deferred spawn cleanup ({cap:?} left before runtime shutdown); offloading cleanup thread"
            );
            if let Some(monitor) = monitor.take() {
                offload_spawn_monitor(monitor);
            }
        }
    }
}

fn offload_cleanup<F>(cleanup: F)
where
    F: Future<Output = ()> + Send + 'static,
{
    let runtime_handle = tokio::runtime::Handle::current();
    OFFLOADED_CLEANUP_THREADS
        .lock()
        .unwrap()
        .push(thread::spawn(move || {
            runtime_handle.block_on(cleanup);
        }));
}

fn offload_spawn_join(handle: JoinHandle<()>) {
    offload_cleanup(async move {
        log_join_result(handle.await);
    });
}

fn offload_spawn_monitor(monitor: JoinHandle<Result<(), tokio::task::JoinError>>) {
    offload_cleanup(async move {
        log_monitor_result(monitor.await);
    });
}

/// Wait for cleanup threads offloaded from the service runtime.
#[cfg_attr(not(windows), allow(dead_code))]
pub(crate) fn join_offloaded_cleanup_threads(timeout: Duration) {
    let threads = std::mem::take(&mut *OFFLOADED_CLEANUP_THREADS.lock().unwrap());
    let deadline = Instant::now() + timeout;
    for thread in threads {
        let remaining = deadline.saturating_duration_since(Instant::now());
        if remaining.is_zero() {
            warn!("offloaded spawn cleanup budget exhausted; abandoning remaining cleanup threads");
            break;
        }
        if !wait_thread_join(thread, remaining) {
            warn!("timed out waiting for offloaded spawn cleanup thread");
        }
    }
}

fn wait_thread_join(thread: ThreadJoinHandle<()>, timeout: Duration) -> bool {
    let (done_tx, done_rx) = std::sync::mpsc::sync_channel(0);
    thread::spawn(move || {
        let _ = thread.join();
        let _ = done_tx.send(());
    });
    done_rx.recv_timeout(timeout).is_ok()
}

fn log_monitor_result(result: Result<Result<(), tokio::task::JoinError>, tokio::task::JoinError>) {
    match result {
        Ok(inner) => log_join_result(inner),
        Err(error) => warn!("deferred spawn monitor failed: {error}"),
    }
}

fn log_join_result(result: Result<(), tokio::task::JoinError>) {
    match result {
        Ok(()) => {}
        Err(error) if error.is_cancelled() => {}
        Err(error) => warn!("deferred spawn task failed: {error}"),
    }
}

#[cfg(test)]
pub(in crate::manager) fn clear_deferred_spawn_joins_for_test() {
    DEFERRED_SPAWN_JOINS.lock().unwrap().clear();
    OFFLOADED_CLEANUP_THREADS.lock().unwrap().clear();
}

#[cfg(test)]
pub(in crate::manager) fn offloaded_cleanup_thread_count_for_test() -> usize {
    OFFLOADED_CLEANUP_THREADS.lock().unwrap().len()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::OnceLock;
    use std::time::Instant;

    async fn test_lock() -> tokio::sync::MutexGuard<'static, ()> {
        static LOCK: OnceLock<tokio::sync::Mutex<()>> = OnceLock::new();
        LOCK.get_or_init(|| tokio::sync::Mutex::new(()))
            .lock()
            .await
    }

    #[tokio::test]
    async fn join_deferred_spawn_tasks_waits_for_registered_handle() {
        let _guard = test_lock().await;
        clear_deferred_spawn_joins_for_test();
        let (release_tx, release_rx) = tokio::sync::oneshot::channel::<()>();
        register_deferred_spawn_join(tokio::spawn(async move {
            release_rx.await.ok();
        }));

        let mut join_task = tokio::spawn(async {
            join_deferred_spawn_tasks(ShutdownBudget::unlimited(Instant::now())).await;
        });
        assert!(
            tokio::time::timeout(Duration::from_millis(50), &mut join_task)
                .await
                .is_err(),
            "join should block until deferred spawn completes"
        );

        release_tx.send(()).unwrap();
        join_task.await.unwrap();
    }

    #[tokio::test]
    async fn zero_budget_offloads_cleanup_thread() {
        let _guard = test_lock().await;
        clear_deferred_spawn_joins_for_test();
        let (release_tx, release_rx) = tokio::sync::oneshot::channel::<()>();
        register_deferred_spawn_join(tokio::spawn(async move {
            release_rx.await.ok();
        }));

        let budget = ShutdownBudget::with_deadline(
            Instant::now().checked_sub(Duration::from_secs(1)).unwrap(),
            Instant::now(),
        );
        join_deferred_spawn_tasks(budget).await;
        assert!(
            offloaded_cleanup_thread_count_for_test() > 0,
            "zero budget should offload cleanup instead of dropping handle"
        );

        release_tx.send(()).unwrap();
        join_offloaded_cleanup_threads(Duration::from_secs(1));
    }
}
