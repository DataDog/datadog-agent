// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod config;
mod env;
mod grpc;
mod ordering;
mod process;
mod shutdown;
mod state;

use crate::config::NamedProcess;
use anyhow::Result;
use log::{info, warn};
use process::ManagedProcess;
use std::sync::Arc;
use tokio::signal::unix::{SignalKind, signal};
use tokio::sync::{RwLock, mpsc, oneshot};

struct ExitEvent {
    index: usize,
    status: std::process::ExitStatus,
}

#[derive(Clone)]
pub struct ProcessManager {
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
}

impl ProcessManager {
    pub(crate) fn new(processes: Vec<ManagedProcess>) -> Self {
        Self {
            processes: Arc::new(RwLock::new(processes)),
        }
    }

    pub async fn read(&self) -> tokio::sync::RwLockReadGuard<'_, Vec<ManagedProcess>> {
        self.processes.read().await
    }

    async fn wire_watchers(&self, exit_tx: &mpsc::UnboundedSender<ExitEvent>) {
        let mut procs = self.processes.write().await;
        for (i, proc) in procs.iter_mut().enumerate() {
            if proc.is_running() {
                spawn_watcher(i, proc, exit_tx.clone());
            }
        }
    }

    async fn handle_exit(&self, event: ExitEvent, restart_tx: &mpsc::UnboundedSender<usize>) {
        let mut procs = self.processes.write().await;
        let proc = &mut procs[event.index];
        info!("[{}] exited with {}", proc.name, event.status);
        proc.set_last_status(event.status);
        if let Some(delay) = proc.handle_restart() {
            let tx = restart_tx.clone();
            let idx = event.index;
            tokio::spawn(async move {
                tokio::time::sleep(delay).await;
                let _ = tx.send(idx);
            });
        }
    }

    async fn complete_restart(&self, index: usize, exit_tx: &mpsc::UnboundedSender<ExitEvent>) {
        let mut procs = self.processes.write().await;
        let proc = &mut procs[index];
        match proc.spawn() {
            Ok(()) => spawn_watcher(index, proc, exit_tx.clone()),
            Err(e) => warn!("[{}] restart failed: {e:#}", proc.name),
        }
    }

    async fn shutdown(&self, startup_order: &[usize]) {
        let mut procs = self.processes.write().await;
        let shutdown_order: Vec<usize> = startup_order.iter().copied().rev().collect();
        shutdown::shutdown_ordered(&mut procs, &shutdown_order).await;
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    simple_logger::init_with_level(log::Level::Info)?;
    info!(
        "dd-procmgrd starting (version {})",
        env!("CARGO_PKG_VERSION")
    );

    let configs = load_configs();
    let startup_order = resolve_startup_order(&configs);
    let processes = start_processes(configs, &startup_order);
    let mgr = ProcessManager::new(processes);

    let (grpc_shutdown_tx, grpc_shutdown_rx) = oneshot::channel::<()>();
    let config_path = config::config_dir().display().to_string();
    let grpc_handle = tokio::spawn(grpc::server::run(
        mgr.clone(),
        config_path,
        grpc_shutdown_rx,
    ));

    let (exit_tx, mut exit_rx) = mpsc::unbounded_channel::<ExitEvent>();
    let (restart_tx, mut restart_rx) = mpsc::unbounded_channel::<usize>();
    mgr.wire_watchers(&exit_tx).await;

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
                mgr.handle_exit(event, &restart_tx).await;
            }
            Some(index) = restart_rx.recv() => {
                mgr.complete_restart(index, &exit_tx).await;
            }
        }
    }

    info!("dd-procmgrd shutting down");

    let _ = grpc_shutdown_tx.send(());
    match grpc_handle.await {
        Ok(Err(e)) => warn!("gRPC server error: {e}"),
        Err(e) => warn!("gRPC server task panicked: {e}"),
        Ok(Ok(())) => {}
    }

    mgr.shutdown(&startup_order).await;
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
