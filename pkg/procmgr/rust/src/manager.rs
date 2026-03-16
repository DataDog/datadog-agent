// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#![allow(clippy::result_large_err)]

use crate::command::{Command, CreateResult, ReloadResult, StartResult, StopResult};
use crate::config::{self, ConfigLoader, ProcessDefinition};
use crate::grpc;
use crate::ordering;
use crate::process::{ManagedProcess, ProcessOrigin};
use crate::shutdown;
use anyhow::Result;
use log::{debug, info, warn};
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
    /// Indices into the `processes` Vec in dependency-resolved startup order.
    /// Recomputed on config reload so that indices stay in sync with the Vec.
    startup_order: Arc<RwLock<Vec<usize>>>,
    config_loader: Arc<dyn ConfigLoader>,
}

impl ProcessManager {
    pub fn new(config_loader: Arc<dyn ConfigLoader>) -> Self {
        let configs = config_loader.load();
        let processes: Vec<ManagedProcess> = configs
            .into_iter()
            .map(|pd| ManagedProcess::new_config(pd.name, pd.config))
            .collect();
        let startup_order = recompute_startup_order(&processes);
        Self {
            processes: Arc::new(RwLock::new(processes)),
            startup_order: Arc::new(RwLock::new(startup_order)),
            config_loader,
        }
    }

    async fn start(&self, exit_tx: &mpsc::Sender<ExitEvent>) {
        let order = self.startup_order.read().await;
        let mut procs = self.processes.write().await;
        for &idx in order.iter() {
            let proc = &mut procs[idx];
            if proc.should_start() {
                match proc.spawn() {
                    Ok(()) => spawn_watcher(proc, exit_tx.clone()),
                    Err(e) => warn!("{e:#}"),
                }
            }
        }
    }

