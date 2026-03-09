// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::command::{Command, ReloadResult};
use crate::config::{self, ConfigLoader, ProcessDefinition};
use crate::grpc;
use crate::ordering;
use crate::process::{ManagedProcess, ProcessOrigin};
use crate::shutdown;
use anyhow::Result;
use log::{info, warn};
use std::sync::Arc;
use tokio::signal::unix::{SignalKind, signal};
use tokio::sync::{RwLock, mpsc, oneshot};
use tonic::Status;

pub(crate) struct ExitEvent {
    pub name: String,
    pub status: std::process::ExitStatus,
}

#[derive(Clone)]
pub struct ProcessManager {
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
    startup_order: Arc<Vec<usize>>,
    config_loader: Arc<dyn ConfigLoader>,
}

impl ProcessManager {
    pub(crate) fn new(config_loader: Arc<dyn ConfigLoader>) -> Self {
        let configs = config_loader.load();
        let startup_order = resolve_startup_order(&configs);
        let processes: Vec<ManagedProcess> = configs
            .into_iter()
            .map(|pd| ManagedProcess::new_config(pd.name, pd.config))
            .collect();
        Self {
            processes: Arc::new(RwLock::new(processes)),
            startup_order: Arc::new(startup_order),
            config_loader,
        }
    }

    async fn start(&self) {
        let mut procs = self.processes.write().await;
        for &idx in self.startup_order.iter() {
            let proc = &mut procs[idx];
            if proc.should_start()
                && let Err(e) = proc.spawn()
            {
                warn!("{e:#}");
            }
        }
    }

    pub(crate) async fn run(&self) -> Result<()> {
        self.start().await;

        let (cmd_tx, mut cmd_rx) = mpsc::channel::<Command>(64);
        let (grpc_shutdown_tx, grpc_shutdown_rx) = oneshot::channel::<()>();
        let grpc_handle = tokio::spawn(grpc::server::run(self.clone(), cmd_tx, grpc_shutdown_rx));

        let (exit_tx, mut exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<String>(256);
        self.wire_watchers(&exit_tx).await;

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
                    self.handle_exit(event, &restart_tx).await;
                }
                Some(name) = restart_rx.recv() => {
                    self.complete_restart(&name, &exit_tx).await;
                }
                Some(cmd) = cmd_rx.recv() => {
                    match cmd {
                        Command::Create { name, config, reply } => {
                            let _ = reply.send(self.handle_create(name, *config).await);
                        }
                        Command::Start { name, reply } => {
                            let _ = reply.send(self.handle_start(&name, &exit_tx).await);
                        }
                        Command::Stop { name, reply } => {
                            let _ = reply.send(self.handle_stop(&name).await);
                        }
                        Command::ReloadConfig { reply } => {
                            let _ = reply.send(self.handle_reload_config(&exit_tx).await);
                        }
                    }
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

        self.shutdown().await;
        info!("dd-procmgrd stopped");
        Ok(())
    }

    pub(crate) async fn processes(&self) -> tokio::sync::RwLockReadGuard<'_, Vec<ManagedProcess>> {
        self.processes.read().await
    }

    pub(crate) fn config_source(&self) -> &str {
        self.config_loader.source()
    }

    pub(crate) fn config_location(&self) -> String {
        self.config_loader.location()
    }

    async fn wire_watchers(&self, exit_tx: &mpsc::Sender<ExitEvent>) {
        let mut procs = self.processes.write().await;
        for proc in procs.iter_mut() {
            if proc.is_running() {
                spawn_watcher(proc, exit_tx.clone());
            }
        }
    }

    pub(crate) async fn handle_exit(&self, event: ExitEvent, restart_tx: &mpsc::Sender<String>) {
        let mut procs = self.processes.write().await;
        let Some(proc) = procs.iter_mut().find(|p| p.name() == event.name) else {
            warn!("exit event for unknown process '{}'", event.name);
            return;
        };
        if !proc.state().is_alive() {
            info!(
                "[{}] ignoring exit event (state: {})",
                proc.name(),
                proc.state()
            );
            return;
        }
        info!("[{}] exited with {}", proc.name(), event.status);
        proc.set_last_status(event.status);
        if let Some(delay) = proc.handle_restart() {
            let tx = restart_tx.clone();
            let name = event.name.clone();
            tokio::spawn(async move {
                tokio::time::sleep(delay).await;
                let _ = tx.send(name).await;
            });
        }
    }

