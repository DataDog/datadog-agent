// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod config;
mod process;

use crate::config::ProcessConfig;
use anyhow::Result;
use log::{info, warn};
use process::ManagedProcess;
use tokio::signal::unix::{SignalKind, signal};

#[tokio::main]
async fn main() -> Result<()> {
    simple_logger::init_with_level(log::Level::Info)?;
    info!(
        "dd-procmgrd starting (version {})",
        env!("CARGO_PKG_VERSION")
    );

    let configs = load_configs();
    let mut processes = start_processes(configs);

    let mut sigterm = signal(SignalKind::terminate())?;
    let mut sigint = signal(SignalKind::interrupt())?;

    tokio::select! {
        _ = sigterm.recv() => info!("received SIGTERM"),
        _ = sigint.recv() => info!("received SIGINT"),
    }

    info!("dd-procmgrd shutting down");
    process::shutdown_all(&mut processes).await;
    info!("dd-procmgrd stopped");
    Ok(())
}

fn load_configs() -> Vec<(String, ProcessConfig)> {
    let config_dir = config::config_dir();

    if !config_dir.is_dir() {
        info!(
            "config directory {} does not exist, no processes to manage",
            config_dir.display()
        );
        return Vec::new();
    }

    let configs = match config::load_configs(&config_dir) {
        Ok(c) => c,
        Err(e) => {
            warn!(
                "cannot read config directory {}: {e:#}",
                config_dir.display()
            );
            return Vec::new();
        }
    };
    info!(
        "loaded {} process config(s) from {}",
        configs.len(),
        config_dir.display()
    );
    configs
}

fn start_processes(configs: Vec<(String, ProcessConfig)>) -> Vec<ManagedProcess> {
    let mut processes = Vec::new();
    for (name, cfg) in configs {
        let mut proc = ManagedProcess::new(name, cfg);
        if proc.should_start()
            && let Err(e) = proc.spawn()
        {
            warn!("{e:#}");
        }
        processes.push(proc);
    }
    processes
}
