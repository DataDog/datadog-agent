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

        cmd.env_clear();
        if let Some(ref path) = self.config.environment_file {
            match parse_environment_file(path) {
                Ok(vars) => {
                    for (k, v) in &vars {
                        cmd.env(k, v);
                    }
                }
                Err(e) => warn!(
                    "[{}] failed to read environment file {path}: {e}",
                    self.name
                ),
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

/// Parse a systemd-style environment file into key-value pairs.
/// Supports `KEY=VALUE`, `KEY="VALUE"`, `KEY='VALUE'`, comments (#), and blank lines.
fn parse_environment_file(path: &str) -> Result<Vec<(String, String)>> {
    let contents = std::fs::read_to_string(path)
        .with_context(|| format!("reading environment file: {path}"))?;
    let mut vars = Vec::new();
    for line in contents.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        if let Some((key, raw_val)) = trimmed.split_once('=') {
            let val = raw_val
                .trim()
                .trim_matches('"')
                .trim_matches('\'')
                .to_string();
            vars.push((key.trim().to_string(), val));
        }
    }
    Ok(vars)
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
            environment_file: None,
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
        cfg.stdout = "null".to_string();
        cfg.stderr = "null".to_string();

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
        cfg.stdout = "null".to_string();
        cfg.stderr = "null".to_string();

        let mut proc = ManagedProcess::new("override".into(), cfg);
        proc.spawn().unwrap();
        let status = proc.wait().await.unwrap();
        assert_eq!(
            status.code(),
            Some(0),
            "config env should override environment_file"
        );
    }

    #[test]
    fn test_parse_environment_file_full() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("env");
        std::fs::write(
            &path,
            r#"# Datadog env
DD_API_KEY=abc123
PATH="/usr/local/bin:/usr/bin"
QUOTED='single'
malformed line without equals
   
# blank lines above are skipped
LANG=en_US.UTF-8
"#,
        )
        .unwrap();

        let vars: HashMap<String, String> = parse_environment_file(path.to_str().unwrap())
            .unwrap()
            .into_iter()
            .collect();

        assert_eq!(vars["DD_API_KEY"], "abc123");
        assert_eq!(vars["PATH"], "/usr/local/bin:/usr/bin");
        assert_eq!(vars["QUOTED"], "single");
        assert_eq!(vars["LANG"], "en_US.UTF-8");
        assert_eq!(vars.len(), 4, "malformed line should be silently skipped");
    }

    #[test]
    fn test_parse_environment_file_missing() {
        assert!(parse_environment_file("/nonexistent/env").is_err());
    }
}
