// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! par-control installation, configuration, and executor-lifecycle scaffold.

use anyhow::Result;
use clap::Parser;
use par_control::bootstrap;
use par_control::config::{LaunchGate, log_level_from_yaml_file};
use par_control::platform;
use par_control::procmgr::ProcmgrLifecycle;
use std::path::PathBuf;
use std::process::ExitCode;

#[derive(Parser)]
#[command(name = "par-control", about = "Private Action Runner control plane")]
struct Cli {
    #[arg(short = 'c', long, default_value = platform::default_config_path())]
    config: PathBuf,

    /// Existing Go Private Action Runner binary used to resolve the Agent's
    /// effective configuration, including secret-backend values.
    #[arg(long = "config-helper")]
    config_helper: PathBuf,

    /// Go one-shot enrollment command. Must be the last option because it
    /// consumes all remaining arguments.
    #[arg(long = "enroll-command", num_args = 1.., allow_hyphen_values = true)]
    enroll_command: Vec<String>,
}

#[tokio::main]
async fn main() -> ExitCode {
    // Report the failure through the logger only. Returning `Err` from `main`
    // would also have anyhow print it to stderr, which dd-procmgrd captures
    // alongside the log, duplicating every failure.
    let code = match run().await {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            log::error!("par-control failed: {error:#}");
            ExitCode::FAILURE
        }
    };
    log::logger().flush();
    code
}

async fn run() -> Result<()> {
    let cli = Cli::parse();

    if let Err(error) = dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        level: log_level_from_yaml_file(&cli.config),
        log_file: platform::default_log_file(),
    }) {
        eprintln!("par-control: could not initialize the logger: {error}");
    }

    // The process definition is installed unconditionally; inactive hosts exit 0.
    let gate = LaunchGate::from_yaml_file(&cli.config)?;
    if !gate.split_mode {
        log::info!("private_action_runner split mode is disabled; par-control is exiting");
        return Ok(());
    }

    let config = bootstrap::load_config_with_bootstrap(
        &cli.config,
        &cli.config_helper,
        &cli.enroll_command,
        gate.self_enroll,
    )?;

    let lifecycle =
        ProcmgrLifecycle::new(&config.procmgr_socket, config.executor_process_name.clone());
    lifecycle.ensure_started().await?;

    tokio::select! {
        _ = shutdown_signal() => {
            // Do not call back into dd-procmgrd here. It serializes every RPC
            // through the loop in `ProcessManager::run`, and by the time we get
            // a stop signal that loop is either gone (daemon shutdown stops the
            // gRPC server before stopping processes) or blocked holding the
            // process write lock inside `handle_stop`, waiting for this very
            // process to exit. Either way an RPC would deadlock until
            // `stop_timeout` expires and the job object kills us. dd-procmgrd
            // owns both processes and stops the executor itself.
            log::info!("par-control is exiting");
            Ok(())
        }
        state = lifecycle.wait_for_failure() => {
            // Only a genuine failure lands here; a clean exit or an explicit stop
            // is the on-demand executor's normal idle path.
            Err(anyhow::anyhow!("executor failed with state {:?}", state?))
        }
    }
}

#[cfg(unix)]
async fn shutdown_signal() {
    use tokio::signal::unix::{SignalKind, signal};
    match signal(SignalKind::terminate()) {
        Ok(mut term) => {
            tokio::select! {
                _ = tokio::signal::ctrl_c() => {},
                _ = term.recv() => {},
            }
        }
        Err(_) => {
            let _ = tokio::signal::ctrl_c().await;
        }
    }
}

/// dd-procmgrd stops children with `GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT)`
/// (see `send_graceful_stop` in `pkg/procmgr/rust/src/platform/windows.rs`), so
/// CTRL_BREAK is the event that matters in production; CTRL_C only covers
/// interactive runs. Missing CTRL_BREAK would mean waiting out `stop_timeout`
/// and being force-killed with the job object.
#[cfg(windows)]
async fn shutdown_signal() {
    match tokio::signal::windows::ctrl_break() {
        Ok(mut ctrl_break) => {
            tokio::select! {
                _ = tokio::signal::ctrl_c() => {},
                _ = ctrl_break.recv() => {},
            }
        }
        Err(error) => {
            log::warn!("could not listen for CTRL_BREAK, falling back to CTRL_C: {error}");
            let _ = tokio::signal::ctrl_c().await;
        }
    }
}
