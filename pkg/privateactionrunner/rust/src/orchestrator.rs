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
use crate::executor::{Dispatcher, SigningKey};
use crate::opms::{Opms, Outcome, Task};
use crate::procmgr::ExecutorLifecycle;
use log::{debug, error, info, warn};
use std::future::Future;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::{Duration, Instant};
use tokio::sync::{RwLock, Semaphore};

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
    pub key_sync_timeout: Duration,
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
            pool_size: config.task_concurrency,
            loop_interval: config.loop_interval,
            ready_timeout: config.ready_timeout,
            key_sync_timeout: config.key_sync_timeout,
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
    key_cache: RwLock<Option<Vec<SigningKey>>>,
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
            key_cache: RwLock::new(None),
        }
    }

    /// Run the loop until `shutdown` resolves, then return (in-flight actions are
    /// left to finish; graceful drain is slice 5).
    pub async fn run<S: Future<Output = ()>>(&self, shutdown: S) {
        let sem = Arc::new(Semaphore::new(self.params.pool_size));
        // Consecutive dequeue-failure count, driving exponential backoff.
        let mut attempt: u32 = 1;
        tokio::pin!(shutdown);

        // Sleep, but wake immediately on shutdown. Without this a stop is delayed
        // by however long the current backoff has to run — up to `wait_before_retry`
        // (5 minutes by default), which is far past the process manager's stop
        // timeout, so the control plane would be SIGKILLed instead of stopping
        // cleanly. Every wait in this loop must go through here.
        macro_rules! sleep_or_shutdown {
            ($duration:expr) => {
                tokio::select! {
                    _ = &mut shutdown => {
                        info!("shutdown requested while waiting; stopping orchestration loop");
                        break;
                    }
                    _ = tokio::time::sleep($duration) => {}
                }
            };
        }

        // Populate the long-lived key cache before dequeuing anything. A cold
        // executor may need roughly a minute for its first verified RC update;
        // paying that cost here means no OPMS lease is held while it waits.
        info!("pre-warming executor signing-key cache");
        loop {
            let prewarm = tokio::select! {
                _ = &mut shutdown => {
                    info!("shutdown requested during executor pre-warm");
                    return;
                }
                result = self.ensure_ready() => result,
            };
            match prewarm {
                Ok(()) => {
                    let key_count = self
                        .key_cache
                        .read()
                        .await
                        .as_ref()
                        .map_or(0, Vec::len);
                    info!("executor signing-key cache ready ({key_count} keys)");
                    break;
                }
                Err(e) => warn!("executor pre-warm failed; retrying: {e:#}"),
            }
            tokio::select! {
                _ = &mut shutdown => {
                    info!("shutdown requested during executor pre-warm backoff");
                    return;
                }
                _ = tokio::time::sleep(self.params.min_backoff) => {}
            }
        }
        let mut idle_since = Instant::now();

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
                        debug!("no task available; next poll in {delay:?}");
                        sleep_or_shutdown!(delay);
                        continue;
                    };
                    idle_since = Instant::now();
                    info!(
                        "dequeued task {} ({}) for job {}",
                        task.task_id, task.action_fqn, task.job_id
                    );
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
                            Ok(o) => {
                                match &o {
                                    Outcome::Success { .. } => {
                                        info!("task {} succeeded", task.task_id)
                                    }
                                    Outcome::Failure {
                                        error_code,
                                        message,
                                        ..
                                    } => info!(
                                        "task {} failed with code {error_code}: {message}",
                                        task.task_id
                                    ),
                                }
                                o
                            }
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
                        sleep_or_shutdown!(self.params.wait_before_retry);
                        attempt = 1;
                    } else {
                        let delay = backoff_delay(
                            attempt,
                            self.params.min_backoff,
                            self.params.max_backoff,
                        );
                        sleep_or_shutdown!(delay);
                        attempt += 1;
                    }
                }
            }
        }
    }

    /// Ensure the executor is started and reports ready, bounded by `ready_timeout`.
    async fn ensure_ready(&self) -> anyhow::Result<()> {
        self.lifecycle.ensure_started().await?;
        self.wait_for_health(false).await?;

        let seed = self
            .key_cache
            .read()
            .await
            .as_ref()
            .cloned()
            .unwrap_or_default();
        let snapshot = tokio::time::timeout(
            self.params.key_sync_timeout,
            self.dispatcher.sync_keys(seed),
        )
        .await
        .map_err(|_| {
            anyhow::anyhow!(
                "executor key synchronization timed out after {:?}",
                self.params.key_sync_timeout
            )
        })??;
        *self.key_cache.write().await = Some(snapshot);

        self.wait_for_health(true).await
    }

    async fn wait_for_health(&self, require_ready: bool) -> anyhow::Result<()> {
        let deadline = Instant::now() + self.params.ready_timeout;
        loop {
            match self.dispatcher.health().await {
                Ok(health) if !require_ready || health.ready => return Ok(()),
                Ok(health) => debug!(
                    "executor is up but not ready yet ({} active actions)",
                    health.active_actions
                ),
                // Expected while the executor is still binding its socket, so
                // this is only interesting at debug level until the deadline
                // turns it into a hard error below.
                Err(e) => debug!("executor health check failed: {e:#}"),
            }
            if Instant::now() >= deadline {
                let state = if require_ready { "ready" } else { "live" };
                anyhow::bail!(
                    "executor not {state} within {:?}",
                    self.params.ready_timeout
                );
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
        sync_calls: usize,
        sync_calls_with_seed: usize,
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
        // dequeue always errors, driving the loop into backoff.
        fail_dequeue: bool,
        // Describe reports the process has exited (crash).
        exited: bool,
        // Hold SyncKeys so tests can prove dequeue waits for pre-warm.
        block_sync: bool,
        sync_release: tokio::sync::Semaphore,
    }

    impl Default for Fakes {
        fn default() -> Self {
            Fakes {
                state: Mutex::new(FakeState::default()),
                tasks_to_serve: 0,
                release: tokio::sync::Semaphore::new(0),
                fail_run: false,
                fail_dequeue: false,
                exited: false,
                block_sync: false,
                sync_release: tokio::sync::Semaphore::new(0),
            }
        }
    }

    impl Opms for Fakes {
        async fn dequeue(&self) -> anyhow::Result<Dequeued> {
            let mut s = self.state.lock().unwrap();
            if self.fail_dequeue {
                s.dequeued += 1;
                anyhow::bail!("simulated OPMS outage");
            }
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

        async fn sync_keys(
            &self,
            keys: Vec<SigningKey>,
        ) -> anyhow::Result<Vec<SigningKey>> {
            if self.block_sync {
                let _ = self.sync_release.acquire().await.unwrap();
            }
            let mut state = self.state.lock().unwrap();
            state.sync_calls += 1;
            if !keys.is_empty() {
                state.sync_calls_with_seed += 1;
                return Ok(keys);
            }
            Ok(vec![SigningKey {
                id: "key-1".into(),
                key_type: "ED25519".into(),
                key: b"pem".to_vec(),
            }])
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
            key_sync_timeout: Duration::from_secs(1),
            idle_timeout: Duration::from_secs(3600),
            heartbeat_interval,
            min_backoff: Duration::from_millis(1),
            max_backoff: Duration::from_millis(10),
            wait_before_retry: Duration::from_millis(20),
            max_attempts: 5,
        }
    }

    #[tokio::test]
    async fn prewarm_populates_key_cache_before_dequeue() {
        let fake = Arc::new(Fakes {
            tasks_to_serve: 1,
            block_sync: true,
            ..Default::default()
        });
        let orch = Orchestrator::new(
            Arc::clone(&fake),
            Arc::clone(&fake),
            Arc::clone(&fake),
            test_params(1, Duration::from_millis(10)),
        );
        let (shutdown_tx, shutdown_rx) = tokio::sync::oneshot::channel::<()>();
        let run = tokio::spawn(async move {
            orch.run(async {
                let _ = shutdown_rx.await;
            })
            .await;
        });

        tokio::time::sleep(Duration::from_millis(30)).await;
        assert_eq!(fake.state.lock().unwrap().dequeued, 0);

        // One permit completes startup pre-warm; the second lets the first task
        // refresh the cache before dispatch.
        fake.sync_release.add_permits(2);
        fake.release.add_permits(1);
        tokio::time::timeout(Duration::from_secs(1), async {
            loop {
                if fake.state.lock().unwrap().published == 1 {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(5)).await;
            }
        })
        .await
        .unwrap();

        let state = fake.state.lock().unwrap();
        assert_eq!(state.sync_calls, 2);
        assert_eq!(state.sync_calls_with_seed, 1);
        drop(state);
        let _ = shutdown_tx.send(());
        run.await.unwrap();
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

    /// A stop must not wait out the current backoff. OPMS being unreachable puts
    /// the loop into a `wait_before_retry` sleep (5 minutes in production), and a
    /// sleep that ignores the shutdown signal means the process manager's stop
    /// timeout expires and SIGKILLs the control plane — potentially mid-action.
    /// Real (unpaused) time on purpose: with paused time a sleep that ignores
    /// shutdown would still complete instantly and the bug would pass unnoticed.
    #[tokio::test]
    async fn shutdown_is_not_delayed_by_dequeue_backoff() {
        let fakes = Arc::new(Fakes {
            fail_dequeue: true,
            ..Default::default()
        });

        // One failure is enough to trip the circuit breaker into the long wait.
        let mut params = test_params(1, Duration::from_secs(3600));
        params.max_attempts = 1;
        params.min_backoff = Duration::from_secs(60);
        params.max_backoff = Duration::from_secs(60);
        params.wait_before_retry = Duration::from_secs(60);

        let orch = Orchestrator::new(
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            params,
        );

        let (tx, rx) = tokio::sync::oneshot::channel::<()>();
        let handle = tokio::spawn(async move {
            orch.run(async {
                let _ = rx.await;
            })
            .await;
        });

        // Wait until the loop has failed a dequeue and is therefore sleeping.
        for _ in 0..100 {
            if fakes.state.lock().unwrap().dequeued > 0 {
                break;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
        assert!(
            fakes.state.lock().unwrap().dequeued > 0,
            "the loop never attempted a dequeue"
        );

        let _ = tx.send(());
        tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("run() must return promptly on shutdown, not after the backoff")
            .expect("orchestrator task panicked");
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
