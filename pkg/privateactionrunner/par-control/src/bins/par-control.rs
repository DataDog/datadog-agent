// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use clap::Parser;
use par_control::config::Config;
use std::path::PathBuf;

#[derive(Parser)]
#[command(name = "par-control", about = "Private Action Runner control plane")]
struct Cli {
    #[arg(short = 'c', long, default_value = "/etc/datadog-agent/datadog.yaml")]
    config: PathBuf,
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();
    let config = Config::from_yaml_file(&cli.config);

    let log_level = config
        .as_ref()
        .map_or(log::LevelFilter::Info, |c| c.log_level);
    if let Err(error) = dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        // The logger API requires a Level. Use Error for Off during
        // initialization, then apply the exact filter below.
        level: log_level.to_level().unwrap_or(log::Level::Error),
        log_file: None,
    }) {
        eprintln!("par-control: could not initialize the logger: {error}");
    }
    log::set_max_level(log_level);
    let config = config?;

    if !config.enabled {
        log::info!("PAR is disabled; par-control is exiting");
        log::logger().flush();
        return Ok(());
    }

    log::info!("par-control started");
    shutdown_signal().await;
    log::info!("par-control exiting");
    log::logger().flush();
    Ok(())
}

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
