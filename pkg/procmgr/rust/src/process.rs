// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::{ProcessConfig, RestartPolicy};
use crate::env::parse_environment_file;
use crate::state::ProcessState;
use anyhow::{Context, Result};
use log::{info, warn};
use nix::sys::signal::{self, Signal};
use nix::unistd::Pid;
use std::process::Stdio;
use tokio::process::{Child, Command};
use tokio::time::{Duration, Instant};

// ---------------------------------------------------------------------------
// RestartTracker
// ---------------------------------------------------------------------------

struct RestartTracker {
    count: u32,
    timestamps: Vec<Instant>,
    current_delay: f64,
    last_spawn_time: Option<Instant>,
}

impl RestartTracker {
    const BACKOFF_MULTIPLIER: f64 = 2.0;
    const MAX_TIMESTAMPS: usize = 100;

    fn new(initial_delay: f64) -> Self {
        Self {
            count: 0,
            timestamps: Vec::new(),
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
        self.timestamps.push(Instant::now());
        if self.timestamps.len() > Self::MAX_TIMESTAMPS {
            self.timestamps.remove(0);
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
// ManagedProcess
// ---------------------------------------------------------------------------

pub struct ManagedProcess {
    pub name: String,
    config: ProcessConfig,
    state: ProcessState,
    pid: Option<u32>,
    child: Option<Child>,
    restarts: RestartTracker,
}

impl ManagedProcess {
    pub fn new(name: String, config: ProcessConfig) -> Self {
        let restarts = RestartTracker::new(config.restart_delay());
        Self {
            name,
            config,
            state: ProcessState::Created,
            pid: None,
            child: None,
            restarts,
        }
    }

    #[cfg(test)]
    pub fn state(&self) -> ProcessState {
        self.state
    }

    fn transition_to(&mut self, next: ProcessState) {
        if !self.state.can_transition_to(next) {
            warn!(
                "[{}] invalid state transition: {} -> {next}, ignoring",
                self.name, self.state
            );
            debug_assert!(
                false,
                "[{}] invalid state transition: {} -> {next}",
                self.name, self.state
            );
            return;
        }
        self.state = next;
    }

    pub fn should_start(&self) -> bool {
        if !self.config.auto_start {
            info!("[{}] auto_start=false, skipping", self.name);
            return false;
        }
        if let Some(ref path) = self.config.condition_path_exists
            && !std::path::Path::new(path).exists()
        {
            info!("[{}] condition_path_exists not met: {path}", self.name);
            return false;
        }
        true
    }

    pub fn spawn(&mut self) -> Result<()> {
        let mut cmd = self.build_command()?;

        let child = cmd
            .spawn()
            .with_context(|| format!("[{}] failed to spawn: {}", self.name, self.config.command))?;

        self.pid = child.id();
        info!(
            "[{}] spawned (pid={}, cmd={})",
            self.name,
            self.pid.map_or("unknown".to_string(), |p| p.to_string()),
            self.config.command
        );
        self.child = Some(child);
        self.transition_to(ProcessState::Running);
        self.restarts.mark_spawned();
        Ok(())
    }

    fn build_command(&self) -> Result<Command> {
        let mut cmd = Command::new(&self.config.command);
        cmd.args(&self.config.args);

        cmd.env_clear();
        if let Some(ref path) = self.config.environment_file {
            let vars = parse_environment_file(path).with_context(|| {
                format!("[{}] failed to read environment file: {path}", self.name)
            })?;
            for (k, v) in &vars {
                cmd.env(k, v);
            }
        }
        for (k, v) in &self.config.env {
            cmd.env(k, v);
        }

        if let Some(ref dir) = self.config.working_dir {
            cmd.current_dir(dir);
        }

        cmd.stdout(stdio_from_str(&self.config.stdout));
        cmd.stderr(stdio_from_str(&self.config.stderr));

        Ok(cmd)
    }

    pub fn is_running(&self) -> bool {
        self.state.is_alive()
    }

    /// Hand the child handle to a watcher task.
    /// The process remains in `Running` state — only the handle moves.
    pub fn take_child(&mut self) -> Option<Child> {
        self.child.take()
    }

    pub fn has_child_handle(&self) -> bool {
        self.child.is_some()
    }

    /// Record that the child exited. Transitions state to Exited or Failed.
    pub fn set_last_status(&mut self, status: std::process::ExitStatus) {
        if status.success() {
            self.transition_to(ProcessState::Exited);
        } else {
            self.transition_to(ProcessState::Failed);
        }
    }

    /// Send a signal using the stored PID (works even after take_child).
    /// The caller must still call `wait()` afterward to reap the child and
    /// avoid zombie processes. This is kept separate from `wait()` so
    /// `shutdown_all()` can fan out SIGTERM to all processes before blocking
    /// on each.
    pub fn send_signal(&self, sig: Signal) {
        if let Some(raw_pid) = self.pid
            && let Err(e) = signal::kill(Pid::from_raw(raw_pid as i32), sig)
        {
            warn!("[{}] failed to send {sig}: {e}", self.name);
        }
    }

    /// Wait for the child to exit. Only works when we still hold the handle.
    pub async fn wait(&mut self) -> Result<std::process::ExitStatus> {
        let child = self.child.as_mut().context("no child handle to wait on")?;
        let status = child.wait().await?;
        info!("[{}] exited with {status}", self.name);
        self.child = None;
        Ok(status)
    }

    pub fn stop_timeout(&self) -> Duration {
        self.config.stop_timeout()
    }

    /// Transition to Stopped and clear PID. Used by shutdown.
    pub fn mark_stopped(&mut self) {
        self.transition_to(ProcessState::Stopped);
        self.pid = None;
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

    /// Single entry point for restart logic: check policy, burst limit, backoff, respawn.
    /// Returns true if the process was restarted.
    pub async fn handle_restart(&mut self) -> bool {
        let should_restart = match (self.state, &self.config.restart) {
            (ProcessState::Exited | ProcessState::Failed, RestartPolicy::Always) => true,
            (ProcessState::Failed, RestartPolicy::OnFailure) => true,
            (ProcessState::Exited, RestartPolicy::OnSuccess) => true,
            (ProcessState::Exited | ProcessState::Failed, _) => false,
            _ => return false,
        };

        if !should_restart {
            if self.config.restart != RestartPolicy::Never {
                info!(
                    "[{}] exit does not match restart policy, not restarting",
                    self.name
                );
            }
            return false;
        }

        if self
            .restarts
            .is_burst_limited(self.config.burst_limit(), self.config.burst_interval())
        {
            warn!("[{}] start limit reached, not restarting", self.name);
            return false;
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
        tokio::time::sleep(delay).await;
        self.restarts
            .advance_backoff(self.config.max_restart_delay());

        match self.spawn() {
            Ok(()) => true,
            Err(e) => {
                warn!("[{}] restart failed: {e:#}", self.name);
                false
            }
        }
    }
}

fn stdio_from_str(s: &str) -> Stdio {
    match s {
        "null" => Stdio::null(),
        _ => Stdio::inherit(),
    }
}

#[cfg(test)]
pub mod tests {
    use super::*;
    use crate::config::ProcessConfig;
    use std::collections::HashMap;

    pub fn make_config(command: &str, args: Vec<&str>) -> ProcessConfig {
        ProcessConfig {
            description: None,
            command: command.to_string(),
            args: args.into_iter().map(String::from).collect(),
            env: HashMap::new(),
            environment_file: None,
            working_dir: None,
            pidfile: None,
            stdout: "null".to_string(),
            stderr: "null".to_string(),
            auto_start: true,
            condition_path_exists: None,
            stop_timeout: None,
            restart: RestartPolicy::Never,
            restart_sec: None,
            restart_max_delay_sec: None,
            start_limit_burst: None,
            start_limit_interval_sec: None,
            runtime_success_sec: None,
        }
    }

    fn exit_status(code: i32) -> std::process::ExitStatus {
        std::process::Command::new("/bin/sh")
            .args(["-c", &format!("exit {code}")])
            .status()
            .unwrap()
    }

    // -- state lifecycle tests --

    #[test]
    fn test_initial_state_is_created() {
        let proc = ManagedProcess::new("test".into(), make_config("/bin/true", vec![]));
        assert_eq!(proc.state(), ProcessState::Created);
        assert!(!proc.is_running());
    }

    #[tokio::test]
    async fn test_state_transitions_spawn_exit_success() {
        let mut proc =
            ManagedProcess::new("t".into(), make_config("/bin/sh", vec!["-c", "exit 0"]));
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
        let mut proc =
            ManagedProcess::new("t".into(), make_config("/bin/sh", vec!["-c", "exit 1"]));
        proc.spawn().unwrap();
        assert_eq!(proc.state(), ProcessState::Running);

        let status = proc.wait().await.unwrap();
        assert!(!status.success());
        proc.set_last_status(status);
        assert_eq!(proc.state(), ProcessState::Failed);
    }

    #[tokio::test]
    async fn test_state_after_take_child_still_running() {
        let mut proc = ManagedProcess::new("t".into(), make_config("/bin/sleep", vec!["60"]));
        proc.spawn().unwrap();
        assert_eq!(proc.state(), ProcessState::Running);

        let child = proc.take_child();
        assert!(child.is_some());
        assert_eq!(
            proc.state(),
            ProcessState::Running,
            "state should remain Running after take_child"
        );
        assert!(proc.is_running());

        // Clean up
        proc.send_signal(Signal::SIGKILL);
        let mut child = child.unwrap();
        let _ = child.wait().await;
    }

    #[tokio::test]
    async fn test_send_signal_works_after_take_child() {
        let mut proc = ManagedProcess::new("t".into(), make_config("/bin/sleep", vec!["60"]));
        proc.spawn().unwrap();
        let mut child = proc.take_child().unwrap();

        proc.send_signal(Signal::SIGTERM);
        let status = child.wait().await.unwrap();
        assert!(
            !status.success(),
            "signal by stored PID should reach child after take_child"
        );
    }

    // -- should_start tests --

    #[test]
    fn test_should_start_auto_start_true_no_condition() {
        let proc = ManagedProcess::new("test".into(), make_config("/usr/bin/true", vec![]));
        assert!(proc.should_start());
    }

    #[test]
    fn test_should_start_auto_start_false() {
        let mut cfg = make_config("/usr/bin/true", vec![]);
        cfg.auto_start = false;
        let proc = ManagedProcess::new("test".into(), cfg);
        assert!(!proc.should_start());
    }

    #[test]
    fn test_should_start_condition_path_exists_met() {
        let mut cfg = make_config("/usr/bin/true", vec![]);
        cfg.condition_path_exists = Some("/usr/bin/true".to_string());
        let proc = ManagedProcess::new("test".into(), cfg);
        assert!(proc.should_start());
    }

    #[test]
    fn test_should_start_condition_path_exists_not_met() {
        let mut cfg = make_config("/usr/bin/true", vec![]);
        cfg.condition_path_exists = Some("/nonexistent/path/binary".to_string());
        let proc = ManagedProcess::new("test".into(), cfg);
        assert!(!proc.should_start());
    }

    // -- spawn tests --

    #[tokio::test]
    async fn test_spawn_and_is_running() {
        let cfg = make_config("/bin/sleep", vec!["60"]);
        let mut proc = ManagedProcess::new("sleeper".into(), cfg);

        assert!(!proc.is_running());
        proc.spawn().unwrap();
        assert!(proc.is_running());

        proc.send_signal(Signal::SIGKILL);
        let status = proc.wait().await.unwrap();
        assert!(!status.success());
    }

    #[tokio::test]
    async fn test_spawn_nonexistent_binary() {
        let cfg = make_config("/nonexistent/binary", vec![]);
        let mut proc = ManagedProcess::new("bad".into(), cfg);
        assert!(proc.spawn().is_err());
        assert!(!proc.is_running());
        assert_eq!(proc.state(), ProcessState::Created);
    }

    #[tokio::test]
    async fn test_spawn_with_env() {
        let mut cfg = make_config("/bin/sh", vec!["-c", "exit $MY_EXIT_CODE"]);
        cfg.env.insert("MY_EXIT_CODE".to_string(), "42".to_string());

        let mut proc = ManagedProcess::new("env-test".into(), cfg);
        proc.spawn().unwrap();
        let status = proc.wait().await.unwrap();
        assert_eq!(status.code(), Some(42));
    }

    #[tokio::test]
    async fn test_spawn_with_args() {
        let cfg = make_config("/bin/sh", vec!["-c", "exit 7"]);
        let mut proc = ManagedProcess::new("args-test".into(), cfg);
        proc.spawn().unwrap();
        let status = proc.wait().await.unwrap();
        assert_eq!(status.code(), Some(7));
    }

    // -- signal tests --

    #[tokio::test]
    async fn test_send_signal_sigterm() {
        let cfg = make_config("/bin/sleep", vec!["60"]);
        let mut proc = ManagedProcess::new("sig-test".into(), cfg);
        proc.spawn().unwrap();

        proc.send_signal(Signal::SIGTERM);
        let status = proc.wait().await.unwrap();
        assert!(!status.success());
    }

    #[test]
    fn test_send_signal_no_child_does_not_panic() {
        let proc = ManagedProcess::new("no-child".into(), make_config("/usr/bin/true", vec![]));
        proc.send_signal(Signal::SIGTERM);
    }

    // -- env tests --

    #[tokio::test]
    async fn test_spawn_does_not_inherit_parent_env() {
        // SAFETY: single-threaded test runtime; no concurrent env access.
        unsafe { std::env::set_var("PROCMGRD_TEST_SECRET", "leaked") };
        let cfg = make_config(
            "/bin/sh",
            vec![
                "-c",
                "test -z \"$PROCMGRD_TEST_SECRET\" && exit 0 || exit 1",
            ],
        );
        let mut proc = ManagedProcess::new("clean-env".into(), cfg);
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

        let mut cfg = make_config(
            "/bin/sh",
            vec!["-c", "test \"$FROM_FILE\" = 'hello' && echo $PATH"],
        );
        cfg.environment_file = Some(env_file.to_str().unwrap().to_string());

        let mut proc = ManagedProcess::new("envfile".into(), cfg);
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

        let mut cfg = make_config(
            "/bin/sh",
            vec![
                "-c",
                "exit $(test \"$MY_VAR\" = 'overridden' && echo 0 || echo 1)",
            ],
        );
        cfg.environment_file = Some(env_file.to_str().unwrap().to_string());
        cfg.env
            .insert("MY_VAR".to_string(), "overridden".to_string());

        let mut proc = ManagedProcess::new("override".into(), cfg);
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
        let mut cfg = make_config("/bin/true", vec![]);
        cfg.environment_file = Some("/nonexistent/env".to_string());
        let mut proc = ManagedProcess::new("bad-envfile".into(), cfg);
        assert!(
            proc.spawn().is_err(),
            "spawn should fail if environment_file is unreadable"
        );
        assert!(!proc.is_running());
    }

    // -- restart policy tests --

    #[test]
    fn test_should_restart_never() {
        let cfg = make_config("/bin/sh", vec![]);
        let proc = ManagedProcess::new("t".into(), cfg);
        assert!(!proc.should_restart(&exit_status(1)));
    }

    #[test]
    fn test_should_restart_always_on_success() {
        let mut cfg = make_config("/bin/sh", vec![]);
        cfg.restart = RestartPolicy::Always;
        let proc = ManagedProcess::new("t".into(), cfg);
        assert!(proc.should_restart(&exit_status(0)));
    }

    #[test]
    fn test_should_restart_always_on_failure() {
        let mut cfg = make_config("/bin/sh", vec![]);
        cfg.restart = RestartPolicy::Always;
        let proc = ManagedProcess::new("t".into(), cfg);
        assert!(proc.should_restart(&exit_status(1)));
    }

    #[test]
    fn test_should_restart_on_failure_with_failure() {
        let mut cfg = make_config("/bin/sh", vec![]);
        cfg.restart = RestartPolicy::OnFailure;
        let proc = ManagedProcess::new("t".into(), cfg);
        assert!(proc.should_restart(&exit_status(1)));
    }

    #[test]
    fn test_should_restart_on_failure_with_success() {
        let mut cfg = make_config("/bin/sh", vec![]);
        cfg.restart = RestartPolicy::OnFailure;
        let proc = ManagedProcess::new("t".into(), cfg);
        assert!(!proc.should_restart(&exit_status(0)));
    }

    #[test]
    fn test_should_restart_on_success_with_success() {
        let mut cfg = make_config("/bin/sh", vec![]);
        cfg.restart = RestartPolicy::OnSuccess;
        let proc = ManagedProcess::new("t".into(), cfg);
        assert!(proc.should_restart(&exit_status(0)));
    }

    #[test]
    fn test_should_restart_on_success_with_failure() {
        let mut cfg = make_config("/bin/sh", vec![]);
        cfg.restart = RestartPolicy::OnSuccess;
        let proc = ManagedProcess::new("t".into(), cfg);
        assert!(!proc.should_restart(&exit_status(1)));
    }

    #[test]
    fn test_burst_limiting() {
        let mut cfg = make_config("/bin/true", vec![]);
        cfg.restart = RestartPolicy::Always;
        cfg.start_limit_burst = Some(3);
        cfg.start_limit_interval_sec = Some(60);
        let mut proc = ManagedProcess::new("burst".into(), cfg);

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
        let mut cfg = make_config("/bin/true", vec![]);
        cfg.restart = RestartPolicy::Always;
        cfg.restart_sec = Some(1.0);
        cfg.restart_max_delay_sec = Some(10.0);
        let mut proc = ManagedProcess::new("backoff".into(), cfg);

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
        let mut cfg = make_config("/bin/true", vec![]);
        cfg.restart = RestartPolicy::Always;
        cfg.restart_sec = Some(1.0);
        cfg.runtime_success_sec = Some(0);
        let mut proc = ManagedProcess::new("reset".into(), cfg);

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
        let cfg = make_config("/bin/true", vec![]);
        let proc = ManagedProcess::new("defaults".into(), cfg);
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
}
