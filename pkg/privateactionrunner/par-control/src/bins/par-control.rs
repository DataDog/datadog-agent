// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use clap::Parser;
use par_control::config::Launch;
use par_control::platform;
use std::path::PathBuf;
use std::process::ExitCode;

#[derive(Parser)]
#[command(name = "par-control", about = "Private Action Runner control plane")]
struct Cli {
    #[arg(short = 'c', long, default_value_os_t = platform::default_config_path())]
    config: PathBuf,
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
    // Resolve the log level first so a config error is logged at that level.
    let launch = Launch::from_yaml_file(&cli.config);
    let log_level = launch
        .as_ref()
        .map_or(log::LevelFilter::Info, |launch| launch.log_level);
    if let Err(error) = dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        level: log_level.to_level().unwrap_or(log::Level::Error),
        // dd-procmgrd redirects stdout and stderr per its process definition.
        log_file: None,
    }) {
        eprintln!("par-control: could not initialize the logger: {error}");
    }
    log::set_max_level(log_level);

    if !launch?.gate.split_mode {
        log::info!("private_action_runner split mode is disabled; par-control is exiting");
        return Ok(());
    }

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
