// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod config;
mod env;
mod process;
mod shutdown;
mod state;

use crate::config::ProcessConfig;
use anyhow::Result;
use log::{info, warn};
use process::ManagedProcess;
use tokio::signal::unix::{SignalKind, signal};
use tokio::sync::mpsc;

struct ExitEvent {
    index: usize,
    status: std::process::ExitStatus,
}

#[tokio::main]
async fn main() -> Result<()> {
    simple_logger::init_with_level(log::Level::Info)?;
    info!(
        "dd-procmgrd starting (version {})",
        env!("CARGO_PKG_VERSION")
    );

    let configs = load_configs();
    let mut processes = start_processes(configs);

    let (exit_tx, mut exit_rx) = mpsc::unbounded_channel::<ExitEvent>();
    for (i, proc) in processes.iter_mut().enumerate() {
        if proc.is_running() {
            spawn_watcher(i, proc, exit_tx.clone());
        }
    }

    let mut sigterm = signal(SignalKind::terminate())?;
    let mut sigint = signal(SignalKind::interrupt())?;

    loop {
        tokio::select! {
            _ = sigterm.recv() => {
                info!("received SIGTERM");
                break;
            }
            _ = sigint.recv() => {
                info!("received SIGINT");
                break;
            }
            Some(event) = exit_rx.recv() => {
                let proc = &mut processes[event.index];
                info!("[{}] exited with {}", proc.name, event.status);
                proc.set_last_status(event.status);
                if proc.handle_restart().await {
                    spawn_watcher(event.index, proc, exit_tx.clone());
                }
            }
        }
    }

    info!("dd-procmgrd shutting down");
    shutdown::shutdown_all(&mut processes).await;
    info!("dd-procmgrd stopped");
    Ok(())
}

/// Spawn a background task that awaits the child's exit and sends the result.
fn spawn_watcher(index: usize, proc: &mut ManagedProcess, tx: mpsc::UnboundedSender<ExitEvent>) {
    if let Some(child) = proc.take_child() {
        let name = proc.name.clone();
        tokio::spawn(async move {
            let mut child = child;
            match child.wait().await {
                Ok(status) => {
                    let _ = tx.send(ExitEvent { index, status });
                }
                Err(e) => {
                    warn!("[{name}] wait error: {e}");
                }
            }
        });
    }
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
