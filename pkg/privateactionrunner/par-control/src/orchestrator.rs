// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Dequeues tasks up to the configured concurrency, starts the executor on
//! demand, dispatches over gRPC, and publishes outcomes to OPMS.

use crate::executor::{Dispatcher, Outcome};
use crate::opms::{HealthCheck, HeartbeatResult, Opms, PublishResult, Task};
use crate::procmgr::ExecutorLifecycle;
use log::{debug, error, info, warn};
use std::future::Future;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::{Duration, Instant};
use tokio::sync::Semaphore;

/// `INTERNAL_ERROR` from the ActionPlatformErrorCode proto; used when dispatch
/// itself fails (e.g. the stream breaks) so the workflow does not hang.
const INTERNAL_ERROR: i32 = 1;

/// Tuning knobs for the orchestrator.
#[derive(Clone)]
pub struct Params {
    pub pool_size: usize,
    pub loop_interval: Duration,
    pub ready_timeout: Duration,
    /// From task dequeue through terminal publication.
    pub heartbeat_interval: Duration,
    /// Runner liveness reporting to OPMS, independent of task flow.
    pub health_check_interval: Duration,
    /// Mirrors the Go circuit breaker.
    pub min_backoff: Duration,
    pub max_backoff: Duration,
    pub wait_before_retry: Duration,
    pub max_attempts: u32,
    pub publish_max_attempts: u32,
    pub publish_min_backoff: Duration,
    pub publish_max_backoff: Duration,
    /// Must remain shorter than par-control's process-manager stop timeout.
    pub drain_timeout: Duration,
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

