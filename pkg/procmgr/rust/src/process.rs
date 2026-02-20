// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::ProcessConfig;
use anyhow::{Context, Result};
use log::{info, warn};
use nix::sys::signal::{self, Signal};
use nix::unistd::Pid;
use std::process::Stdio;
use tokio::process::{Child, Command};
use tokio::time::{Duration, timeout};

pub const DEFAULT_STOP_TIMEOUT: Duration = Duration::from_secs(90);

pub struct ManagedProcess {
    pub name: String,
    config: ProcessConfig,
    child: Option<Child>,
}

impl ManagedProcess {
    pub fn new(name: String, config: ProcessConfig) -> Self {
        Self {
            name,
            config,
            child: None,
        }
    }

    /// Check `condition_path_exists` and return false if the path is missing.
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
        let mut cmd = Command::new(&self.config.command);
        cmd.args(&self.config.args);

        for (k, v) in &self.config.env {
            cmd.env(k, v);
        }

        if let Some(ref dir) = self.config.working_dir {
            cmd.current_dir(dir);
        }

        cmd.stdout(stdio_from_str(&self.config.stdout));
        cmd.stderr(stdio_from_str(&self.config.stderr));

        let child = cmd
            .spawn()
            .with_context(|| format!("[{}] failed to spawn: {}", self.name, self.config.command))?;

        let pid = child.id().unwrap_or(0);
        info!(
            "[{}] spawned (pid={}, cmd={})",
            self.name, pid, self.config.command
        );
        self.child = Some(child);
        Ok(())
    }

    pub fn is_running(&self) -> bool {
        self.child.is_some()
    }

    pub fn send_signal(&self, sig: Signal) {
        if let Some(ref child) = self.child
            && let Some(pid) = child.id()
            && let Err(e) = signal::kill(Pid::from_raw(pid as i32), sig)
        {
            warn!("[{}] failed to send {sig}: {e}", self.name);
        }
    }

    /// Wait for the child to exit. Returns the exit status.
    pub async fn wait(&mut self) -> Result<std::process::ExitStatus> {
        let child = self.child.as_mut().context("no child process to wait on")?;
        let status = child.wait().await?;
        info!("[{}] exited with {status}", self.name);
        self.child = None;
        Ok(status)
    }
}

fn stdio_from_str(s: &str) -> Stdio {
    match s {
        "null" => Stdio::null(),
        _ => Stdio::inherit(),
    }
}

/// Send SIGTERM to all running processes, wait up to `stop_timeout`, then SIGKILL stragglers.
pub async fn shutdown_all(processes: &mut [ManagedProcess], stop_timeout: Duration) {
    for proc in processes.iter() {
        if proc.is_running() {
            info!("[{}] sending SIGTERM", proc.name);
            proc.send_signal(Signal::SIGTERM);
        }
    }

    let wait_all = async {
        for proc in processes.iter_mut() {
            if proc.is_running() {
                let _ = proc.wait().await;
            }
        }
    };

    if timeout(stop_timeout, wait_all).await.is_err() {
        warn!(
            "shutdown timeout ({}s) reached, sending SIGKILL",
            stop_timeout.as_secs()
        );
        for proc in processes.iter() {
            if proc.is_running() {
                info!("[{}] sending SIGKILL", proc.name);
                proc.send_signal(Signal::SIGKILL);
            }
        }
        for proc in processes.iter_mut() {
            if proc.is_running() {
                let _ = proc.wait().await;
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::ProcessConfig;
    use std::collections::HashMap;

    fn make_config(command: &str, args: Vec<&str>) -> ProcessConfig {
        ProcessConfig {
            description: None,
            command: command.to_string(),
            args: args.into_iter().map(String::from).collect(),
            env: HashMap::new(),
            working_dir: None,
            pidfile: None,
            stdout: "null".to_string(),
            stderr: "null".to_string(),
            auto_start: true,
            condition_path_exists: None,
        }
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
        assert!(!proc.is_running());
    }

    #[tokio::test]
    async fn test_spawn_nonexistent_binary() {
        let cfg = make_config("/nonexistent/binary", vec![]);
        let mut proc = ManagedProcess::new("bad".into(), cfg);
        assert!(proc.spawn().is_err());
        assert!(!proc.is_running());
    }

    #[tokio::test]
    async fn test_spawn_with_env() {
        let mut cfg = make_config("/bin/sh", vec!["-c", "exit $MY_EXIT_CODE"]);
        cfg.env.insert("MY_EXIT_CODE".to_string(), "42".to_string());
        cfg.stdout = "null".to_string();
        cfg.stderr = "null".to_string();

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

    // -- shutdown_all tests --

    #[tokio::test]
    async fn test_shutdown_all_graceful() {
        let cfg1 = make_config("/bin/sleep", vec!["60"]);
        let cfg2 = make_config("/bin/sleep", vec!["60"]);

        let mut p1 = ManagedProcess::new("p1".into(), cfg1);
        let mut p2 = ManagedProcess::new("p2".into(), cfg2);
        p1.spawn().unwrap();
        p2.spawn().unwrap();

        let mut procs = vec![p1, p2];
        shutdown_all(&mut procs, Duration::from_secs(5)).await;

        assert!(!procs[0].is_running());
        assert!(!procs[1].is_running());
    }

    #[tokio::test]
    async fn test_shutdown_all_empty() {
        let mut procs: Vec<ManagedProcess> = vec![];
        shutdown_all(&mut procs, Duration::from_secs(1)).await;
    }

    #[tokio::test]
    async fn test_shutdown_all_sigkill_on_timeout() {
        let cfg = make_config("/bin/sh", vec!["-c", "trap '' TERM; sleep 60"]);
        let mut proc = ManagedProcess::new("stubborn".into(), cfg);
        proc.spawn().unwrap();

        let mut procs = vec![proc];
        shutdown_all(&mut procs, Duration::from_secs(1)).await;

        assert!(!procs[0].is_running());
    }
}
