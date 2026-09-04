// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::{ProcessConfig, RestartPolicy};
use crate::env::expand_env_vars;
use crate::handle::ProcessHandle;
use crate::platform;
use crate::spawn::{SpawnProfile, profile_for};
use crate::state::ProcessState;
use anyhow::{Context, Result, bail};
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

// ---------------------------------------------------------------------------
// RestartTracker
// ---------------------------------------------------------------------------

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

    /// Record a restart. Resets backoff if the process ran long enough.
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

// ---------------------------------------------------------------------------
// ProcessOrigin
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessOrigin {
    Config,
    Runtime,
}

// ---------------------------------------------------------------------------
// ManagedProcess
// ---------------------------------------------------------------------------

pub struct ManagedProcess {
    name: String,
    uuid: String,
    config: ProcessConfig,
    profile: SpawnProfile,
    user: String,
    state: ProcessState,
    pid: Option<u32>,
    handle: Option<ProcessHandle>,
    watcher_handle: Option<JoinHandle<()>>,
    restarts: RestartTracker,
    stop_requested: bool,
    origin: ProcessOrigin,
    last_exit_status: Option<std::process::ExitStatus>,
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
        let profile = profile_for(&name);
        let user = platform::intended_spawn_user(&name, profile);
        let restarts = RestartTracker::new(config.restart_delay());
        Self {
            name,
            uuid,
            config,
            profile,
            user,
            state: ProcessState::Created,
            pid: None,
            handle: None,
            watcher_handle: None,
            restarts,
            stop_requested: false,
            origin,
            last_exit_status: None,
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

    pub(crate) fn profile(&self) -> SpawnProfile {
        self.profile
    }

    pub fn user(&self) -> &str {
        &self.user
    }

    pub(crate) fn set_intended_user(&mut self, user: String) {
        self.user = user;
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

    #[must_use]
    pub fn should_start(&self) -> bool {
        if !self.config.auto_start {
            info!("[{}] auto_start=false, skipping", self.name);
            return false;
        }
        if let Some(ref raw) = self.config.condition_path_exists {
            let path = expand_env_vars(raw);
            if !std::path::Path::new(&path).exists() {
                info!("[{}] condition_path_exists not met: {path}", self.name);
                return false;
            }
        }
        true
    }

    pub(crate) fn spawn(&mut self, exit_tx: mpsc::Sender<ExitEvent>) -> Result<()> {
        if !self.state.can_transition_to(ProcessState::Starting) {
            bail!("[{}] cannot spawn: invalid state {}", self.name, self.state);
        }
        self.stop_requested = false;
        self.transition_to(ProcessState::Starting);
        let result = self.try_spawn();
        if result.is_err() {
            #[cfg(windows)]
            self.clear_windows_spawn_resources();
            self.transition_to(ProcessState::Failed);
            return result;
        }
        self.start_exit_watcher(exit_tx);
        result
    }

    fn start_exit_watcher(&mut self, tx: mpsc::Sender<ExitEvent>) {
        let Some(mut handle) = self.take_handle() else {
            return;
        };
        let name = self.name().to_owned();
        let pid = self.pid().unwrap_or(0);
        let watcher_handle = tokio::spawn(async move {
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
            let _ = tx.try_send(ExitEvent {
                name,
                pid,
                status,
            });
        });
        self.set_watcher_handle(watcher_handle);
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

    pub(crate) fn take_handle(&mut self) -> Option<ProcessHandle> {
        self.handle.take()
    }

    fn has_child_handle(&self) -> bool {
        self.handle.is_some()
    }

    fn set_watcher_handle(&mut self, handle: JoinHandle<()>) {
        self.watcher_handle = Some(handle);
    }

    fn take_watcher_handle(&mut self) -> Option<JoinHandle<()>> {
        self.watcher_handle.take()
    }

    /// Record that the child exited. If a stop was explicitly requested via
    /// `request_stop`, the process transitions to Stopped (skipping restarts).
    /// Otherwise it transitions to Exited or Failed based on the exit code.
    pub fn set_last_status(&mut self, status: std::process::ExitStatus) {
        self.last_exit_status = Some(status);
        self.pid = None;
        #[cfg(windows)]
        self.clear_windows_spawn_resources();
        if self.stop_requested {
            self.stop_requested = false;
            self.transition_to(ProcessState::Stopped);
        } else if status.success() {
            self.transition_to(ProcessState::Exited);
        } else {
            self.transition_to(ProcessState::Failed);
        }
    }

    /// Mark the process for stop and send a graceful-stop signal. The watcher
    /// will observe the exit and `set_last_status` will transition to Stopped.
    pub fn request_stop(&mut self) {
        if self.is_running() {
            self.stop_requested = true;
            info!("[{}] sending graceful stop (stop requested)", self.name);
            self.graceful_stop();
        }
    }

    /// Send a graceful-stop signal (SIGTERM on Unix, CTRL_BREAK on Windows).
    fn graceful_stop(&self) {
        if let Some(pid) = self.pid
            && let Err(e) = platform::send_graceful_stop(pid)
        {
            warn!("[{}] graceful stop failed: {e}", self.name);
        }
    }

    /// Force-kill the process and all descendants.
    ///
    /// On Unix this sends SIGKILL to the entire process group.
    /// On Windows this terminates the Job Object (all descendants), falling
    /// back to `TerminateProcess` on the direct child if no job is available.
    fn force_kill(&mut self) {
        #[cfg(windows)]
        if let Some(ref job) = self.job_object {
            if let Err(e) = job.terminate() {
                warn!("[{}] job object terminate failed: {e}", self.name);
            } else {
                return;
            }
        }

        if let Some(pid) = self.pid
            && let Err(e) = platform::send_force_kill(pid)
        {
            warn!("[{}] force kill failed: {e}", self.name);
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

    /// Wait for the child to exit. Only works when we still hold the handle.
    pub async fn wait(&mut self) -> Result<std::process::ExitStatus> {
        let handle = self.handle.as_mut().context("no child handle to wait on")?;
        let status = handle.wait().await?;
        info!("[{}] exited with {status}", self.name);
        self.handle = None;
        Ok(status)
    }

    fn stop_timeout(&self) -> Duration {
        self.config.stop_timeout()
    }

    /// Wait for the process to stop after a graceful-stop signal has been sent.
    /// Escalates to force-kill if the process doesn't exit within `stop_timeout`.
    pub async fn wait_for_stop(&mut self) {
        if !self.is_running() {
            return;
        }
        let stop = self.stop_timeout();
        if let Some(handle) = self.take_watcher_handle() {
            tokio::pin!(handle);
            if time::timeout(stop, &mut handle).await.is_err() {
                warn!(
                    "[{}] stop timeout ({}s) reached, force-killing",
                    self.name,
                    stop.as_secs()
                );
                self.force_kill();
                if time::timeout(Self::FORCE_KILL_TIMEOUT, handle)
                    .await
                    .is_err()
                {
                    warn!("[{}] still running after force-kill, giving up", self.name);
                }
            }
        } else if self.has_child_handle() && time::timeout(stop, self.wait()).await.is_err() {
            warn!(
                "[{}] stop timeout ({}s) reached, force-killing",
                self.name,
                stop.as_secs()
            );
            self.force_kill();
            if time::timeout(Self::FORCE_KILL_TIMEOUT, self.wait())
                .await
                .is_err()
            {
                warn!("[{}] still running after force-kill, giving up", self.name);
            }
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

    /// Check restart policy and burst limits. If a restart is warranted,
    /// record the attempt, advance the backoff, and return the delay the
    /// caller should sleep before calling [`spawn`]. Returns `None` if the
    /// process should not be restarted.
    #[must_use]
    pub fn handle_restart(&mut self) -> Option<Duration> {
        let should_restart = match (self.state, &self.config.restart) {
            (ProcessState::Exited | ProcessState::Failed, RestartPolicy::Always) => true,
            (ProcessState::Failed, RestartPolicy::OnFailure) => true,
            (ProcessState::Exited, RestartPolicy::OnSuccess) => true,
            (ProcessState::Exited | ProcessState::Failed, _) => false,
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
    use crate::env::{expand_vars_with, try_expand_vars_with};
    use crate::test_helpers;
    #[cfg(unix)]
    use nix::sys::signal::Signal;

    fn spawn_ok(proc: &mut ManagedProcess) -> mpsc::Receiver<ExitEvent> {
        let (tx, rx) = test_exit_channel();
        proc.spawn(tx).unwrap();
        rx
    }

    // -- ${VAR} expansion tests --

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

    // -- state lifecycle tests --

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
    async fn test_state_after_take_handle_still_running() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "t".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        let _exit_rx = spawn_ok(&mut proc);
        assert_eq!(proc.state(), ProcessState::Running);

        // Watcher owns the wait handle after a successful spawn.
        let child = proc.take_handle();
        assert!(
            child.is_none(),
            "exit watcher takes the child handle on spawn"
        );
        assert_eq!(
            proc.state(),
            ProcessState::Running,
            "state should remain Running after spawn takes the handle for the watcher"
        );
        assert!(proc.is_running());

        if let Some(pid) = proc.pid() {
            test_helpers::cleanup_process(pid);
        }
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
        let mut exit_rx = spawn_ok(&mut proc);

        proc.send_signal(Signal::SIGTERM);
        let status = exit_rx.recv().await.expect("exit event").status;
        assert!(
            !status.success(),
            "signal by stored PID should reach child after spawn (watcher owns the handle)"
        );
    }

    // -- should_start tests --

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

    #[test]
    fn test_should_start_condition_path_exists_not_met() {
        let (cmd, args) = test_helpers::true_cmd();
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.condition_path_exists = Some("/nonexistent/path/binary".to_string());
        let proc = ManagedProcess::new_config("test".into(), test_helpers::test_uuid(), cfg);
        assert!(!proc.should_start());
    }

    // -- spawn tests --

    #[tokio::test]
    async fn test_spawn_and_is_running() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "sleeper".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );

        assert!(!proc.is_running());
        let mut exit_rx = spawn_ok(&mut proc);
        assert!(proc.is_running());

        proc.request_stop();
        let status = exit_rx.recv().await.expect("exit event").status;
        proc.set_last_status(status);
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
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        let mut exit_rx = spawn_ok(&mut proc);
        proc.request_stop();
        let status = exit_rx.recv().await.expect("exit event").status;
        proc.set_last_status(status);
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

    // -- env tests --

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
        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
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
        let mut exit_rx = spawn_ok(&mut proc);
        let status = exit_rx.recv().await.expect("exit event").status;
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

    // -- restart policy tests --

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
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut proc = ManagedProcess::new_config(
            "svc".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config(cmd, args),
        );
        let mut exit_rx = spawn_ok(&mut proc);
        assert_eq!(proc.state(), ProcessState::Running);

        proc.request_stop();
        let status = exit_rx.recv().await.expect("exit event").status;
        proc.set_last_status(status);

        assert_eq!(proc.state(), ProcessState::Stopped);
    }

    #[tokio::test]
    async fn test_stop_start_then_crash_restarts_on_failure() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::OnFailure;
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);
        let mut exit_rx = spawn_ok(&mut proc);

        proc.request_stop();
        proc.wait_for_stop().await;
        let _ = exit_rx.try_recv();

        let mut exit_rx = spawn_ok(&mut proc);
        test_helpers::cleanup_process(proc.pid().expect("running pid"));
        let status = exit_rx.recv().await.expect("exit event").status;
        proc.set_last_status(status);

        assert_eq!(proc.state(), ProcessState::Failed);
        assert!(
            proc.handle_restart().is_some(),
            "on-failure should restart after stop -> start -> external kill"
        );
    }

    #[tokio::test]
    async fn test_stop_requested_skips_restart() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let mut cfg = test_helpers::make_config(cmd, args);
        cfg.restart = RestartPolicy::Always;
        let mut proc = ManagedProcess::new_config("svc".into(), test_helpers::test_uuid(), cfg);
        let mut exit_rx = spawn_ok(&mut proc);

        proc.request_stop();
        let status = exit_rx.recv().await.expect("exit event").status;
        proc.set_last_status(status);

        assert_eq!(proc.state(), ProcessState::Stopped);
        assert!(
            proc.handle_restart().is_none(),
            "stopped process should not restart even with Always policy"
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

    // -- stdio_from_str (YAML stdout/stderr: inherit, "", null, file path, unopenable path) --

    fn stdio_from_str(s: &str) -> std::process::Stdio {
        use std::process::Stdio;
        match s {
            "null" => Stdio::null(),
            "inherit" | "" => Stdio::inherit(),
            path => match std::fs::OpenOptions::new()
                .create(true)
                .append(true)
                .open(path)
            {
                Ok(f) => f.into(),
                Err(_) => Stdio::inherit(),
            },
        }
    }

    #[test]
    fn test_stdio_from_str_inherit_spawns() {
        let (cmd, args) = test_helpers::true_cmd();
        let status = std::process::Command::new(cmd)
            .args(&args)
            .stdout(stdio_from_str("inherit"))
            .stderr(stdio_from_str("inherit"))
            .status()
            .unwrap();
        assert!(status.success());
    }

    #[test]
    fn test_stdio_from_str_empty_string_matches_inherit() {
        let (cmd, args) = test_helpers::true_cmd();
        let status = std::process::Command::new(cmd)
            .args(&args)
            .stdout(stdio_from_str(""))
            .status()
            .unwrap();
        assert!(status.success());
    }

    #[test]
    fn test_stdio_from_str_null_discards_child_stdout() {
        let (sh, flag) = test_helpers::shell_cmd();
        let out = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo hello")
            .stdout(stdio_from_str("null"))
            .output()
            .unwrap();
        assert!(
            out.stdout.is_empty(),
            "stdout should be discarded, got {:?}",
            String::from_utf8_lossy(&out.stdout)
        );
    }

    #[test]
    fn test_stdio_from_str_writable_path_redirect() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("pmgr_stdio_redirect.log");
        let path_str = path.to_str().unwrap();
        let (sh, flag) = test_helpers::shell_cmd();
        let status = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo fileline")
            .stdout(stdio_from_str(path_str))
            .status()
            .unwrap();
        assert!(status.success());
        let contents = std::fs::read_to_string(&path).unwrap();
        assert!(
            contents.contains("fileline"),
            "expected fileline in log, got {contents:?}"
        );
    }

    #[test]
    fn test_stdio_from_str_path_appends_on_respawn() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("pmgr_stdio_append.log");
        let path_str = path.to_str().unwrap();
        let (sh, flag) = test_helpers::shell_cmd();
        for msg in ["first", "second"] {
            let status = std::process::Command::new(sh)
                .arg(flag)
                .arg(format!("echo {msg}"))
                .stdout(stdio_from_str(path_str))
                .status()
                .unwrap();
            assert!(status.success());
        }
        let contents = std::fs::read_to_string(&path).unwrap();
        assert!(contents.contains("first"), "got {contents:?}");
        assert!(contents.contains("second"), "got {contents:?}");
    }

    #[test]
    fn test_stdio_from_str_unopenable_path_falls_back_to_inherit() {
        let (sh, flag) = test_helpers::shell_cmd();
        #[cfg(unix)]
        let bad_path = "/nonexistent_dir_pmgr_stdio/out.log";
        #[cfg(windows)]
        let bad_path = r"C:\nonexistent_dir_pmgr_stdio\out.log";
        let out = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo fallback_ok")
            .stdout(stdio_from_str(bad_path))
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "spawn with unopenable stdout path should still succeed (falls back to inherit)"
        );
        // Child stdout is inherited (not piped), so `out.stdout` is empty; success is the signal.
    }
}