    /// Run the loop until `shutdown` resolves, then stop dequeuing and wait for
    /// every in-flight action to publish its result.
    pub async fn run<S: Future<Output = ()>>(&self, shutdown: S) {
        let sem = Arc::new(Semaphore::new(self.params.pool_size));
        // Consecutive dequeue-failure count, driving exponential backoff.
        let mut attempt: u32 = 1;
        tokio::pin!(shutdown);

        // Liveness reporting runs for the whole process lifetime, deliberately
        // decoupled from key readiness and task flow: an idle or wedged runner
        // still has to tell OPMS it is alive, exactly like the Go CommonRunner's
        // health-check loop, which starts before the workflow runner is ready.
        let (stop_health, health_done) =
            spawn_health_checks(Arc::clone(&self.opms), self.params.health_check_interval);

        // All loop sleeps must remain interruptible by shutdown.
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

        // The executor remains stopped until work arrives. The first cold task
        // populates the signing-key cache, which seeds later cold starts.
        loop {
            // Acquire capacity before leasing work from OPMS.
            let permit = tokio::select! {
                _ = &mut shutdown => {
                    info!("shutdown requested; stopping orchestration loop");
                    break;
                }
                permit = Arc::clone(&sem).acquire_owned() => {
                    permit.expect("semaphore unexpectedly closed")
                }
            };

            let dequeued = tokio::select! {
                _ = &mut shutdown => {
                    info!("shutdown requested during dequeue; stopping orchestration loop");
                    drop(permit);
                    break;
                }
                result = self.opms.dequeue() => result,
            };

            match dequeued {
                Ok(dequeued) => {
                    // A successful dequeue (task or empty) resets the backoff.
                    attempt = 1;
                    let Some(task) = dequeued.task else {
                        drop(permit);
                        // Honor a server-requested poll delay, else the idle interval.
                        let delay = dequeued.retry_after.unwrap_or(self.params.loop_interval);
                        debug!("no task available; next poll in {delay:?}");
                        sleep_or_shutdown!(delay);
                        continue;
                    };
                    info!(
                        "dequeued task {} ({}) for job {}",
                        task.task_id, task.action_fqn, task.job_id
                    );
                    // Start protecting the OPMS lease before cold-starting the
                    // executor or synchronizing keys. Keep it protected until
                    // terminal publication finishes.
                    let (stop_hb, hb_done) = spawn_heartbeats(
                        Arc::clone(&self.opms),
                        task.clone(),
                        self.params.heartbeat_interval,
                    );
                    let ready = tokio::select! {
                        _ = &mut shutdown => {
                            info!("shutdown requested before task {} could be dispatched", task.task_id);
                            let outcome = dispatch_failure("runner stopped before dispatch");
                            if let Err(e) = publish_with_retry(
                                Arc::clone(&self.opms),
                                &task,
                                &outcome,
                                self.params.publish_max_attempts,
                                self.params.publish_min_backoff,
                                self.params.publish_max_backoff,
                            ).await {
                                error!("failed to publish shutdown failure for task {}: {e:#}", task.task_id);
                            }
                            stop_heartbeats(stop_hb, hb_done).await;
                            drop(permit);
                            break;
                        }
                        result = self.ensure_ready() => result,
                    };
                    if let Err(e) = ready {
                        error!("executor did not become ready: {e:#}");
                        let outcome = dispatch_failure(&format!("executor unavailable: {e}"));
                        if let Err(pe) = publish_with_retry(
                            Arc::clone(&self.opms),
                            &task,
                            &outcome,
                            self.params.publish_max_attempts,
                            self.params.publish_min_backoff,
                            self.params.publish_max_backoff,
                        )
                        .await
                        {
                            error!("failed to publish executor-unavailable failure: {pe:#}");
                        }
                        stop_heartbeats(stop_hb, hb_done).await;
                        drop(permit);
                        continue;
                    }

                    self.inflight.fetch_add(1, Ordering::SeqCst);
                    let opms = Arc::clone(&self.opms);
                    let dispatcher = Arc::clone(&self.dispatcher);
                    let lifecycle = Arc::clone(&self.lifecycle);
                    let inflight = Arc::clone(&self.inflight);
                    let publish_max_attempts = self.params.publish_max_attempts;
                    let publish_min_backoff = self.params.publish_min_backoff;
                    let publish_max_backoff = self.params.publish_max_backoff;
                    tokio::spawn(async move {
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
                                // Do not retry a task after a possible executor crash: the
                                // action may already have changed external state.
                                match lifecycle.has_exited().await {
                                    Ok(true) => {
                                        error!("executor crashed during task {}", task.task_id);
                                        crash_failure()
                                    }
                                    _ => dispatch_failure(&format!("action dispatch failed: {e}")),
                                }
                            }
                        };

                        if let Err(e) = publish_with_retry(
                            Arc::clone(&opms),
                            &task,
                            &outcome,
                            publish_max_attempts,
                            publish_min_backoff,
                            publish_max_backoff,
                        )
                        .await
                        {
                            error!("failed to publish result for task {}: {e:#}", task.task_id);
                        }

                        stop_heartbeats(stop_hb, hb_done).await;
                        inflight.fetch_sub(1, Ordering::SeqCst);
                        drop(permit);
                    });
                }
                Err(e) => {
                    error!("dequeue failed (attempt {attempt}): {e:#}");
                    drop(permit);
                    // Prefer server pacing to the local backoff or circuit-breaker delay.
                    let fallback_delay = if attempt >= self.params.max_attempts {
                        warn!("dequeue circuit breaker tripped after {attempt} attempts");
                        attempt = 1;
                        self.params.wait_before_retry
                    } else {
                        let delay = backoff_delay(
                            attempt,
                            self.params.min_backoff,
                            self.params.max_backoff,
                        );
                        attempt += 1;
                        delay
                    };
                    let delay = e.retry_after.unwrap_or(fallback_delay);
                    debug!("dequeue failed; next poll in {delay:?}");
                    sleep_or_shutdown!(delay);
                }
            }
        }

        let inflight = self.inflight.load(Ordering::SeqCst);
        if inflight > 0 {
            info!("waiting for {inflight} in-flight action(s) to finish");
        }
        let drain_permits: u32 = self
            .params
            .pool_size
            .try_into()
            .expect("executor pool size exceeds semaphore capacity");
        match tokio::time::timeout(self.params.drain_timeout, sem.acquire_many(drain_permits)).await
        {
            Ok(Ok(_drained)) if inflight > 0 => info!("all in-flight actions finished"),
            Ok(Ok(_drained)) => {}
            Ok(Err(_)) => error!("semaphore unexpectedly closed while draining"),
            Err(_) => error!(
                "graceful drain exceeded {:?}; forcing control-plane shutdown before procmgr's stop deadline",
                self.params.drain_timeout
            ),
        }

        stop_health_checks(stop_health, health_done).await;
    }

    /// Ensure the executor is started and reports ready, bounded by `ready_timeout`.
    async fn ensure_ready(&self) -> anyhow::Result<()> {
        self.lifecycle.ensure_started().await?;
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
}