    pub async fn run(&self) -> Result<()> {
        let (cmd_tx, mut cmd_rx) = mpsc::channel::<Command>(64);
        let (grpc_shutdown_tx, grpc_shutdown_rx) = oneshot::channel::<()>();
        let grpc_handle = tokio::spawn(grpc::server::run(self.clone(), cmd_tx, grpc_shutdown_rx));

        let (exit_tx, mut exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<String>(256);
        self.start(&exit_tx).await;

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
                            let _ = reply.send(self.handle_create(name, *config, &exit_tx).await);
                        }
                        Command::Start { name_or_uuid, reply } => {
                            let _ = reply.send(self.handle_start(&name_or_uuid, &exit_tx).await);
                        }
                        Command::Stop { name_or_uuid, reply } => {
                            let _ = reply.send(self.handle_stop(&name_or_uuid).await);
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

    pub(crate) async fn handle_exit(&self, event: ExitEvent, restart_tx: &mpsc::Sender<String>) {
        let mut procs = self.processes.write().await;
        let Some(proc) = procs.iter_mut().find(|p| p.name() == event.name) else {
            warn!("exit event for unknown process '{}'", event.name);
            return;
        };
        if !proc.state().is_alive() {
            debug!(
                "[{}] exit event after stop, skipping restart (state: {})",
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
        exit_tx: &mpsc::Sender<ExitEvent>,
    ) -> Result<CreateResult, Status> {
        if name.is_empty() {
            return Err(Status::invalid_argument("name must not be empty"));
        }
        if !name
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_' || c == '.')
        {
            return Err(Status::invalid_argument(
                "name must only contain ASCII alphanumeric characters, hyphens, underscores, or dots",
            ));
        }
        if config.command.is_empty() {
            return Err(Status::invalid_argument("command must not be empty"));
        }
        let uuid;
        {
            let mut procs = self.processes.write().await;
            if procs.iter().any(|p| p.name() == name) {
                return Err(Status::already_exists(format!(
                    "process '{name}' already exists"
                )));
            }
            let proc = ManagedProcess::new_runtime(name.clone(), config);
            uuid = proc.uuid().to_owned();
            info!("[{name}] created via RPC (uuid={uuid})");
            procs.push(proc);
            let proc = procs.last_mut().unwrap();
            if proc.should_start() {
                match proc.spawn() {
                    Ok(()) => spawn_watcher(proc, exit_tx.clone()),
                    Err(e) => warn!("[{name}] auto-start failed: {e:#}"),
                }
            }
        }
        self.update_startup_order().await;
        Ok(CreateResult { uuid })
    }

    pub(crate) async fn handle_start(
        &self,
        name_or_uuid: &str,
        exit_tx: &mpsc::Sender<ExitEvent>,
    ) -> Result<StartResult, Status> {
        let mut procs = self.processes.write().await;
        let idx = resolve_index(&procs, name_or_uuid)?;
        let proc = &mut procs[idx];

        if proc.is_running() {
            return Err(Status::failed_precondition(format!(
                "process '{}' is already running",
                proc.name()
            )));
        }
        proc.spawn()
            .map_err(|e| Status::internal(format!("failed to start '{}': {e:#}", proc.name())))?;
        let uuid = proc.uuid().to_owned();
        let pid = proc.pid();
        let state = proc.state();
        spawn_watcher(proc, exit_tx.clone());
        Ok(StartResult { uuid, pid, state })
    }

    pub(crate) async fn handle_stop(&self, name_or_uuid: &str) -> Result<StopResult, Status> {
        let mut procs = self.processes.write().await;
        let idx = resolve_index(&procs, name_or_uuid)?;
        let proc = &mut procs[idx];

        if !proc.is_running() {
            return Err(Status::failed_precondition(format!(
                "process '{}' is not running",
                proc.name()
            )));
        }
        let uuid = proc.uuid().to_owned();
        proc.request_stop();
        proc.wait_for_stop().await;
        let state = proc.state();
        Ok(StopResult { uuid, state })
    }

    pub(crate) async fn handle_reload_config(
        &self,
        exit_tx: &mpsc::Sender<ExitEvent>,
    ) -> Result<ReloadResult, Status> {
        let new_configs = self.config_loader.load();
        let new_names: std::collections::HashSet<&str> =
            new_configs.iter().map(|c| c.name.as_str()).collect();

        let mut removed = Vec::new();
        let mut stopped_procs = Vec::new();
        {
            let mut procs = self.processes.write().await;
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
                    stopped_procs.push(proc);
                } else {
                    i += 1;
                }
            }
        }

        for proc in &mut stopped_procs {
            proc.wait_for_stop().await;
        }

        let mut added = Vec::new();
        let mut modified = Vec::new();
        let mut modified_running: Vec<String> = Vec::new();
        let mut unchanged = Vec::new();
        {
            let mut procs = self.processes.write().await;
            for np in new_configs {
                if let Some(existing) = procs.iter_mut().find(|p| p.name() == np.name) {
                    if *existing.config() != np.config {
                        info!("[{}] config changed, updating", np.name);
                        if existing.is_running() {
                            existing.request_stop();
                            modified_running.push(np.name.clone());
                        }
                        existing.set_config(np.config);
                        modified.push(np.name);
                    } else {
                        unchanged.push(np.name);
                    }
                } else {
                    info!("[{}] new config found, adding", np.name);
                    let mut proc = ManagedProcess::new_config(np.name.clone(), np.config);
                    if proc.should_start() {
                        if let Err(e) = proc.spawn() {
                            warn!("[{}] failed to start: {e:#}", np.name);
                        } else {
                            spawn_watcher(&mut proc, exit_tx.clone());
                        }
                    }
                    added.push(np.name);
                    procs.push(proc);
                }
            }
        }

        // Wait for modified processes that were running to stop, then restart
        // with the new config.
        {
            let mut procs = self.processes.write().await;
            for name in &modified_running {
                if let Some(proc) = procs.iter_mut().find(|p| p.name() == *name) {
                    proc.wait_for_stop().await;
                    info!("[{name}] restarting with updated config");
                    if let Err(e) = proc.spawn() {
                        warn!("[{name}] failed to restart: {e:#}");
                    } else {
                        spawn_watcher(proc, exit_tx.clone());
                    }
                }
            }
        }

        self.update_startup_order().await;
        Ok(ReloadResult {
            added,
            removed,
            modified,
            unchanged,
        })
    }

    async fn update_startup_order(&self) {
        let new_order = recompute_startup_order(&self.processes.read().await);
        *self.startup_order.write().await = new_order;
    }

    async fn shutdown(&self) {
        let order: Vec<usize> = self
            .startup_order
            .read()
            .await
            .iter()
            .copied()
            .rev()
            .collect();
        let mut procs = self.processes.write().await;
        shutdown::shutdown_ordered(&mut procs, &order).await;
    }
}

pub fn looks_like_uuid_prefix(s: &str) -> bool {
    s.len() >= 4 && s.chars().all(|c| c.is_ascii_hexdigit() || c == '-')
}

fn resolve_by_uuid_prefix(procs: &[ManagedProcess], prefix: &str) -> Option<Result<usize, Status>> {
    let mut matches: Vec<usize> = procs
        .iter()
        .enumerate()
        .filter(|(_, p)| p.uuid().starts_with(prefix))
        .map(|(i, _)| i)
        .collect();
    match matches.len() {
        0 => None,
        1 => Some(Ok(matches.remove(0))),
        _ => Some(Err(Status::invalid_argument(format!(
            "UUID prefix '{prefix}' is ambiguous ({} matches)",
            matches.len()
        )))),
    }
}

fn resolve_index(procs: &[ManagedProcess], name_or_uuid: &str) -> Result<usize, Status> {
    if looks_like_uuid_prefix(name_or_uuid)
        && let Some(result) = resolve_by_uuid_prefix(procs, name_or_uuid)
    {
        return result;
    }
    procs
        .iter()
        .position(|p| p.name() == name_or_uuid)
        .ok_or_else(|| Status::not_found(format!("process '{name_or_uuid}' not found")))
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
            let _ = tx.try_send(ExitEvent {
                name: name.clone(),
                status,
            });
        });
        proc.set_watcher_handle(handle);
    }
}

