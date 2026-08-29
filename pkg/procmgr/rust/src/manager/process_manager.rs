// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::catalog::ProcessCatalog;
use super::{
    ExitEvent, PendingRestart, RuntimeContext, Supervisor, enqueue_pending_restart, resolve_index,
    spawn::{
        SpawnKind, SpawnProcessOutcome, log_skipped_pending_restart, pending_restart_still_valid,
        reserve_spawn, spawn_process, spawn_process_background,
    },
};
use crate::command::{Command, CreateResult, StartResult, StopResult};
use crate::config::{self, ConfigLoader};
use crate::process::ManagedProcess;
#[cfg(windows)]
use crate::shutdown;
use crate::state::ProcessState;
use crate::uuid_gen::UuidGenerator;
use log::{debug, info, warn};
use std::sync::Arc;
use tonic::Status;

#[derive(Clone)]
pub struct ProcessManager {
    pub(in crate::manager) catalog: Arc<ProcessCatalog>,
}

impl ProcessManager {
    pub fn new(config_loader: Arc<dyn ConfigLoader>, uuid_gen: Arc<dyn UuidGenerator>) -> Self {
        Self {
            catalog: Arc::new(ProcessCatalog::load(config_loader, uuid_gen)),
        }
    }

    pub fn supervisor(self) -> Supervisor {
        Supervisor::new(self)
    }

    pub(crate) async fn processes(&self) -> tokio::sync::RwLockReadGuard<'_, Vec<ManagedProcess>> {
        self.catalog.read_processes().await
    }

    pub(crate) fn config_source(&self) -> &str {
        self.catalog.config_source()
    }

    pub(crate) fn config_location(&self) -> &str {
        self.catalog.config_location()
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
            Command::ReloadConfig { reply } => {
                let _ = reply.send(self.handle_reload_config(ctx).await);
            }
        }
    }

    pub(in crate::manager) async fn handle_exit(&self, event: ExitEvent, ctx: &RuntimeContext) {
        let mut procs = self.catalog.write_processes().await;
        let Some(proc) = procs.iter_mut().find(|p| p.name() == event.name) else {
            warn!("exit event for unknown process '{}'", event.name);
            return;
        };

        if proc.pid() == Some(event.pid) && proc.state() == crate::state::ProcessState::Stopping {
            debug!(
                "[{}] ignoring exit event during stop wait (pid={})",
                proc.name(),
                event.pid
            );
            return;
        }

        if proc.pid() == Some(event.pid) && proc.state().is_alive() {
            info!("[{}] exited with {}", proc.name(), event.status);
            proc.set_last_status(event.status);
            #[cfg(windows)]
            proc.ensure_windows_spawn_resources_released(shutdown::ShutdownBudget::unlimited(
                std::time::Instant::now(),
            ))
            .await;
            if ctx.lifecycle.spawns_allowed() {
                enqueue_pending_restart(proc, ctx);
            }
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
            let procs = self.catalog.read_processes().await;
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
            Arc::clone(&self.catalog),
            idx,
            ctx.clone(),
            SpawnKind::Restart(pending),
            None,
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
        let (uuid, auto_start_idx, auto_start, warnings) =
            self.catalog.append_runtime(&name, config).await?;
        if auto_start {
            let reservation =
                reserve_spawn(&self.catalog, auto_start_idx, &SpawnKind::CreateAutoStart)
                    .await
                    .map_err(|e| {
                        Status::internal(format!(
                            "failed to reserve auto-start for '{name}': {e:#}"
                        ))
                    })?;
            if let Some(token) = reservation {
                spawn_process_background(
                    Arc::clone(&self.catalog),
                    auto_start_idx,
                    ctx.clone(),
                    SpawnKind::CreateAutoStart,
                    Some(token),
                );
            }
        }
        Ok(CreateResult { uuid, warnings })
    }

    pub(in crate::manager) async fn handle_start(
        &self,
        name_or_uuid: &str,
        ctx: &RuntimeContext,
    ) -> Result<StartResult, Status> {
        let (idx, name) = {
            let procs = self.catalog.read_processes().await;
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

        let outcome = spawn_process(Arc::clone(&self.catalog), idx, ctx, SpawnKind::Manual, None)
            .await
            .map_err(|e| Status::internal(format!("failed to start '{name}': {e:#}")))?;

        match outcome {
            SpawnProcessOutcome::NotStarted => Err(Status::failed_precondition(format!(
                "process '{name}' is already starting or running",
            ))),
            SpawnProcessOutcome::Committed(snapshot) => Ok(StartResult {
                uuid: snapshot.uuid,
                pid: snapshot.pid,
                state: snapshot.state,
            }),
        }
    }

    pub(in crate::manager) async fn handle_stop(
        &self,
        name_or_uuid: &str,
    ) -> Result<StopResult, Status> {
        let (idx, uuid) = {
            let mut procs = self.catalog.write_processes().await;
            let idx = resolve_index(&procs, name_or_uuid)?;
            let proc = &mut procs[idx];

            if !proc.is_running() {
                return Err(Status::failed_precondition(format!(
                    "process '{}' is not running",
                    proc.name()
                )));
            }
            let uuid = proc.uuid().to_owned();
            if proc.state() == ProcessState::Stopping {
                debug!(
                    "[{}] Stop coalesced onto in-flight stop request",
                    proc.name()
                );
            } else {
                proc.request_stop();
            }
            (idx, uuid)
        };

        self.catalog
            .wait_for_process_stop(idx, crate::shutdown::ShutdownBudget::for_single_stop())
            .await;

        let state = self.catalog.read_processes().await[idx].state();
        Ok(StopResult { uuid, state })
    }

    pub(in crate::manager) async fn shutdown(&self) {
        self.catalog.shutdown().await;
    }
}