fn dispatch_failure(detail: &str) -> Outcome {
    Outcome::Failure {
        error_code: INTERNAL_ERROR,
        message: detail.to_string(),
        external_message: "The action could not be executed.".to_string(),
    }
}

/// Failure published when the executor exits mid-action.
fn crash_failure() -> Outcome {
    Outcome::Failure {
        error_code: INTERNAL_ERROR,
        message: "executor process exited before the action completed".to_string(),
        external_message: "The action was interrupted because the executor stopped unexpectedly."
            .to_string(),
    }
}

/// Publish a terminal result, retrying transport and retryable HTTP failures.
/// A client rejection is terminal: retrying the same invalid request cannot help.
async fn publish_with_retry<O: Opms + 'static>(
    opms: Arc<O>,
    task: &Task,
    outcome: &Outcome,
    max_attempts: u32,
    min_backoff: Duration,
    max_backoff: Duration,
) -> anyhow::Result<PublishResult> {
    let max_attempts = max_attempts.max(1);
    let mut attempt = 1;
    loop {
        match opms.publish(task, outcome).await {
            Ok(PublishResult::Published) => return Ok(PublishResult::Published),
            Ok(PublishResult::Rejected { status, detail }) => {
                error!(
                    "OPMS rejected terminal result for task {} with status {status}: {detail}",
                    task.task_id
                );
                return Ok(PublishResult::Rejected { status, detail });
            }
            Err(error) if attempt < max_attempts => {
                let delay = backoff_delay(attempt, min_backoff, max_backoff);
                warn!(
                    "publishing task {} failed (attempt {attempt}/{max_attempts}); retrying in {delay:?}: {error:#}",
                    task.task_id
                );
                tokio::time::sleep(delay).await;
                attempt += 1;
            }
            Err(error) => return Err(error),
        }
    }
}

/// How often a *successful* health check is logged at info level. Mirrors the Go
/// loop's `ddlog.NewLogLimit(1, 10*time.Minute)`: liveness reporting every 30
/// seconds would otherwise be 2,880 identical info lines per day on an idle host.
const HEALTH_CHECK_LOG_INTERVAL: Duration = Duration::from_secs(600);

/// Spawn the OPMS runner health-check loop. It reports liveness every `interval`
/// until the returned sender is fired/dropped, honoring a server-requested
/// pacing hint (`X-Retry-After-Ms`) the same way the Go loop does — including on
/// a rejected check, so a throttling OPMS is not answered with more traffic.
///
/// A failed check is logged and retried on the next tick: this loop must never
/// abort, because giving up would make the runner look permanently dead to OPMS
/// while it is in fact still dequeuing and executing actions.
fn spawn_health_checks<O: Opms + 'static>(
    opms: Arc<O>,
    interval: Duration,
) -> (
    tokio::sync::oneshot::Sender<()>,
    tokio::task::JoinHandle<()>,
) {
    let (stop_tx, mut stop_rx) = tokio::sync::oneshot::channel::<()>();
    let handle = tokio::spawn(async move {
        // The first check is sent after one full interval, matching Go's
        // `time.NewTimer(defaultInterval)`.
        let mut delay = interval;
        let mut last_info: Option<Instant> = None;
        loop {
            tokio::select! {
                _ = &mut stop_rx => return,
                _ = tokio::time::sleep(delay) => {}
            }
            match opms.health_check().await {
                Ok(check) => {
                    delay = check.retry_after.unwrap_or(interval);
                    log_health_check(&check, &mut last_info);
                }
                Err(e) => {
                    error!("OPMS health check failed: {e:#}");
                    delay = interval;
                }
            }
        }
    });
    (stop_tx, handle)
}

