// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Correctness — mirrors system-probe-lite lints.
#![deny(clippy::indexing_slicing)]
#![deny(clippy::string_slice)]
#![deny(clippy::cast_possible_wrap)]
#![deny(clippy::undocumented_unsafe_blocks)]
// Panicking code
#![deny(clippy::unwrap_used)]
#![deny(clippy::expect_used)]
#![deny(clippy::panic)]
#![deny(clippy::unimplemented)]
#![deny(clippy::todo)]
// Debug code
#![deny(clippy::dbg_macro)]
#![deny(clippy::print_stdout)]
#![deny(clippy::print_stderr)]

use std::env;
use std::path::PathBuf;
use std::time::Duration;

use anyhow::{Context, Result};
use log::{error, info, warn};
use tokio::signal::unix::{SignalKind, signal};

mod cli;
mod config;
mod executor;
mod executor_client;
mod jwt;
mod opms;
mod par_config;

use cli::Args;
use config::Config;
use executor::ExecutorManager;
use jwt::load_signing_key;
use opms::OPMSClient;
use par_config::ParConfig;

// ── PID file helpers (identical to system-probe-lite) ──────────────────────

fn remove_pid_file(path: &std::path::Path) {
    if let Err(e) = std::fs::remove_file(path) {
        error!("Failed to remove PID file: {}", e);
    } else {
        info!("Removed PID file at {}", path.display());
    }
}

// ── Capability dropping ─────────────────────────────────────────────────────

/// Drop all Linux capabilities from the control plane.
/// The control plane has no need for elevated privileges — it only polls OPMS
/// over HTTPS and manages a child process.  Capabilities required for action
/// execution are held by the executor binary (via file capabilities).
#[cfg(target_os = "linux")]
fn drop_capabilities() {
    use caps::{CapSet, all};
    match caps::clear(None, CapSet::Effective)
        .and_then(|_| caps::clear(None, CapSet::Permitted))
        .and_then(|_| caps::clear(None, CapSet::Inheritable))
    {
        Ok(()) => info!("par-control: dropped all capabilities"),
        Err(e) => warn!("par-control: failed to drop capabilities: {e}"),
    }
    // Lower bounding set to prevent re-acquisition.
    for cap in all() {
        let _ = caps::drop(None, CapSet::Bounding, cap);
    }
}

#[cfg(not(target_os = "linux"))]
fn drop_capabilities() {}

// ── Main control loop ───────────────────────────────────────────────────────

/// Resolves the executor binary path.
/// Default: par-executor binary sibling of this process, matching the PAR
/// container layout where both binaries ship in the same image.
fn resolve_executor_binary(cli_path: Option<PathBuf>) -> Result<PathBuf> {
    if let Some(p) = cli_path {
        return Ok(p);
    }
    let exe = std::env::current_exe().context("cannot determine own executable path")?;
    let sibling = exe
        .parent()
        .ok_or_else(|| anyhow::anyhow!("executable has no parent directory"))?
        .join("par-executor");
    Ok(sibling)
}

async fn run(cfg: Config) -> Result<()> {
    let signing_key =
        load_signing_key(&cfg.private_key_b64).context("failed to load signing key")?;

    let opms = OPMSClient::new(
        &cfg.dd_api_host,
        cfg.org_id,
        cfg.runner_id.clone(),
        signing_key,
    )
    .context("failed to create OPMS client")?;

    let executor = ExecutorManager::new(
        cfg.executor_binary.clone(),
        cfg.executor_socket.clone(),
        cfg.executor_idle_timeout,
        cfg.executor_start_timeout,
    );

    info!(
        "par-control: starting OPMS polling (host={}, runner={})",
        cfg.dd_api_host, cfg.runner_id,
    );

    // Spawn watchdog: periodically health-pings the executor while it's running.
    {
        let executor_w = executor.clone();
        let health_interval = Duration::from_secs(30);
        tokio::spawn(async move {
            let mut consecutive_failures: u32 = 0;
            loop {
                tokio::time::sleep(health_interval).await;
                if !executor_w.is_healthy().await {
                    consecutive_failures += 1;
                    warn!(
                        "par-control: executor health check failed ({}/3)",
                        consecutive_failures
                    );
                    if consecutive_failures >= 3 {
                        error!("par-control: executor unresponsive, killing it");
                        executor_w.kill().await;
                        consecutive_failures = 0;
                    }
                } else {
                    consecutive_failures = 0;
                }
            }
        });
    }

    loop {
        let task = match opms.dequeue_task().await {
            Ok(Some(t)) => t,
            Ok(None) => {
                // Empty queue — back off and retry.
                tokio::time::sleep(cfg.loop_interval).await;
                continue;
            }
            Err(e) => {
                error!("par-control: dequeue_task failed: {e}");
                tokio::time::sleep(cfg.loop_interval).await;
                continue;
            }
        };

        info!(
            "par-control: dequeued task {} ({})",
            task.task_id, task.action_fqn
        );

        let opms_t = opms.clone();
        let executor_t = executor.clone();
        let task_timeout = cfg.task_timeout;
        let heartbeat_interval = cfg.heartbeat_interval;

        tokio::spawn(async move {
            handle_task(task, opms_t, executor_t, task_timeout, heartbeat_interval).await;
        });
    }
}

