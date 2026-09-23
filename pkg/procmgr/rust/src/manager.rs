// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#![allow(clippy::result_large_err)]

use crate::command::{Command, CreateResult, ReloadResult, StartResult, StopResult};
use crate::config::{self, ConfigLoader, ProcessDefinition};
use crate::grpc;
use crate::ordering;
use crate::platform;
use crate::process::{ExitEvent, ManagedProcess, ProcessOrigin};
use crate::shutdown;
use crate::state::ProcessState;
use crate::uuid_gen::UuidGenerator;
use anyhow::Result;
use log::{debug, info, warn};
use std::sync::Arc;
use tokio::sync::{RwLock, mpsc, oneshot};
use tonic::Status;

#[derive(Clone)]
pub struct ProcessManager {
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
    /// Indices into the `processes` Vec in dependency-resolved startup order.
    /// Recomputed on config reload so that indices stay in sync with the Vec.
    startup_order: Arc<RwLock<Vec<usize>>>,
    config_loader: Arc<dyn ConfigLoader>,
    uuid_gen: Arc<dyn UuidGenerator>,
}

impl ProcessManager {
    pub fn new(config_loader: Arc<dyn ConfigLoader>, uuid_gen: Arc<dyn UuidGenerator>) -> Self {
        let configs = config_loader.load();
        let processes: Vec<ManagedProcess> = configs
            .into_iter()
            .map(|pd| ManagedProcess::new_config(pd.name, uuid_gen.generate(), pd.config))
            .collect();
        let startup_result = recompute_startup_order(&processes);
        Self {
            processes: Arc::new(RwLock::new(processes)),
            startup_order: Arc::new(RwLock::new(startup_result.order)),
            config_loader,
            uuid_gen,
        }
    }

