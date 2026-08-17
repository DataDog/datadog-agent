// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::{ProcessConfig, RestartPolicy};
use crate::env::expand_env_vars;
use crate::handle::ProcessHandle;
use crate::platform;
use crate::state::ProcessState;
use anyhow::{Context, Result, bail};
use log::{debug, info, warn};
use std::collections::VecDeque;
use tokio::task::JoinHandle;
use tokio::time::{self, Duration, Instant};

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
        if let Some(spawn_time) = self.last_spawn_time
            && spawn_time.elapsed() >= runtime_success
        {
            self.current_delay = base_delay;
            self.count = 0;
            self.last_spawn_time = None;
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

pub struct ManagedProcess {
    name: String,
    uuid: String,
    config: ProcessConfig,
    state: ProcessState,
    pid: Option<u32>,
    handle: Option<ProcessHandle>,
    watcher_handle: Option<JoinHandle<Option<std::process::ExitStatus>>>,
    restarts: RestartTracker,
    origin: ProcessOrigin,
    last_exit_status: Option<std::process::ExitStatus>,
    last_start_conditions_met: Option<bool>,
    config_generation: u64,
    had_successful_run: bool,
    #[cfg(windows)]
    job_object: Option<platform::JobObject>,
    #[cfg(windows)]
    user_profile: Option<platform::UserProfileGuard>,
}

impl ManagedProcess {
    pub(crate) const FORCE_KILL_TIMEOUT: Duration = Duration::from_secs(10);

    pub fn new_config(name: String, uuid: String, config: ProcessConfig) -> Self {
        Self::new_inner(name, uuid, config, ProcessOrigin::Config)
    }

    pub fn new_runtime(name: String, uuid: String, config: ProcessConfig) -> Self {
        Self::new_inner(name, uuid, config, ProcessOrigin::Runtime)
    }

    fn new_inner(name: String, uuid: String, config: ProcessConfig, origin: ProcessOrigin) -> Self {
        let restarts = RestartTracker::new(config.restart_delay());
        Self {
            name,
            uuid,
            config,
            state: ProcessState::Created,
            pid: None,
            handle: None,
            watcher_handle: None,
            restarts,
            origin,
            last_exit_status: None,
            last_start_conditions_met: None,
            config_generation: 0,
            had_successful_run: false,
            #[cfg(windows)]
            job_object: None,
            #[cfg(windows)]
            user_profile: None,
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
        self.user_profile = None;
    }

    #[cfg(windows)]
    fn prepare_windows_spawn_resource_release(&mut self) {
        let Some(job) = self.job_object.as_ref() else {
            self.user_profile = None;
            return;
        };
        match job.active_process_count() {
            Ok(0) => self.clear_windows_spawn_resources(),
            Ok(_) => info!(
                "[{}] job still has active members; waiting before releasing profile",
                self.name
            ),
            Err(e) => {
                warn!(
                    "[{}] failed to query job active processes: {e:#}",
                    self.name
                );
                self.clear_windows_spawn_resources();
            }
        }
    }

    #[cfg(windows)]
    pub(crate) async fn ensure_windows_spawn_resources_released(&mut self) {
        let Some(job) = self.job_object.as_ref() else {
            self.user_profile = None;
            return;
        };
        if job.may_have_active_members() {
            if let Err(e) = job.terminate() {
                warn!(
                    "[{}] failed to terminate residual job members: {e:#}",
                    self.name
                );
            }
            if !Self::wait_for_job_empty(job, Self::FORCE_KILL_TIMEOUT).await {
                warn!(
                    "[{}] timed out waiting for job members to exit before releasing profile",
                    self.name
                );
            }
        }
        self.clear_windows_spawn_resources();
    }

    #[cfg(windows)]
    async fn wait_for_job_empty(job: &platform::JobObject, timeout: Duration) -> bool {
        const POLL_INTERVAL: Duration = Duration::from_millis(100);
        let deadline = Instant::now() + timeout;
        loop {
            match job.active_process_count() {
                Ok(0) => return true,
                Ok(_) => {
                    if Instant::now() >= deadline {
                        return matches!(job.active_process_count(), Ok(0));
                    }
                    time::sleep(
                        POLL_INTERVAL.min(deadline.saturating_duration_since(Instant::now())),
                    )
                    .await;
                }
                Err(_) => return false,
            }
        }
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
        self.config_generation += 1;
        self.restarts = RestartTracker::new(config.restart_delay());
        self.had_successful_run = false;
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

    pub(crate) fn record_config_gate_met(&mut self) {
        self.last_start_conditions_met = Some(self.start_conditions_met());
    }

    pub(crate) fn last_start_conditions_met(&self) -> Option<bool> {
        self.last_start_conditions_met
    }

    pub(crate) fn config_generation(&self) -> u64 {
        self.config_generation
    }

    #[cfg(test)]
    pub(crate) fn has_ever_run_successfully(&self) -> bool {
        self.had_successful_run || self.restarts.last_spawn_time.is_some()
    }

    #[cfg(test)]
    pub(crate) fn set_command_for_test(&mut self, command: String) {
        self.config.command = command;
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

    fn try_spawn(&mut self) -> Result<()> {
        #[cfg(windows)]
        let _console_guard = platform::console_lock();

        let handle = platform::spawn_child_handle(self)?;

        self.pid = handle.id();
        info!(
            "[{}] spawned (pid={}, cmd={})",
            self.name,
            self.pid.map_or("unknown".to_string(), |p| p.to_string()),
            self.config.command
        );

        self.handle = Some(handle);
        self.transition_to(ProcessState::Running);
        self.restarts.mark_spawned();
        Ok(())
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
                // Job terminate is async; keep job/profile until exit is observed.
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
        self.transition_to(ProcessState::Stopped);
        self.pid = None;
    }

    pub fn set_last_status(&mut self, status: std::process::ExitStatus) {
        self.last_exit_status = Some(status);
        self.pid = None;
        #[cfg(windows)]
        self.prepare_windows_spawn_resource_release();
        if self.state == ProcessState::Stopping {
            self.transition_to(ProcessState::Stopped);
        } else if status.success() {
            self.transition_to(ProcessState::Exited);
        } else {
            self.transition_to(ProcessState::Failed);
        }
    }

    async fn force_kill_and_wait_watcher(
        &mut self,
        graceful_timeout: Duration,
        handle: &mut core::pin::Pin<&mut JoinHandle<Option<std::process::ExitStatus>>>,
    ) -> Option<std::process::ExitStatus> {
        warn!(
            "[{}] stop timeout ({}s) reached, force-killing",
            self.name,
            graceful_timeout.as_secs()
        );
        self.force_kill();
        match time::timeout(Self::FORCE_KILL_TIMEOUT, &mut **handle).await {
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

    async fn force_kill_and_wait_handle(
        &mut self,
        graceful_timeout: Duration,
    ) -> Option<std::process::ExitStatus> {
        warn!(
            "[{}] stop timeout ({}s) reached, force-killing",
            self.name,
            graceful_timeout.as_secs()
        );
        self.force_kill();
        match time::timeout(Self::FORCE_KILL_TIMEOUT, self.wait()).await {
            Ok(Ok(status)) => Some(status),
            Ok(Err(e)) => {
                warn!("[{}] wait failed after force-kill: {e:#}", self.name);
                None
            }
            Err(_) => {
                warn!("[{}] still running after force-kill, giving up", self.name);
                None
            }
        }
    }

    async fn wait_for_stop_exit(&mut self) -> bool {
        let timeout = self.stop_timeout();

        if let Some(handle) = self.watcher_handle.take() {
            tokio::pin!(handle);
            let status = match time::timeout(timeout, &mut handle).await {
                Ok(Ok(status)) => status,
                Ok(Err(e)) => {
                    warn!("[{}] watcher join failed: {e:#}", self.name);
                    None
                }
                Err(_) => self.force_kill_and_wait_watcher(timeout, &mut handle).await,
            };
            return status.is_some_and(|status| {
                self.set_last_status(status);
                true
            });
        }

        if self.has_child_handle() {
            let status = match time::timeout(timeout, self.wait()).await {
                Ok(Ok(status)) => Some(status),
                Ok(Err(e)) => {
                    warn!("[{}] wait failed during stop: {e:#}", self.name);
                    None
                }
                Err(_) => self.force_kill_and_wait_handle(timeout).await,
            };
            return status.is_some_and(|status| {
                self.set_last_status(status);
                true
            });
        }

        false
    }

    /// Mark the process for stop and send a graceful-stop signal. Does not wait
    /// for exit; pair with [`Self::wait_for_stop`] when shutting down many processes.
    pub fn request_stop(&mut self) {
        match self.state {
            ProcessState::Running => {
                self.transition_to(ProcessState::Stopping);
                info!("[{}] stopping (graceful)", self.name);
                self.graceful_stop();
            }
            ProcessState::Stopping => {
                debug!("[{}] stop requested while already stopping", self.name);
            }
            _ => {}
        }
    }

    /// Wait for a process that was signaled with [`Self::request_stop`] to exit,
    /// force-killing on timeout and releasing Windows spawn resources.
    pub async fn wait_for_stop(&mut self) {
        match self.state {
            ProcessState::Running | ProcessState::Stopping => {
                if !self.wait_for_stop_exit().await {
                    self.mark_stopped();
                }
                #[cfg(windows)]
                self.ensure_windows_spawn_resources_released().await;
            }
            _ => {
                #[cfg(windows)]
                self.ensure_windows_spawn_resources_released().await;
            }
        }
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
            _ => {
                #[cfg(windows)]
                self.ensure_windows_spawn_resources_released().await;
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
        let (delay, met_runtime_success) =
            self.restarts.next_restart_delay(&self.config, &self.name)?;
        if met_runtime_success {
            self.had_successful_run = true;
        }
        Some(delay)
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
    fn test_try_expand_vars_substitutes_known() {
        let lookup = |name: &str| match name {
            "DD_INVENTORIES_FIRST_RUN_DELAY" => Some("5".to_string()),
            _ => None,
        };
        assert_eq!(
            try_expand_vars_with("${DD_INVENTORIES_FIRST_RUN_DELAY}", lookup),
            Some("5".to_string())
        );
    }

    #[test]
    fn test_try_expand_vars_none_when_unset() {
        let lookup = |_: &str| None;
        assert_eq!(
            try_expand_vars_with("${DD_INVENTORIES_FIRST_RUN_DELAY}", lookup),
            None,
            "an unset optional variable must yield None so the caller can omit the env var"
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

        let mut bad_cfg = proc.config().clone();
        bad_cfg.command = "/nonexistent/binary".to_string();
        proc.set_config(bad_cfg);
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
        proc.transition_to(ProcessState::Failed);
        assert!(proc.has_ever_run_successfully());

        assert!(proc.schedule_restart().is_some());
        assert!(
            proc.has_ever_run_successfully(),
            "successful-run state should survive the first restart_delay"
        );

        assert!(proc.schedule_restart().is_some());
        assert!(
            proc.has_ever_run_successfully(),
            "successful-run state should survive subsequent restart_delay calls"
        );
    }

    #[test]
    fn test_set_config_resets_successful_run_state() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args.clone());
        cfg.restart = RestartPolicy::Always;
        cfg.restart_sec = Some(1.0);
        let mut proc =
            ManagedProcess::new_config("config-swap".into(), test_helpers::test_uuid(), cfg);

        proc.had_successful_run = true;
        let mut next_cfg = test_helpers::make_config(cmd, args);
        next_cfg.restart_sec = Some(2.0);

        proc.set_config(next_cfg);

        assert!(
            !proc.had_successful_run,
            "set_config should reset successful-run state for a fresh reload start"
        );
        assert!(!proc.has_ever_run_successfully());
        assert_eq!(proc.config_generation(), 1);
        assert!((proc.restarts.current_delay - 2.0).abs() < 0.001);
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
}
