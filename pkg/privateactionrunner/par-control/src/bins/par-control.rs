// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use clap::Parser;
use par_control::config::Config;
use par_control::platform;
use par_control::procmgr::ProcmgrLifecycle;
use std::path::PathBuf;
use std::process::ExitCode;

#[derive(Parser)]
#[command(name = "par-control", about = "Private Action Runner control plane")]
struct Cli {
    #[arg(short = 'c', long, default_value = platform::default_config_path())]
    config: PathBuf,
}

#[tokio::main]
async fn main() -> ExitCode {
    // Avoid logging errors twice through both anyhow and dd-procmgrd's stderr capture.
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
    let config = Config::from_yaml_file(&cli.config);

    let log_level = config
        .as_ref()
        .map_or(log::LevelFilter::Info, |c| c.log_level);
    if let Err(error) = dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        // The logger requires a Level; the filter below handles Off.
        level: log_level.to_level().unwrap_or(log::Level::Error),
        log_file: platform::default_log_file(),
    }) {
        eprintln!("par-control: could not initialize the logger: {error}");
    }
    log::set_max_level(log_level);
    let config = config?;

    if !config.split_mode {
        log::info!("private_action_runner split mode is disabled; par-control is exiting");
        return Ok(());
    }

    let lifecycle = ProcmgrLifecycle::from_env();
    lifecycle.ensure_started().await?;

    tokio::select! {
        _ = shutdown_signal() => {
            // Do not call dd-procmgrd here: its command loop may be waiting for
            // par-control to exit while holding the process lock.
            log::info!("par-control is exiting");
            Ok(())
        }
        state = lifecycle.wait_for_failure() => {
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

/// Listen for the CTRL_BREAK event dd-procmgrd uses for graceful Windows stops.
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
