// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::{ProcessConfig, RestartPolicy};
use crate::env::expand_env_vars;
use crate::handle::ProcessHandle;
use crate::platform;
use crate::shutdown::ShutdownBudget;
use crate::spawn::{SpawnProfile, profile_for};
use crate::state::ProcessState;
use anyhow::{Context, Result, bail};
use log::{debug, info, warn};
use std::collections::VecDeque;
use std::pin::Pin;
use std::sync::Arc;
use tokio::sync::Notify;
use tokio::sync::mpsc;
use tokio::task::JoinHandle;
use tokio::time::{self, Duration, Instant};

pub(crate) struct ProcessExit {
    pub name: String,
    pub pid: u32,
    pub status: std::process::ExitStatus,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct SpawnReservationToken(u64);

struct RestartTracker {
    count: u32,
    timestamps: VecDeque<Instant>,
    current_delay: f64,
    last_spawn_time: Option<Instant>,
}

impl RestartTracker {
    const BACKOFF_MULTIPLIER: f64 = 2.0;
    const MAX_TIMESTAMPS: usize = 100;

    fn new(initial_delay: f64) -> Self {
        Self {
            count: 0,
            timestamps: VecDeque::new(),
            current_delay: initial_delay,
            last_spawn_time: None,
        }
    }

    fn mark_spawned(&mut self) {
        self.last_spawn_time = Some(Instant::now());
    }

    fn is_burst_limited(&self, burst: u32, interval: Duration) -> bool {
        let cutoff = Instant::now() - interval;
        let recent = self.timestamps.iter().filter(|t| **t > cutoff).count() as u32;
        recent >= burst
    }

    /// Returns `true` when the prior run met `runtime_success_sec`.
    fn record(&mut self, base_delay: f64, runtime_success: Duration) -> bool {
        let mut met_runtime_success = false;
        if let Some(spawn_time) = self.last_spawn_time.take()
            && spawn_time.elapsed() >= runtime_success
        {
            self.current_delay = base_delay;
            self.count = 0;
            met_runtime_success = true;
        }
        self.count += 1;
        self.timestamps.push_back(Instant::now());
        if self.timestamps.len() > Self::MAX_TIMESTAMPS {
            self.timestamps.pop_front();
        }
        met_runtime_success
    }

    fn advance_backoff(&mut self, max_delay: f64) {
        self.current_delay = (self.current_delay * Self::BACKOFF_MULTIPLIER).min(max_delay);
    }

    fn delay(&self) -> Duration {
        Duration::from_secs_f64(self.current_delay)
    }

