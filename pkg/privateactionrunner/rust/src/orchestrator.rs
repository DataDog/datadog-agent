// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! The control-plane orchestration loop: pace dequeue to the runner pool size,
//! spawn the executor on demand and gate dispatch on its readiness, run each
//! action over gRPC, and publish the outcome to OPMS. Concurrency is bounded by
//! a semaphore sized to the pool: a slot is acquired *before* dequeuing and
//! released when the action's result stream completes, so the control plane
//! never dequeues more work than it can run (PRD control-plane responsibilities).
//!
//! Heartbeats (slice 4), full drain/idle semantics (slice 5), and crash
//! fail-and-report via `Describe` (slice 6) build on this spine.

use crate::config::Config;
use crate::executor::Dispatcher;
use crate::opms::{Opms, Outcome, Task};
use crate::procmgr::ExecutorLifecycle;
use log::{error, info, warn};
use std::future::Future;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::{Duration, Instant};
use tokio::sync::Semaphore;

/// `INTERNAL_ERROR` from the ActionPlatformErrorCode proto; used when dispatch
/// itself fails (e.g. the stream breaks) so the workflow does not hang.
const INTERNAL_ERROR: i32 = 1;

/// Tuning knobs, projected from [`Config`] so the orchestrator is testable
/// without a full runner identity.
#[derive(Clone)]
pub struct Params {
    pub pool_size: usize,
    pub loop_interval: Duration,
    pub ready_timeout: Duration,
    pub idle_timeout: Duration,
    /// Only while an action's result stream is open.
    pub heartbeat_interval: Duration,
    /// Mirrors the Go circuit breaker.
    pub min_backoff: Duration,
    pub max_backoff: Duration,
    pub wait_before_retry: Duration,
    pub max_attempts: u32,
}

impl Params {
    pub fn from_config(config: &Config) -> Self {
        Params {
            pool_size: config.runner_pool_size,
            loop_interval: config.loop_interval,
            ready_timeout: config.ready_timeout,
            idle_timeout: config.idle_timeout,
            heartbeat_interval: config.heartbeat_interval,
            min_backoff: config.min_backoff,
            max_backoff: config.max_backoff,
            wait_before_retry: config.wait_before_retry,
            max_attempts: config.max_attempts,
        }
    }
}

/// Exponential dequeue backoff: `min_backoff * 2^(attempt-1)`, capped at `max_backoff`.
fn backoff_delay(attempt: u32, min: Duration, max: Duration) -> Duration {
    let factor = 2u32
        .checked_pow(attempt.saturating_sub(1))
        .unwrap_or(u32::MAX);
    min.saturating_mul(factor).min(max)
}

/// Ties OPMS, the executor lifecycle, and the executor dispatcher together.
pub struct Orchestrator<O, L, D> {
    opms: Arc<O>,
    lifecycle: Arc<L>,
    dispatcher: Arc<D>,
    params: Params,
    inflight: Arc<AtomicUsize>,
}

