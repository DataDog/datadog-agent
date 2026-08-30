// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::shutdown::ShutdownBudget;
use log::warn;
use std::sync::Mutex;
use std::time::Duration;
use tokio::task::JoinHandle;

static DEFERRED_SPAWN_JOINS: Mutex<Vec<JoinHandle<()>>> = Mutex::new(Vec::new());

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
            "deferred spawn cleanup budget exhausted before runtime shutdown; aborting deferred spawn"
        );
        handle.abort();
        return;
    }

    if !budget.is_bounded() {
        log_join_result(handle.await);
        return;
    }

    let abort = handle.abort_handle();
    tokio::select! {
        result = handle => log_join_result(result),
        _ = tokio::time::sleep(cap) => {
            warn!(
                "timed out waiting for deferred spawn cleanup ({cap:?} left before runtime shutdown); aborting deferred spawn"
            );
            abort.abort();
        }
    }
}

fn log_join_result(result: Result<(), tokio::task::JoinError>) {
    match result {
        Ok(()) => {}
        Err(error) if error.is_cancelled() => {}
        Err(error) => warn!("deferred spawn task failed: {error}"),
    }
}

#[cfg(all(test, not(windows)))]
pub(in crate::manager) fn clear_deferred_spawn_joins_for_test() {
    DEFERRED_SPAWN_JOINS.lock().unwrap().clear();
}

#[cfg(all(test, not(windows)))]
mod tests {
    use super::*;
    use std::sync::OnceLock;
    use std::sync::{
        Arc,
        atomic::{AtomicBool, Ordering},
    };
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
    async fn zero_budget_aborts_deferred_spawn() {
        let _guard = test_lock().await;
        clear_deferred_spawn_joins_for_test();
        let completed = Arc::new(AtomicBool::new(false));
        let completed_flag = Arc::clone(&completed);
        register_deferred_spawn_join(tokio::spawn(async move {
            tokio::time::sleep(Duration::from_secs(60)).await;
            completed_flag.store(true, Ordering::SeqCst);
        }));

        let budget = ShutdownBudget::with_deadline(
            Instant::now().checked_sub(Duration::from_secs(1)).unwrap(),
            Instant::now(),
        );
        join_deferred_spawn_tasks(budget).await;
        tokio::time::sleep(Duration::from_millis(100)).await;
        assert!(
            !completed.load(Ordering::SeqCst),
            "zero budget should abort deferred spawn instead of letting it run"
        );
    }
}