/// Build `ProcessDefinition`s from the live processes Vec and resolve their
/// dependency order. Because the definitions are built in the same index order
/// as the Vec, the returned indices can be used directly for indexing into it.
fn recompute_startup_order(procs: &[ManagedProcess]) -> Vec<usize> {
    let defs: Vec<ProcessDefinition> = procs
        .iter()
        .map(|p| ProcessDefinition {
            name: p.name().to_string(),
            config: p.config().clone(),
        })
        .collect();
    let result = ordering::resolve_order(&defs);
    if !result.skipped.is_empty() {
        warn!(
            "dependency cycle detected, skipping processes: {}",
            result.skipped.join(", ")
        );
    }
    let names: Vec<&str> = result.order.iter().map(|&i| procs[i].name()).collect();
    debug!("startup order: {}", names.join(" -> "));
    result.order
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{MutableConfigLoader, ProcessConfig, StaticConfigLoader};

    fn loader(defs: Vec<ProcessDefinition>) -> Arc<dyn ConfigLoader> {
        Arc::new(StaticConfigLoader::new(defs))
    }

    fn sleep_def(name: &str) -> ProcessDefinition {
        ProcessDefinition {
            name: name.to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        }
    }

    #[tokio::test]
    async fn test_complete_restart_skips_already_running() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![sleep_def("svc")]));
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        mgr.handle_start("svc", &exit_tx).await?;
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
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_updates_modified_config() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        mgr.handle_start("svc-a", &exit_tx).await?;
        let old_pid = {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
            assert_eq!(procs[0].config().args, vec!["60"]);
            procs[0].pid().unwrap()
        };

        // Reload with modified config (different args)
        config_loader.set(vec![ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["120".to_string()],
                ..Default::default()
            },
        }]);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert!(result.modified.contains(&"svc-a".to_string()));
        assert!(result.added.is_empty());
        assert!(result.removed.is_empty());
        assert!(result.unchanged.is_empty());

        // Config should be updated and process restarted with a new PID
        let procs = mgr.processes().await;
        assert_eq!(procs[0].config().args, vec!["120"]);
        assert!(
            procs[0].is_running(),
            "modified running process should be restarted"
        );
        assert_ne!(
            procs[0].pid().unwrap(),
            old_pid,
            "restarted process should have a different PID"
        );

        nix::sys::signal::kill(
            nix::unistd::Pid::from_raw(procs[0].pid().unwrap() as i32),
            nix::sys::signal::Signal::SIGKILL,
        )
        .ok();
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_modified_not_running_stays_stopped() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        // Don't start svc-a — leave it in Created state
        config_loader.set(vec![ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["120".to_string()],
                ..Default::default()
            },
        }]);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert!(result.modified.contains(&"svc-a".to_string()));

        let procs = mgr.processes().await;
        assert_eq!(procs[0].config().args, vec!["120"]);
        assert!(
            !procs[0].is_running(),
            "non-running modified process should not be started"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_unchanged_config_not_modified() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        // Reload with the exact same config
        config_loader.set(vec![sleep_def("svc-a")]);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert!(result.unchanged.contains(&"svc-a".to_string()));
        assert!(result.modified.is_empty());
        Ok(())
    }

    #[tokio::test]
    async fn test_create_rejects_empty_name() {
        let mgr = ProcessManager::new(loader(vec![]));
        let config = ProcessConfig {
            command: "/bin/echo".to_string(),
            ..Default::default()
        };
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let err = mgr
            .handle_create("".to_string(), config, &exit_tx)
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
    }

    #[tokio::test]
    async fn test_create_rejects_invalid_name() {
        let mgr = ProcessManager::new(loader(vec![]));
        let config = ProcessConfig {
            command: "/bin/echo".to_string(),
            ..Default::default()
        };
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let err = mgr
            .handle_create("bad name!".to_string(), config, &exit_tx)
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
    }

    #[tokio::test]
    async fn test_create_accepts_valid_name() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![]));
        let config = ProcessConfig {
            command: "/bin/echo".to_string(),
            ..Default::default()
        };
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        mgr.handle_create("my-svc_v2.0".to_string(), config, &exit_tx)
            .await?;
        let procs = mgr.processes().await;
        assert_eq!(procs[0].name(), "my-svc_v2.0");
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_preserves_runtime_created_processes() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![]));
        let config = ProcessConfig {
            command: "/bin/echo".to_string(),
            ..Default::default()
        };
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        mgr.handle_create("runtime-svc".to_string(), config, &exit_tx)
            .await?;

        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert!(
            !result.removed.contains(&"runtime-svc".to_string()),
            "runtime-created process should not be removed by reload"
        );

        let procs = mgr.processes().await;
        assert_eq!(procs.len(), 1);
        assert_eq!(procs[0].name(), "runtime-svc");
        Ok(())
    }

    #[tokio::test]
    async fn test_shutdown_after_reload_removes_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![
            sleep_def("svc-a"),
            sleep_def("svc-b"),
        ]));
        let mgr = ProcessManager::new(config_loader.clone());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        mgr.handle_start("svc-a", &exit_tx).await?;
        mgr.handle_start("svc-b", &exit_tx).await?;

        // Reload removes svc-b
        config_loader.set(vec![sleep_def("svc-a")]);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert!(result.removed.contains(&"svc-b".to_string()));

        // Shutdown must not panic despite svc-b being gone from the Vec
        mgr.shutdown().await;

        let procs = mgr.processes().await;
        assert!(
            procs.iter().all(|p| !p.is_running()),
            "all remaining processes should be stopped"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_shutdown_after_reload_adds_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        mgr.handle_start("svc-a", &exit_tx).await?;

        // Reload adds svc-b
        config_loader.set(vec![sleep_def("svc-a"), sleep_def("svc-b")]);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert!(result.added.contains(&"svc-b".to_string()));

        // svc-b auto-started by reload; start svc-a again is already running
        mgr.shutdown().await;

        let procs = mgr.processes().await;
        assert!(
            procs.iter().all(|p| !p.is_running()),
            "all processes (including reload-added) should be stopped"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_shutdown_after_reload_with_runtime_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        mgr.handle_start("svc-a", &exit_tx).await?;

        // Create a runtime process
        mgr.handle_create(
            "runtime-svc".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                auto_start: false,
                ..Default::default()
            },
            &exit_tx,
        )
        .await?;
        mgr.handle_start("runtime-svc", &exit_tx).await?;

        // Reload removes svc-a but preserves runtime-svc
        config_loader.set(vec![]);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert!(result.removed.contains(&"svc-a".to_string()));

        mgr.shutdown().await;

        let procs = mgr.processes().await;
        assert!(
            procs.iter().all(|p| !p.is_running()),
            "runtime-created process should also be shut down"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_startup_order_indices_match_processes() {
        let mgr = ProcessManager::new(loader(vec![
            sleep_def("alpha"),
            sleep_def("bravo"),
            sleep_def("charlie"),
        ]));

        let order = mgr.startup_order.read().await;
        let procs = mgr.processes().await;
        let names: Vec<&str> = order.iter().map(|&i| procs[i].name()).collect();
        assert_eq!(names, vec!["alpha", "bravo", "charlie"]);
    }

    #[tokio::test]
    async fn test_create_includes_runtime_process_in_startup_order() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![sleep_def("svc-a")]));
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        mgr.handle_create(
            "svc-b".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                after: vec!["svc-a".to_string()],
                auto_start: false,
                ..Default::default()
            },
            &exit_tx,
        )
        .await?;

        let order = mgr.startup_order.read().await;
        let procs = mgr.processes().await;
        let names: Vec<&str> = order.iter().map(|&i| procs[i].name()).collect();
        assert_eq!(
            names,
            vec!["svc-a", "svc-b"],
            "runtime process with after-dep should appear in startup order"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_create_auto_start_spawns_process() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![]));
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        mgr.handle_create(
            "auto-svc".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                auto_start: true,
                ..Default::default()
            },
            &exit_tx,
        )
        .await?;

        {
            let procs = mgr.processes().await;
            assert_eq!(procs.len(), 1);
            assert!(
                procs[0].is_running(),
                "process with auto_start=true should be running after create"
            );
            assert!(
                procs[0].pid().is_some(),
                "running process should have a PID"
            );
        }

        mgr.shutdown().await;
        Ok(())
    }

    #[tokio::test]
    async fn test_create_auto_start_false_stays_created() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![]));
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        mgr.handle_create(
            "manual-svc".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                auto_start: false,
                ..Default::default()
            },
            &exit_tx,
        )
        .await?;

        let procs = mgr.processes().await;
        assert_eq!(procs.len(), 1);
        assert!(
            !procs[0].is_running(),
            "process with auto_start=false should not be running after create"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_create_auto_start_bad_command_still_created() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![]));
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let result = mgr
            .handle_create(
                "bad-cmd".to_string(),
                ProcessConfig {
                    command: "/nonexistent/binary".to_string(),
                    auto_start: true,
                    ..Default::default()
                },
                &exit_tx,
            )
            .await;

        assert!(result.is_ok(), "create should succeed even if spawn fails");
        let procs = mgr.processes().await;
        assert_eq!(procs.len(), 1);
        assert_eq!(procs[0].name(), "bad-cmd");
        assert!(
            !procs[0].is_running(),
            "process with bad command should not be running"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_create_auto_start_condition_not_met() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![]));
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        mgr.handle_create(
            "cond-svc".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                auto_start: true,
                condition_path_exists: Some("/nonexistent/path/that/should/not/exist".to_string()),
                ..Default::default()
            },
            &exit_tx,
        )
        .await?;

        let procs = mgr.processes().await;
        assert_eq!(procs.len(), 1);
        assert!(
            !procs[0].is_running(),
            "process should not start when condition_path_exists is not met"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_recomputes_startup_order() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        {
            let order = mgr.startup_order.read().await;
            assert_eq!(*order, vec![0], "single process at index 0");
        }

        // Reload with a new process that has an after-dependency, which
        // forces a non-alphabetical order (svc-b before svc-api).
        config_loader.set(vec![
            ProcessDefinition {
                name: "svc-api".to_string(),
                config: ProcessConfig {
                    command: "/bin/sleep".to_string(),
                    args: vec!["60".to_string()],
                    after: vec!["svc-b".to_string()],
                    ..Default::default()
                },
            },
            sleep_def("svc-b"),
        ]);
        mgr.handle_reload_config(&exit_tx).await?;

        let order = mgr.startup_order.read().await;
        let procs = mgr.processes().await;
        let names: Vec<&str> = order.iter().map(|&i| procs[i].name()).collect();
        assert_eq!(
            names,
            vec!["svc-b", "svc-api"],
            "startup order should be recomputed with dependency constraints"
        );
        Ok(())
    }
}