impl<O, L, D> Orchestrator<O, L, D>
where
    O: Opms + 'static,
    L: ExecutorLifecycle,
    D: Dispatcher,
{
    pub fn new(opms: Arc<O>, lifecycle: Arc<L>, dispatcher: Arc<D>, params: Params) -> Self {
        Orchestrator {
            opms,
            lifecycle,
            dispatcher,
            params,
            inflight: Arc::new(AtomicUsize::new(0)),
        }
    }

    /// Run the loop until `shutdown` resolves, then return (in-flight actions are
    /// left to finish; graceful drain is slice 5).
    pub async fn run<S: Future<Output = ()>>(&self, shutdown: S) {
        let sem = Arc::new(Semaphore::new(self.params.pool_size));
        let mut idle_since = Instant::now();
        // Consecutive dequeue-failure count, driving exponential backoff.
        let mut attempt: u32 = 1;
        tokio::pin!(shutdown);

        loop {
            // Acquire a pool slot *before* dequeuing so we never hold OPMS leases
            // for work we cannot run. When the pool is full this future is
            // pending, which naturally pauses dequeuing.
            let permit = tokio::select! {
                _ = &mut shutdown => {
                    info!("shutdown requested; stopping orchestration loop");
                    break;
                }
                permit = Arc::clone(&sem).acquire_owned() => {
                    permit.expect("semaphore unexpectedly closed")
                }
            };

            match self.opms.dequeue().await {
                Ok(dequeued) => {
                    // A successful dequeue (task or empty) resets the backoff.
                    attempt = 1;
                    let Some(task) = dequeued.task else {
                        drop(permit);
                        self.maybe_stop_idle_executor(&mut idle_since).await;
                        // Honor a server-requested poll delay, else the idle interval.
                        let delay = dequeued.retry_after.unwrap_or(self.params.loop_interval);
                        tokio::time::sleep(delay).await;
                        continue;
                    };
                    idle_since = Instant::now();
                    if let Err(e) = self.ensure_ready().await {
                        error!("executor did not become ready: {e:#}");
                        let outcome = dispatch_failure(&format!("executor unavailable: {e}"));
                        if let Err(pe) = self.opms.publish(&task, &outcome).await {
                            error!("failed to publish executor-unavailable failure: {pe:#}");
                        }
                        drop(permit);
                        continue;
                    }

                    self.inflight.fetch_add(1, Ordering::SeqCst);
                    let opms = Arc::clone(&self.opms);
                    let dispatcher = Arc::clone(&self.dispatcher);
                    let lifecycle = Arc::clone(&self.lifecycle);
                    let inflight = Arc::clone(&self.inflight);
                    let heartbeat_interval = self.params.heartbeat_interval;
                    tokio::spawn(async move {
                        // Slice 4: keep the task's OPMS lease alive with heartbeats
                        // for as long as its result stream is open. Ownership lives
                        // entirely here; the executor never heartbeats.
                        let (stop_hb, hb_done) =
                            spawn_heartbeats(Arc::clone(&opms), task.clone(), heartbeat_interval);

                        let outcome = match dispatcher.run_action(task.raw.clone()).await {
                            Ok(o) => o,
                            Err(e) => {
                                warn!("run_action failed for task {}: {e:#}", task.task_id);
                                // Slice 6: a broken stream plus an exited process is a
                                // crash — report a failure, never silently retry. A
                                // fresh executor is started on the next dequeue.
                                match lifecycle.has_exited().await {
                                    Ok(true) => {
                                        error!("executor crashed during task {}", task.task_id);
                                        crash_failure()
                                    }
                                    _ => dispatch_failure(&format!("action dispatch failed: {e}")),
                                }
                            }
                        };

                        let _ = stop_hb.send(());
                        let _ = hb_done.await;

                        if let Err(e) = opms.publish(&task, &outcome).await {
                            error!("failed to publish result for task {}: {e:#}", task.task_id);
                        }
                        inflight.fetch_sub(1, Ordering::SeqCst);
                        drop(permit);
                    });
                }
                Err(e) => {
                    error!("dequeue failed (attempt {attempt}): {e:#}");
                    drop(permit);
                    // Circuit breaker: exponential backoff, and after max_attempts
                    // consecutive failures pause for a longer cool-off, then reset.
                    if attempt >= self.params.max_attempts {
                        warn!(
                            "dequeue circuit breaker tripped after {} attempts; waiting {:?}",
                            attempt, self.params.wait_before_retry
                        );
                        tokio::time::sleep(self.params.wait_before_retry).await;
                        attempt = 1;
                    } else {
                        let delay = backoff_delay(
                            attempt,
                            self.params.min_backoff,
                            self.params.max_backoff,
                        );
                        tokio::time::sleep(delay).await;
                        attempt += 1;
                    }
                }
            }
        }
    }

    /// Ensure the executor is started and reports ready, bounded by `ready_timeout`.
    async fn ensure_ready(&self) -> anyhow::Result<()> {
        self.lifecycle.ensure_started().await?;
        let deadline = Instant::now() + self.params.ready_timeout;
        loop {
            if let Ok(health) = self.dispatcher.health().await
                && health.ready
            {
                return Ok(());
            }
            if Instant::now() >= deadline {
                anyhow::bail!("executor not ready within {:?}", self.params.ready_timeout);
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }

    /// Stop the executor after an idle period with no in-flight work, making the
    /// control plane the single termination authority.
    async fn maybe_stop_idle_executor(&self, idle_since: &mut Instant) {
        if self.inflight.load(Ordering::SeqCst) != 0 {
            *idle_since = Instant::now();
            return;
        }
        if idle_since.elapsed() < self.params.idle_timeout {
            return;
        }
        match self.lifecycle.is_running().await {
            Ok(true) => {
                info!(
                    "executor idle for {:?}; stopping it",
                    self.params.idle_timeout
                );
                if let Err(e) = self.lifecycle.stop().await {
                    warn!("failed to stop idle executor: {e:#}");
                }
                *idle_since = Instant::now();
            }
            Ok(false) => {}
            Err(e) => warn!("failed to check executor liveness: {e:#}"),
        }
    }
}

fn dispatch_failure(detail: &str) -> Outcome {
    Outcome::Failure {
        error_code: INTERNAL_ERROR,
        message: detail.to_string(),
        external_message: "The action could not be executed.".to_string(),
    }
}

/// Failure published when the executor process exited mid-action (a crash). The
/// action is never auto-retried — a mutating action must not run twice.
fn crash_failure() -> Outcome {
    Outcome::Failure {
        error_code: INTERNAL_ERROR,
        message: "executor process exited before the action completed".to_string(),
        external_message: "The action was interrupted because the executor stopped unexpectedly."
            .to_string(),
    }
}

/// Spawn a task that heartbeats `task`'s OPMS lease every `interval` until the
/// returned sender is dropped/fired. The first heartbeat is emitted after one
/// full interval (the immediate `interval` tick is consumed).
fn spawn_heartbeats<O: Opms + 'static>(
    opms: Arc<O>,
    task: Task,
    interval: Duration,
) -> (
    tokio::sync::oneshot::Sender<()>,
    tokio::task::JoinHandle<()>,
) {
    let (stop_tx, mut stop_rx) = tokio::sync::oneshot::channel::<()>();
    let handle = tokio::spawn(async move {
        let mut ticker = tokio::time::interval(interval);
        ticker.tick().await; // consume the immediate first tick
        loop {
            tokio::select! {
                _ = &mut stop_rx => return,
                _ = ticker.tick() => {
                    if let Err(e) = opms.heartbeat(&task).await {
                        warn!("heartbeat failed for task {}: {e:#}", task.task_id);
                    }
                }
            }
        }
    });
    (stop_tx, handle)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::executor::Health;
    use crate::opms::{Dequeued, Task};
    use std::sync::Mutex;

    #[derive(Default)]
    struct FakeState {
        dequeued: usize,
        published: usize,
        failures: usize,
        heartbeats: usize,
        max_concurrent: usize,
        concurrent: usize,
    }

    struct Fakes {
        state: Mutex<FakeState>,
        // Number of tasks to hand out before returning empty.
        tasks_to_serve: usize,
        // Gate so dispatch overlaps to exercise pool bounding. When 0-permit and
        // never released, run_action blocks (used to keep a stream "open").
        release: tokio::sync::Semaphore,
        // run_action returns an error (broken stream).
        fail_run: bool,
        // Describe reports the process has exited (crash).
        exited: bool,
    }

    impl Default for Fakes {
        fn default() -> Self {
            Fakes {
                state: Mutex::new(FakeState::default()),
                tasks_to_serve: 0,
                release: tokio::sync::Semaphore::new(0),
                fail_run: false,
                exited: false,
            }
        }
    }

    impl Opms for Fakes {
        async fn dequeue(&self) -> anyhow::Result<Dequeued> {
            let mut s = self.state.lock().unwrap();
            if s.dequeued >= self.tasks_to_serve {
                return Ok(Dequeued::default());
            }
            s.dequeued += 1;
            let id = s.dequeued;
            Ok(Dequeued {
                task: Some(Task {
                    raw: format!("{{\"data\":{{\"id\":\"t{id}\"}}}}").into_bytes(),
                    task_id: format!("t{id}"),
                    job_id: "j".into(),
                    action_fqn: "b.a".into(),
                    client: 1,
                }),
                retry_after: None,
            })
        }

        async fn publish(&self, _task: &Task, outcome: &Outcome) -> anyhow::Result<()> {
            let mut s = self.state.lock().unwrap();
            s.published += 1;
            if matches!(outcome, Outcome::Failure { .. }) {
                s.failures += 1;
            }
            Ok(())
        }

        async fn heartbeat(&self, _task: &Task) -> anyhow::Result<()> {
            self.state.lock().unwrap().heartbeats += 1;
            Ok(())
        }
    }

    impl ExecutorLifecycle for Fakes {
        async fn ensure_started(&self) -> anyhow::Result<()> {
            Ok(())
        }
        async fn is_running(&self) -> anyhow::Result<bool> {
            Ok(true)
        }
        async fn has_exited(&self) -> anyhow::Result<bool> {
            Ok(self.exited)
        }
        async fn stop(&self) -> anyhow::Result<()> {
            Ok(())
        }
    }

    impl Dispatcher for Fakes {
        async fn health(&self) -> anyhow::Result<Health> {
            Ok(Health {
                ready: true,
                active_actions: 0,
            })
        }

        async fn run_action(&self, _raw: Vec<u8>) -> anyhow::Result<Outcome> {
            if self.fail_run {
                anyhow::bail!("simulated broken stream");
            }
            {
                let mut s = self.state.lock().unwrap();
                s.concurrent += 1;
                s.max_concurrent = s.max_concurrent.max(s.concurrent);
            }
            // Block until the test releases, so multiple actions overlap.
            let _ = self.release.acquire().await.unwrap();
            self.state.lock().unwrap().concurrent -= 1;
            Ok(Outcome::Success {
                output_json: b"{}".to_vec(),
            })
        }
    }

    fn test_params(pool_size: usize, heartbeat_interval: Duration) -> Params {
        Params {
            pool_size,
            loop_interval: Duration::from_millis(5),
            ready_timeout: Duration::from_secs(1),
            idle_timeout: Duration::from_secs(3600),
            heartbeat_interval,
            min_backoff: Duration::from_millis(1),
            max_backoff: Duration::from_millis(10),
            wait_before_retry: Duration::from_millis(20),
            max_attempts: 5,
        }
    }

    #[test]
    fn backoff_is_exponential_and_capped() {
        let min = Duration::from_secs(1);
        let max = Duration::from_secs(30);
        assert_eq!(backoff_delay(1, min, max), Duration::from_secs(1));
        assert_eq!(backoff_delay(2, min, max), Duration::from_secs(2));
        assert_eq!(backoff_delay(4, min, max), Duration::from_secs(8));
        assert_eq!(backoff_delay(6, min, max), Duration::from_secs(30)); // capped
        assert_eq!(backoff_delay(100, min, max), Duration::from_secs(30)); // no overflow
    }

    #[tokio::test]
    async fn never_exceeds_pool_size_and_publishes_every_task() {
        let pool = 2;
        let tasks = 5;
        let fakes = Arc::new(Fakes {
            tasks_to_serve: tasks,
            release: tokio::sync::Semaphore::new(0),
            ..Default::default()
        });

        let orch = Orchestrator::new(
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            test_params(pool, Duration::from_secs(3600)),
        );

        let (tx, rx) = tokio::sync::oneshot::channel::<()>();
        let handle = tokio::spawn(async move {
            orch.run(async {
                let _ = rx.await;
            })
            .await;
        });

        // Let the loop fill the pool, then let actions drain in waves.
        tokio::time::sleep(Duration::from_millis(50)).await;
        fakes.release.add_permits(tasks);
        tokio::time::sleep(Duration::from_millis(100)).await;

        let _ = tx.send(());
        let _ = handle.await;

        let s = fakes.state.lock().unwrap();
        assert!(
            s.max_concurrent <= pool,
            "max concurrent {} exceeded pool {}",
            s.max_concurrent,
            pool
        );
        assert_eq!(
            s.published, tasks,
            "every dequeued task should be published"
        );
    }

    #[tokio::test]
    async fn heartbeats_while_stream_open_and_stop_after() {
        // One task whose stream stays open (never released) so heartbeats fire.
        let fakes = Arc::new(Fakes {
            tasks_to_serve: 1,
            release: tokio::sync::Semaphore::new(0),
            ..Default::default()
        });

        let orch = Orchestrator::new(
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            test_params(1, Duration::from_millis(10)),
        );

        let (tx, rx) = tokio::sync::oneshot::channel::<()>();
        let handle = tokio::spawn(async move {
            orch.run(async {
                let _ = rx.await;
            })
            .await;
        });

        // Stream is open this whole window → several heartbeats should fire.
        tokio::time::sleep(Duration::from_millis(80)).await;
        let during = fakes.state.lock().unwrap().heartbeats;
        assert!(during >= 1, "expected heartbeats while the stream was open");

        // Close the stream; heartbeats must stop promptly.
        fakes.release.add_permits(1);
        tokio::time::sleep(Duration::from_millis(20)).await;
        let after_close = fakes.state.lock().unwrap().heartbeats;
        tokio::time::sleep(Duration::from_millis(40)).await;
        let later = fakes.state.lock().unwrap().heartbeats;
        assert_eq!(
            after_close, later,
            "heartbeats must stop once the stream closes"
        );

        let _ = tx.send(());
        let _ = handle.await;
    }

    #[tokio::test]
    async fn crash_publishes_failure_and_does_not_retry() {
        // run_action errors and the process reports exited → crash fail-report.
        let fakes = Arc::new(Fakes {
            tasks_to_serve: 1,
            fail_run: true,
            exited: true,
            ..Default::default()
        });

        let orch = Orchestrator::new(
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            test_params(1, Duration::from_secs(3600)),
        );

        let (tx, rx) = tokio::sync::oneshot::channel::<()>();
        let handle = tokio::spawn(async move {
            orch.run(async {
                let _ = rx.await;
            })
            .await;
        });

        tokio::time::sleep(Duration::from_millis(60)).await;
        let _ = tx.send(());
        let _ = handle.await;

        let s = fakes.state.lock().unwrap();
        assert_eq!(
            s.dequeued, 1,
            "the crashing task is dequeued once, not retried"
        );
        assert_eq!(s.failures, 1, "a crash publishes exactly one failure");
    }
}
