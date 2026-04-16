// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Executor lifecycle management.
//!
//! Implements the state machine from the PAR dual-process design:
//!
//!   IDLE ──── task arrives ──→ STARTING (fork+exec par-executor)
//!     ↑                            │ poll GET /debug/ready
//!     │                            ↓
//!     └── executor exits ←── RUNNING (POST /execute, heartbeats)
//!
//! The executor is a shared, lock-protected resource.  Multiple concurrent
//! task-dispatch tokio tasks call `ensure_running` before dispatching.

use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use log::{error, info, warn};
use tokio::process::{Child, Command};
use tokio::sync::Mutex;

use crate::executor_client::ExecutorClient;

// ── Executor state ──────────────────────────────────────────────────────────

enum State {
    Idle,
    Running(Child),
}

// ── ExecutorManager ─────────────────────────────────────────────────────────

/// Manages the lifecycle of the par-executor child process.
/// Clone-safe via Arc — multiple tokio tasks share one instance.
#[derive(Clone)]
pub struct ExecutorManager {
    state: Arc<Mutex<State>>,
    binary: PathBuf,
    socket_path: String,
    idle_timeout: u32,
    start_timeout: Duration,
    client: Arc<ExecutorClient>,
    /// Forwarded to par-executor as --cfgpath (datadog.yaml path).
    cfgpath: Option<std::path::PathBuf>,
    /// Forwarded to par-executor as --extracfgpath (-E) for each entry.
    extracfg: Vec<std::path::PathBuf>,
    /// Enrolled identity injected as env vars so par-executor can load the
    /// identity without it being pre-written to datadog.yaml.
    pub urn: String,
    pub private_key_b64: String,
}

impl ExecutorManager {
    pub fn new(
        binary: PathBuf,
        socket_path: String,
        idle_timeout: u32,
        start_timeout: Duration,
        cfgpath: Option<std::path::PathBuf>,
        extracfg: Vec<std::path::PathBuf>,
    ) -> Self {
        let client = Arc::new(ExecutorClient::new(socket_path.clone()));
        ExecutorManager {
            state: Arc::new(Mutex::new(State::Idle)),
            binary,
            socket_path,
            idle_timeout,
            start_timeout,
            client,
            cfgpath,
            extracfg,
            urn: String::new(),
            private_key_b64: String::new(),
        }
    }

    /// Returns a reference to the UDS client for dispatching to the executor.
    pub fn client(&self) -> &ExecutorClient {
        &self.client
    }

    /// Ensures the executor process is running and ready to accept requests.
    ///
    /// If the process has exited (idle timeout) or was never started, spawns a
    /// new one and waits for GET /debug/ready to return 200 OK.
    pub async fn ensure_running(&self) -> Result<()> {
        let mut state = self.state.lock().await;

        // Check if existing process is still alive.
        if let State::Running(child) = &mut *state {
            match child.try_wait() {
                Ok(Some(exit)) => {
                    info!("par-executor exited ({}); will respawn on next task", exit);
                    *state = State::Idle;
                }
                Ok(None) => {
                    return Ok(()); // still running
                }
                Err(e) => {
                    warn!("failed to check executor status: {e}; treating as dead");
                    *state = State::Idle;
                }
            }
        }

        // Spawn fresh executor process.
        info!(
            "spawning par-executor: binary={} socket={}",
            self.binary.display(),
            self.socket_path
        );

        let mut cmd = Command::new(&self.binary);
        cmd.arg("run")
            .arg("--socket").arg(&self.socket_path)
            .arg("--idle-timeout-seconds").arg(self.idle_timeout.to_string());

        // Inject enrolled identity as env vars so par-executor can load
        // the URN and private key even when they're not in datadog.yaml
        // (e.g. after in-memory self-enrollment by par-control).
        if !self.urn.is_empty() {
            cmd.env("DD_PRIVATE_ACTION_RUNNER_URN", &self.urn);
        }
        if !self.private_key_b64.is_empty() {
            cmd.env("DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY", &self.private_key_b64);
        }
        // Also skip verification so par-executor doesn't wait for RC keys
        // when the identity was just enrolled (the executor will get fresh
        // keys on next reconciliation).
        cmd.env("DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION", "true");

        // Forward config paths so par-executor can load the allowlist,
        // rshell paths, and other PAR settings from datadog.yaml and
        // the operator-mounted privateactionrunner.yaml.
        if let Some(ref cfgpath) = self.cfgpath {
            cmd.arg("--cfgpath").arg(cfgpath);
        }
        for extra in &self.extracfg {
            cmd.arg("--extracfgpath").arg(extra);
        }

        let child = cmd
            .spawn()
            .with_context(|| {
                format!("failed to spawn par-executor: {}", self.binary.display())
            })?;

        *state = State::Running(child);
        drop(state); // release lock while waiting for ready

        self.wait_for_ready().await
    }

    /// Polls GET /debug/ready until the executor responds 200 OK or the
    /// start_timeout elapses.
    async fn wait_for_ready(&self) -> Result<()> {
        let deadline = tokio::time::Instant::now() + self.start_timeout;
        let poll_interval = Duration::from_millis(200);

        while tokio::time::Instant::now() < deadline {
            if self.client.ping().await {
                info!("par-executor is ready");
                return Ok(());
            }
            tokio::time::sleep(poll_interval).await;
        }

        // Timed out — kill the stuck process and reset to Idle.
        warn!("par-executor did not become ready within {:?}; killing", self.start_timeout);
        self.kill().await;
        anyhow::bail!(
            "par-executor did not signal ready within {:?}",
            self.start_timeout
        )
    }

    /// Health check ping. Called periodically by the watchdog task.
    /// Returns false if the executor is unresponsive; callers should kill it.
    pub async fn is_healthy(&self) -> bool {
        self.client.ping().await
    }

    /// Kills the executor process and resets to Idle state.
    pub async fn kill(&self) {
        let mut state = self.state.lock().await;
        if let State::Running(child) = &mut *state {
            if let Err(e) = child.kill().await {
                error!("failed to kill par-executor: {e}");
            }
        }
        *state = State::Idle;
    }
}