    async fn start(&self, exit_tx: &mpsc::Sender<ExitEvent>) {
        let order = self.startup_order.read().await;
        let mut procs = self.processes.write().await;
        for &idx in order.iter() {
            let proc = &mut procs[idx];
            if proc.should_start()
                && let Err(e) = proc.spawn(exit_tx.clone())
            {
                warn!("{e:#}");
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

        let shutdown = platform::shutdown_signal();
        tokio::pin!(shutdown);

        loop {
            tokio::select! {
                _ = &mut shutdown => {
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
        if proc.pid() != Some(event.pid) {
            debug!(
                "[{}] ignoring stale exit event for pid {} (current pid: {:?})",
                proc.name(),
                event.pid,
                proc.pid()
            );
            return;
        }
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
        // `handle_restart` decided at exit time; the gate can close during the
        // backoff delay, so re-check it here.
        if !proc.may_respawn() {
            info!("[{name}] restart skipped: start conditions not met");
            proc.mark_restart_blocked_by_conditions();
            return;
        }
        if let Err(e) = proc.spawn(exit_tx.clone()) {
            warn!("[{}] restart failed: {e:#}", proc.name());
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
            let proc = ManagedProcess::new_runtime(name.clone(), self.uuid_gen.generate(), config);
            uuid = proc.uuid().to_owned();
            info!("[{name}] created via RPC (uuid={uuid})");
            procs.push(proc);
            let proc = procs.last_mut().unwrap();
            if proc.should_start()
                && let Err(e) = proc.spawn(exit_tx.clone())
            {
                warn!("[{name}] auto-start failed: {e:#}");
            }
        }
        let warnings = self.update_startup_order().await;
        Ok(CreateResult { uuid, warnings })
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
        proc.spawn(exit_tx.clone())
            .map_err(|e| Status::internal(format!("failed to start '{}': {e:#}", proc.name())))?;
        let uuid = proc.uuid().to_owned();
        let pid = proc.pid();
        let state = proc.state();
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
                    let mut proc = ManagedProcess::new_config(
                        np.name.clone(),
                        self.uuid_gen.generate(),
                        np.config,
                    );
                    if proc.should_start()
                        && let Err(e) = proc.spawn(exit_tx.clone())
                    {
                        warn!("[{}] failed to start: {e:#}", np.name);
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
                let Some(proc) = procs.iter_mut().find(|p| p.name() == *name) else {
                    continue;
                };
                proc.wait_for_stop().await;
                if !proc.may_respawn() {
                    info!("[{name}] not restarting after reload: start conditions not met");
                    proc.mark_restart_blocked_by_conditions();
                    continue;
                }
                info!("[{name}] restarting with updated config");
                if let Err(e) = proc.spawn(exit_tx.clone()) {
                    warn!("[{name}] failed to restart: {e:#}");
                }
            }
        }

        // Recomputed before the gate re-evaluation below, which walks it to
        // start candidates in dependency order and to inherit its exclusion of
        // processes caught in a dependency cycle.
        self.update_startup_order().await;

        // Two ways a closed condition leaves work for reload, and both stay
        // narrow enough not to resurrect anything else.
        //
        // A process whose conditions were unmet at boot never started, so no
        // earlier reload step covers it. `Created` is the only state meaning
        // "never started", and a process declaring no condition was never
        // blocked in the first place.
        //
        // A process whose conditions closed mid-restart was already running,
        // and the skip left it in `Exited`, `Failed`, or `Stopped`. Those
        // states are also reached by a completed one-shot, a policy mismatch,
        // the burst limit, a failed spawn, and an operator stop, so the guard
        // keys off the recorded skip reason rather than the state.
        // `may_respawn`, not `should_start`: `auto_start` governs boot only, so
        // consulting it here would strand a manually started process forever.
        {
            let candidates: std::collections::HashSet<&str> = unchanged
                .iter()
                .chain(modified.iter())
                .map(String::as_str)
                .collect();
            let order = self.startup_order.read().await;
            let mut procs = self.processes.write().await;
            for &idx in order.iter() {
                let proc = &mut procs[idx];
                if !candidates.contains(proc.name()) {
                    continue;
                }
                let eligible = match proc.state() {
                    ProcessState::Created => proc.has_start_conditions() && proc.should_start(),
                    ProcessState::Exited | ProcessState::Failed | ProcessState::Stopped => {
                        proc.restart_blocked_by_conditions() && proc.may_respawn()
                    }
                    _ => false,
                };
                if !eligible {
                    continue;
                }
                let name = proc.name().to_owned();
                info!("[{name}] start conditions now met after reload, starting");
                if let Err(e) = proc.spawn(exit_tx.clone()) {
                    warn!("[{name}] failed to start after gate re-eval: {e:#}");
                }
            }
        }

        Ok(ReloadResult {
            added,
            removed,
            modified,
            unchanged,
        })
    }

    async fn update_startup_order(&self) -> Vec<String> {
        let result = recompute_startup_order(&self.processes.read().await);
        *self.startup_order.write().await = result.order;
        result.warnings
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
    s.len() >= 8 && s.chars().all(|c| c.is_ascii_hexdigit() || c == '-')
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

/// Build `ProcessDefinition`s from the live processes Vec and resolve their
/// dependency order. Because the definitions are built in the same index order
/// as the Vec, the returned indices can be used directly for indexing into it.
struct StartupOrderResult {
    order: Vec<usize>,
    warnings: Vec<String>,
}

fn recompute_startup_order(procs: &[ManagedProcess]) -> StartupOrderResult {
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
    StartupOrderResult {
        order: result.order,
        warnings: result.warnings,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{MutableConfigLoader, ProcessConfig, StaticConfigLoader};
    use crate::process::ExitEvent;
    use crate::test_helpers;
    use crate::uuid_gen::{SequentialUuidGenerator, V4UuidGenerator};

    fn loader(defs: Vec<ProcessDefinition>) -> Arc<dyn ConfigLoader> {
        Arc::new(StaticConfigLoader::new(defs))
    }

    fn uuid_gen() -> Arc<dyn UuidGenerator> {
        Arc::new(V4UuidGenerator)
    }

    fn sleep_def(name: &str) -> ProcessDefinition {
        sleep_def_secs(name, test_helpers::TEST_SLEEP_SECS)
    }

    fn sleep_def_secs(name: &str, secs: u32) -> ProcessDefinition {
        ProcessDefinition {
            name: name.to_string(),
            config: test_helpers::sleep_test_config(secs),
        }
    }

    fn true_def(name: &str) -> ProcessDefinition {
        let (cmd, args) = test_helpers::true_cmd();
        ProcessDefinition {
            name: name.to_string(),
            config: test_helpers::make_config(cmd, args),
        }
    }

    #[test]
    fn test_resolve_index_ambiguous_uuid_prefix() {
        let mk = |name: &str, uuid: &str| {
            ManagedProcess::new_config(
                name.to_string(),
                uuid.to_string(),
                test_helpers::make_config("true", vec![]),
            )
        };
        let procs = vec![
            mk("svc-a", "aabbccdd-1111-0000-0000-000000000000"),
            mk("svc-b", "aabbccdd-2222-0000-0000-000000000000"),
        ];

        let err = resolve_index(&procs, "aabbccdd").unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
        assert!(
            err.message().contains("ambiguous"),
            "error should mention ambiguity: {}",
            err.message()
        );
        assert_eq!(resolve_index(&procs, "aabbccdd-1").unwrap(), 0);
        assert_eq!(resolve_index(&procs, "aabbccdd-2").unwrap(), 1);
    }

    #[tokio::test]
    async fn test_complete_restart_skips_already_running() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![sleep_def("svc")]), uuid_gen());
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

        test_helpers::cleanup_process(procs[0].pid().unwrap());
        Ok(())
    }

    async fn reload_modified_running_svc_a(
        mgr: &ProcessManager,
        config_loader: &MutableConfigLoader,
        exit_tx: &mpsc::Sender<ExitEvent>,
    ) -> anyhow::Result<ReloadResult> {
        mgr.handle_start("svc-a", exit_tx).await?;
        config_loader.set(vec![sleep_def_secs(
            "svc-a",
            test_helpers::ALT_TEST_SLEEP_SECS,
        )]);
        mgr.handle_reload_config(exit_tx).await.map_err(Into::into)
    }

    fn alt_sleep_args() -> Vec<String> {
        sleep_def_secs("_", test_helpers::ALT_TEST_SLEEP_SECS)
            .config
            .args
    }

    async fn cleanup_first_process(mgr: &ProcessManager) {
        let procs = mgr.processes().await;
        if let Some(pid) = procs.first().and_then(|p| p.pid()) {
            test_helpers::cleanup_process(pid);
        }
    }

    #[tokio::test]
    async fn test_reload_modified_01_result_contains_service() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        let result = reload_modified_running_svc_a(&mgr, &config_loader, &exit_tx).await?;

        assert!(
            result.modified.contains(&"svc-a".to_string()),
            "modified: {:?}",
            result.modified
        );
        assert!(result.added.is_empty(), "added: {:?}", result.added);
        assert!(result.removed.is_empty(), "removed: {:?}", result.removed);
        assert!(
            result.unchanged.is_empty(),
            "unchanged: {:?}",
            result.unchanged
        );

        cleanup_first_process(&mgr).await;
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_modified_02_updates_stored_config() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        reload_modified_running_svc_a(&mgr, &config_loader, &exit_tx).await?;

        let procs = mgr.processes().await;
        assert_eq!(
            procs[0].config().args,
            alt_sleep_args(),
            "stored config args after reload"
        );

        cleanup_first_process(&mgr).await;
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_modified_03_restarts_running_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        reload_modified_running_svc_a(&mgr, &config_loader, &exit_tx).await?;

        let procs = mgr.processes().await;
        assert!(
            procs[0].is_running(),
            "modified running process should be restarted (state={})",
            procs[0].state()
        );
        let pid = procs[0].pid().expect("restarted process should have a PID");
        test_helpers::cleanup_process(pid);
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_modified_not_running_stays_stopped() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        // Don't start svc-a — leave it in Created state
        config_loader.set(vec![sleep_def_secs(
            "svc-a",
            test_helpers::ALT_TEST_SLEEP_SECS,
        )]);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert!(result.modified.contains(&"svc-a".to_string()));

        let procs = mgr.processes().await;
        let expected_args = sleep_def_secs("_", test_helpers::ALT_TEST_SLEEP_SECS)
            .config
            .args;
        assert_eq!(procs[0].config().args, expected_args);
        assert!(
            !procs[0].is_running(),
            "non-running modified process should not be started"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_unchanged_config_not_modified() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
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
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (cmd, args) = test_helpers::true_cmd();
        let config = ProcessConfig {
            command: cmd.to_string(),
            args,
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
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (cmd, args) = test_helpers::true_cmd();
        let config = ProcessConfig {
            command: cmd.to_string(),
            args,
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
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (cmd, args) = test_helpers::true_cmd();
        let config = ProcessConfig {
            command: cmd.to_string(),
            args,
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
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (cmd, args) = test_helpers::true_cmd();
        let config = ProcessConfig {
            command: cmd.to_string(),
            args,
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
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
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
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
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
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        mgr.handle_start("svc-a", &exit_tx).await?;

        // Create a runtime process
        let mut cfg = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        cfg.auto_start = false;
        mgr.handle_create("runtime-svc".to_string(), cfg, &exit_tx)
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
        let mgr = ProcessManager::new(
            loader(vec![
                sleep_def("alpha"),
                sleep_def("bravo"),
                sleep_def("charlie"),
            ]),
            uuid_gen(),
        );

        let order = mgr.startup_order.read().await;
        let procs = mgr.processes().await;
        let names: Vec<&str> = order.iter().map(|&i| procs[i].name()).collect();
        assert_eq!(names, vec!["alpha", "bravo", "charlie"]);
    }

    #[tokio::test]
    async fn test_create_includes_runtime_process_in_startup_order() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![sleep_def("svc-a")]), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let mut cfg = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        cfg.after = vec!["svc-a".to_string()];
        cfg.auto_start = false;
        mgr.handle_create("svc-b".to_string(), cfg, &exit_tx)
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
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let mut cfg = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        cfg.auto_start = true;
        mgr.handle_create("auto-svc".to_string(), cfg, &exit_tx)
            .await?;

        let pid = {
            let procs = mgr.processes().await;
            assert_eq!(procs.len(), 1);
            assert!(
                procs[0].is_running(),
                "process with auto_start=true should be running after create"
            );
            procs[0].pid().expect("running process should have a PID")
        };
        test_helpers::cleanup_process(pid);
        Ok(())
    }

    #[tokio::test]
    async fn test_create_auto_start_false_stays_created() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let mut cfg = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        cfg.auto_start = false;
        mgr.handle_create("manual-svc".to_string(), cfg, &exit_tx)
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
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
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
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let mut cfg = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        cfg.auto_start = true;
        cfg.condition_path_exists = Some("/nonexistent/path/that/should/not/exist".to_string());
        mgr.handle_create("cond-svc".to_string(), cfg, &exit_tx)
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
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        {
            let order = mgr.startup_order.read().await;
            assert_eq!(*order, vec![0], "single process at index 0");
        }

        // Reload with a new process that has an after-dependency, which
        // forces a non-alphabetical order (svc-b before svc-api).
        let mut cfg = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        cfg.after = vec!["svc-b".to_string()];
        config_loader.set(vec![
            ProcessDefinition {
                name: "svc-api".to_string(),
                config: cfg,
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

    #[tokio::test]
    async fn test_ambiguous_uuid_prefix_returns_error() {
        // Both UUIDs share the first 8 characters ("aabbccdd"), which is the
        // length shown by `dd-procmgr list`.
        let uuid_gen: Arc<dyn UuidGenerator> = Arc::new(SequentialUuidGenerator::new(vec![
            "aabbccdd-1111-0000-0000-000000000000",
            "aabbccdd-2222-0000-0000-000000000000",
        ]));
        let mgr = ProcessManager::new(loader(vec![true_def("svc-a"), true_def("svc-b")]), uuid_gen);
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        let err: Status = mgr.handle_start("aabbccdd", &exit_tx).await.unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
        assert!(
            err.message().contains("ambiguous"),
            "error should mention ambiguity: {}",
            err.message()
        );

        // A longer, unambiguous prefix should resolve through handle_start.
        // Use an immediate-exit child so this test does not depend on ping stop
        // behavior on Windows (see test_resolve_index_ambiguous_uuid_prefix).
        let start = mgr
            .handle_start("aabbccdd-1", &exit_tx)
            .await
            .expect("unambiguous prefix should resolve");
        assert_eq!(start.uuid, "aabbccdd-1111-0000-0000-000000000000");
    }

    #[cfg(not(windows))]
    mod config_gate {
        use super::*;
        use crate::config::RestartPolicy;
        use crate::config_gate::{ConditionConfigFile, TestEnvGuard, test_env_guard};
        use std::io::Write;

        /// Every test in this module evaluates gates, which read the live process
        /// environment, so each one needs the shared guard that excludes the
        /// `config_gate` tests mutating `DD_*` values.
        fn gate_env() -> (TestEnvGuard, tempfile::TempDir) {
            (test_env_guard(), tempfile::tempdir().unwrap())
        }

        fn gate_on_process_collection(agent_yaml: &str) -> Vec<ConditionConfigFile> {
            vec![ConditionConfigFile {
                path: agent_yaml.to_string(),
                keys: vec!["process_config.process_collection.enabled".into()],
            }]
        }

        fn gated_sleep_def(name: &str, agent_yaml: &str) -> ProcessDefinition {
            gated_sleep_def_secs(name, agent_yaml, test_helpers::TEST_SLEEP_SECS)
        }

        fn gated_sleep_def_secs(name: &str, agent_yaml: &str, secs: u32) -> ProcessDefinition {
            let (cmd, args) = test_helpers::sleep_cmd(secs);
            ProcessDefinition {
                name: name.to_string(),
                config: ProcessConfig {
                    command: cmd.to_string(),
                    args,
                    condition_config_any: gate_on_process_collection(agent_yaml),
                    ..Default::default()
                },
            }
        }

        fn gated_on_failure_sleep_def(name: &str, agent_yaml: &str) -> ProcessDefinition {
            let (cmd, args) = test_helpers::sleep_cmd(test_helpers::TEST_SLEEP_SECS);
            ProcessDefinition {
                name: name.to_string(),
                config: ProcessConfig {
                    command: cmd.to_string(),
                    args,
                    restart: RestartPolicy::OnFailure,
                    restart_sec: Some(2.0),
                    condition_config_any: gate_on_process_collection(agent_yaml),
                    ..Default::default()
                },
            }
        }

        /// `container_collection` and `process_discovery` both default to true
        /// and both enable process collection, so they have to be pinned false
        /// or the gate is open regardless of the key under test.
        fn write_agent_yaml(dir: &std::path::Path, process_collection_enabled: bool) -> String {
            let path = dir.join("datadog.yaml");
            let body = format!(
                "process_config:\n  process_collection:\n    enabled: {process_collection_enabled}\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n"
            );
            let mut file = std::fs::File::create(&path).unwrap();
            file.write_all(body.as_bytes()).unwrap();
            path.to_string_lossy().into_owned()
        }

        fn exit_status(success: bool) -> std::process::ExitStatus {
            let (cmd, args) = if success {
                test_helpers::true_cmd()
            } else {
                test_helpers::false_cmd()
            };
            std::process::Command::new(cmd)
                .args(args)
                .status()
                .expect("shell builtin should run")
        }

        /// Kills the child and reports the exit to the manager, which is what a
        /// crash looks like from `run`'s point of view.
        async fn crash(mgr: &ProcessManager, name: &str, restart_tx: &mpsc::Sender<String>) {
            let pid = mgr.processes().await[0]
                .pid()
                .expect("process should be running");
            test_helpers::cleanup_process(pid);
            let event = ExitEvent {
                name: name.to_string(),
                pid,
                status: exit_status(false),
            };
            mgr.handle_exit(event, restart_tx).await;
        }

        async fn assert_stranded_by_gate(mgr: &ProcessManager, expected: ProcessState) {
            let procs = mgr.processes().await;
            assert_eq!(procs[0].state(), expected);
            assert!(
                procs[0].restart_blocked_by_conditions(),
                "the closed gate is the reason the respawn was skipped, and reload needs it recorded"
            );
        }

        #[tokio::test]
        async fn test_auto_start_runs_when_config_gate_open() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let mgr = ProcessManager::new(
                loader(vec![gated_sleep_def("gated-svc", &yaml)]),
                uuid_gen(),
            );
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            mgr.start(&exit_tx).await;
            assert!(
                mgr.processes().await[0].is_running(),
                "process should auto-start when condition_config_any is met"
            );

            cleanup_first_process(&mgr).await;
            Ok(())
        }

        #[tokio::test]
        async fn test_auto_start_skips_when_config_gate_closed() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), false);
            let mgr = ProcessManager::new(
                loader(vec![gated_sleep_def("gated-svc", &yaml)]),
                uuid_gen(),
            );
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            mgr.start(&exit_tx).await;
            let procs = mgr.processes().await;
            assert!(
                !procs[0].is_running(),
                "process should not auto-start when condition_config_any is not met"
            );
            assert_eq!(
                procs[0].state(),
                ProcessState::Created,
                "a gated process that never started stays Created"
            );
            Ok(())
        }

        #[tokio::test]
        async fn test_on_failure_restart_skips_when_config_gate_closes() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let mgr = ProcessManager::new(
                loader(vec![gated_on_failure_sleep_def("gated-svc", &yaml)]),
                uuid_gen(),
            );
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            mgr.start(&exit_tx).await;
            let pid = {
                let procs = mgr.processes().await;
                assert!(procs[0].is_running());
                procs[0].pid().unwrap()
            };

            // The gate closes between the exit and the queued restart firing.
            write_agent_yaml(dir.path(), false);
            {
                let mut procs = mgr.processes.write().await;
                let (cmd, args) = test_helpers::false_cmd();
                let status = std::process::Command::new(cmd).args(args).status()?;
                procs[0].set_last_status(status);
            }
            test_helpers::cleanup_process(pid);

            mgr.complete_restart("gated-svc", &exit_tx).await;
            assert!(
                !mgr.processes().await[0].is_running(),
                "on-failure restart should skip when the config gate is closed"
            );
            Ok(())
        }

        #[tokio::test]
        async fn test_reload_starts_created_process_when_gate_opens() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), false);
            let config_loader = Arc::new(MutableConfigLoader::new(vec![gated_sleep_def(
                "gated-svc",
                &yaml,
            )]));
            let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            mgr.start(&exit_tx).await;
            assert!(!mgr.processes().await[0].is_running());

            // Only the gated yaml changes; the process config is untouched.
            write_agent_yaml(dir.path(), true);
            let result = mgr.handle_reload_config(&exit_tx).await?;

            assert!(
                result.unchanged.contains(&"gated-svc".to_string()),
                "process config did not change, so reload should report it unchanged (modified: {:?})",
                result.modified
            );
            assert!(
                mgr.processes().await[0].is_running(),
                "reload should start a Created process whose gate has opened"
            );

            cleanup_first_process(&mgr).await;
            Ok(())
        }

        #[tokio::test]
        async fn test_reload_does_not_start_cycle_skipped_process() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), false);
            let mut a = gated_sleep_def("svc-a", &yaml);
            let mut b = gated_sleep_def("svc-b", &yaml);
            a.config.after = vec!["svc-b".to_string()];
            b.config.after = vec!["svc-a".to_string()];
            let config_loader = Arc::new(MutableConfigLoader::new(vec![a, b]));
            let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            mgr.start(&exit_tx).await;
            write_agent_yaml(dir.path(), true);
            mgr.handle_reload_config(&exit_tx).await?;

