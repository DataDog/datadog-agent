// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::{ProcessConfig, RestartPolicy};
use crate::env::expand_env_vars;
use crate::handle::ProcessHandle;
use crate::platform;
use crate::spawn::SpawnProfile;
use crate::state::ProcessState;
use anyhow::{Result, bail};
use log::{info, warn};
use std::collections::VecDeque;
use tokio::sync::mpsc;
use tokio::task::JoinHandle;
use tokio::time::{self, Duration, Instant};

pub(crate) struct ExitEvent {
    pub name: String,
    pub pid: u32,
    pub status: std::process::ExitStatus,
}

#[cfg(test)]
pub(crate) fn test_exit_channel() -> (mpsc::Sender<ExitEvent>, mpsc::Receiver<ExitEvent>) {
    mpsc::channel(8)
}

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
        // An `Instant` is measured from boot, so an interval longer than the current uptime
        // has no representable cutoff. Everything recorded so far is inside that window.
        let recent = match Instant::now().checked_sub(interval) {
            Some(cutoff) => self.timestamps.iter().filter(|t| **t > cutoff).count(),
            None => self.timestamps.len(),
        } as u32;
        recent >= burst
    }

    fn record(&mut self, base_delay: f64, runtime_success: Duration) {
        if let Some(spawn_time) = self.last_spawn_time
            && spawn_time.elapsed() >= runtime_success
        {
            self.current_delay = base_delay;
            self.count = 0;
        }
        self.count += 1;
        self.timestamps.push_back(Instant::now());
        if self.timestamps.len() > Self::MAX_TIMESTAMPS {
            self.timestamps.pop_front();
        }
    }

    fn advance_backoff(&mut self, max_delay: f64) {
        self.current_delay = (self.current_delay * Self::BACKOFF_MULTIPLIER).min(max_delay);
    }

    fn delay(&self) -> Duration {
        Duration::from_secs_f64(self.current_delay)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessOrigin {
    Config,
    Runtime,
}

/// Why a respawn was skipped by a closed start condition, and what recovering
/// it still owes the restart accounting.
///
/// Reload needs the skip *reason*, not the state: `Exited`, `Crashed`,
/// `Failed`, and `Stopped` are also reached by a completed one-shot, a policy
/// mismatch, the burst limit, a failed spawn, and an operator stop, none of
/// which an unrelated reload may restart.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum RestartBlock {
    /// No respawn was skipped for a closed condition.
    None,
    /// Skipped at exit time, before `handle_restart` recorded the restart,
    /// because the gate is checked first so that a closed gate consumes no
    /// burst budget. Recovery owes that recording.
    AccountingOwed,
    /// Skipped once the restart had already been recorded (the backoff
    /// re-check), or for a respawn that is not a restart at all (the reload of
    /// a running process). Recovery must not record anything.
    AlreadyAccounted,
}

/// What it takes to force-kill a child, held apart from [`ManagedProcess`] so a
/// stop can escalate after the manager has released it.
struct ProcessKiller {
    #[cfg(not(windows))]
    pid: Option<u32>,
    #[cfg(windows)]
    job_object: Option<platform::JobObject>,
    #[cfg(windows)]
    identity: Option<crate::handle::RetainedProcessHandle>,
}

impl ProcessKiller {
    fn force_kill(&mut self, name: &str) {
        #[cfg(windows)]
        if let Some(ref job) = self.job_object {
            if let Err(e) = job.terminate() {
                warn!("[{name}] job object terminate failed: {e}");
            } else {
                info!("[{name}] force-terminated workload job");
                self.job_object = None;
                return;
            }
        }
        #[cfg(windows)]
        if let Some(identity) = &self.identity {
            warn!("[{name}] falling back to retained-handle termination");
            if let Err(e) = identity.terminate() { warn!("[{name}] retained-handle termination failed: {e}"); }
        }

        #[cfg(not(windows))]
        if let Some(pid) = self.pid
            && let Err(e) = platform::send_force_kill(pid)
        {
            warn!("[{name}] force kill failed: {e}");
        }
    }

}

/// The waiting half of a stop, self-contained so that it can be awaited without
/// the lock guarding the process it belongs to.
///
/// A stop runs for as long as `stop_timeout`, which read-only RPCs must not
/// queue behind: from the outside, a manager that cannot answer `list` for a
/// minute and a half is indistinguishable from a dead one.
pub(crate) struct StopWait {
    name: String,
    handle: JoinHandle<()>,
    /// How long to let the child exit on its own. `None` means it was never
    /// asked to, so there is nothing for a full `stop_timeout` to wait for.
    timeout: Option<Duration>,
    killer: ProcessKiller,
    #[cfg(windows)]
    helper: Option<platform::SignalingHelper>,
}

