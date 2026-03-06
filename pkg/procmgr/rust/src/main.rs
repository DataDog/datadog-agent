// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod config;
mod env;
mod ordering;
mod process;
mod shutdown;
mod state;

use crate::config::NamedProcess;
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
    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PROCMGR",
        level: log::Level::Info,
        log_file: None,
    })?;
    info!(
        "dd-procmgrd starting (version {})",
        env!("CARGO_PKG_VERSION")
    );

    let configs = load_configs();
    let startup_order = resolve_startup_order(&configs);
    let mut processes = start_processes(configs, &startup_order);

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
    let shutdown_order: Vec<usize> = startup_order.iter().copied().rev().collect();
    shutdown::shutdown_ordered(&mut processes, &shutdown_order).await;
    info!("dd-procmgrd stopped");
    Ok(())
}

/// Spawn a background task that awaits the child's exit and sends the result.
fn spawn_watcher(index: usize, proc: &mut ManagedProcess, tx: mpsc::UnboundedSender<ExitEvent>) {
    if let Some(child) = proc.take_child() {
        let name = proc.name.clone();
        let handle = tokio::spawn(async move {
            let mut child = child;
            match child.wait().await {
                Ok(status) => {
                    let _ = tx.send(ExitEvent { index, status });
                }
                Err(e) => {
                    warn!("[{name}] wait error: {e}, killing process");
                    let _ = child.kill().await;
                }
            }
        });
        proc.set_watcher_handle(handle);
    }
}

fn load_configs() -> Vec<NamedProcess> {
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

fn resolve_startup_order(configs: &[NamedProcess]) -> Vec<usize> {
    let result = ordering::resolve_order(configs);
    if !result.skipped.is_empty() {
        warn!(
            "dependency cycle detected, skipping processes: {}",
            result.skipped.join(", ")
        );
    }
    let names: Vec<&str> = result
        .order
        .iter()
        .map(|&i| configs[i].0.as_str())
        .collect();
    info!("startup order: {}", names.join(" -> "));
    result.order
}

fn start_processes(configs: Vec<NamedProcess>, startup_order: &[usize]) -> Vec<ManagedProcess> {
    let mut processes: Vec<ManagedProcess> = configs
        .into_iter()
        .map(|(name, cfg)| ManagedProcess::new(name, cfg))
        .collect();

    for &idx in startup_order {
        let proc = &mut processes[idx];
        if proc.should_start()
            && let Err(e) = proc.spawn()
        {
            warn!("{e:#}");
        }
    }
    processes
}