/// Stop the health-check loop and wait for it, so a shutting-down control plane
/// does not leave a request in flight past the drain deadline.
async fn stop_health_checks(
    stop: tokio::sync::oneshot::Sender<()>,
    done: tokio::task::JoinHandle<()>,
) {
    let _ = stop.send(());
    let _ = done.await;
}

fn log_health_check(check: &HealthCheck, last_info: &mut Option<Instant>) {
    let server_time = check.server_time.as_deref().unwrap_or("unknown");
    if !check.ok() {
        error!(
            "OPMS health check failed with status {}: {}",
            check.status, check.detail
        );
        return;
    }
    if last_info.is_none_or(|at| at.elapsed() >= HEALTH_CHECK_LOG_INTERVAL) {
        info!("OPMS health check succeeded (server time {server_time})");
        *last_info = Some(Instant::now());
    } else {
        debug!("OPMS health check succeeded (server time {server_time})");
    }
}

/// Spawn a task that heartbeats `task`'s OPMS lease every `interval` until the
/// returned sender is dropped/fired or OPMS reports the task missing. The first
/// heartbeat is emitted after one full interval (the immediate `interval` tick
/// is consumed).
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
                    match opms.heartbeat(&task).await {
                        Ok(HeartbeatResult::Alive) => {}
                        Ok(HeartbeatResult::NotFound) => {
                            info!("task {} is no longer known to OPMS; stopping heartbeats", task.task_id);
                            return;
                        }
                        Err(e) => warn!("heartbeat failed for task {}: {e:#}", task.task_id),
                    }
                }
            }
        }
    });
    (stop_tx, handle)
}