            let procs = mgr.processes().await;
            assert!(
                procs.iter().all(|p| !p.is_running()),
                "reload must not start processes the dependency resolver excluded for a cycle"
            );
            Ok(())
        }

        #[tokio::test]
        async fn test_reload_does_not_restart_stopped_process() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let config_loader = Arc::new(MutableConfigLoader::new(vec![gated_sleep_def(
                "gated-svc",
                &yaml,
            )]));
            let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            mgr.start(&exit_tx).await;
            assert!(mgr.processes().await[0].is_running());
            mgr.handle_stop("gated-svc").await?;

            let result = mgr.handle_reload_config(&exit_tx).await?;

            assert!(result.unchanged.contains(&"gated-svc".to_string()));
            let procs = mgr.processes().await;
            assert!(
                !procs[0].is_running(),
                "reload must not resurrect a process an operator stopped (state={})",
                procs[0].state()
            );
            Ok(())
        }

        #[tokio::test]
        async fn test_manual_start_bypasses_closed_gate() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), false);
            let mgr = ProcessManager::new(
                loader(vec![gated_sleep_def("gated-svc", &yaml)]),
                uuid_gen(),
            );
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            mgr.start(&exit_tx).await;
            assert!(!mgr.processes().await[0].is_running());

            // An explicit operator command is not a boot-time policy decision.
            mgr.handle_start("gated-svc", &exit_tx).await?;
            assert!(
                mgr.processes().await[0].is_running(),
                "an explicit start should run even with a closed gate"
            );

            cleanup_first_process(&mgr).await;
            Ok(())
        }

        // Recovery: a gate that closes while the process is restarting strands
        // it outside `Created`, and reopening the gate has to bring it back.
        // One test per site that can skip the respawn.

        #[tokio::test]
        async fn test_reload_restarts_process_stranded_at_exit() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let mgr = ProcessManager::new(
                loader(vec![gated_on_failure_sleep_def("gated-svc", &yaml)]),
                uuid_gen(),
            );
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
            let (restart_tx, mut restart_rx) = mpsc::channel::<String>(8);

            mgr.start(&exit_tx).await;
            assert!(mgr.processes().await[0].is_running());

            // The gate closes before the crash is handled, so `handle_restart`
            // skips without queueing a backoff.
            write_agent_yaml(dir.path(), false);
            crash(&mgr, "gated-svc", &restart_tx).await;
            assert!(
                restart_rx.try_recv().is_err(),
                "a closed gate must not queue a restart"
            );
            assert_stranded_by_gate(&mgr, ProcessState::Failed).await;

            write_agent_yaml(dir.path(), true);
            let result = mgr.handle_reload_config(&exit_tx).await?;

            assert!(result.unchanged.contains(&"gated-svc".to_string()));
            assert!(
                mgr.processes().await[0].is_running(),
                "reload must restart a process the closed gate stranded at exit"
            );

            cleanup_first_process(&mgr).await;
            Ok(())
        }

        #[tokio::test]
        async fn test_reload_restarts_process_stranded_during_backoff() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let mgr = ProcessManager::new(
                loader(vec![gated_on_failure_sleep_def("gated-svc", &yaml)]),
                uuid_gen(),
            );
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            mgr.start(&exit_tx).await;
            let pid = mgr.processes().await[0].pid().unwrap();

            // The exit is recorded with the gate still open, so the restart is
            // queued; the gate then closes during the backoff delay.
            {
                let mut procs = mgr.processes.write().await;
                procs[0].set_last_status(exit_status(false));
            }
            test_helpers::cleanup_process(pid);
            write_agent_yaml(dir.path(), false);
            mgr.complete_restart("gated-svc", &exit_tx).await;
            assert_stranded_by_gate(&mgr, ProcessState::Failed).await;

            write_agent_yaml(dir.path(), true);
            mgr.handle_reload_config(&exit_tx).await?;

            assert!(
                mgr.processes().await[0].is_running(),
                "reload must restart a process whose gate closed during the backoff"
            );

            cleanup_first_process(&mgr).await;
            Ok(())
        }

        #[tokio::test]
        async fn test_reload_restarts_process_stranded_by_its_own_reload() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let config_loader = Arc::new(MutableConfigLoader::new(vec![gated_sleep_def(
                "gated-svc",
                &yaml,
            )]));
            let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            mgr.start(&exit_tx).await;
            assert!(mgr.processes().await[0].is_running());

            // A config change and a gate close land in the same reload: the
            // process is stopped for the restart, then the gate blocks it.
            write_agent_yaml(dir.path(), false);
            config_loader.set(vec![gated_sleep_def_secs(
                "gated-svc",
                &yaml,
                test_helpers::ALT_TEST_SLEEP_SECS,
            )]);
            let result = mgr.handle_reload_config(&exit_tx).await?;
            assert!(result.modified.contains(&"gated-svc".to_string()));
            assert_stranded_by_gate(&mgr, ProcessState::Stopped).await;

            write_agent_yaml(dir.path(), true);
            mgr.handle_reload_config(&exit_tx).await?;

            assert!(
                mgr.processes().await[0].is_running(),
                "reload must restart a process its own earlier reload left Stopped behind a gate"
            );

            cleanup_first_process(&mgr).await;
            Ok(())
        }

        #[tokio::test]
        async fn test_gate_flap_recovers_manually_started_no_auto_start_process()
        -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let mut def = gated_on_failure_sleep_def("manual-svc", &yaml);
            def.config.auto_start = false;
            let mgr = ProcessManager::new(loader(vec![def]), uuid_gen());
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
            let (restart_tx, _restart_rx) = mpsc::channel::<String>(8);

            mgr.start(&exit_tx).await;
            assert_eq!(mgr.processes().await[0].state(), ProcessState::Created);

            mgr.handle_start("manual-svc", &exit_tx).await?;
            write_agent_yaml(dir.path(), false);
            crash(&mgr, "manual-svc", &restart_tx).await;
            assert_stranded_by_gate(&mgr, ProcessState::Failed).await;

            write_agent_yaml(dir.path(), true);
            mgr.handle_reload_config(&exit_tx).await?;

            assert!(
                mgr.processes().await[0].is_running(),
                "recovery must use may_respawn, so auto_start=false cannot strand a process an operator started"
            );

            cleanup_first_process(&mgr).await;
            Ok(())
        }

        // Negatives: the other routes into a terminal state reach the same
        // guard, and reload must leave every one of them alone. Together with
        // `test_reload_does_not_restart_stopped_process` above, which covers an
        // operator stop, these are what keep the recovery narrow.

        #[tokio::test]
        async fn test_reload_does_not_rerun_completed_one_shot() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let (cmd, args) = test_helpers::true_cmd();
            let mut config = test_helpers::make_config(cmd, args);
            config.condition_config_any = gate_on_process_collection(&yaml);
            let mgr = ProcessManager::new(
                loader(vec![ProcessDefinition {
                    name: "one-shot".to_string(),
                    config,
                }]),
                uuid_gen(),
            );
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
            let (restart_tx, _restart_rx) = mpsc::channel::<String>(8);

            mgr.start(&exit_tx).await;
            let pid = mgr.processes().await[0].pid().unwrap();
            let event = ExitEvent {
                name: "one-shot".to_string(),
                pid,
                status: exit_status(true),
            };
            mgr.handle_exit(event, &restart_tx).await;
            assert_eq!(mgr.processes().await[0].state(), ProcessState::Exited);

            mgr.handle_reload_config(&exit_tx).await?;

            let procs = mgr.processes().await;
            assert_eq!(
                procs[0].state(),
                ProcessState::Exited,
                "a restart=never one-shot that ran to completion must not re-run on reload"
            );
            Ok(())
        }

        #[tokio::test]
        async fn test_reload_does_not_bypass_burst_limit() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let mut def = gated_on_failure_sleep_def("crasher", &yaml);
            def.config.start_limit_burst = Some(1);
            def.config.start_limit_interval_sec = Some(3600);
            let mgr = ProcessManager::new(loader(vec![def]), uuid_gen());
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
            let (restart_tx, _restart_rx) = mpsc::channel::<String>(8);

            // The gate stays open throughout: the burst limit is the only
            // reason the second crash does not restart.
            mgr.start(&exit_tx).await;
            crash(&mgr, "crasher", &restart_tx).await;
            mgr.handle_start("crasher", &exit_tx).await?;
            crash(&mgr, "crasher", &restart_tx).await;

            {
                let procs = mgr.processes().await;
                assert_eq!(procs[0].state(), ProcessState::Failed);
                assert!(
                    !procs[0].restart_blocked_by_conditions(),
                    "the burst limit is not a condition skip"
                );
            }

            mgr.handle_reload_config(&exit_tx).await?;

            assert!(
                !mgr.processes().await[0].is_running(),
                "reload must not restart a crash loop the burst limit stopped"
            );
            Ok(())
        }

        /// A binary that spawns only once it is written, so that a reload
        /// retrying the spawn is visible as a `Running` process. Asserting
        /// only that the process is not running would pass either way: a
        /// retried bad spawn lands back in `Failed`.
        fn write_late_binary(path: &std::path::Path) {
            use std::os::unix::fs::PermissionsExt;
            let (cmd, args) = test_helpers::sleep_cmd(test_helpers::TEST_SLEEP_SECS);
            std::fs::write(path, format!("#!/bin/sh\nexec {cmd} {}\n", args.join(" "))).unwrap();
            std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o755)).unwrap();
        }

        #[tokio::test]
        async fn test_reload_does_not_restart_after_spawn_failure() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let binary = dir.path().join("late-binary");
            let mut def = gated_on_failure_sleep_def("broken", &yaml);
            def.config.command = binary.to_string_lossy().into_owned();
            def.config.args = vec![];
            let mgr = ProcessManager::new(loader(vec![def]), uuid_gen());
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

            // The gate is open, so the failure is the spawn itself.
            mgr.start(&exit_tx).await;
            {
                let procs = mgr.processes().await;
                assert_eq!(procs[0].state(), ProcessState::Failed);
                assert!(!procs[0].restart_blocked_by_conditions());
            }

            write_late_binary(&binary);
            mgr.handle_reload_config(&exit_tx).await?;

            let procs = mgr.processes().await;
            assert!(
                !procs[0].is_running(),
                "reload must not hot-loop a binary that cannot be spawned"
            );
            Ok(())
        }
    }
}