    fn next_restart_delay(
        &mut self,
        config: &ProcessConfig,
        process_name: &str,
    ) -> Option<(Duration, bool)> {
        if self.is_burst_limited(config.burst_limit(), config.burst_interval()) {
            warn!("[{process_name}] start limit reached, not restarting");
            return None;
        }
        let met_runtime_success = self.record(config.restart_delay(), config.runtime_success());
        let delay = self.delay();
        info!(
            "[{process_name}] restart #{} in {:.1}s",
            self.count,
            delay.as_secs_f64()
        );
        self.advance_backoff(config.max_restart_delay());
        Some((delay, met_runtime_success))
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum RestartDecision {
    Allowed,
    Denied,
    NotApplicable,
}

impl RestartDecision {
    fn is_allowed(self) -> bool {
        matches!(self, RestartDecision::Allowed)
    }
}

fn restart_allowed_for_state(state: ProcessState, policy: &RestartPolicy) -> RestartDecision {
    match (state, policy) {
        (ProcessState::Exited | ProcessState::Failed, RestartPolicy::Always) => {
            RestartDecision::Allowed
        }
        (ProcessState::Failed, RestartPolicy::OnFailure) => RestartDecision::Allowed,
        (ProcessState::Exited, RestartPolicy::OnSuccess) => RestartDecision::Allowed,
        (ProcessState::Exited | ProcessState::Failed, _) => RestartDecision::Denied,
        _ => RestartDecision::NotApplicable,
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessOrigin {
    Config,
    Runtime,
}

enum ForceKillWaitTarget<'a> {
    Watcher(&'a mut Pin<&'a mut JoinHandle<Option<std::process::ExitStatus>>>),
    Child,
}

pub(crate) enum StopWaitPlan {
    Watcher {
        name: String,
        handle: JoinHandle<Option<std::process::ExitStatus>>,
        graceful_budget: Duration,
    },
    Child {
        name: String,
        handle: ProcessHandle,
        graceful_budget: Duration,
    },
}

pub(crate) enum StopWaitResult {
    Exited(Option<std::process::ExitStatus>),
    WatcherTimedOut(JoinHandle<Option<std::process::ExitStatus>>),
    ChildTimedOut(ProcessHandle),
    JoinFailed,
}

impl StopWaitResult {
    /// Drop a wait outcome that no longer matches the owning child generation.
    fn drop_if_unowned(self) {
        match self {
            Self::Exited(_) | Self::JoinFailed => {}
            Self::WatcherTimedOut(handle) => {
                handle.abort();
            }
            Self::ChildTimedOut(handle) => {
                drop(handle);
            }
        }
    }
}

/// Child generation that owns an in-flight stop wait.
///
/// Duplicate `Stop` RPCs coalesce on [`Notify`] (see [`coalesced_run_stop_wait`]).
/// The generation stamp prevents a stale watcher from mutating a replacement child
/// when `Start` commits before the old wait finishes.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
struct StopWaitGeneration(Option<u64>);

impl StopWaitGeneration {
    fn claim(&mut self, spawn_seq: u64) {
        if self.0.is_none() {
            self.0 = Some(spawn_seq);
        }
    }

    fn owns(&self, spawn_seq: u64) -> bool {
        self.0 == Some(spawn_seq)
    }

    fn clear(&mut self) {
        self.0 = None;
    }
}

impl StopWaitPlan {
    pub(crate) async fn execute(self) -> StopWaitResult {
        match self {
            Self::Watcher {
                name,
                mut handle,
                graceful_budget,
            } => match time::timeout(graceful_budget, &mut handle).await {
                Ok(Ok(status)) => StopWaitResult::Exited(status),
                Ok(Err(error)) => {
                    warn!("[{name}] watcher join failed during stop: {error:#}");
                    StopWaitResult::JoinFailed
                }
                Err(_) => StopWaitResult::WatcherTimedOut(handle),
            },
            Self::Child {
                name,
                mut handle,
                graceful_budget,
            } => match time::timeout(graceful_budget, handle.wait()).await {
                Ok(Ok(status)) => StopWaitResult::Exited(Some(status)),
                Ok(Err(error)) => {
                    warn!("[{name}] wait failed during stop: {error:#}");
                    StopWaitResult::JoinFailed
                }
                Err(_) => StopWaitResult::ChildTimedOut(handle),
            },
        }
    }
}

#[cfg(windows)]
const DEFERRED_SPAWN_USER: &str = "unknown";

const JOB_DRAIN_TIMEOUT: Duration = Duration::from_secs(10);

pub(crate) struct ManagedChildSpawnData {
    process_name: String,
    handle: ProcessHandle,
    intended_user: String,
    #[cfg(windows)]
    job_object: platform::JobObject,
    #[cfg(windows)]
    user_profile: Option<platform::UserProfileGuard>,
}

impl ManagedChildSpawnData {
    #[cfg(unix)]
    pub(crate) fn uncommitted(
        process_name: &str,
        handle: ProcessHandle,
        intended_user: String,
    ) -> Self {
        Self {
            process_name: process_name.to_owned(),
            handle,
            intended_user,
        }
    }

    #[cfg(windows)]
    pub(crate) fn uncommitted(
        process_name: &str,
        handle: ProcessHandle,
        intended_user: String,
        job_object: platform::JobObject,
        user_profile: Option<platform::UserProfileGuard>,
    ) -> Self {
        Self {
            process_name: process_name.to_owned(),
            handle,
            intended_user,
            job_object,
            user_profile,
        }
    }

    #[cfg(test)]
    pub(crate) fn pid(&self) -> Option<u32> {
        self.handle.id()
    }

    #[cfg(unix)]
    pub(crate) fn abort_uncommitted_sync(self) {
        let Self {
            process_name,
            handle,
            intended_user: _,
        } = self;

        if let Err(e) = handle.sync_abort_teardown() {
            warn!("[{process_name}] failed to terminate uncommitted spawn: {e:#}");
        }
    }

    #[cfg(windows)]
    pub(crate) fn abort_uncommitted_sync(self) {
        let Self {
            process_name,
            handle,
            intended_user: _,
            job_object,
            user_profile,
        } = self;

        if let Err(e) = job_object.terminate() {
            warn!("[{process_name}] failed to terminate uncommitted spawn job: {e:#}");
        }
        let pid = handle.id().unwrap_or(0);
        if let Err(e) = handle.sync_abort_teardown() {
            warn!("[{process_name}] failed to terminate uncommitted spawn (pid={pid}): {e:#}");
        }

        let timeout = JOB_DRAIN_TIMEOUT;
        if !job_object.wait_until_empty(timeout) {
            warn!(
                "[{process_name}] timed out waiting for uncommitted spawn job to drain before releasing profile"
            );
        }

        drop(user_profile);
    }

    pub(crate) fn commit_into(self, proc: &mut ManagedProcess) {
        proc.pid = self.handle.id();
        proc.user = self.intended_user;
        #[cfg(windows)]
        {
            proc.wait_control = Some(self.handle.wait_control());
            proc.job_object = Some(self.job_object);
            proc.user_profile = self.user_profile;
        }
        proc.handle = Some(self.handle);
    }
}

pub(crate) struct ManagedChildSpawn {
    inner: Option<ManagedChildSpawnData>,
}

impl From<ManagedChildSpawnData> for ManagedChildSpawn {
    fn from(data: ManagedChildSpawnData) -> Self {
        Self { inner: Some(data) }
    }
}

impl ManagedChildSpawn {
    #[cfg(test)]
    pub(crate) fn pid(&self) -> Option<u32> {
        self.inner.as_ref()?.pid()
    }

    pub(crate) fn drain(mut self) -> Option<ManagedChildSpawnData> {
        self.inner.take()
    }

    pub(crate) fn abort(mut self) {
        if let Some(data) = self.inner.take() {
            data.abort_uncommitted_sync();
        }
    }
}

impl Drop for ManagedChildSpawn {
    fn drop(&mut self) {
        if let Some(data) = self.inner.take() {
            data.abort_uncommitted_sync();
        }
    }
}

pub struct ManagedProcess {
    name: String,
    uuid: String,
    config: ProcessConfig,
    profile: SpawnProfile,
    user: String,
    state: ProcessState,
    pid: Option<u32>,
    handle: Option<ProcessHandle>,
    watcher_handle: Option<JoinHandle<Option<std::process::ExitStatus>>>,
    restarts: RestartTracker,
    spawn_seq: u64,
    stop_wait_generation: StopWaitGeneration,
    stop_wait_notify: Arc<Notify>,
    reservation_seq: u64,
    spawn_reservation: Option<SpawnReservationToken>,
    origin: ProcessOrigin,
    last_exit_status: Option<std::process::ExitStatus>,
    #[cfg(windows)]
    job_object: Option<platform::JobObject>,
    #[cfg(windows)]
    user_profile: Option<platform::UserProfileGuard>,
}

impl ManagedProcess {
    pub(crate) const FORCE_KILL_TIMEOUT: Duration = JOB_DRAIN_TIMEOUT;

    pub fn new_config(name: String, uuid: String, config: ProcessConfig) -> Self {
        Self::new_inner(name, uuid, config, ProcessOrigin::Config)
    }

    pub fn new_runtime(name: String, uuid: String, config: ProcessConfig) -> Self {
        Self::new_inner(name, uuid, config, ProcessOrigin::Runtime)
    }

    fn new_inner(name: String, uuid: String, config: ProcessConfig, origin: ProcessOrigin) -> Self {
        let profile = profile_for(&name);
        let restarts = RestartTracker::new(config.restart_delay());
        Self {
            name,
            uuid,
            config,
            profile,
            user: Self::initial_intended_user(),
            state: ProcessState::Created,
            pid: None,
            handle: None,
            watcher_handle: None,
            restarts,
            spawn_seq: 0,
            stop_wait_generation: StopWaitGeneration::default(),
            stop_wait_notify: Arc::new(Notify::new()),
            reservation_seq: 0,
            spawn_reservation: None,
            origin,
            last_exit_status: None,
            #[cfg(windows)]
            job_object: None,
            #[cfg(windows)]
            user_profile: None,
        }
    }

    fn initial_intended_user() -> String {
        #[cfg(unix)]
        {
            platform::spawn_user_for_supervisor()
        }
        #[cfg(windows)]
        {
            DEFERRED_SPAWN_USER.to_string()
        }
    }

    pub fn origin(&self) -> ProcessOrigin {
        self.origin
    }

    pub fn uuid(&self) -> &str {
        &self.uuid
    }

    pub fn name(&self) -> &str {
        &self.name
    }

    pub fn state(&self) -> ProcessState {
        self.state
    }

    pub fn pid(&self) -> Option<u32> {
        self.pid
    }

    pub fn config(&self) -> &ProcessConfig {
        &self.config
    }

    #[cfg(windows)]
    pub(crate) fn clear_windows_spawn_resources(&mut self) {
        self.job_object = None;
        self.user_profile = None;
    }

    pub fn profile(&self) -> SpawnProfile {
        self.profile
    }

    pub fn user(&self) -> &str {
        &self.user
    }

    pub fn restart_count(&self) -> u32 {
        self.restarts.count
    }

    pub(crate) fn spawn_seq(&self) -> u64 {
        self.spawn_seq
    }

    pub(crate) fn stop_wait_notify(&self) -> Arc<Notify> {
        Arc::clone(&self.stop_wait_notify)
    }

    fn inc_spawn_seq(&mut self) {
        self.spawn_seq += 1;
    }

    pub fn last_exit_code(&self) -> Option<i32> {
        self.last_exit_status.and_then(|s| s.code())
    }

    pub fn last_signal(&self) -> Option<i32> {
        self.last_exit_status
            .and_then(|s| platform::last_signal(&s))
    }

    #[cfg(test)]
    pub(crate) fn config_mut(&mut self) -> &mut ProcessConfig {
        &mut self.config
    }

    fn transition_to(&mut self, next: ProcessState) {
        if !self.state.can_transition_to(next) {
            let msg = format!(
                "[{}] invalid state transition: {} -> {next}",
                self.name, self.state
            );
            warn!("{msg}, ignoring");
            if cfg!(debug_assertions) {
                panic!("{msg}");
            }
            return;
        }
        let notify_coalesced_waiters = self.state.awaiting_stop() && !next.awaiting_stop();
        self.state = next;
        if notify_coalesced_waiters {
            self.stop_wait_notify.notify_waiters();
        }
    }

    fn condition_path_exists_met(&self) -> bool {
        let Some(raw) = &self.config.condition_path_exists else {
            return true;
        };
        let path = expand_env_vars(raw);
        if std::path::Path::new(&path).exists() {
            return true;
        }
        info!("[{}] condition_path_exists not met: {path}", self.name);
        false
    }

    #[must_use]
    pub(crate) fn start_conditions_met(&self) -> bool {
        self.condition_path_exists_met()
    }

    #[must_use]
    pub fn may_auto_start(&self) -> bool {
        if !self.config.auto_start {
            info!("[{}] auto_start=false, skipping", self.name);
            return false;
        }
        self.start_conditions_met()
    }

    #[must_use]
    pub(crate) fn may_respawn(&self) -> bool {
        if self.config.auto_start {
            self.may_auto_start()
        } else {
            self.start_conditions_met()
        }
    }

    #[must_use]
    pub(crate) fn should_complete_pending_restart(&self) -> bool {
        self.restart_eligibility().is_allowed() && self.may_respawn()
    }

    pub(crate) fn begin_spawn_reservation(&mut self) -> Result<SpawnReservationToken> {
        if !self.state.can_transition_to(ProcessState::Starting) {
            bail!(
                "[{}] cannot begin spawn: invalid state {}",
                self.name,
                self.state
            );
        }
        self.reservation_seq = self.reservation_seq.wrapping_add(1);
        let token = SpawnReservationToken(self.reservation_seq);
        self.spawn_reservation = Some(token);
        self.transition_to(ProcessState::Starting);
        Ok(token)
    }

    pub(crate) fn may_commit_spawn_reservation(&self, token: SpawnReservationToken) -> bool {
        self.spawn_reservation == Some(token)
    }

    pub(crate) fn cancel_spawn_reservation(&mut self) {
        self.clear_spawn_reservation_token();
        if self.state == ProcessState::Starting && !self.has_child_handle() {
            self.transition_to(ProcessState::Stopped);
        }
    }

    fn clear_spawn_reservation_token(&mut self) {
        self.spawn_reservation = None;
    }

    pub(crate) fn commit_spawn_from_outcome(
        &mut self,
        outcome: ManagedChildSpawn,
        exit_tx: mpsc::Sender<ProcessExit>,
    ) -> Result<()> {
        if self.state != ProcessState::Starting {
            bail!(
                "[{}] cannot commit spawn: expected Starting, got {}",
                self.name,
                self.state
            );
        }
        if let Err(e) = self.apply_managed_child_spawn(outcome) {
            #[cfg(windows)]
            self.clear_windows_spawn_resources();
            self.clear_spawn_reservation_token();
            self.transition_to(ProcessState::Failed);
            return Err(e);
        }
        self.clear_spawn_reservation_token();
        self.attach_exit_watcher(exit_tx);
        Ok(())
    }

    pub fn spawn(&mut self) -> Result<()> {
        if !self.state.can_transition_to(ProcessState::Starting) {
            bail!("[{}] cannot spawn: invalid state {}", self.name, self.state);
        }
        self.transition_to(ProcessState::Starting);
        let result = self.try_spawn();
        if result.is_err() {
            #[cfg(windows)]
            self.clear_windows_spawn_resources();
            self.transition_to(ProcessState::Failed);
        }
        result
    }

    pub(crate) fn apply_managed_child_spawn(&mut self, outcome: ManagedChildSpawn) -> Result<()> {
        let Some(outcome) = outcome.drain() else {
            bail!("managed child spawn already consumed");
        };
        outcome.commit_into(self);
        info!(
            "[{}] spawned (pid={}, cmd={})",
            self.name,
            self.pid.map_or("unknown".to_string(), |p| p.to_string()),
            self.config.command
        );
        self.transition_to(ProcessState::Running);
        self.inc_spawn_seq();
        self.restarts.mark_spawned();
        Ok(())
    }

    pub(crate) fn mark_spawn_failed(&mut self) {
        self.clear_spawn_reservation_token();
        if self.state.can_transition_to(ProcessState::Starting) {
            self.transition_to(ProcessState::Starting);
        }
        if self.state.can_transition_to(ProcessState::Failed) {
            self.transition_to(ProcessState::Failed);
        }
        #[cfg(windows)]
        self.clear_windows_spawn_resources();
    }

    fn attach_exit_watcher(&mut self, exit_tx: mpsc::Sender<ProcessExit>) {
        if let Some(mut proc_handle) = self.take_handle() {
            let name = self.name().to_owned();
            let pid = self.pid().unwrap_or(0);
            let watcher_handle = tokio::spawn(async move {
                let status = match proc_handle.wait().await {
                    Ok(status) => status,
                    Err(e) => {
                        warn!("[{name}] wait error: {e}, killing process");
                        let _ = proc_handle.kill().await;
                        match proc_handle.wait().await {
                            Ok(s) => s,
                            Err(e2) => {
                                warn!("[{name}] failed to reap after kill: {e2}");
                                return None;
                            }
                        }
                    }
                };
                if let Err(e) = exit_tx
                    .send(ProcessExit {
                        name: name.clone(),
                        pid,
                        status,
                    })
                    .await
                {
                    warn!("[{name}] failed to deliver exit event (pid={pid}): {e}");
                }
                Some(status)
            });
            self.set_watcher_handle(watcher_handle);
        }
    }

    fn try_spawn(&mut self) -> Result<()> {
        #[cfg(windows)]
        let _console_guard = platform::console_lock();

        let outcome = platform::spawn_managed_child(self.name(), self.config())?;
        self.apply_managed_child_spawn(outcome)
    }

    pub fn is_running(&self) -> bool {
        self.state.is_alive()
    }

    pub fn take_handle(&mut self) -> Option<ProcessHandle> {
        self.handle.take()
    }

    fn has_child_handle(&self) -> bool {
        self.handle.is_some()
    }

    pub(crate) fn set_watcher_handle(
        &mut self,
        handle: JoinHandle<Option<std::process::ExitStatus>>,
    ) {
        self.watcher_handle = Some(handle);
    }

    fn graceful_stop(&self) {
        if let Some(pid) = self.pid
            && let Err(e) = platform::send_graceful_stop(pid)
        {
            warn!("[{}] graceful stop failed: {e}", self.name);
        }
    }

    fn force_kill(&mut self) {
        #[cfg(windows)]
        if let Some(ref job) = self.job_object {
            if let Err(e) = job.terminate() {
                warn!("[{}] job object terminate failed: {e}", self.name);
            } else {
                self.clear_windows_spawn_resources();
                return;
            }
        }

        if let Some(pid) = self.pid
            && let Err(e) = platform::send_force_kill(pid)
        {
            warn!("[{}] force kill failed: {e}", self.name);
        }
    }

    fn stop_timeout(&self) -> Duration {
        self.config.stop_timeout()
    }

    fn mark_stopped(&mut self) {
        #[cfg(windows)]
        {
            self.cancel_process_wait();
            self.wait_control = None;
            self.clear_windows_spawn_resources();
        }
        self.stop_wait_generation.clear();
        self.transition_to(ProcessState::Stopped);
        self.pid = None;
    }

    fn apply_stop_exit(&mut self, status: Option<std::process::ExitStatus>) {
        if !self.state.is_alive() {
            return;
        }
        if let Some(status) = status {
            self.set_last_status(status);
        } else {
            self.mark_stopped();
        }
    }

    pub(crate) fn plan_stop_wait(&mut self, budget: ShutdownBudget) -> Option<StopWaitPlan> {
        if !self.state.awaiting_stop() {
            return None;
        }

        let graceful_budget = budget.graceful_budget(self.stop_timeout());
        if let Some(handle) = self.watcher_handle.take() {
            self.stop_wait_generation.claim(self.spawn_seq);
            return Some(StopWaitPlan::Watcher {
                name: self.name.clone(),
                handle,
                graceful_budget,
            });
        }

        if let Some(handle) = self.take_handle() {
            self.stop_wait_generation.claim(self.spawn_seq);
            return Some(StopWaitPlan::Child {
                name: self.name.clone(),
                handle,
                graceful_budget,
            });
        }

        None
    }

    pub(crate) async fn finalize_stop_wait(
        &mut self,
        result: StopWaitResult,
        budget: ShutdownBudget,
    ) -> bool {
        if !self.stop_wait_generation.owns(self.spawn_seq) {
            result.drop_if_unowned();
            return false;
        }

        let graceful_budget = budget.graceful_budget(self.stop_timeout());
        match result {
            StopWaitResult::Exited(status) => self.apply_stop_exit(status),
            StopWaitResult::JoinFailed => {
                if self.state.is_alive() {
                    self.mark_stopped();
                }
            }
            StopWaitResult::WatcherTimedOut(handle) => {
                if self.state.is_alive() {
                    tokio::pin!(handle);
                    if self
                        .force_kill_and_wait(
                            graceful_budget,
                            budget,
                            ForceKillWaitTarget::Watcher(&mut handle),
                        )
                        .await
                        .is_none()
                        && self.state.is_alive()
                    {
                        self.mark_stopped();
                    }
                }
            }
            StopWaitResult::ChildTimedOut(handle) => {
                self.handle = Some(handle);
                if self.state.is_alive()
                    && self
                        .force_kill_and_wait(graceful_budget, budget, ForceKillWaitTarget::Child)
                        .await
                        .is_none()
                    && self.state.is_alive()
                {
                    self.mark_stopped();
                }
            }
        }
        self.state.awaiting_stop()
    }

    pub(crate) async fn complete_stop_wait(&mut self, budget: ShutdownBudget) {
        if self.state.awaiting_stop() && self.stop_wait_generation.owns(self.spawn_seq) {
            self.mark_stopped();
        } else {
            self.stop_wait_generation.clear();
        }
        self.release_stop_wait_resources(budget).await;
    }

    async fn release_stop_wait_resources(&mut self, _budget: ShutdownBudget) {
        #[cfg(windows)]
        self.ensure_windows_spawn_resources_released(_budget).await;
    }

    pub(crate) async fn wait_for_stop_since(&mut self, budget: ShutdownBudget) {
        if coalesced_run_stop_wait(self, budget).await {
            self.complete_stop_wait(budget).await;
        }
    }

    pub fn set_last_status(&mut self, status: std::process::ExitStatus) {
        self.last_exit_status = Some(status);
        self.pid = None;
        #[cfg(windows)]
        self.clear_windows_spawn_resources();
        if self.state == ProcessState::Stopping {
            self.stop_wait_generation.clear();
            self.transition_to(ProcessState::Stopped);
        } else if status.success() {
            self.transition_to(ProcessState::Exited);
        } else {
            self.transition_to(ProcessState::Failed);
        }
    }

    async fn force_kill_and_wait(
        &mut self,
        graceful_budget: Duration,
        budget: ShutdownBudget,
        target: ForceKillWaitTarget<'_>,
    ) -> Option<std::process::ExitStatus> {
        warn!(
            "[{}] stop timeout ({}s) reached, force-killing",
            self.name,
            graceful_budget.as_secs()
        );
        self.force_kill();

        let force_timeout = budget.remaining_cap(Self::FORCE_KILL_TIMEOUT);
        if force_timeout.is_zero() {
            warn!(
                "[{}] shutdown deadline reached; skipping force-kill wait",
                self.name
            );
            return None;
        }

        match target {
            ForceKillWaitTarget::Watcher(handle) => {
                match time::timeout(force_timeout, &mut **handle).await {
                    Ok(Ok(status)) => status,
                    Ok(Err(e)) => {
                        warn!(
                            "[{}] watcher join failed after force-kill: {e:#}",
                            self.name
                        );
                        None
                    }
                    Err(_) => {
                        warn!("[{}] still running after force-kill, giving up", self.name);
                        None
                    }
                }
            }
            ForceKillWaitTarget::Child => match time::timeout(force_timeout, self.wait()).await {
                Ok(Ok(status)) => Some(status),
                Ok(Err(e)) => {
                    warn!("[{}] wait failed after force-kill: {e:#}", self.name);
                    None
                }
                Err(_) => {
                    warn!("[{}] still running after force-kill, giving up", self.name);
                    None
                }
            },
        }
    }

    pub fn request_stop(&mut self) {
        match self.state {
            ProcessState::Running => {
                self.transition_to(ProcessState::Stopping);
                info!("[{}] stopping (graceful)", self.name);
                self.graceful_stop();
            }
            ProcessState::Starting if !self.has_child_handle() => {
                info!("[{}] cancelling in-flight spawn", self.name);
                self.clear_spawn_reservation_token();
                self.transition_to(ProcessState::Stopped);
            }
            ProcessState::Stopping => {
                debug!("[{}] stop requested while already stopping", self.name);
            }
            _ => {}
        }
    }

    pub async fn wait_for_stop(&mut self) {
        self.wait_for_stop_since(ShutdownBudget::unlimited(Instant::now().into()))
            .await;
    }

    pub async fn stop(&mut self) {
        let started = Instant::now();

        match self.state {
            ProcessState::Running | ProcessState::Stopping => {
                self.request_stop();
                self.wait_for_stop().await;
                debug!(
                    "[{}] stop finished (state={}), took {:?}",
                    self.name,
                    self.state,
                    started.elapsed()
                );
            }
            ProcessState::Starting if !self.has_child_handle() => {
                self.request_stop();
                debug!(
                    "[{}] stop finished (state={}, cancelled in-flight spawn), took {:?}",
                    self.name,
                    self.state,
                    started.elapsed()
                );
            }
            _ => {
                debug!(
                    "[{}] stop finished (state={}, no-op), took {:?}",
                    self.name,
                    self.state,
                    started.elapsed()
                );
            }
        }
    }

    #[cfg(unix)]
    pub fn send_signal(&self, sig: nix::sys::signal::Signal) {
        if let Some(pid) = self.pid {
            match platform::process_group_id(pid) {
                Ok(pgid) => {
                    if let Err(e) = nix::sys::signal::kill(pgid, sig) {
                        warn!("[{}] failed to send {sig} to pgid {pid}: {e}", self.name);
                    }
                }
                Err(e) => {
                    warn!("[{}] {e}", self.name);
                }
            }
        }
    }

    pub async fn wait(&mut self) -> Result<std::process::ExitStatus> {
        let handle = self
            .handle
            .as_mut()
            .context("no process handle to wait on")?;
        let status = handle.wait().await?;
        info!("[{}] exited with {status}", self.name);
        self.handle = None;
        Ok(status)
    }

    #[cfg(test)]
    pub fn should_restart(&self, status: &std::process::ExitStatus) -> bool {
        let state = if status.success() {
            ProcessState::Exited
        } else {
            ProcessState::Failed
        };
        restart_allowed_for_state(state, &self.config.restart).is_allowed()
    }

    #[cfg(test)]
    pub fn restart_policy(&self) -> &RestartPolicy {
        &self.config.restart
    }

    fn restart_eligibility(&self) -> RestartDecision {
        restart_allowed_for_state(self.state, &self.config.restart)
    }

    fn log_restart_policy_skip(&self) {
        if self.config.restart != RestartPolicy::Never {
            info!(
                "[{}] exit does not match restart policy (state={}, restart={}), not restarting",
                self.name, self.state, self.config.restart,
            );
        } else {
            debug!(
                "[{}] not restarting (state={}, restart={})",
                self.name, self.state, self.config.restart,
            );
        }
    }

    #[must_use]
    pub fn schedule_restart(&mut self) -> Option<Duration> {
        match self.restart_eligibility() {
            RestartDecision::Allowed => self.record_restart_delay(),
            RestartDecision::Denied => {
                self.log_restart_policy_skip();
                None
            }
            RestartDecision::NotApplicable => None,
        }
    }

    fn record_restart_delay(&mut self) -> Option<Duration> {
        self.restarts
            .next_restart_delay(&self.config, &self.name)
            .map(|(delay, _)| delay)
    }
}

pub(crate) trait StopWaitContext {
    async fn stop_wait_active(&mut self) -> bool;

    async fn await_stop_progress(&mut self) -> bool;

    async fn plan_stop(&mut self, budget: ShutdownBudget) -> Option<StopWaitPlan>;

    async fn finalize_stop(&mut self, result: StopWaitResult, budget: ShutdownBudget) -> bool;
}

pub(crate) async fn run_stop_wait<C: StopWaitContext>(ctx: &mut C, budget: ShutdownBudget) -> bool {
    let mut participated = false;
    while let Some(plan) = ctx.plan_stop(budget).await {
        participated = true;
        let result = plan.execute().await;
        if !ctx.finalize_stop(result, budget).await {
            break;
        }
    }
    participated
}

/// Runs stop wait, coalescing duplicate waiters until one owns the handles.
///
/// Returns `true` when this caller owned the stop and should run final cleanup.
pub(crate) async fn coalesced_run_stop_wait<C: StopWaitContext>(
    ctx: &mut C,
    budget: ShutdownBudget,
) -> bool {
    loop {
        if !ctx.stop_wait_active().await {
            return false;
        }
        if run_stop_wait(ctx, budget).await {
            return true;
        }
        if !ctx.await_stop_progress().await {
            return false;
        }
    }
}

impl StopWaitContext for ManagedProcess {
    async fn stop_wait_active(&mut self) -> bool {
        self.state.awaiting_stop()
    }

    async fn await_stop_progress(&mut self) -> bool {
        if !self.state.awaiting_stop() {
            return false;
        }
        self.stop_wait_notify.notified().await;
        self.state.awaiting_stop()
    }

    async fn plan_stop(&mut self, budget: ShutdownBudget) -> Option<StopWaitPlan> {
        self.plan_stop_wait(budget)
    }

    async fn finalize_stop(&mut self, result: StopWaitResult, budget: ShutdownBudget) -> bool {
        self.finalize_stop_wait(result, budget).await
    }
}

#[cfg(test)]
pub mod tests {
    use super::*;
    use crate::config::ProcessConfig;
    use crate::env::expand_vars_with;
    use crate::test_helpers;
    #[cfg(unix)]
    use nix::sys::signal::Signal;

    #[test]
    fn test_expand_vars_substitutes_known() {
        let lookup = |name: &str| match name {
            "DD_CONF_DIR" => Some("/etc/datadog-agent-exp".to_string()),
            _ => None,
        };
        assert_eq!(
            expand_vars_with("${DD_CONF_DIR}/otel-config.yaml", lookup),
            "/etc/datadog-agent-exp/otel-config.yaml"
        );
        // Multiple references and a leading dash (optional environment_file form) are preserved.
        assert_eq!(
            expand_vars_with("-${DD_CONF_DIR}/environment", lookup),
            "-/etc/datadog-agent-exp/environment"
        );
    }

    #[test]
    fn test_expand_vars_leaves_unknown_literal() {
        let lookup = |_: &str| None;
        assert_eq!(
            expand_vars_with("${MISSING}/x", lookup),
            "${MISSING}/x",
            "unset variables must be left literal so misconfiguration fails loudly"
        );
    }

    #[test]
    fn test_expand_vars_no_placeholder_untouched() {
        let lookup = |_: &str| Some("should-not-be-used".to_string());
        let path = "/opt/datadog-packages/datadog-agent/stable/embedded/bin/otel-agent";
        assert_eq!(expand_vars_with(path, lookup), path);
        // A dangling `${` with no closing brace is emitted verbatim.
        assert_eq!(expand_vars_with("a ${ b", lookup), "a ${ b");
    }

    #[test]
    fn test_initial_state_is_created() {
        let (cmd, args) = test_helpers::true_cmd();
        let proc = ManagedProcess::new_config(
            "test".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        assert_eq!(proc.state(), ProcessState::Created);
        assert!(!proc.is_running());
    }

    #[tokio::test]
    async fn test_state_transitions_spawn_exit_success() {
        let (cmd, args) = test_helpers::exit_cmd(0);
        let mut proc = ManagedProcess::new_config(
            "t".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        assert_eq!(proc.state(), ProcessState::Created);

        proc.spawn().unwrap();
        assert_eq!(proc.state(), ProcessState::Running);
        assert!(proc.is_running());

        let status = proc.wait().await.unwrap();
        assert!(status.success());
        proc.set_last_status(status);
        assert_eq!(proc.state(), ProcessState::Exited);
        assert!(!proc.is_running());
    }

    #[tokio::test]
    async fn test_state_transitions_spawn_exit_failure() {
        let (cmd, args) = test_helpers::exit_cmd(1);
        let mut proc = ManagedProcess::new_config(
            "t".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.spawn().unwrap();
        assert_eq!(proc.state(), ProcessState::Running);

        let status = proc.wait().await.unwrap();
        assert!(!status.success());
        proc.set_last_status(status);
        assert_eq!(proc.state(), ProcessState::Failed);
    }

    #[tokio::test]
    async fn test_state_after_take_handle_still_running() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "t".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.spawn().unwrap();
        assert_eq!(proc.state(), ProcessState::Running);

        let handle = proc.take_handle();
        assert!(handle.is_some());
        assert_eq!(
            proc.state(),
            ProcessState::Running,
            "state should remain Running after take_handle"
        );
        assert!(proc.is_running());

        if let Some(pid) = proc.pid() {
            test_helpers::cleanup_process(pid);
        }
        let mut handle = handle.unwrap();
        let _ = handle.wait().await;
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn test_send_signal_works_after_take_handle() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "t".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.spawn().unwrap();
        let mut handle = proc.take_handle().unwrap();

        proc.send_signal(Signal::SIGTERM);
        let status = handle.wait().await.unwrap();
        assert!(
            !status.success(),
            "signal by stored PID should reach handle after take_handle"
        );
    }

    #[test]
    fn test_may_auto_start_auto_start_true_no_condition() {
        let (cmd, args) = test_helpers::true_cmd();
        let proc = ManagedProcess::new_config(
            "test".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        assert!(proc.may_auto_start());
    }

    #[test]
    fn test_may_auto_start_auto_start_false() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.auto_start = false;
        let proc = ManagedProcess::new_config("test".into(), test_helpers::test_uuid(), cfg);
        assert!(!proc.may_auto_start());
    }

    #[test]
    fn test_may_auto_start_condition_path_exists_met() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        let exe = std::env::current_exe().unwrap();
        cfg.condition_path_exists = Some(exe.to_str().unwrap().to_string());
        let proc = ManagedProcess::new_config("test".into(), test_helpers::test_uuid(), cfg);
        assert!(proc.may_auto_start());
    }

    #[test]
    fn test_may_auto_start_condition_path_exists_not_met() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.condition_path_exists = Some("/nonexistent/path/binary".to_string());
        let proc = ManagedProcess::new_config("test".into(), test_helpers::test_uuid(), cfg);
        assert!(!proc.may_auto_start());
    }

    #[tokio::test]
    async fn test_cancel_spawn_reservation_from_starting() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );

        proc.begin_spawn_reservation().unwrap();
        assert_eq!(proc.state(), ProcessState::Starting);
        assert!(proc.is_running());

        proc.stop().await;
        assert_eq!(proc.state(), ProcessState::Stopped);
        assert!(!proc.is_running());
    }

    #[tokio::test]
    async fn test_spawn_and_is_running() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "sleeper".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );

        assert!(!proc.is_running());
        proc.spawn().unwrap();
        assert!(proc.is_running());

        proc.stop().await;
        assert_eq!(proc.state(), ProcessState::Stopped);
    }

    #[tokio::test]
    async fn test_spawn_nonexistent_binary() {
        let cfg = test_helpers::make_config("/nonexistent/binary", vec![]);
        let mut proc = ManagedProcess::new_config("bad".into(), test_helpers::test_uuid(), cfg);
        assert!(proc.spawn().is_err());
        assert!(!proc.is_running());
        assert_eq!(proc.state(), ProcessState::Failed);
    }

    #[tokio::test]
    async fn test_spawn_failure_after_stop_goes_through_starting_to_failed() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.spawn().unwrap();
        proc.stop().await;
        assert_eq!(proc.state(), ProcessState::Stopped);

        proc.config_mut().command = "/nonexistent/binary".to_string();
        assert!(proc.spawn().is_err());
        assert_eq!(
            proc.state(),
            ProcessState::Failed,
            "Stopped -> Starting -> Failed is the spawn-failure path"
        );
    }

    #[tokio::test]
    async fn test_spawn_with_env() {
        let (cmd, args) = test_helpers::exit_env_cmd("MY_EXIT_CODE");
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.env.insert("MY_EXIT_CODE".to_string(), "42".to_string());

        let mut proc =
            ManagedProcess::new_config("env-test".into(), test_helpers::test_uuid(), cfg);
        proc.spawn().unwrap();
        let status = proc.wait().await.unwrap();
        assert_eq!(status.code(), Some(42));
    }

    #[tokio::test]
    async fn test_spawn_with_args() {
        let (cmd, args) = test_helpers::exit_cmd(7);
        let mut proc = ManagedProcess::new_config(
            "args-test".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.spawn().unwrap();
        let status = proc.wait().await.unwrap();
        assert_eq!(status.code(), Some(7));
    }

    #[tokio::test]
    async fn test_spawn_refreshes_intended_user_before_running() {
        use crate::spawn::spawn_user_for;

        let (cmd, args) = test_helpers::true_cmd();
        let mut proc = ManagedProcess::new_config(
            "spawn-user-refresh".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        let expected = spawn_user_for(proc.name(), proc.profile());
        proc.spawn().unwrap();
        assert_eq!(proc.user(), expected);
        assert!(proc.is_running());
        let _ = proc.wait().await;
    }

    // -- signal tests (Unix-only: test the raw send_signal API) --

    #[cfg(unix)]
    #[tokio::test]
    async fn test_send_signal_sigterm() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "sig-test".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.spawn().unwrap();

        proc.send_signal(Signal::SIGTERM);
        let status = proc.wait().await.unwrap();
        assert!(!status.success());
    }

    #[cfg(unix)]
    #[test]
    fn test_send_signal_no_child_does_not_panic() {
        let (cmd, args) = test_helpers::true_cmd();
        let proc = ManagedProcess::new_config(
            "no-child".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.send_signal(Signal::SIGTERM);
    }

    #[tokio::test]
    async fn test_spawn_does_not_inherit_parent_env() {
        // SAFETY: single-threaded test runtime; no concurrent env access.
        unsafe { std::env::set_var("PROCMGRD_TEST_SECRET", "leaked") };
        let (sh, flag) = test_helpers::shell_cmd();
        #[cfg(unix)]
        let script = "test -z \"$PROCMGRD_TEST_SECRET\" && exit 0 || exit 1";
        #[cfg(windows)]
        let script = "if defined PROCMGRD_TEST_SECRET (exit 1) else (exit 0)";
        let cfg = test_helpers::make_config(sh, vec![flag.into(), script.into()]);
        let mut proc =
            ManagedProcess::new_config("clean-env".into(), test_helpers::test_uuid(), cfg);
        proc.spawn().unwrap();
        let status = proc.wait().await.unwrap();
        assert_eq!(
            status.code(),
            Some(0),
            "child should NOT see PROCMGRD_TEST_SECRET"
        );
        unsafe { std::env::remove_var("PROCMGRD_TEST_SECRET") };
    }

    #[tokio::test]
    async fn test_spawn_with_environment_file() {
        let dir = tempfile::tempdir().unwrap();
        let env_file = dir.path().join("env");
        std::fs::write(&env_file, "# comment\nFROM_FILE=hello\nPATH=/usr/bin\n\n").unwrap();

        let (sh, flag) = test_helpers::shell_cmd();
        #[cfg(unix)]
        let script = "test \"$FROM_FILE\" = 'hello' && echo $PATH";
        #[cfg(windows)]
        let script = "if \"%FROM_FILE%\"==\"hello\" (echo %PATH%) else (exit 1)";
        let mut cfg = test_helpers::make_config(sh, vec![flag.into(), script.into()]);
        cfg.environment_file = Some(env_file.to_str().unwrap().to_string());

        let mut proc = ManagedProcess::new_config("envfile".into(), test_helpers::test_uuid(), cfg);
        proc.spawn().unwrap();
        let status = proc.wait().await.unwrap();
        assert_eq!(
            status.code(),
            Some(0),
            "child should see vars from env file"
        );
    }

    #[tokio::test]
    async fn test_env_overrides_environment_file() {
        let dir = tempfile::tempdir().unwrap();
        let env_file = dir.path().join("env");
        std::fs::write(&env_file, "MY_VAR=from_file\n").unwrap();

        let (sh, flag) = test_helpers::shell_cmd();
        #[cfg(unix)]
        let script = "exit $(test \"$MY_VAR\" = 'overridden' && echo 0 || echo 1)";
        #[cfg(windows)]
        let script = "if \"%MY_VAR%\"==\"overridden\" (exit 0) else (exit 1)";
        let mut cfg = test_helpers::make_config(sh, vec![flag.into(), script.into()]);
        cfg.environment_file = Some(env_file.to_str().unwrap().to_string());
        cfg.env
            .insert("MY_VAR".to_string(), "overridden".to_string());

        let mut proc =
            ManagedProcess::new_config("override".into(), test_helpers::test_uuid(), cfg);
        proc.spawn().unwrap();
        let status = proc.wait().await.unwrap();
        assert_eq!(
            status.code(),
            Some(0),
            "config env should override environment_file"
        );
    }

    #[tokio::test]
    async fn test_spawn_fails_on_missing_environment_file() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.environment_file = Some("/nonexistent/env".to_string());
        let mut proc =
            ManagedProcess::new_config("bad-envfile".into(), test_helpers::test_uuid(), cfg);
        assert!(
            proc.spawn().is_err(),
            "spawn should fail if environment_file is missing without - prefix"
        );
        assert!(!proc.is_running());
    }

    #[tokio::test]
    async fn test_spawn_skips_missing_optional_environment_file() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.environment_file = Some("-/nonexistent/env".to_string());
        let mut proc =
            ManagedProcess::new_config("optional-envfile".into(), test_helpers::test_uuid(), cfg);
        proc.spawn().unwrap();
        let status = proc.wait().await.unwrap();
        assert!(
            status.success(),
            "spawn should succeed when optional environment_file (- prefix) is missing"
        );
    }

    #[test]
    fn test_should_restart_never() {
        let (cmd, args) = test_helpers::true_cmd();
        let proc = ManagedProcess::new_config(
            "t".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        assert!(!proc.should_restart(&test_helpers::exit_status(1)));
    }

    #[test]
    fn test_should_restart_always_on_success() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        let proc = ManagedProcess::new_config("t".into(), test_helpers::test_uuid(), cfg);
        assert!(proc.should_restart(&test_helpers::exit_status(0)));
    }

    #[test]
    fn test_should_restart_always_on_failure() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        let proc = ManagedProcess::new_config("t".into(), test_helpers::test_uuid(), cfg);
        assert!(proc.should_restart(&test_helpers::exit_status(1)));
    }

    #[test]
    fn test_should_restart_on_failure_with_failure() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::OnFailure;
        let proc = ManagedProcess::new_config("t".into(), test_helpers::test_uuid(), cfg);
        assert!(proc.should_restart(&test_helpers::exit_status(1)));
    }

    #[test]
    fn test_should_restart_on_failure_with_success() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::OnFailure;
        let proc = ManagedProcess::new_config("t".into(), test_helpers::test_uuid(), cfg);
        assert!(!proc.should_restart(&test_helpers::exit_status(0)));
    }

    #[test]
    fn test_should_restart_on_success_with_success() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::OnSuccess;
        let proc = ManagedProcess::new_config("t".into(), test_helpers::test_uuid(), cfg);
        assert!(proc.should_restart(&test_helpers::exit_status(0)));
    }

    #[test]
    fn test_should_restart_on_success_with_failure() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::OnSuccess;
        let proc = ManagedProcess::new_config("t".into(), test_helpers::test_uuid(), cfg);
        assert!(!proc.should_restart(&test_helpers::exit_status(1)));
    }

    #[test]
    fn test_burst_limiting() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.start_limit_burst = Some(3);
        cfg.start_limit_interval_sec = Some(60);
        let mut proc = ManagedProcess::new_config("burst".into(), test_helpers::test_uuid(), cfg);

        let burst = proc.config.burst_limit();
        let interval = proc.config.burst_interval();

        assert!(!proc.restarts.is_burst_limited(burst, interval));
        proc.restarts
            .record(proc.config.restart_delay(), proc.config.runtime_success());
        assert!(!proc.restarts.is_burst_limited(burst, interval));
        proc.restarts
            .record(proc.config.restart_delay(), proc.config.runtime_success());
        assert!(!proc.restarts.is_burst_limited(burst, interval));
        proc.restarts
            .record(proc.config.restart_delay(), proc.config.runtime_success());
        assert!(
            proc.restarts.is_burst_limited(burst, interval),
            "should be limited after 3 restarts"
        );
    }

    #[test]
    fn test_backoff_increases() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.restart_sec = Some(1.0);
        cfg.restart_max_delay_sec = Some(10.0);
        let mut proc = ManagedProcess::new_config("backoff".into(), test_helpers::test_uuid(), cfg);

        assert!((proc.restarts.current_delay - 1.0).abs() < 0.001);
        proc.restarts.advance_backoff(10.0);
        assert!((proc.restarts.current_delay - 2.0).abs() < 0.001);
        proc.restarts.advance_backoff(10.0);
        assert!((proc.restarts.current_delay - 4.0).abs() < 0.001);
        proc.restarts.advance_backoff(10.0);
        assert!((proc.restarts.current_delay - 8.0).abs() < 0.001);
        proc.restarts.advance_backoff(10.0);
        assert!(
            (proc.restarts.current_delay - 10.0).abs() < 0.001,
            "should cap at max_delay"
        );
    }

    #[test]
    fn test_backoff_resets_on_long_runtime() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.restart_sec = Some(1.0);
        cfg.runtime_success_sec = Some(0);
        let mut proc = ManagedProcess::new_config("reset".into(), test_helpers::test_uuid(), cfg);

        proc.restarts.last_spawn_time = Some(Instant::now() - Duration::from_secs(5));
        proc.restarts.current_delay = 16.0;
        proc.restarts.count = 5;

        assert!(
            proc.restarts
                .record(proc.config.restart_delay(), proc.config.runtime_success()),
            "record should report runtime success"
        );
        assert!(
            (proc.restarts.current_delay - 1.0).abs() < 0.001,
            "delay should reset after long runtime"
        );
        assert_eq!(proc.restarts.count, 1, "counter should reset to 1");
        assert!(
            proc.restarts.last_spawn_time.is_none(),
            "successful-run timestamp should be consumed after one reset"
        );
    }

    #[test]
    fn test_successful_run_persists_across_restart_delays() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.restart_sec = Some(0.0);
        cfg.runtime_success_sec = Some(0);
        let mut proc =
            ManagedProcess::new_config("retry-chain".into(), test_helpers::test_uuid(), cfg);

        proc.restarts.last_spawn_time = Some(Instant::now() - Duration::from_secs(5));
        proc.transition_to(ProcessState::Starting);
        proc.transition_to(ProcessState::Failed);

        assert!(proc.schedule_restart().is_some());
        assert!(proc.schedule_restart().is_some());
        assert!(proc.schedule_restart().is_some());
    }

    #[test]
    fn test_short_run_timestamp_consumed_prevents_downtime_false_reset() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.restart_sec = Some(1.0);
        cfg.runtime_success_sec = Some(30);
        let mut proc =
            ManagedProcess::new_config("short-run".into(), test_helpers::test_uuid(), cfg);

        proc.restarts.last_spawn_time = Some(Instant::now() - Duration::from_secs(1));
        proc.restarts.current_delay = 8.0;
        proc.restarts.count = 3;

        assert!(
            !proc
                .restarts
                .record(proc.config.restart_delay(), proc.config.runtime_success()),
            "short run should not count as runtime success"
        );
        assert_eq!(proc.restarts.count, 4);
        assert!(
            (proc.restarts.current_delay - 8.0).abs() < 0.001,
            "backoff should not reset after short run"
        );
        assert!(
            proc.restarts.last_spawn_time.is_none(),
            "spawn timestamp must be consumed on first record"
        );

        proc.restarts.advance_backoff(10.0);
        proc.restarts
            .record(proc.config.restart_delay(), proc.config.runtime_success());
        assert_eq!(
            proc.restarts.count, 5,
            "failed respawn after delay should not reset count"
        );
        assert!(
            (proc.restarts.current_delay - 10.0).abs() < 0.001,
            "failed respawn after delay should preserve advanced backoff"
        );
    }

    #[test]
    fn test_runtime_success_reset_applies_once_per_successful_run() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.restart_sec = Some(1.0);
        cfg.runtime_success_sec = Some(0);
        let mut proc =
            ManagedProcess::new_config("reset-once".into(), test_helpers::test_uuid(), cfg);

        proc.restarts.last_spawn_time = Some(Instant::now() - Duration::from_secs(5));
        proc.restarts.current_delay = 16.0;
        proc.restarts.count = 5;

        proc.restarts
            .record(proc.config.restart_delay(), proc.config.runtime_success());
        assert!((proc.restarts.current_delay - 1.0).abs() < 0.001);
        assert_eq!(proc.restarts.count, 1);

        proc.restarts.advance_backoff(10.0);
        proc.restarts
            .record(proc.config.restart_delay(), proc.config.runtime_success());
        assert_eq!(
            proc.restarts.count, 2,
            "failed respawn should not reset count"
        );
        assert!(
            (proc.restarts.current_delay - 2.0).abs() < 0.001,
            "failed respawn should preserve advanced backoff"
        );
    }

    #[test]
    fn test_restart_config_defaults() {
        let (cmd, args) = test_helpers::true_cmd();
        let proc = ManagedProcess::new_config(
            "defaults".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        assert_eq!(*proc.restart_policy(), RestartPolicy::Never);
        assert!((proc.restarts.current_delay - 1.0).abs() < 0.001);
        assert_eq!(proc.restarts.count, 0);
    }

    #[test]
    fn test_restart_config_from_yaml() {
        let yaml = r#"
command: /bin/sleep
args: ["60"]
restart: on-failure
restart_sec: 2.5
restart_max_delay_sec: 30
start_limit_burst: 10
start_limit_interval_sec: 120
runtime_success_sec: 5
"#;
        let cfg: ProcessConfig = serde_yaml::from_str(yaml).unwrap();
        assert_eq!(cfg.restart, RestartPolicy::OnFailure);
        assert_eq!(cfg.restart_sec, Some(2.5));
        assert_eq!(cfg.restart_max_delay_sec, Some(30.0));
        assert_eq!(cfg.start_limit_burst, Some(10));
        assert_eq!(cfg.start_limit_interval_sec, Some(120));
        assert_eq!(cfg.runtime_success_sec, Some(5));
    }

    #[tokio::test]
    async fn test_stop_transitions_to_stopped() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.spawn().unwrap();
        assert_eq!(proc.state(), ProcessState::Running);

        proc.stop().await;

        assert_eq!(proc.state(), ProcessState::Stopped);
    }

    #[tokio::test]
    async fn test_stop_start_then_crash_restarts_on_failure() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::OnFailure;
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);
        proc.spawn().unwrap();

        proc.stop().await;

        proc.spawn().unwrap();
        let mut handle = proc.take_handle().unwrap();
        handle.kill().await.expect("kill handle");
        let status = handle.wait().await.unwrap();
        proc.set_last_status(status);

        assert_eq!(proc.state(), ProcessState::Failed);
        assert!(
            proc.schedule_restart().is_some(),
            "on-failure should restart after stop -> start -> external kill"
        );
    }

    #[tokio::test]
    async fn test_stop_skips_restart() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);
        proc.spawn().unwrap();

        proc.stop().await;

        assert_eq!(proc.state(), ProcessState::Stopped);
        assert!(
            proc.schedule_restart().is_none(),
            "stopped process should not restart even with Always policy"
        );
    }

    #[tokio::test]
    async fn test_normal_exit_not_affected_by_stopping() {
        let (cmd, args) = test_helpers::exit_cmd(1);
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.spawn().unwrap();

        let mut handle = proc.take_handle().unwrap();
        let status = handle.wait().await.unwrap();
        proc.set_last_status(status);

        assert_eq!(
            proc.state(),
            ProcessState::Failed,
            "non-zero exit without Stopping state should be Failed"
        );
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn managed_child_spawn_drop_aborts_uncommitted_child() {
        use nix::sys::wait::{WaitPidFlag, WaitStatus, waitpid};
        use nix::unistd::Pid;
        use std::time::Duration;

        let (cmd, args) = test_helpers::sleep_cmd(120);
        let cfg = test_helpers::make_config(cmd, args);
        let spawn =
            crate::platform::spawn_managed_child("drop-abort-test", &cfg).expect("spawn child");
        let pid = spawn.pid().expect("spawned child should have a pid");
        drop(spawn);

        tokio::time::sleep(Duration::from_millis(100)).await;
        match waitpid(Pid::from_raw(pid as i32), Some(WaitPidFlag::WNOHANG)) {
            Ok(WaitStatus::StillAlive) => {
                panic!("drop should abort an unconsumed ManagedChildSpawn (pid={pid})")
            }
            Ok(_) => {}
            Err(nix::errno::Errno::ECHILD) => {}
            Err(e) => panic!("waitpid failed for pid={pid}: {e}"),
        }
    }
}