impl StopWait {
    pub(crate) async fn run(self) {
        let StopWait {
            name,
            handle,
            timeout,
            mut killer,
            #[cfg(windows)]
            helper,
        } = self;
        tokio::pin!(handle);
        let started = Instant::now();

        // On Linux, request_stop() already knows whether SIGTERM was sent.
        // On Windows, creating the helper does not establish that CTRL_BREAK
        // was generated; we must read the helper's report to find out.
        #[cfg(windows)]
        let timeout = if let Some(mut helper) = helper {
            let delivered = tokio::select! {
                // If both branches are ready, prefer the completed watcher:
                // once the child has exited, the delivery report is irrelevant.
                biased;
                _ = &mut handle => return,
                delivered = platform::wait_for_graceful_stop(
                    &mut helper, &name, timeout.unwrap_or(ManagedProcess::UNDELIVERED_STOP_GRACE),
                ) => delivered,
            };
            // Confirmed generation gets the configured graceful-exit allowance.
            // Failure or an unconfirmed report gets the short allowance below.
            if delivered { timeout } else { None }
        } else {
            timeout
        };

        // Even when signaling fails, allow 2 seconds for an already-exited
        // child's watcher to finish before attempting force termination.
        let grace = timeout.unwrap_or(ManagedProcess::UNDELIVERED_STOP_GRACE);
        if time::timeout(grace.saturating_sub(started.elapsed()), &mut handle).await.is_ok() {
            return;
        }

        match timeout {
            None => warn!("[{name}] graceful stop was not delivered, force-killing"),
            Some(stop) => warn!(
                "[{name}] stop timeout ({}s) reached, force-killing",
                stop.as_secs()
            ),
        }

        killer.force_kill(&name);
        if time::timeout(ManagedProcess::FORCE_KILL_TIMEOUT, &mut handle)
            .await
            .is_err()
        {
            warn!("[{name}] still running after force-kill, giving up");
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
    watcher_handle: Option<JoinHandle<()>>,
    restarts: RestartTracker,
    stop_requested: bool,
    /// Set by `request_stop` when signaling fails immediately. On Windows this
    /// records native creation failure; `StopWait` resolves actual delivery
    /// from the helper's report.
    graceful_stop_failed: bool,
    /// Set only where a respawn was skipped because a start condition was
    /// closed, and cleared in `spawn()`.
    restart_block: RestartBlock,
    origin: ProcessOrigin,
    last_exit_status: Option<std::process::ExitStatus>,
    #[cfg(windows)]
    job_object: Option<platform::JobObject>,
    #[cfg(windows)]
    shutdown_identity: Option<crate::handle::RetainedProcessHandle>,
    #[cfg(windows)]
    user_profile: Option<platform::UserProfileGuard>,
    #[cfg(windows)]
    signal_helper: Option<platform::SignalingHelper>,
    #[cfg(windows)]
    agent_credential: Option<platform::SpawnCredential>,
}

impl ManagedProcess {
    pub(crate) const FORCE_KILL_TIMEOUT: Duration = Duration::from_secs(10);

    /// How long to give the exit watcher when the graceful stop never reached
    /// the child.
    ///
    /// Sending it fails both when the child cannot hear us and when it has
    /// already exited, and the two are indistinguishable from here: on Unix a
    /// child that is gone answers the signal with ESRCH. Waiting briefly settles
    /// that case without force-killing a pid that has already been reaped, and
    /// which by then may belong to something else, while still costing a child
    /// that genuinely cannot hear us almost nothing against a 90s stop_timeout.
    pub(crate) const UNDELIVERED_STOP_GRACE: Duration = Duration::from_secs(2);

    pub fn new_config(name: String, uuid: String, config: ProcessConfig) -> Self {
        Self::new_inner(name, uuid, config, ProcessOrigin::Config)
    }

    pub fn new_runtime(name: String, uuid: String, config: ProcessConfig) -> Self {
        Self::new_inner(name, uuid, config, ProcessOrigin::Runtime)
    }

    fn new_inner(name: String, uuid: String, config: ProcessConfig, origin: ProcessOrigin) -> Self {
        let restarts = RestartTracker::new(config.restart_delay());
        let profile = SpawnProfile::profile_for(&name);
        #[cfg(windows)]
        let (user, agent_credential) = platform::initial_spawn_identity(&name, profile);
        #[cfg(not(windows))]
        let user = platform::initial_spawn_identity(&name, profile);
        Self {
            name,
            uuid,
            config,
            profile,
            user,
            state: ProcessState::Created,
            pid: None,
            watcher_handle: None,
            restarts,
            stop_requested: false,
            graceful_stop_failed: false,
            restart_block: RestartBlock::None,
            origin,
            last_exit_status: None,
            #[cfg(windows)]
            job_object: None,
            #[cfg(windows)]
            shutdown_identity: None,
            #[cfg(windows)]
            user_profile: None,
            #[cfg(windows)]
            signal_helper: None,
            #[cfg(windows)]
            agent_credential,
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
    pub(crate) fn set_job_object(&mut self, job: platform::JobObject) {
        self.job_object = Some(job);
    }

    #[cfg(windows)]
    pub(crate) fn set_user_profile_guard(&mut self, profile: platform::UserProfileGuard) {
        self.user_profile = Some(profile);
    }

    #[cfg(windows)]
    pub(crate) fn clear_windows_spawn_resources(&mut self) {
        self.job_object = None;
        self.shutdown_identity = None;
        self.user_profile = None;
        self.signal_helper = None;
    }

    pub(crate) fn profile(&self) -> SpawnProfile {
        self.profile
    }

    pub fn user(&self) -> &str {
        &self.user
    }

    #[cfg(windows)]
    pub(crate) fn set_intended_user(&mut self, user: String) {
        self.user = user;
    }

    #[cfg(windows)]
    pub(crate) fn agent_credential(&self) -> Option<&platform::SpawnCredential> {
        self.agent_credential.as_ref()
    }

    pub fn restart_count(&self) -> u32 {
        self.restarts.count
    }

    pub fn last_exit_code(&self) -> Option<i32> {
        self.last_exit_status.and_then(|s| s.code())
    }

    pub fn last_signal(&self) -> Option<i32> {
        self.last_exit_status
            .and_then(|s| platform::last_signal(&s))
    }

    pub fn set_config(&mut self, config: ProcessConfig) {
        self.restarts = RestartTracker::new(config.restart_delay());
        self.config = config;
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
        self.state = next;
    }

    /// Put a never-spawned process into `Running` so tests can drive
    /// `set_last_status` with a synthetic exit status. Classification and
    /// restart-policy decisions do not need a live child.
    #[cfg(test)]
    pub(crate) fn force_running_for_test(&mut self) {
        self.transition_to(ProcessState::Starting);
        self.transition_to(ProcessState::Running);
    }

    #[must_use]
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
    fn config_gate_met(&self) -> bool {
        if !crate::config_gate::condition_config_any_met(&self.config.condition_config_any) {
            info!(
                "[{}] condition_config_any not met: {}",
                self.name,
                crate::config_gate::condition_config_summary(&self.config.condition_config_any)
            );
            return false;
        }
        if !crate::config_gate::condition_config_none_met(&self.config.condition_config_none) {
            info!(
                "[{}] condition_config_none vetoed: {}",
                self.name,
                crate::config_gate::condition_config_summary(&self.config.condition_config_none)
            );
            return false;
        }
        true
    }

    #[must_use]
    fn start_conditions_met(&self) -> bool {
        self.condition_path_exists_met() && self.config_gate_met()
    }

    /// Conditions only, deliberately ignoring `auto_start`: a process that was
    /// started once should keep its restart policy even though `auto_start`
    /// only governs boot.
    #[must_use]
    pub(crate) fn may_respawn(&self) -> bool {
        self.start_conditions_met()
    }

    /// Whether any start condition is declared. Reload re-evaluates conditions
    /// only for such processes: one with no conditions that is still `Created`
    /// was never attempted rather than blocked, and starting it on an unrelated
    /// reload would override whatever left it alone.
    #[must_use]
    pub(crate) fn has_start_conditions(&self) -> bool {
        self.config.condition_path_exists.is_some()
            || !self.config.condition_config_any.is_empty()
            || !self.config.condition_config_none.is_empty()
    }

    /// Whether a respawn this process was otherwise due was skipped because a
    /// start condition was closed. Reload restarts exactly these, so it must
    /// only ever be set right after a `may_respawn()` check fails.
    #[must_use]
    pub(crate) fn restart_blocked_by_conditions(&self) -> bool {
        self.restart_block != RestartBlock::None
    }

    /// Records a respawn skipped by a closed condition that needs no further
    /// accounting: the backoff re-check, where `handle_restart` already
    /// recorded the restart, and the reload of a running process, which is not
    /// a restart-policy respawn at all. The exit-time skip is marked inside
    /// `handle_restart`, which is the only case that owes a recording.
    pub(crate) fn mark_restart_blocked_already_accounted(&mut self) {
        self.restart_block = RestartBlock::AlreadyAccounted;
    }

    /// Whether the restart burst window currently holds as many restarts as
    /// `start_limit_burst` allows.
    #[must_use]
    pub(crate) fn restart_burst_exhausted(&self) -> bool {
        self.restarts
            .is_burst_limited(self.config.burst_limit(), self.config.burst_interval())
    }

    /// Whether recovering a condition skip would take a burst slot the limit
    /// has already refused.
    ///
    /// Only an exit-time skip owes the check. The gate is tested before the
    /// limit, so that skip records nothing and can outlive a budget earlier
    /// crashes already spent. The backoff re-check recorded its restart when
    /// the limit admitted it, and the reload of a running process is not a
    /// restart, so neither may be refused for a window that is already full.
    #[must_use]
    pub(crate) fn recovered_restart_exceeds_burst(&self) -> bool {
        self.restart_block == RestartBlock::AccountingOwed && self.restart_burst_exhausted()
    }

    /// Accounts for a restart that a closed gate skipped before it could be
    /// recorded. Call immediately before respawning a recovered process, and
    /// after the burst budget has been checked, since this spends from it.
    ///
    /// The backoff is deliberately not advanced: a recovered process spawns
    /// immediately rather than waiting out a delay, so there is no delay to
    /// grow.
    pub(crate) fn record_recovered_restart(&mut self) {
        if self.restart_block != RestartBlock::AccountingOwed {
            return;
        }
        self.restarts
            .record(self.config.restart_delay(), self.config.runtime_success());
    }

    #[must_use]
    pub fn should_start(&self) -> bool {
        if !self.config.auto_start {
            info!("[{}] auto_start=false, skipping", self.name);
            return false;
        }
        self.start_conditions_met()
    }

    pub(crate) fn spawn(&mut self, exit_tx: mpsc::Sender<ExitEvent>) -> Result<()> {
        if !self.state.can_transition_to(ProcessState::Starting) {
            bail!("[{}] cannot spawn: invalid state {}", self.name, self.state);
        }
        self.stop_requested = false;
        self.graceful_stop_failed = false;
        // The single clear site, which is what keeps the reason from going
        // stale: boot, restart, manual start, and reload all land here.
        self.restart_block = RestartBlock::None;
        self.transition_to(ProcessState::Starting);
        match self.try_spawn() {
            Ok(handle) => {
                self.start_exit_watcher(handle, exit_tx);
                Ok(())
            }
            Err(e) => {
                #[cfg(windows)]
                self.clear_windows_spawn_resources();
                self.transition_to(ProcessState::Failed);
                Err(e)
            }
        }
    }

    fn start_exit_watcher(&mut self, mut handle: ProcessHandle, tx: mpsc::Sender<ExitEvent>) {
        let name = self.name().to_owned();
        let pid = self.pid().unwrap_or(0);
        self.watcher_handle = Some(tokio::spawn(async move {
            let status = match handle.wait().await {
                Ok(status) => status,
                Err(e) => {
                    warn!("[{name}] wait error: {e}, killing process");
                    let _ = handle.kill().await;
                    match handle.wait().await {
                        Ok(s) => s,
                        Err(e2) => {
                            warn!("[{name}] failed to reap after kill: {e2}");
                            return;
                        }
                    }
                }
            };
            let _ = tx.try_send(ExitEvent { name, pid, status });
        }));
    }

    fn try_spawn(&mut self) -> Result<ProcessHandle> {
        let handle = self.spawn_child_handle()?;
        #[cfg(windows)]
        {
            // Retention must succeed before the watcher owns the child. Dropping
            // the job on failure kills the entire newly created workload.
            self.shutdown_identity = Some(handle.retain_for_shutdown().inspect_err(|_| {
                if let Some(job) = &self.job_object {
                    let _ = job.terminate();
                }
            })?);
        }

        self.pid = handle.id();
        info!(
            "[{}] spawned (pid={}, cmd={})",
            self.name,
            self.pid.map_or("unknown".to_string(), |p| p.to_string()),
            self.config.command
        );

        self.transition_to(ProcessState::Running);
        self.restarts.mark_spawned();
        Ok(handle)
    }

    pub fn is_running(&self) -> bool {
        self.state.is_alive()
    }

    pub fn set_last_status(&mut self, status: std::process::ExitStatus) {
        self.last_exit_status = Some(status);
        self.pid = None;
        self.watcher_handle = None;
        // Main-process exit is permission to close the whole job. A detached
        // StopWait owns any resources it still needs until its watcher completes.
        #[cfg(windows)]
        self.clear_windows_spawn_resources();
        // Order matters, first match wins. `stop_requested` stays first because
        // procmgr's own force kill sends SIGKILL on Unix and
        // `TerminateProcess(h, 1)` on Windows, either of which the branches
        // below would otherwise read as a crash or a failure.
        if self.stop_requested {
            self.stop_requested = false;
            self.transition_to(ProcessState::Stopped);
        } else if status.success() {
            self.transition_to(ProcessState::Exited);
        } else if platform::is_crash_exit(&status) {
            // Died without returning a value: a signal on Unix, a fatal
            // exception code on Windows. Different operator response from
            // `Failed`, where the child diagnosed its own problem and reported
            // it through an exit code.
            self.transition_to(ProcessState::Crashed);
        } else {
            self.transition_to(ProcessState::Failed);
        }
    }

    pub fn request_stop(&mut self) {
        if !self.mark_stop_requested() {
            return;
        }
        info!("[{}] sending graceful stop (stop requested)", self.name);
        self.graceful_stop_failed = !self.graceful_stop();
    }

    /// Record that a stop is under way, without signalling the child. Returns
    /// false when there is nothing running to stop or a stop is already under way.
    fn mark_stop_requested(&mut self) -> bool {
        if !self.is_running() || self.stop_requested {
            return false;
        }
        self.stop_requested = true;
        // The manager lock is free while the child goes down, so this state is
        // what `list`, `describe`, and `status` report for as long as the stop
        // runs. Reporting `Running` throughout is how a stop that takes its
        // time reads from the outside as a process that ignored one.
        if matches!(self.state, ProcessState::Running) {
            self.transition_to(ProcessState::Stopping);
        }
        true
    }

    /// Ask the child to exit. Returns whether the request reached it.
    #[cfg(not(windows))]
    fn graceful_stop(&self) -> bool {
        let Some(pid) = self.pid else {
            warn!("[{}] no pid to send a graceful stop to", self.name);
            return false;
        };
        if let Err(e) = platform::send_graceful_stop(pid) {
            warn!("[{}] graceful stop failed: {e}", self.name);
            return false;
        }
        true
    }

    /// Ask the child to exit. Returns whether the helper was created,
    /// not whether the console signal was delivered. `StopWait` reads the report.
    #[cfg(windows)]
    fn graceful_stop(&mut self) -> bool {
        let (Some(target), Some(job)) = (&self.shutdown_identity, &self.job_object) else {
            warn!("[{}] cannot launch signaling helper: missing retained identity or job", self.name);
            return false;
        };
        match platform::send_graceful_stop(target, job) {
            Ok(helper) => {
                self.signal_helper = Some(helper);
                true
            }
            Err(error) => {
                warn!("[{}] graceful stop launch failed: {error:#}", self.name);
                false
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

    fn stop_timeout(&self) -> Duration {
        self.config.stop_timeout()
    }

    pub async fn wait_for_stop(&mut self) {
        if !self.is_running() {
            return;
        }
        if let Some(wait) = self.take_stop_wait() {
            wait.run().await;
        }
        self.finish_stop();
    }

    /// Detach the part of a stop that only waits, so a caller holding a shared
    /// lock can release it first. `finish_stop` must follow once the returned
    /// [`StopWait`] has run. Returns `None` when there is nothing to wait for.
    pub(crate) fn take_stop_wait(&mut self) -> Option<StopWait> {
        if !self.is_running() {
            return None;
        }
        let handle = self.watcher_handle.take()?;
        Some(StopWait {
            name: self.name.clone(),
            handle,
            // A stop that never reached the child leaves nothing to wait for:
            // the timeout would pass with the process untouched.
            timeout: (!self.graceful_stop_failed).then(|| self.stop_timeout()),
            killer: ProcessKiller {
                #[cfg(not(windows))]
                pid: self.pid,
                #[cfg(windows)]
                job_object: self.job_object.take(),
                #[cfg(windows)]
                identity: self.shutdown_identity.take(),
            },
            #[cfg(windows)]
            helper: self.signal_helper.take(),
        })
    }

    /// Record the stop that [`StopWait::run`] carried out. A process that is no
    /// longer alive was never this stop's to mark.
    pub(crate) fn finish_stop(&mut self) {
        if !self.is_running() {
            return;
        }
        self.mark_stopped();
    }

    fn mark_stopped(&mut self) {
        self.stop_requested = false;
        self.transition_to(ProcessState::Stopped);
        self.pid = None;
        #[cfg(windows)]
        self.clear_windows_spawn_resources();
    }

    #[cfg(test)]
    pub fn should_restart(&self, status: &std::process::ExitStatus) -> bool {
        match self.config.restart {
            RestartPolicy::Never => false,
            RestartPolicy::Always => true,
            RestartPolicy::OnFailure => !status.success(),
            RestartPolicy::OnSuccess => status.success(),
        }
    }

    #[cfg(test)]
    pub fn restart_policy(&self) -> &RestartPolicy {
        &self.config.restart
    }

    #[must_use]
    pub fn handle_restart(&mut self) -> Option<Duration> {
        // `Crashed` is restartable exactly like `Failed`: both are unsuccessful
        // exits, and treating them differently would silently stop supervising
        // a segfaulting child under `restart: always`.
        let should_restart = match (self.state, &self.config.restart) {
            (
                ProcessState::Exited | ProcessState::Crashed | ProcessState::Failed,
                RestartPolicy::Always,
            ) => true,
            (ProcessState::Crashed | ProcessState::Failed, RestartPolicy::OnFailure) => true,
            (ProcessState::Exited, RestartPolicy::OnSuccess) => true,
            (ProcessState::Exited | ProcessState::Crashed | ProcessState::Failed, _) => false,
            _ => return None,
        };

        if !should_restart {
            if self.config.restart != RestartPolicy::Never {
                info!(
                    "[{}] exit does not match restart policy, not restarting",
                    self.name
                );
            }
            return None;
        }

        // Checked before the burst limit so a closed gate neither consumes
        // burst budget nor advances the backoff.
        if !self.may_respawn() {
            info!("[{}] start conditions not met, not restarting", self.name);
            self.restart_block = RestartBlock::AccountingOwed;
            return None;
        }

        if self
            .restarts
            .is_burst_limited(self.config.burst_limit(), self.config.burst_interval())
        {
            warn!("[{}] start limit reached, not restarting", self.name);
            return None;
        }

        self.restarts
            .record(self.config.restart_delay(), self.config.runtime_success());
        let delay = self.restarts.delay();
        info!(
            "[{}] restart #{} in {:.1}s",
            self.name,
            self.restarts.count,
            delay.as_secs_f64()
        );
        self.restarts
            .advance_backoff(self.config.max_restart_delay());
        Some(delay)
    }
}

#[cfg(test)]
pub mod tests {
    use super::*;
    use crate::config::ProcessConfig;
    use crate::test_helpers;
    #[cfg(unix)]
    use nix::sys::signal::Signal;

    fn spawn_ok(proc: &mut ManagedProcess) -> mpsc::Receiver<ExitEvent> {
        let (tx, rx) = test_exit_channel();
        proc.spawn(tx).unwrap();
        rx
    }

    #[cfg(windows)]
    fn injected_stop(job: platform::JobObject, outcome: platform::SignalOutcome, timeout: Duration) -> StopWait {
        let faults = job.faults.clone();
        let handle = tokio::spawn(async move {
            while faults.terminate_calls.load(std::sync::atomic::Ordering::SeqCst) == 0 {
                time::sleep(Duration::from_millis(5)).await;
            }
        });
        StopWait { name: "injected-stop".into(), handle, timeout: Some(timeout),
            killer: ProcessKiller { job_object: Some(job), identity: None },
            helper: Some(platform::SignalingHelper::test_report(outcome)) }
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn normal_exit_closes_job_and_kills_remaining_descendants() {
        use crate::handle::OwnedProcessHandle;
        use std::sync::Arc;
        use windows_sys::Win32::System::JobObjects::{
            JobObjectBasicProcessIdList, QueryInformationJobObject,
        };
        use windows_sys::Win32::System::Threading::{
            OpenProcess, PROCESS_QUERY_LIMITED_INFORMATION, PROCESS_SYNCHRONIZE,
        };

        let mut config = test_helpers::make_config("cmd.exe", vec![
            "/C".into(),
            "ping -n 3 127.0.0.1 >nul & start /b ping -n 301 127.0.0.1 >nul".into(),
        ]);
        config.restart = RestartPolicy::Never;
        let mut proc = ManagedProcess::new_config(
            "normal-exit".into(), test_helpers::test_uuid(), config,
        );
        let mut exit_rx = spawn_ok(&mut proc);
        let event = time::timeout(Duration::from_secs(10), exit_rx.recv())
            .await.unwrap().expect("main-process exit event");
        assert!(event.status.success());

        // Inspect the still-open job only in the test. Holding process handles
        // does not keep the job open, so its final close must kill these members.
        #[repr(C)]
        struct ProcessIds {
            assigned: u32,
            count: u32,
            ids: [usize; 64],
        }
        let job = proc.job_object.as_ref().unwrap();
        let weak_faults = Arc::downgrade(&job.faults);
        let mut ids: ProcessIds = unsafe { std::mem::zeroed() };
        assert_ne!(unsafe {
            QueryInformationJobObject(
                job.raw_handle(), JobObjectBasicProcessIdList,
                (&mut ids as *mut ProcessIds).cast(), std::mem::size_of_val(&ids) as u32,
                std::ptr::null_mut(),
            )
        }, 0);
        assert!(ids.count > 0 && ids.count as usize <= ids.ids.len(), "expected surviving descendants");
        let descendants: Vec<_> = ids.ids[..ids.count as usize].iter()
            .filter(|&&pid| pid as u32 != event.pid)
            .map(|&pid| {
                let handle = unsafe { OpenProcess(PROCESS_SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION, 0, pid as u32) };
                assert!(!handle.is_null());
                let handle = unsafe { OwnedProcessHandle::from_raw(handle) };
                assert!(matches!(platform::wait_for_process_exit_ms(handle.get(), 0).unwrap(),
                    platform::ProcessWaitOutcome::TimedOut));
                handle
            }).collect();
        assert!(!descendants.is_empty());

        proc.set_last_status(event.status);
        assert_eq!(proc.state(), ProcessState::Exited);
        assert!(proc.pid().is_none());
        assert!(proc.job_object.is_none());
        assert!(proc.shutdown_identity.is_none());
        assert!(weak_faults.upgrade().is_none(), "normal exit must drop the owned job");
        assert!(proc.handle_restart().is_none());
        let deadline = Instant::now() + Duration::from_secs(5);
        for descendant in descendants {
            while matches!(platform::wait_for_process_exit_ms(descendant.get(), 0).unwrap(),
                platform::ProcessWaitOutcome::TimedOut)
            {
                assert!(Instant::now() < deadline, "job close did not terminate descendant");
                time::sleep(Duration::from_millis(10)).await;
            }
        }
    }

    #[cfg(windows)]
    #[test]
    fn normal_exit_closes_job_even_when_accounting_is_unavailable() {
        use std::sync::{Arc, atomic::Ordering};
        let mut proc = ManagedProcess::new_config(
            "normal-exit-query-failure".into(), test_helpers::test_uuid(),
            test_helpers::graceful_stop_test_config(),
        );
        proc.force_running_for_test();
        proc.set_job_object(platform::JobObject::new().unwrap());
        let job = proc.job_object.as_ref().unwrap();
        job.faults.query_error.store(true, Ordering::SeqCst);
        let weak_faults = Arc::downgrade(&job.faults);
        proc.set_last_status(test_helpers::exit_status(0));
        assert_eq!(proc.state(), ProcessState::Exited);
        assert!(proc.job_object.is_none());
        assert!(weak_faults.upgrade().is_none());
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn graceful_stop_creates_helper_before_wait_stage() {
        let mut proc = ManagedProcess::new_config(
            "schedule-graceful-stop".into(), test_helpers::test_uuid(),
            test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS),
        );
        let _exit_rx = spawn_ok(&mut proc);
        let timeout = proc.stop_timeout();
        proc.request_stop();
        assert_eq!(proc.state(), ProcessState::Stopping);
        assert!(proc.signal_helper.is_some(), "graceful stop must create the helper synchronously");
        assert!(!proc.graceful_stop_failed, "successful creation is not a delivery result");
        let wait = proc.take_stop_wait().unwrap();
        assert_eq!(wait.timeout, Some(timeout));
        assert!(proc.signal_helper.is_none(), "the waiting stage owns the helper report");
        wait.run().await;
        proc.finish_stop();
        assert_eq!(proc.state(), ProcessState::Stopped);
    }

    #[test]
    fn repeated_stop_request_preserves_original_delivery_result() {
        for failed in [false, true] {
            let mut proc = ManagedProcess::new_config(
                "repeated-stop".into(), test_helpers::test_uuid(),
                test_helpers::graceful_stop_test_config(),
            );
            proc.force_running_for_test();
            assert!(proc.mark_stop_requested());
            proc.graceful_stop_failed = failed;
            assert!(!proc.mark_stop_requested(), "a second request must be rejected on every platform");
            // With no PID/Windows resources, retrying would overwrite a
            // successful first delivery with failure instead of preserving it.
            proc.request_stop();
            assert_eq!(proc.graceful_stop_failed, failed);
            assert_eq!(proc.state(), ProcessState::Stopping);
        }
    }

    #[cfg(windows)]
    #[test]
    fn graceful_stop_rejects_missing_resources() {
        let mut proc = ManagedProcess::new_config(
            "missing-graceful-stop-resources".into(), test_helpers::test_uuid(),
            test_helpers::graceful_stop_test_config(),
        );
        assert!(!proc.graceful_stop());
        assert!(proc.signal_helper.is_none());
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn shared_wait_generated_report_uses_configured_timeout_not_job_accounting() {
        use std::sync::{Arc, atomic::Ordering};
        let job = platform::JobObject::new().unwrap();
        let faults = Arc::clone(&job.faults);
        faults.query_error.store(true, Ordering::SeqCst);
        let grace = Duration::from_millis(30);
        let stop = injected_stop(job, platform::SignalOutcome::Generated, grace);
        // An earlier workload may be awaited first. This wait still receives
        // its existing relative allowance when its own waiting pass starts.
        time::sleep(Duration::from_millis(50)).await;
        let started = Instant::now();
        stop.run().await;
        assert!(started.elapsed() >= grace);
        assert_eq!(faults.terminate_calls.load(Ordering::SeqCst), 1);
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn shared_wait_delivery_failure_uses_existing_undelivered_allowance() {
        use std::sync::{Arc, atomic::Ordering};
        for outcome in [platform::SignalOutcome::Failed, platform::SignalOutcome::AlreadyExited] {
            let job = platform::JobObject::new().unwrap();
            let faults = Arc::clone(&job.faults);
            let stop = injected_stop(job, outcome, Duration::from_millis(30));
            let started = Instant::now();
            stop.run().await;
            assert!(started.elapsed() >= ManagedProcess::UNDELIVERED_STOP_GRACE);
            assert_eq!(faults.terminate_calls.load(Ordering::SeqCst), 1);
        }
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn shared_wait_main_exit_does_not_wait_for_helper_or_job_drainage() {
        use std::sync::{Arc, atomic::Ordering};
        let job = platform::JobObject::new().unwrap();
        let faults = Arc::clone(&job.faults);
        faults.query_error.store(true, Ordering::SeqCst);
        let stop = StopWait {
            name: "main-exit".into(), handle: tokio::spawn(async {}),
            timeout: Some(Duration::from_secs(90)),
            killer: ProcessKiller { job_object: Some(job), identity: None },
            helper: Some(platform::SignalingHelper::test_unreported()),
        };
        time::timeout(Duration::from_secs(1), stop.run()).await.unwrap();
        assert_eq!(faults.terminate_calls.load(Ordering::SeqCst), 0,
            "main exit must rely on kill-on-close without explicit job termination");
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn main_exit_during_stop_keeps_existing_exit_classification() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut proc = ManagedProcess::new_config("main-exit-stop".into(),
            test_helpers::test_uuid(), test_helpers::make_config(cmd, args));
        let mut exit_rx = spawn_ok(&mut proc);
        proc.request_stop();
        let wait = proc.take_stop_wait().unwrap();
        let event = time::timeout(Duration::from_secs(5), exit_rx.recv()).await.unwrap().unwrap();
        proc.set_last_status(event.status);
        assert_eq!(proc.state(), ProcessState::Stopped);
        assert!(proc.pid().is_none());
        assert!(proc.handle_restart().is_none());
        wait.run().await;
        proc.finish_stop();
        assert!(proc.job_object.is_none());
    }

    #[tokio::test]
    async fn shared_wait_captures_old_timeout_before_reload_updates_config() {
        let mut proc = ManagedProcess::new_config("reload-timeout".into(),
            test_helpers::test_uuid(), test_helpers::graceful_stop_test_config());
        proc.force_running_for_test();
        proc.watcher_handle = Some(tokio::spawn(async {}));
        assert!(proc.mark_stop_requested());
        let wait = proc.take_stop_wait().unwrap();
        let old_timeout = wait.timeout;
        let mut replacement = proc.config().clone();
        replacement.stop_timeout = Some(90);
        proc.set_config(replacement);
        assert_eq!(old_timeout, Some(Duration::from_secs(5)));
        assert_eq!(wait.timeout, old_timeout);
        wait.run().await;
        proc.finish_stop();
        assert_eq!(proc.state(), ProcessState::Stopped);
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

        let mut exit_rx = spawn_ok(&mut proc);
        assert_eq!(proc.state(), ProcessState::Running);
        assert!(proc.is_running());

        let status = exit_rx.recv().await.expect("exit event").status;
        assert!(status.success());
        proc.set_last_status(status);
        assert_eq!(proc.state(), ProcessState::Exited);
        assert!(!proc.is_running());
    }

    /// `cleanup_process` force-kills, which is SIGKILL on Unix but
    /// `TerminateProcess(h, 1)` on Windows. Only the Unix form is a death
    /// without a returned value; Windows cannot tell the killer's exit code
    /// from the child's own.
    #[cfg(unix)]
    const EXTERNAL_KILL_STATE: ProcessState = ProcessState::Crashed;
    #[cfg(windows)]
    const EXTERNAL_KILL_STATE: ProcessState = ProcessState::Failed;

    /// Drive `set_last_status` from a synthetic `Running` process. No child is
    /// spawned: classification only reads the exit status and `stop_requested`.
    fn state_after_exit(status: std::process::ExitStatus, stop_requested: bool) -> ProcessState {
        let (cmd, args) = test_helpers::true_cmd();
        let mut proc = ManagedProcess::new_config(
            "classify".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        proc.force_running_for_test();
        if stop_requested {
            proc.request_stop();
        }
        proc.set_last_status(status);
        proc.state()
    }

    #[test]
    fn test_classify_crash_exit_as_crashed() {
        assert_eq!(
            state_after_exit(test_helpers::crash_exit_status(), false),
            ProcessState::Crashed,
            "a death without a returned value is a crash, not a failure"
        );
    }

    /// SIGKILL is the OOM-killer case, which is the one `Crashed` exists for.
    #[cfg(unix)]
    #[test]
    fn test_classify_sigkill_as_crashed() {
        let status = test_helpers::signal_exit_status(Signal::SIGKILL as i32);
        assert_eq!(state_after_exit(status, false), ProcessState::Crashed);
    }

    #[test]
    fn test_classify_nonzero_exit_as_failed() {
        assert_eq!(
            state_after_exit(test_helpers::exit_status(1), false),
            ProcessState::Failed,
            "the child returned a value, so it did not crash"
        );
    }

    #[test]
    fn test_classify_zero_exit_as_exited() {
        assert_eq!(
            state_after_exit(test_helpers::exit_status(0), false),
            ProcessState::Exited
        );
    }

    /// procmgr's own force kill is a SIGKILL on Unix, so the stop branch has to
    /// win or every forced stop would be reported as a crash.
    #[test]
    fn test_stop_request_wins_over_crash_exit() {
        assert_eq!(
            state_after_exit(test_helpers::crash_exit_status(), true),
            ProcessState::Stopped
        );
    }

    /// A crash is an unsuccessful exit, so it restarts exactly where a failure
    /// does. Getting this wrong leaves a segfaulting child unsupervised under
    /// `restart: always`.
    #[test]
    fn test_restart_policies_treat_crash_like_failure() {
        for (policy, expected) in [
            (RestartPolicy::Always, true),
            (RestartPolicy::OnFailure, true),
            (RestartPolicy::OnSuccess, false),
            (RestartPolicy::Never, false),
        ] {
            let (cmd, args) = test_helpers::true_cmd();
            let mut cfg = test_helpers::make_config(cmd, args);
            cfg.restart = policy.clone();
            let mut proc =
                ManagedProcess::new_config("crashy".into(), test_helpers::test_uuid(), cfg);
            proc.force_running_for_test();
            proc.set_last_status(test_helpers::crash_exit_status());

            assert_eq!(proc.state(), ProcessState::Crashed);
            assert_eq!(
                proc.handle_restart().is_some(),
                expected,
                "restart decision for a crash under {policy:?}"
            );
        }
    }

    #[tokio::test]
    async fn test_state_transitions_spawn_exit_failure() {
        let (cmd, args) = test_helpers::exit_cmd(1);
        let mut proc = ManagedProcess::new_config(
            "t".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        let mut exit_rx = spawn_ok(&mut proc);
        assert_eq!(proc.state(), ProcessState::Running);

        let status = exit_rx.recv().await.expect("exit event").status;
        assert!(!status.success());
        proc.set_last_status(status);
        assert_eq!(proc.state(), ProcessState::Failed);
    }

    #[tokio::test]
    async fn test_state_after_spawn_watcher_owns_handle() {
        let mut proc = ManagedProcess::new_config(
            "t".into(),
            test_helpers::test_uuid(),
            test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS),
        );
        let mut exit_rx = spawn_ok(&mut proc);
        assert_eq!(proc.state(), ProcessState::Running);
        assert!(proc.is_running());
        assert!(proc.pid().is_some());

        if let Some(pid) = proc.pid() {
            test_helpers::cleanup_process(pid);
        }
        let _ = tokio::time::timeout(std::time::Duration::from_secs(5), exit_rx.recv())
            .await
            .expect("timed out waiting for watcher after external kill");
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn test_send_signal_works_after_spawn() {
        let mut proc = ManagedProcess::new_config(
            "t".into(),
            test_helpers::test_uuid(),
            test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS),
        );
        let mut exit_rx = spawn_ok(&mut proc);

        proc.send_signal(Signal::SIGTERM);
        let status = exit_rx.recv().await.expect("exit event").status;
        assert!(
            !status.success(),
            "signal by stored PID should reach child after spawn (watcher owns the handle)"
        );
    }

    #[test]
    fn test_should_start_auto_start_true_no_condition() {
        let (cmd, args) = test_helpers::true_cmd();
        let proc = ManagedProcess::new_config(
            "test".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        assert!(proc.should_start());
    }

    #[test]
    fn test_should_start_auto_start_false() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.auto_start = false;
        let proc = ManagedProcess::new_config("test".into(), test_helpers::test_uuid(), cfg);
        assert!(!proc.should_start());
    }

    #[test]
    fn test_should_start_condition_path_exists_met() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        let exe = std::env::current_exe().unwrap();
        cfg.condition_path_exists = Some(exe.to_str().unwrap().to_string());
        let proc = ManagedProcess::new_config("test".into(), test_helpers::test_uuid(), cfg);
        assert!(proc.should_start());
    }

    /// The veto has to be consulted by the start path, not merely parsed. The pair also
    /// pins the direction: only the true value blocks.
    #[test]
    fn test_should_start_honours_condition_config_none() {
        for (body, expected_start) in [
            ("system_probe_config:\n  external: true\n", false),
            ("system_probe_config:\n  external: false\n", true),
        ] {
            let _env = crate::config_gate::test_env_guard();
            let dir = tempfile::tempdir().unwrap();
            let sysprobe = dir.path().join("system-probe.yaml");
            std::fs::write(&sysprobe, body).unwrap();

            let (cmd, args) = test_helpers::true_cmd();
            let mut cfg = test_helpers::make_config(cmd, args);
            cfg.condition_config_none = vec![crate::config_gate::ConditionConfigFile {
                path: sysprobe.to_string_lossy().into_owned(),
                keys: vec!["system_probe_config.external".to_string()],
            }];
            let proc = ManagedProcess::new_config("test".into(), test_helpers::test_uuid(), cfg);

            assert_eq!(proc.should_start(), expected_start, "for {body:?}");
        }
    }

    #[test]
    fn test_should_start_condition_path_exists_not_met() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.condition_path_exists = Some("/nonexistent/path/binary".to_string());
        let proc = ManagedProcess::new_config("test".into(), test_helpers::test_uuid(), cfg);
        assert!(!proc.should_start());
    }

    #[tokio::test]
    async fn test_spawn_and_is_running() {
        let mut proc = ManagedProcess::new_config(
            "sleeper".into(),
            test_helpers::test_uuid(),
            test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS),
        );

        assert!(!proc.is_running());
        let mut exit_rx = spawn_ok(&mut proc);
        assert!(proc.is_running());

        proc.request_stop();
        proc.wait_for_stop().await;
        let _ = exit_rx.try_recv();
        assert_eq!(proc.state(), ProcessState::Stopped);
    }

    #[tokio::test]
    async fn test_spawn_nonexistent_binary() {
        let cfg = test_helpers::make_config("/nonexistent/binary", vec![]);
        let mut proc = ManagedProcess::new_config("bad".into(), test_helpers::test_uuid(), cfg);
        assert!(proc.spawn(test_exit_channel().0).is_err());
        assert!(!proc.is_running());
        assert_eq!(proc.state(), ProcessState::Failed);
    }

    #[tokio::test]
    async fn test_spawn_failure_after_stop_goes_through_starting_to_failed() {
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS),
        );
        let mut exit_rx = spawn_ok(&mut proc);
        proc.request_stop();
        proc.wait_for_stop().await;
        let _ = exit_rx.try_recv();
        assert_eq!(proc.state(), ProcessState::Stopped);

        let mut bad_cfg = proc.config().clone();
        bad_cfg.command = "/nonexistent/binary".to_string();
        proc.set_config(bad_cfg);
        assert!(proc.spawn(test_exit_channel().0).is_err());
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
        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
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
        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
        assert_eq!(status.code(), Some(7));
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn test_send_signal_sigterm() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "sig-test".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        let mut exit_rx = spawn_ok(&mut proc);

        proc.send_signal(Signal::SIGTERM);
        let status = exit_rx.recv().await.expect("exit event").status;
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
        let _env = test_helpers::EnvGuard::set(&[("PROCMGRD_TEST_SECRET", "leaked")]).await;
        let (sh, flag) = test_helpers::shell_cmd();
        #[cfg(unix)]
        let script = "test -z \"$PROCMGRD_TEST_SECRET\" && exit 0 || exit 1";
        #[cfg(windows)]
        let script = "if defined PROCMGRD_TEST_SECRET (exit 1) else (exit 0)";
        let cfg = test_helpers::make_config(sh, vec![flag.into(), script.into()]);
        let mut proc =
            ManagedProcess::new_config("clean-env".into(), test_helpers::test_uuid(), cfg);
        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
        assert_eq!(
            status.code(),
            Some(0),
            "child should NOT see PROCMGRD_TEST_SECRET"
        );
    }

    #[tokio::test]
    async fn test_spawn_inherits_opted_in_parent_env() {
        let _env = test_helpers::EnvGuard::set(&[
            ("DD_PM_INHERIT_ENV_PREFIXES", "INHERITED_PREFIX_"),
            ("DD_PM_INHERIT_ENV_NAMES", " INHERITED_EXACT, "),
            ("INHERITED_PREFIX_VALUE", "prefix"),
            ("INHERITED_PREFIX_FILE", "parent"),
            ("INHERITED_EXACT", "parent"),
            ("NOT_INHERITED", "secret"),
        ])
        .await;

        let dir = tempfile::tempdir().unwrap();
        let env_file = dir.path().join("env");
        std::fs::write(&env_file, "INHERITED_PREFIX_FILE=file\n").unwrap();

        let (sh, flag) = test_helpers::shell_cmd();
        #[cfg(unix)]
        let script = "test \"$INHERITED_PREFIX_VALUE\" = prefix && test \"$INHERITED_PREFIX_FILE\" = file && test \"$INHERITED_EXACT\" = override && test -z \"$NOT_INHERITED\"";
        #[cfg(windows)]
        let script = "if not \"%INHERITED_PREFIX_VALUE%\"==\"prefix\" exit 1 & if not \"%INHERITED_PREFIX_FILE%\"==\"file\" exit 1 & if not \"%INHERITED_EXACT%\"==\"override\" exit 1 & if defined NOT_INHERITED exit 1 & exit 0";
        let mut cfg = test_helpers::make_config(sh, vec![flag.into(), script.into()]);
        cfg.environment_file = Some(env_file.to_str().unwrap().to_string());
        cfg.env
            .insert("INHERITED_EXACT".to_string(), "override".to_string());

        let mut proc =
            ManagedProcess::new_config("inherited-env".into(), test_helpers::test_uuid(), cfg);
        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
        assert_eq!(status.code(), Some(0));
    }

    #[tokio::test]
    async fn test_spawn_with_environment_file() {
        let dir = tempfile::tempdir().unwrap();
        let env_file = dir.path().join("env");
        std::fs::write(&env_file, "# comment\nEXIT_CODE=42\n\n").unwrap();

        let (cmd, args) = test_helpers::exit_env_cmd("EXIT_CODE");
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.environment_file = Some(env_file.to_str().unwrap().to_string());

        let mut proc = ManagedProcess::new_config("envfile".into(), test_helpers::test_uuid(), cfg);
        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
        assert_eq!(
            status.code(),
            Some(42),
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
        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
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
            proc.spawn(test_exit_channel().0).is_err(),
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
        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
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

    /// An `Instant` is measured from boot, so an interval longer than the host's uptime has
    /// no representable cutoff. Subtracting it used to panic, which on Windows hit every
    /// host booted less than `start_limit_interval_sec` ago.
    #[test]
    fn test_burst_interval_longer_than_uptime() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.start_limit_burst = Some(2);
        let mut proc = ManagedProcess::new_config("uptime".into(), test_helpers::test_uuid(), cfg);
        let burst = proc.config.burst_limit();

        assert!(!proc.restarts.is_burst_limited(burst, Duration::MAX));
        proc.restarts
            .record(proc.config.restart_delay(), proc.config.runtime_success());
        assert!(!proc.restarts.is_burst_limited(burst, Duration::MAX));
        proc.restarts
            .record(proc.config.restart_delay(), proc.config.runtime_success());
        assert!(
            proc.restarts.is_burst_limited(burst, Duration::MAX),
            "the whole history counts when the window starts before boot"
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

        proc.restarts
            .record(proc.config.restart_delay(), proc.config.runtime_success());
        assert!(
            (proc.restarts.current_delay - 1.0).abs() < 0.001,
            "delay should reset after long runtime"
        );
        assert_eq!(proc.restarts.count, 1, "counter should reset to 1");
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
    async fn test_stop_requested_transitions_to_stopped() {
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS),
        );
        let mut exit_rx = spawn_ok(&mut proc);
        assert_eq!(proc.state(), ProcessState::Running);

        proc.request_stop();
        proc.wait_for_stop().await;
        let _ = exit_rx.try_recv();

        assert_eq!(proc.state(), ProcessState::Stopped);
    }

    /// A graceful stop that never reached the child leaves nothing to wait for,
    /// so a full `stop_timeout` only postpones the kill. Windows gets here
    /// whenever `AttachConsole` cannot reach the child and `CTRL_BREAK` goes
    /// undelivered.
    #[tokio::test]
    async fn test_undelivered_graceful_stop_skips_the_stop_timeout() {
        let mut cfg = test_helpers::sleep_test_config(60);
        cfg.stop_timeout = Some(30);
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);
        let mut exit_rx = spawn_ok(&mut proc);

        // An undelivered stop, staged rather than provoked: making the platform
        // call fail against a live child is not portable. Nothing is sent, so
        // the child stays up until the force kill whatever it does with signals.
        assert!(proc.mark_stop_requested());
        proc.graceful_stop_failed = true;

        let started = std::time::Instant::now();
        proc.wait_for_stop().await;
        let _ = exit_rx.try_recv();

        assert_eq!(proc.state(), ProcessState::Stopped);
        assert!(
            started.elapsed() < std::time::Duration::from_secs(20),
            "an undelivered stop should force-kill instead of waiting out stop_timeout (took {:?})",
            started.elapsed()
        );
    }

    #[tokio::test]
    async fn test_stop_start_then_crash_restarts_on_failure() {
        let mut cfg = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        cfg.restart = RestartPolicy::OnFailure;
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);
        let mut exit_rx = spawn_ok(&mut proc);

        proc.request_stop();
        proc.wait_for_stop().await;
        let _ = exit_rx.try_recv();

        let mut exit_rx = spawn_ok(&mut proc);
        if let Some(pid) = proc.pid() {
            test_helpers::cleanup_process(pid);
        }
        let status = tokio::time::timeout(std::time::Duration::from_secs(5), exit_rx.recv())
            .await
            .expect("timed out waiting for external kill exit")
            .expect("exit event")
            .status;
        proc.set_last_status(status);

        assert_eq!(proc.state(), EXTERNAL_KILL_STATE);
        assert!(
            proc.handle_restart().is_some(),
            "on-failure should restart after stop -> start -> external kill"
        );
    }

    #[tokio::test]
    async fn test_stop_requested_skips_restart() {
        let mut cfg = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        cfg.restart = RestartPolicy::Always;
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);
        let mut exit_rx = spawn_ok(&mut proc);

        proc.request_stop();
        proc.wait_for_stop().await;
        let _ = exit_rx.try_recv();

        assert_eq!(proc.state(), ProcessState::Stopped);
        assert!(
            proc.handle_restart().is_none(),
            "stopped process should not restart even with Always policy"
        );
    }

    #[tokio::test]
    async fn test_unmet_condition_skips_restart() {
        let (cmd, args) = test_helpers::exit_cmd(1);
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.condition_path_exists = Some("/nonexistent/path/binary".to_string());
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);

        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
        proc.set_last_status(status);

        assert!(
            proc.handle_restart().is_none(),
            "restart should be skipped while a start condition is unmet"
        );
        assert_eq!(
            proc.restart_count(),
            0,
            "a skipped restart must not consume burst budget"
        );
        assert!(
            proc.restart_blocked_by_conditions(),
            "the skip reason must be recorded so reload can recover the process"
        );
    }

    /// The flag exists so that reload can start a process that a closed
    /// condition kept from restarting, and only such a process. Each test
    /// below drives one route into a terminal state and asserts whether that
    /// route is one reload may act on.
    fn condition_gated_config(gate: &std::path::Path) -> ProcessConfig {
        let (cmd, args) = test_helpers::exit_cmd(1);
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.condition_path_exists = Some(gate.to_string_lossy().into_owned());
        cfg
    }

    async fn run_to_exit(proc: &mut ManagedProcess) {
        let mut exit_rx = spawn_ok(proc);
        let status = exit_rx.recv().await.expect("exit event").status;
        proc.set_last_status(status);
    }

    #[tokio::test]
    async fn test_spawn_clears_restart_blocked_flag() {
        let dir = tempfile::tempdir().unwrap();
        let gate = dir.path().join("gate");
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            condition_gated_config(&gate),
        );

        run_to_exit(&mut proc).await;
        assert!(proc.handle_restart().is_none());
        assert!(proc.restart_blocked_by_conditions());

        // The condition reopens and the process is spawned again, which is the
        // only place the flag is cleared.
        std::fs::write(&gate, b"").unwrap();
        assert!(proc.may_respawn());
        run_to_exit(&mut proc).await;
        assert!(
            !proc.restart_blocked_by_conditions(),
            "spawning must clear the recorded skip reason"
        );
    }

    #[tokio::test]
    async fn test_completed_one_shot_does_not_record_restart_block() {
        let (cmd, args) = test_helpers::exit_cmd(0);
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Never;
        cfg.condition_path_exists = Some("/nonexistent/path/binary".to_string());
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);

        run_to_exit(&mut proc).await;

        assert_eq!(proc.state(), ProcessState::Exited);
        assert!(proc.handle_restart().is_none());
        assert!(
            !proc.restart_blocked_by_conditions(),
            "a one-shot that ran to completion was not blocked by its condition"
        );
    }

    #[tokio::test]
    async fn test_policy_mismatch_does_not_record_restart_block() {
        let (cmd, args) = test_helpers::exit_cmd(0);
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::OnFailure;
        cfg.condition_path_exists = Some("/nonexistent/path/binary".to_string());
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);

        run_to_exit(&mut proc).await;

        assert_eq!(proc.state(), ProcessState::Exited);
        assert!(proc.handle_restart().is_none());
        assert!(
            !proc.restart_blocked_by_conditions(),
            "a clean exit under on-failure was left alone by policy, not by the condition"
        );
    }

    #[tokio::test]
    async fn test_burst_limit_does_not_record_restart_block() {
        let (cmd, args) = test_helpers::exit_cmd(1);
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        cfg.start_limit_burst = Some(1);
        cfg.start_limit_interval_sec = Some(3600);
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);

        run_to_exit(&mut proc).await;
        assert!(proc.handle_restart().is_some(), "first restart is allowed");

        run_to_exit(&mut proc).await;
        assert!(
            proc.handle_restart().is_none(),
            "the second exit is over the burst limit"
        );
        assert!(
            !proc.restart_blocked_by_conditions(),
            "the burst limit must not look like a condition skip, or reload would bypass it"
        );
    }