    pub(crate) async fn complete_restart(&self, name: &str, exit_tx: &mpsc::Sender<ExitEvent>) {
        let mut procs = self.processes.write().await;
        let Some(proc) = procs.iter_mut().find(|p| p.name() == name) else {
            warn!("restart for unknown process '{name}'");
            return;
        };
        if proc.is_running() {
            info!("[{name}] already running, skipping queued restart");
            return;
        }
        match proc.spawn() {
            Ok(()) => spawn_watcher(proc, exit_tx.clone()),
            Err(e) => warn!("[{}] restart failed: {e:#}", proc.name()),
        }
    }

    pub(crate) async fn handle_create(
        &self,
        name: String,
        config: config::ProcessConfig,
    ) -> Result<(), Status> {
        if config.command.is_empty() {
            return Err(Status::invalid_argument("command must not be empty"));
        }
        let mut procs = self.processes.write().await;
        if procs.iter().any(|p| p.name() == name) {
            return Err(Status::already_exists(format!(
                "process '{name}' already exists"
            )));
        }
        let proc = ManagedProcess::new_runtime(name.clone(), config);
        info!("[{name}] created via RPC");
        procs.push(proc);
        Ok(())
    }

    pub(crate) async fn handle_start(
        &self,
        name: &str,
        exit_tx: &mpsc::Sender<ExitEvent>,
    ) -> Result<(), Status> {
        let mut procs = self.processes.write().await;
        let proc = procs
            .iter_mut()
            .find(|p| p.name() == name)
            .ok_or_else(|| Status::not_found(format!("process '{name}' not found")))?;

        if proc.is_running() {
            return Err(Status::failed_precondition(format!(
                "process '{name}' is already running"
            )));
        }
        proc.spawn()
            .map_err(|e| Status::internal(format!("failed to start '{name}': {e:#}")))?;
        spawn_watcher(proc, exit_tx.clone());
        Ok(())
    }

    pub(crate) async fn handle_stop(&self, name: &str) -> Result<(), Status> {
        let mut procs = self.processes.write().await;
        let proc = procs
            .iter_mut()
            .find(|p| p.name() == name)
            .ok_or_else(|| Status::not_found(format!("process '{name}' not found")))?;

        if !proc.is_running() {
            return Err(Status::failed_precondition(format!(
                "process '{name}' is not running"
            )));
        }
        proc.request_stop();
        proc.wait_for_stop().await;
        Ok(())
    }

    pub(crate) async fn handle_reload_config(
        &self,
        exit_tx: &mpsc::Sender<ExitEvent>,
    ) -> Result<ReloadResult, Status> {
        let new_configs = self.config_loader.load();
        let new_names: std::collections::HashSet<&str> =
            new_configs.iter().map(|np| np.name.as_str()).collect();

        let mut removed_procs;
        let existing_names: std::collections::HashSet<String>;
        let mut removed = Vec::new();

        {
            let mut procs = self.processes.write().await;
            existing_names = procs.iter().map(|p| p.name().to_owned()).collect();

            removed_procs = Vec::new();
            let mut i = 0;
            while i < procs.len() {
                if procs[i].origin() == ProcessOrigin::Config
                    && !new_names.contains(procs[i].name())
                {
                    let mut proc = procs.remove(i);
                    info!("[{}] config removed, stopping", proc.name());
                    if proc.is_running() {
                        proc.request_stop();
                    }
                    removed.push(proc.name().to_owned());
                    removed_procs.push(proc);
                } else {
                    i += 1;
                }
            }
        }

        for proc in &mut removed_procs {
            proc.wait_for_stop().await;
        }

        let mut added = Vec::new();
        let mut unchanged = Vec::new();
        {
            let mut procs = self.processes.write().await;
            for np in new_configs {
                if existing_names.contains(&np.name) {
                    unchanged.push(np.name);
                } else {
                    let name = np.name;
                    info!("[{name}] new config found, adding");
                    let mut proc = ManagedProcess::new_config(name.clone(), np.config);
                    if proc.should_start() {
                        if let Err(e) = proc.spawn() {
                            warn!("[{name}] failed to start: {e:#}");
                        } else {
                            spawn_watcher(&mut proc, exit_tx.clone());
                        }
                    }
                    procs.push(proc);
                    added.push(name);
                }
            }
        }

        Ok(ReloadResult {
            added,
            removed,
            unchanged,
        })
    }