async fn stop_heartbeats(
    stop: tokio::sync::oneshot::Sender<()>,
    done: tokio::task::JoinHandle<()>,
) {
    let _ = stop.send(());
    let _ = done.await;
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
        publish_attempts: usize,
        publish_failures_remaining: usize,
        published: usize,
        failures: usize,
        heartbeats: usize,
        health_checks: usize,
        max_concurrent: usize,
        concurrent: usize,
        executor_health_checks: usize,
        not_ready_checks_remaining: usize,
        ensure_started_calls: usize,
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
        dequeue_retry_after: Option<Duration>,
        // Hold dequeue so tests can prove shutdown cancels a long poll.
        block_dequeue: bool,
        dequeue_release: tokio::sync::Semaphore,
        // Describe reports the process has exited (crash).
        exited: bool,
        // Hold terminal publication so tests can verify heartbeats continue.
        block_publish: bool,
        publish_release: tokio::sync::Semaphore,
        // Simulate OPMS forgetting a task on its first heartbeat.
        heartbeat_not_found: bool,
        // Health checks answer with this status (200 unless a test overrides it).
        health_check_status: u16,
        // Health checks fail at the transport level.
        fail_health_check: bool,
    }

    impl Default for Fakes {
        fn default() -> Self {
            Fakes {
                state: Mutex::new(FakeState::default()),
                tasks_to_serve: 0,
                release: tokio::sync::Semaphore::new(0),
                fail_run: false,
                fail_dequeue: false,
                dequeue_retry_after: None,
                block_dequeue: false,
                dequeue_release: tokio::sync::Semaphore::new(0),
                exited: false,
                block_publish: false,
                publish_release: tokio::sync::Semaphore::new(0),
                heartbeat_not_found: false,
                health_check_status: 200,
                fail_health_check: false,
            }
        }
    }

    impl Opms for Fakes {
        async fn dequeue(&self) -> std::result::Result<Dequeued, crate::opms::DequeueError> {
            if self.block_dequeue {
                self.state.lock().unwrap().dequeued += 1;
                let _ = self.dequeue_release.acquire().await.unwrap();
            }
            let mut s = self.state.lock().unwrap();
            if self.fail_dequeue {
                s.dequeued += 1;
                return Err(crate::opms::DequeueError::new(
                    anyhow::anyhow!("simulated OPMS outage"),
                    self.dequeue_retry_after,
                ));
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

        async fn publish(&self, _task: &Task, outcome: &Outcome) -> anyhow::Result<PublishResult> {
            {
                let mut s = self.state.lock().unwrap();
                s.publish_attempts += 1;
                if s.publish_failures_remaining > 0 {
                    s.publish_failures_remaining -= 1;
                    anyhow::bail!("simulated publish outage");
                }
            }
            if self.block_publish {
                self.publish_release.acquire().await.unwrap().forget();
            }
            let mut s = self.state.lock().unwrap();
            s.published += 1;
            if matches!(outcome, Outcome::Failure { .. }) {
                s.failures += 1;
            }
            Ok(PublishResult::Published)
        }

        async fn heartbeat(&self, _task: &Task) -> anyhow::Result<HeartbeatResult> {
            self.state.lock().unwrap().heartbeats += 1;
            if self.heartbeat_not_found {
                return Ok(HeartbeatResult::NotFound);
            }
            Ok(HeartbeatResult::Alive)
        }

        async fn health_check(&self) -> anyhow::Result<HealthCheck> {
            self.state.lock().unwrap().health_checks += 1;
            if self.fail_health_check {
                anyhow::bail!("simulated health-check outage");
            }
            Ok(HealthCheck {
                status: self.health_check_status,
                server_time: Some("2026-02-03T04:05:06Z".into()),
                retry_after: None,
                detail: String::new(),
            })
        }
    }

    impl ExecutorLifecycle for Fakes {
        async fn ensure_started(&self) -> anyhow::Result<()> {
            self.state.lock().unwrap().ensure_started_calls += 1;
            Ok(())
        }
        async fn has_exited(&self) -> anyhow::Result<bool> {
            Ok(self.exited)
        }
    }

    impl Dispatcher for Fakes {
        async fn health(&self) -> anyhow::Result<Health> {
            let mut state = self.state.lock().unwrap();
            state.executor_health_checks += 1;
            let ready = state.not_ready_checks_remaining == 0;
            state.not_ready_checks_remaining = state.not_ready_checks_remaining.saturating_sub(1);
            Ok(Health {
                ready,
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
            self.release.acquire().await.unwrap().forget();
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
            heartbeat_interval,
            // Long enough that only tests that opt in observe a health check.
            health_check_interval: Duration::from_secs(3600),
            min_backoff: Duration::from_millis(1),
            max_backoff: Duration::from_millis(10),
            wait_before_retry: Duration::from_millis(20),
            max_attempts: 5,
            publish_max_attempts: 3,
            publish_min_backoff: Duration::from_millis(1),
            publish_max_backoff: Duration::from_millis(5),
            drain_timeout: Duration::from_secs(1),
        }
    }

    #[tokio::test]
    async fn idle_control_plane_does_not_start_executor() {
        let fake = Arc::new(Fakes::default());
        let orch = Orchestrator::new(
            Arc::clone(&fake),
            Arc::clone(&fake),
            Arc::clone(&fake),
            test_params(1, Duration::from_secs(3600)),
        );
        let (shutdown_tx, shutdown_rx) = tokio::sync::oneshot::channel::<()>();
        let run = tokio::spawn(async move {
            orch.run(async {
                let _ = shutdown_rx.await;
            })
            .await;
        });

        tokio::time::sleep(Duration::from_millis(30)).await;
        {
            let state = fake.state.lock().unwrap();
            assert_eq!(state.ensure_started_calls, 0);
        }

        let _ = shutdown_tx.send(());
        run.await.unwrap();
    }

    #[tokio::test]
    async fn dispatch_waits_for_executor_readiness() {
        let fake = Arc::new(Fakes {
            state: Mutex::new(FakeState {
                not_ready_checks_remaining: 3,
                ..Default::default()
            }),
            tasks_to_serve: 1,
            ..Default::default()
        });
        fake.release.add_permits(1);
        let orch = Orchestrator::new(
            Arc::clone(&fake),
            Arc::clone(&fake),
            Arc::clone(&fake),
            test_params(1, Duration::from_secs(3600)),
        );
        let (shutdown_tx, shutdown_rx) = tokio::sync::oneshot::channel::<()>();
        let run = tokio::spawn(async move {
            orch.run(async {
                let _ = shutdown_rx.await;
            })
            .await;
        });

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
        assert!(fake.state.lock().unwrap().executor_health_checks >= 4);

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
    async fn terminal_publication_retries_transient_failures() {
        let fakes = Arc::new(Fakes {
            state: Mutex::new(FakeState {
                publish_failures_remaining: 2,
                ..Default::default()
            }),
            ..Default::default()
        });
        let task = Task {
            raw: Vec::new(),
            task_id: "task-1".into(),
            job_id: "job-1".into(),
            action_fqn: "bundle.action".into(),
            client: 1,
        };
        let outcome = Outcome::Success {
            output_json: b"{}".to_vec(),
        };

        let result = publish_with_retry(
            Arc::clone(&fakes),
            &task,
            &outcome,
            3,
            Duration::from_millis(1),
            Duration::from_millis(2),
        )
        .await
        .unwrap();

        assert_eq!(result, PublishResult::Published);
        let state = fakes.state.lock().unwrap();
        assert_eq!(state.publish_attempts, 3);
        assert_eq!(state.published, 1);
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
    async fn heartbeats_from_dequeue_through_terminal_publication() {
        let fakes = Arc::new(Fakes {
            state: Mutex::new(FakeState {
                not_ready_checks_remaining: usize::MAX,
                ..Default::default()
            }),
            tasks_to_serve: 1,
            block_publish: true,
            ..Default::default()
        });

        let orch = Orchestrator::new(
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            test_params(1, Duration::from_millis(5)),
        );

        let (tx, rx) = tokio::sync::oneshot::channel::<()>();
        let handle = tokio::spawn(async move {
            orch.run(async {
                let _ = rx.await;
            })
            .await;
        });

        // The lease is protected while a cold executor waits for readiness.
        tokio::time::timeout(Duration::from_secs(1), async {
            loop {
                if fakes.state.lock().unwrap().heartbeats >= 2 {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(2)).await;
            }
        })
        .await
        .expect("heartbeats did not start during cold-start readiness");
        // Finish readiness and execution, but hold the terminal OPMS request.
        fakes.state.lock().unwrap().not_ready_checks_remaining = 0;
        fakes.release.add_permits(1);
        tokio::time::timeout(Duration::from_secs(1), async {
            loop {
                if fakes.state.lock().unwrap().publish_attempts == 1 {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(2)).await;
            }
        })
        .await
        .expect("terminal publication did not start");
        let before_publish = fakes.state.lock().unwrap().heartbeats;
        tokio::time::sleep(Duration::from_millis(20)).await;
        assert!(
            fakes.state.lock().unwrap().heartbeats > before_publish,
            "heartbeats must continue until terminal publication completes"
        );

        fakes.publish_release.add_permits(1);
        tokio::time::timeout(Duration::from_secs(1), async {
            loop {
                if fakes.state.lock().unwrap().published == 1 {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(2)).await;
            }
        })
        .await
        .expect("terminal publication did not complete");
        tokio::time::sleep(Duration::from_millis(10)).await;
        let after_publish = fakes.state.lock().unwrap().heartbeats;
        tokio::time::sleep(Duration::from_millis(20)).await;
        assert_eq!(
            fakes.state.lock().unwrap().heartbeats,
            after_publish,
            "heartbeats must stop after terminal publication"
        );

        let _ = tx.send(());
        let _ = handle.await;
    }

    /// Uses real time because paused time would hide an uninterruptible sleep.
    #[tokio::test]
    async fn heartbeat_not_found_stops_the_heartbeat_loop() {
        let fakes = Arc::new(Fakes {
            heartbeat_not_found: true,
            ..Default::default()
        });
        let task = Task {
            raw: Vec::new(),
            task_id: "t1".into(),
            job_id: "j1".into(),
            action_fqn: "bundle.action".into(),
            client: 1,
        };

        let (_stop, done) = spawn_heartbeats(Arc::clone(&fakes), task, Duration::from_millis(1));
        tokio::time::timeout(Duration::from_millis(100), done)
            .await
            .expect("heartbeat loop did not stop after OPMS returned not found")
            .expect("heartbeat task panicked");

        assert_eq!(fakes.state.lock().unwrap().heartbeats, 1);
    }

    #[tokio::test]
    async fn shutdown_cancels_an_in_progress_dequeue() {
        let fakes = Arc::new(Fakes {
            block_dequeue: true,
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

        for _ in 0..100 {
            if fakes.state.lock().unwrap().dequeued > 0 {
                break;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
        assert!(
            fakes.state.lock().unwrap().dequeued > 0,
            "the loop never started a dequeue"
        );

        let _ = tx.send(());
        tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("run() must cancel the in-progress dequeue on shutdown")
            .expect("orchestrator task panicked");
    }

    #[tokio::test]
    async fn shutdown_cancels_readiness_and_reports_dequeued_task() {
        let fakes = Arc::new(Fakes {
            state: Mutex::new(FakeState {
                not_ready_checks_remaining: usize::MAX,
                ..Default::default()
            }),
            tasks_to_serve: 1,
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

        for _ in 0..100 {
            if fakes.state.lock().unwrap().dequeued > 0 {
                break;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
        assert_eq!(fakes.state.lock().unwrap().dequeued, 1);

        let _ = tx.send(());
        tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("shutdown must cancel executor readiness")
            .expect("orchestrator task panicked");
        let state = fakes.state.lock().unwrap();
        assert_eq!(state.published, 1);
        assert_eq!(state.failures, 1);
    }

    #[tokio::test]
    async fn shutdown_waits_for_inflight_actions_to_publish() {
        let fakes = Arc::new(Fakes {
            tasks_to_serve: 1,
            ..Default::default()
        });
        let orch = Orchestrator::new(
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            Arc::clone(&fakes),
            test_params(1, Duration::from_secs(3600)),
        );

        let (tx, rx) = tokio::sync::oneshot::channel::<()>();
        let mut handle = tokio::spawn(async move {
            orch.run(async {
                let _ = rx.await;
            })
            .await;
        });

        for _ in 0..100 {
            if fakes.state.lock().unwrap().concurrent > 0 {
                break;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
        assert_eq!(fakes.state.lock().unwrap().concurrent, 1);

        let _ = tx.send(());
        assert!(
            tokio::time::timeout(Duration::from_millis(50), &mut handle)
                .await
                .is_err(),
            "run() returned before its in-flight action finished"
        );

        fakes.release.add_permits(1);
        tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("run() did not return after its in-flight action finished")
            .expect("orchestrator task panicked");
        assert_eq!(fakes.state.lock().unwrap().published, 1);
    }

    #[tokio::test]
    async fn shutdown_drain_is_bounded() {
        let fakes = Arc::new(Fakes {
            tasks_to_serve: 1,
            release: tokio::sync::Semaphore::new(0),
            ..Default::default()
        });
        let mut params = test_params(1, Duration::from_secs(1));
        params.drain_timeout = Duration::from_millis(20);
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

        tokio::time::timeout(Duration::from_secs(1), async {
            loop {
                if fakes.state.lock().unwrap().concurrent == 1 {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(1)).await;
            }
        })
        .await
        .expect("task did not start");

        tx.send(()).unwrap();
        tokio::time::timeout(Duration::from_millis(100), handle)
            .await
            .expect("run() exceeded its configured drain budget")
            .expect("orchestrator task panicked");
    }

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
    async fn dequeue_errors_honor_server_pacing() {
        let fakes = Arc::new(Fakes {
            fail_dequeue: true,
            dequeue_retry_after: Some(Duration::from_millis(10)),
            ..Default::default()
        });
        let mut params = test_params(1, Duration::from_secs(3600));
        params.max_attempts = 1;
        params.min_backoff = Duration::from_secs(3600);
        params.max_backoff = Duration::from_secs(3600);
        params.wait_before_retry = Duration::from_secs(3600);
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

        tokio::time::timeout(Duration::from_secs(1), async {
            while fakes.state.lock().unwrap().dequeued < 2 {
                tokio::time::sleep(Duration::from_millis(1)).await;
            }
        })
        .await
        .expect("server retry delay was ignored in favor of local backoff");

        tx.send(()).unwrap();
        handle.await.unwrap();
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

    /// The always-on control plane must report liveness to OPMS on its own
    /// schedule, and keep reporting it while the runner is idle. Without this the
    /// split runner is invisible to OPMS between tasks, even though the monolith
    /// it replaces health-checks every 30 seconds.
    #[tokio::test]
    async fn reports_liveness_to_opms_while_idle() {
        let fakes = Arc::new(Fakes::default());
        let mut params = test_params(1, Duration::from_secs(3600));
        params.health_check_interval = Duration::from_millis(5);
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

        tokio::time::timeout(Duration::from_secs(2), async {
            loop {
                if fakes.state.lock().unwrap().health_checks >= 3 {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(2)).await;
            }
        })
        .await
        .expect("no periodic OPMS health check was sent");

        let _ = tx.send(());
        tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("run() must return promptly")
            .expect("orchestrator task panicked");

        // The loop is owned by run(): once it returns, nothing keeps polling OPMS.
        let after_shutdown = fakes.state.lock().unwrap().health_checks;
        tokio::time::sleep(Duration::from_millis(40)).await;
        assert_eq!(
            fakes.state.lock().unwrap().health_checks,
            after_shutdown,
            "health checks must stop when the control plane stops"
        );
    }

    /// Liveness reporting must survive a failing health check while the control
    /// plane remains idle. Those are exactly the situations where operators need
    /// the runner's health signal.
    #[tokio::test]
    async fn liveness_reporting_survives_failures_while_idle() {
        let fakes = Arc::new(Fakes {
            fail_health_check: true,
            ..Default::default()
        });
        let mut params = test_params(1, Duration::from_secs(3600));
        params.health_check_interval = Duration::from_millis(5);
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

        tokio::time::timeout(Duration::from_secs(2), async {
            loop {
                if fakes.state.lock().unwrap().health_checks >= 3 {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(2)).await;
            }
        })
        .await
        .expect("a failing health check must be retried, not abandoned");

        assert_eq!(
            fakes.state.lock().unwrap().ensure_started_calls,
            0,
            "idle liveness checks must not start the executor"
        );

        let _ = tx.send(());
        tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("shutdown must stop the health-check loop too")
            .expect("orchestrator task panicked");
    }

    /// A rejected health check is logged, not retried immediately, and the
    /// server's pacing hint is honored (mirrors the Go loop's use of
    /// `HealthCheckData.RetryAfter`, which is populated even on error).
    #[tokio::test]
    async fn rejected_health_check_is_paced_by_the_server() {
        let opms = Arc::new(Fakes {
            health_check_status: 429,
            ..Default::default()
        });
        let (stop, done) = spawn_health_checks(Arc::clone(&opms), Duration::from_millis(5));
        tokio::time::timeout(Duration::from_secs(2), async {
            loop {
                if opms.state.lock().unwrap().health_checks >= 2 {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(2)).await;
            }
        })
        .await
        .expect("a rejected health check must be retried on the next tick");
        stop_health_checks(stop, done).await;

        let mut last_info = None;
        // A 200 logs at info the first time, then throttles; a rejection always
        // logs at error. Exercised here so the formatting path cannot panic.
        log_health_check(
            &HealthCheck {
                status: 200,
                server_time: None,
                retry_after: None,
                detail: String::new(),
            },
            &mut last_info,
        );
        assert!(last_info.is_some());
        let first = last_info;
        log_health_check(
            &HealthCheck {
                status: 200,
                server_time: Some("2026-02-03T04:05:06Z".into()),
                retry_after: None,
                detail: String::new(),
            },
            &mut last_info,
        );
        assert_eq!(
            last_info, first,
            "a second success inside the log-limit window must not reset it"
        );
    }
}
