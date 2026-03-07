// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::command::ReloadResult;
use crate::config::{self, NamedProcess};
use crate::ordering;
use crate::process::ManagedProcess;
use crate::shutdown;
use log::{info, warn};
use std::sync::Arc;
use tokio::sync::{RwLock, mpsc};
use tonic::Status;

pub(crate) struct ExitEvent {
    pub name: String,
    pub status: std::process::ExitStatus,
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

    pub(crate) async fn wire_watchers(&self, exit_tx: &mpsc::UnboundedSender<ExitEvent>) {
        let mut procs = self.processes.write().await;
        for proc in procs.iter_mut() {
            if proc.is_running() {
                spawn_watcher(proc, exit_tx.clone());
            }
        }
    }

    pub(crate) async fn handle_exit(
        &self,
        event: ExitEvent,
        restart_tx: &mpsc::UnboundedSender<String>,
    ) {
        let mut procs = self.processes.write().await;
        let Some(proc) = procs.iter_mut().find(|p| p.name() == event.name) else {
            warn!("exit event for unknown process '{}'", event.name);
            return;
        };
        info!("[{}] exited with {}", proc.name(), event.status);
        proc.set_last_status(event.status);
        if let Some(delay) = proc.handle_restart() {
            let tx = restart_tx.clone();
            let name = event.name.clone();
            tokio::spawn(async move {
                tokio::time::sleep(delay).await;
                let _ = tx.send(name);
            });
        }
    }

    pub(crate) async fn complete_restart(
        &self,
        name: &str,
        exit_tx: &mpsc::UnboundedSender<ExitEvent>,
    ) {
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
        exit_tx: &mpsc::UnboundedSender<ExitEvent>,
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
        exit_tx: &mpsc::UnboundedSender<ExitEvent>,
    ) -> Result<ReloadResult, Status> {
        let new_configs = load_configs();
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
                if !procs[i].is_runtime_created() && !new_names.contains(procs[i].name()) {
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
                    let mut proc = ManagedProcess::new(name.clone(), np.config);
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

    pub(crate) async fn shutdown(&self, startup_order: &[usize]) {
        let mut procs = self.processes.write().await;
        let ordered_set: std::collections::HashSet<usize> =
            startup_order.iter().copied().collect();
        let runtime_indices: Vec<usize> = (0..procs.len())
            .filter(|i| !ordered_set.contains(i))
            .collect();

        let mut shutdown_order: Vec<usize> = startup_order.iter().copied().rev().collect();
        shutdown_order.extend(runtime_indices);
        shutdown::shutdown_ordered(&mut procs, &shutdown_order).await;
    }
}

/// Spawn a background task that awaits the child's exit and sends the result.
pub(crate) fn spawn_watcher(proc: &mut ManagedProcess, tx: mpsc::UnboundedSender<ExitEvent>) {
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
            let _ = tx.send(ExitEvent {
                name: name.clone(),
                status,
            });
        });
        proc.set_watcher_handle(handle);
    }
}

pub(crate) fn load_configs() -> Vec<NamedProcess> {
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

pub fn resolve_startup_order(configs: &[NamedProcess]) -> Vec<usize> {
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

pub fn start_processes(configs: Vec<NamedProcess>, startup_order: &[usize]) -> Vec<ManagedProcess> {
    let mut processes: Vec<ManagedProcess> = configs
        .into_iter()
        .map(|np| ManagedProcess::new(np.name, np.config))
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::ProcessConfig;

    #[tokio::test]
    async fn test_complete_restart_skips_already_running() {
        let proc = ManagedProcess::new(
            "svc".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        );
        let mgr = ProcessManager::new(vec![proc]);
        let (exit_tx, _exit_rx) = mpsc::unbounded_channel::<ExitEvent>();

        mgr.handle_start("svc", &exit_tx).await.unwrap();
        {
            let procs = mgr.read().await;
            assert!(procs[0].is_running());
        }

        mgr.complete_restart("svc", &exit_tx).await;

        let procs = mgr.read().await;
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
        let mgr = ProcessManager::new(vec![]);
        let config = ProcessConfig {
            command: "/bin/echo".to_string(),
            ..Default::default()
        };
        mgr.handle_create("runtime-svc".to_string(), config)
            .await
            .unwrap();

        let (exit_tx, _exit_rx) = mpsc::unbounded_channel::<ExitEvent>();

        let result = mgr.handle_reload_config(&exit_tx).await.unwrap();
        assert!(
            !result.removed.contains(&"runtime-svc".to_string()),
            "runtime-created process should not be removed by reload"
        );

        let procs = mgr.read().await;
        assert_eq!(procs.len(), 1);
        assert_eq!(procs[0].name(), "runtime-svc");
    }
}