    async fn shutdown(&self) {
        let mut procs = self.processes.write().await;
        let ordered_set: std::collections::HashSet<usize> =
            self.startup_order.iter().copied().collect();
        let runtime_indices: Vec<usize> = (0..procs.len())
            .filter(|i| !ordered_set.contains(i))
            .collect();

        let mut shutdown_order: Vec<usize> = self.startup_order.iter().copied().rev().collect();
        shutdown_order.extend(runtime_indices);
        shutdown::shutdown_ordered(&mut procs, &shutdown_order).await;
    }
}

/// Spawn a background task that awaits the child's exit and sends the result.
fn spawn_watcher(proc: &mut ManagedProcess, tx: mpsc::Sender<ExitEvent>) {
    if let Some(child) = proc.take_child() {
        let name = proc.name().to_owned();
        let handle = tokio::spawn(async move {
            let mut child = child;
            let status = match child.wait().await {
                Ok(status) => status,
                Err(e) => {
                    warn!("[{name}] wait error: {e}, killing process");
                    let _ = child.kill().await;
                    match child.wait().await {
                        Ok(s) => s,
                        Err(e2) => {
                            warn!("[{name}] failed to reap after kill: {e2}");
                            return;
                        }
                    }
                }
            };
            let _ = tx
                .send(ExitEvent {
                    name: name.clone(),
                    status,
                })
                .await;
        });
        proc.set_watcher_handle(handle);
    }
}

fn resolve_startup_order(configs: &[ProcessDefinition]) -> Vec<usize> {
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
        .map(|&i| configs[i].name.as_str())
        .collect();
    info!("startup order: {}", names.join(" -> "));
    result.order
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{ProcessConfig, StaticConfigLoader};

    fn loader(defs: Vec<ProcessDefinition>) -> Arc<dyn ConfigLoader> {
        Arc::new(StaticConfigLoader::new(defs))
    }

    #[tokio::test]
    async fn test_complete_restart_skips_already_running() {
        let mgr = ProcessManager::new(loader(vec![ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        }]));
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        mgr.handle_start("svc", &exit_tx).await.unwrap();
        {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
        }

        mgr.complete_restart("svc", &exit_tx).await;

        let procs = mgr.processes().await;
        assert_eq!(procs.len(), 1);
        assert!(procs[0].is_running());

        nix::sys::signal::kill(
            nix::unistd::Pid::from_raw(procs[0].pid().unwrap() as i32),
            nix::sys::signal::Signal::SIGKILL,
        )
        .ok();
    }

    #[tokio::test]
    async fn test_reload_preserves_runtime_created_processes() {
        let mgr = ProcessManager::new(loader(vec![]));
        let config = ProcessConfig {
            command: "/bin/echo".to_string(),
            ..Default::default()
        };
        mgr.handle_create("runtime-svc".to_string(), config)
            .await
            .unwrap();

        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        let result = mgr.handle_reload_config(&exit_tx).await.unwrap();
        assert!(
            !result.removed.contains(&"runtime-svc".to_string()),
            "runtime-created process should not be removed by reload"
        );

        let procs = mgr.processes().await;
        assert_eq!(procs.len(), 1);
        assert_eq!(procs[0].name(), "runtime-svc");
    }
}
