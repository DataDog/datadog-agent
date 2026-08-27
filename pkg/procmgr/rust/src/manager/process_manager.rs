// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::{
    ExitEvent, PendingRestart, RuntimeContext, Supervisor, enqueue_pending_restart,
    find_index_by_name, resolve_index,
    spawn::{
        SpawnKind, log_skipped_pending_restart, pending_restart_still_valid, spawn_process,
        spawn_process_background,
    },
};
use crate::command::{Command, CreateResult, StartResult, StopResult};
use crate::config::{self, ConfigLoader, ProcessDefinition};
use crate::ordering;
use crate::process::ManagedProcess;
use crate::shutdown;
use crate::uuid_gen::UuidGenerator;
use anyhow::Result;
use log::{debug, info, warn};
use std::sync::Arc;
use tokio::sync::RwLock;
use tonic::Status;

#[derive(Clone)]
pub struct ProcessManager {
    pub(in crate::manager) processes: Arc<RwLock<Vec<ManagedProcess>>>,
    pub(in crate::manager) startup_order: Arc<RwLock<Vec<usize>>>,
    pub(in crate::manager) config_loader: Arc<dyn ConfigLoader>,
    pub(in crate::manager) uuid_gen: Arc<dyn UuidGenerator>,
}

impl ProcessManager {
    pub fn new(config_loader: Arc<dyn ConfigLoader>, uuid_gen: Arc<dyn UuidGenerator>) -> Self {
        let (processes, startup_order) = load_catalog(config_loader.as_ref(), uuid_gen.as_ref());
        Self {
            processes: Arc::new(RwLock::new(processes)),
            startup_order: Arc::new(RwLock::new(startup_order)),
            config_loader,
            uuid_gen,
        }
    }

    pub fn supervisor(self) -> Supervisor {
        Supervisor::new(self)
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

    pub(in crate::manager) async fn handle_command(&self, ctx: &RuntimeContext, cmd: Command) {
        match cmd {
            Command::Create {
                name,
                config,
                reply,
            } => {
                let _ = reply.send(self.handle_create(name, *config, ctx).await);
            }
            Command::Start {
                name_or_uuid,
                reply,
            } => {
                let _ = reply.send(self.handle_start(&name_or_uuid, ctx).await);
            }
            Command::Stop {
                name_or_uuid,
                reply,
            } => {
                let _ = reply.send(self.handle_stop(&name_or_uuid).await);
            }
        }
    }

    pub(in crate::manager) async fn handle_exit(&self, event: ExitEvent, ctx: &RuntimeContext) {
        let mut procs = self.processes.write().await;
        let Some(proc) = procs.iter_mut().find(|p| p.name() == event.name) else {
            warn!("exit event for unknown process '{}'", event.name);
            return;
        };

        if proc.pid() == Some(event.pid) && proc.state().is_alive() {
            info!("[{}] exited with {}", proc.name(), event.status);
            proc.set_last_status(event.status);
            #[cfg(windows)]
            proc.ensure_windows_spawn_resources_released(shutdown::ShutdownBudget::unlimited(
                std::time::Instant::now(),
            ))
            .await;
            enqueue_pending_restart(proc, ctx);
            return;
        }

        let name = proc.name().to_owned();
        let current_pid = proc.pid();
        let state = proc.state();
        drop(procs);

        debug!(
            "[{name}] ignoring stale exit event for pid {} (current pid: {current_pid:?}, state: {state})",
            event.pid
        );
    }

    pub(in crate::manager) async fn complete_restart(
        &self,
        pending: PendingRestart,
        ctx: &RuntimeContext,
    ) {
        let idx = {
            let procs = self.processes.read().await;
            let Some((idx, proc)) = procs
                .iter()
                .enumerate()
                .find(|(_, p)| p.uuid() == pending.uuid)
            else {
                warn!("restart for unknown process '{}'", pending.uuid);
                return;
            };
            if !pending_restart_still_valid(proc, &pending) {
                log_skipped_pending_restart(proc, &pending);
                return;
            }
            idx
        };

        spawn_process_background(
            Arc::clone(&self.processes),
            idx,
            ctx.clone(),
            SpawnKind::Restart(pending),
        );
    }

    pub(in crate::manager) async fn handle_create(
        &self,
        name: String,
        config: config::ProcessConfig,
        ctx: &RuntimeContext,
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
        let (uuid, auto_start_idx, auto_start) = {
            let mut procs = self.processes.write().await;
            if find_index_by_name(&procs, &name).is_some() {
                return Err(Status::already_exists(format!(
                    "process '{name}' already exists"
                )));
            }
            let proc = ManagedProcess::new_runtime(name.clone(), self.uuid_gen.generate(), config);
            let uuid = proc.uuid().to_owned();
            let auto_start = proc.may_auto_start();
            info!("[{name}] created via RPC (uuid={uuid})");
            procs.push(proc);
            (uuid, procs.len() - 1, auto_start)
        };
        if auto_start {
            spawn_process_background(
                Arc::clone(&self.processes),
                auto_start_idx,
                ctx.clone(),
                SpawnKind::CreateAutoStart,
            );
        }
        let warnings = self.update_startup_order().await;
        Ok(CreateResult { uuid, warnings })
    }

    pub(in crate::manager) async fn handle_start(
        &self,
        name_or_uuid: &str,
        ctx: &RuntimeContext,
    ) -> Result<StartResult, Status> {
        let (idx, name) = {
            let procs = self.processes.read().await;
            let idx = resolve_index(&procs, name_or_uuid)?;
            let proc = &procs[idx];
            let name = proc.name().to_owned();

            if proc.is_running() {
                return Err(Status::failed_precondition(format!(
                    "process '{name}' is already running",
                )));
            }
            (idx, name)
        };

        spawn_process(Arc::clone(&self.processes), idx, ctx, SpawnKind::Manual)
            .await
            .map_err(|e| Status::internal(format!("failed to start '{name}': {e:#}")))?;

        let procs = self.processes.read().await;
        let proc = &procs[idx];
        if !proc.is_running() {
            return Err(Status::internal(format!(
                "failed to start '{name}': process is not running"
            )));
        }
        Ok(StartResult {
            uuid: proc.uuid().to_owned(),
            pid: proc.pid(),
            state: proc.state(),
        })
    }

    pub(in crate::manager) async fn handle_stop(
        &self,
        name_or_uuid: &str,
    ) -> Result<StopResult, Status> {
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
        proc.stop().await;
        let state = proc.state();
        Ok(StopResult { uuid, state })
    }

    pub(in crate::manager) async fn update_startup_order(&self) -> Vec<String> {
        let result = recompute_startup_order(&self.processes.read().await);
        *self.startup_order.write().await = result.order;
        result.warnings
    }

    pub(in crate::manager) async fn shutdown(&self) {
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

struct StartupOrderResult {
    order: Vec<usize>,
    warnings: Vec<String>,
}

fn load_catalog(
    config_loader: &dyn ConfigLoader,
    uuid_gen: &dyn UuidGenerator,
) -> (Vec<ManagedProcess>, Vec<usize>) {
    let processes: Vec<ManagedProcess> = config_loader
        .load()
        .into_iter()
        .map(|pd| ManagedProcess::new_config(pd.name, uuid_gen.generate(), pd.config))
        .collect();
    let startup_order = recompute_startup_order(&processes).order;
    (processes, startup_order)
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
