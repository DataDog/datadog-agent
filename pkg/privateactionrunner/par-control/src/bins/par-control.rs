// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use clap::Parser;
use par_control::bootstrap;
use std::process::ExitCode;

#[derive(Parser)]
#[command(name = "par-control", about = "Private Action Runner control plane")]
struct Cli {
    /// The Go command that resolves the control-plane configuration. It receives
    /// the Agent config path, so par-control takes no --config of its own.
    #[arg(
        long = "bootstrap-command",
        num_args = 1..,
        allow_hyphen_values = true
    )]
    bootstrap_command: Vec<String>,
}

#[tokio::main]
async fn main() -> ExitCode {
    // Return ExitCode so main does not print the error a second time.
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

    // The logger's level filter is immutable after initialization, so it starts
    // at Trace and the configured level is applied with set_max_level once
    // bootstrap reports it. Until then, bootstrap's own logs are not filtered.
    if let Err(error) = dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        level: log::Level::Trace,
        // dd-procmgrd redirects stdout and stderr per its process definition.
        log_file: None,
    }) {
        eprintln!("par-control: could not initialize the logger: {error}");
    }
    log::set_max_level(log::LevelFilter::Info);

    let bootstrapped = bootstrap::run_bootstrap(&cli.bootstrap_command)?;
    log::set_max_level(bootstrapped.log_level());

    if !bootstrapped.split_mode {
        log::info!(
            "private_action_runner.split_enabled is not enabled; \
             the monolithic runner owns OPMS polling. Exiting."
        );
        return Ok(());
    }

    let _config = bootstrapped.into_config()?;

    log::info!("par-control started");
    shutdown_signal().await;
    log::info!("par-control is exiting");
    Ok(())
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

/// Handle CTRL_BREAK, which dd-procmgrd uses for graceful Windows stops.
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