/// Handles a single dequeued task:
/// 1. Ensures executor is running (spawning if needed).
/// 2. Dispatches to executor via POST /execute.
/// 3. Sends heartbeats to OPMS while waiting for the response.
/// 4. Publishes success or failure back to OPMS.
async fn handle_task(
    task: opms::Task,
    opms: OPMSClient,
    executor: ExecutorManager,
    task_timeout: Duration,
    heartbeat_interval: Duration,
) {
    // Ensure executor is running (may spawn it).
    if let Err(e) = executor.ensure_running().await {
        error!("par-control: failed to start executor for task {}: {e}", task.task_id);
        if let Err(pe) = opms.publish_failure(&task, 1, &e.to_string()).await {
            error!("par-control: publish_failure failed: {pe}");
        }
        return;
    }

    // Heartbeat goroutine — runs concurrently with /execute.
    let heartbeat_task = {
        let opms_hb = opms.clone();
        let task_hb = task.clone();
        tokio::spawn(async move {
            loop {
                tokio::time::sleep(heartbeat_interval).await;
                if let Err(e) = opms_hb.heartbeat(&task_hb).await {
                    warn!("par-control: heartbeat failed for {}: {e}", task_hb.task_id);
                }
            }
        })
    };

    // Dispatch.
    let result = executor
        .client()
        .execute(&task.raw, task_timeout)
        .await;

    heartbeat_task.abort();

    match result {
        Ok(resp) if resp.error_code == 0 => {
            let output = resp.output.unwrap_or(serde_json::Value::Null);
            if let Err(e) = opms.publish_success(&task, &output).await {
                error!("par-control: publish_success failed for {}: {e}", task.task_id);
            }
        }
        Ok(resp) => {
            if let Err(e) = opms.publish_failure(&task, resp.error_code, &resp.error_details).await {
                error!("par-control: publish_failure failed for {}: {e}", task.task_id);
            }
        }
        Err(e) => {
            error!("par-control: executor dispatch failed for {}: {e}", task.task_id);
            if let Err(pe) = opms.publish_failure(&task, 1, &e.to_string()).await {
                error!("par-control: publish_failure failed: {pe}");
            }
        }
    }
}

// ── Entry point ─────────────────────────────────────────────────────────────

#[tokio::main(flavor = "current_thread")]
async fn main() -> Result<()> {
    let args = Args::parse(env::args()).context("failed to parse arguments")?;

    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        level: args.log_level,
        log_file: args.log_file.clone(),
    })
    .context("failed to initialize logger")?;

    info!("par-control starting");
    for arg in &args.unknown_args {
        warn!("unknown argument: {arg}");
    }

    // Read PAR config from datadog.yaml.
    let par_cfg = ParConfig::from_file(&args.config_path)
        .with_context(|| format!("failed to load config from {}", args.config_path.display()))?;

    let executor_binary = resolve_executor_binary(args.executor_binary)
        .context("failed to resolve executor binary path")?;

    info!("par-control: executor binary = {}", executor_binary.display());
    info!("par-control: executor socket = {}", args.executor_socket);

    let cfg = Config::new(executor_binary, args.executor_socket, par_cfg);

    drop_capabilities();

    // Signal handling — identical to system-probe-lite.
    let mut sigterm = signal(SignalKind::terminate()).context("failed to setup SIGTERM handler")?;
    let mut sigint = signal(SignalKind::interrupt()).context("failed to setup SIGINT handler")?;

    let result = tokio::select! {
        _ = sigterm.recv() => { info!("par-control: received SIGTERM, shutting down"); Ok(()) }
        _ = sigint.recv()  => { info!("par-control: received SIGINT, shutting down");  Ok(()) }
        r = run(cfg)       => r
    };

    // Cleanup PID file on exit — mirrors system-probe-lite.
    if let Some(path) = args.pid_path {
        remove_pid_file(&path);
    }

    result
}
