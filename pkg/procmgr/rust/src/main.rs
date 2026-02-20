// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod config;
mod process;

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

    let mut processes = start_processes()?;

    let mut sigterm = signal(SignalKind::terminate())?;
    let mut sigint = signal(SignalKind::interrupt())?;

    tokio::select! {
        _ = sigterm.recv() => info!("received SIGTERM"),
        _ = sigint.recv() => info!("received SIGINT"),
    }

    info!("dd-procmgrd shutting down");
    process::shutdown_all(&mut processes, process::DEFAULT_STOP_TIMEOUT).await;
    info!("dd-procmgrd stopped");
    Ok(())
}

fn start_processes() -> Result<Vec<ManagedProcess>> {
    let config_dir = config::config_dir();
    let mut processes = Vec::new();

    if !config_dir.is_dir() {
        info!(
            "config directory {} does not exist, no processes to manage",
            config_dir.display()
        );
        return Ok(processes);
    }

    let configs = config::load_configs(&config_dir)?;
    info!(
        "loaded {} process config(s) from {}",
        configs.len(),
        config_dir.display()
    );

    for (name, cfg) in configs {
        let mut proc = ManagedProcess::new(name, cfg);
        if proc.should_start()
            && let Err(e) = proc.spawn()
        {
            warn!("{e:#}");
        }
        processes.push(proc);
    }

    Ok(processes)
}