    #[tokio::test]
    async fn test_spawn_failure_does_not_record_restart_block() {
        let dir = tempfile::tempdir().unwrap();
        let gate = dir.path().join("gate");
        std::fs::write(&gate, b"").unwrap();
        let mut cfg = condition_gated_config(&gate);
        cfg.command = "/nonexistent/binary".to_string();
        cfg.args = vec![];
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);

        let (tx, _rx) = test_exit_channel();
        assert!(proc.spawn(tx).is_err(), "a missing binary must not spawn");

        assert_eq!(proc.state(), ProcessState::Failed);
        assert!(
            !proc.restart_blocked_by_conditions(),
            "a failed spawn must not look like a condition skip, or reload would hot-loop it"
        );
    }

    #[tokio::test]
    async fn test_operator_stop_does_not_record_restart_block() {
        let dir = tempfile::tempdir().unwrap();
        let gate = dir.path().join("gate");
        std::fs::write(&gate, b"").unwrap();
        let mut cfg = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        cfg.restart = RestartPolicy::Always;
        cfg.condition_path_exists = Some(gate.to_string_lossy().into_owned());
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);

        let mut exit_rx = spawn_ok(&mut proc);
        proc.request_stop();
        proc.wait_for_stop().await;
        let _ = exit_rx.try_recv();

        assert_eq!(proc.state(), ProcessState::Stopped);
        assert!(
            !proc.restart_blocked_by_conditions(),
            "an operator stop is deliberate and must survive an unrelated reload"
        );
    }

    #[tokio::test]
    async fn test_normal_exit_not_affected_by_stop_flag() {
        let (cmd, args) = test_helpers::exit_cmd(1);
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        let mut exit_rx = spawn_ok(&mut proc);

        let status = exit_rx.recv().await.expect("exit event").status;
        proc.set_last_status(status);

        assert_eq!(
            proc.state(),
            ProcessState::Failed,
            "without stop_requested, non-zero exit should be Failed"
        );
    }
}
