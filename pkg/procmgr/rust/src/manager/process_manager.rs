use super::{
    ExitEvent, PendingRestart, RuntimeHandles, Supervisor, find_index_by_name, queue_restart,
    resolve_index, try_spawn_and_watch,
};
use crate::command::{CreateResult, StartResult, StopResult};
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
        let configs = config_loader.load();
        let mut processes: Vec<ManagedProcess> = configs
            .into_iter()
            .map(|pd| ManagedProcess::new_config(pd.name, uuid_gen.generate(), pd.config))
            .collect();
        for proc in &mut processes {
            proc.record_config_gate_met();
        }
        let startup_result = recompute_startup_order(&processes);
        Self {
            processes: Arc::new(RwLock::new(processes)),
            startup_order: Arc::new(RwLock::new(startup_result.order)),
            config_loader,
            uuid_gen,
        }
    }

    /// Wrap this manager in a [`Supervisor`] for daemon execution.
    pub fn supervisor(self) -> Supervisor {
        Supervisor::new(self)
    }

    pub(in crate::manager) async fn start_configured_processes(&self, handles: &RuntimeHandles) {
        let order = self.startup_order.read().await;
        let mut procs = self.processes.write().await;
        for &idx in order.iter() {
            let proc = &mut procs[idx];
            let name = proc.name().to_owned();
            if proc.may_auto_start()
                && let Err(e) = try_spawn_and_watch(proc, handles)
            {
                warn!("[{name}] auto-start failed: {e:#}");
                queue_restart(proc, handles);
            }
            proc.record_config_gate_met();
        }
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

    pub(in crate::manager) async fn handle_exit(
        &self,
        event: ExitEvent,
        handles: &RuntimeHandles,
    ) {
        let mut procs = self.processes.write().await;
        let Some(proc) = procs.iter_mut().find(|p| p.name() == event.name) else {
            warn!("exit event for unknown process '{}'", event.name);
            return;
        };

        if proc.pid() == Some(event.pid) && proc.state().is_alive() {
            info!("[{}] exited with {}", proc.name(), event.status);
            proc.set_last_status(event.status);
            #[cfg(windows)]
            proc.ensure_windows_spawn_resources_released().await;
            queue_restart(proc, handles);
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
        handles: &RuntimeHandles,
    ) {
        let mut procs = self.processes.write().await;
        let Some(proc) = procs.iter_mut().find(|p| p.uuid() == pending.uuid) else {
            warn!("restart for unknown process '{}'", pending.uuid);
            return;
        };
        let name = proc.name().to_owned();
        if proc.is_running() {
            info!("[{name}] already running, skipping queued restart");
            return;
        }
        if pending.config_generation != proc.config_generation() {
            info!("[{name}] discarding stale retry after config reload");
            return;
        }
        if !proc.should_complete_pending_restart() {
            info!("[{name}] not restarting: policy or start conditions not met");
            proc.record_config_gate_met();
            return;
        }
        if let Err(e) = try_spawn_and_watch(proc, handles) {
            warn!("[{name}] restart failed: {e:#}");
            queue_restart(proc, handles);
        }
        proc.record_config_gate_met();
    }

    pub(in crate::manager) async fn handle_create(
        &self,
        name: String,
        config: config::ProcessConfig,
        handles: &RuntimeHandles,
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
            if find_index_by_name(&procs, &name).is_some() {
                return Err(Status::already_exists(format!(
                    "process '{name}' already exists"
                )));
            }
            let proc = ManagedProcess::new_runtime(name.clone(), self.uuid_gen.generate(), config);
            uuid = proc.uuid().to_owned();
            info!("[{name}] created via RPC (uuid={uuid})");
            procs.push(proc);
            let proc = procs.last_mut().unwrap();
            if proc.may_auto_start()
                && let Err(e) = try_spawn_and_watch(proc, handles)
            {
                warn!("[{name}] auto-start failed: {e:#}");
            }
        }
        let warnings = self.update_startup_order().await;
        Ok(CreateResult { uuid, warnings })
    }

    pub(in crate::manager) async fn handle_start(
        &self,
        name_or_uuid: &str,
        handles: &RuntimeHandles,
    ) -> Result<StartResult, Status> {
        let mut procs = self.processes.write().await;
        let idx = resolve_index(&procs, name_or_uuid)?;
        let proc = &mut procs[idx];
        let name = proc.name().to_owned();

        if proc.is_running() {
            return Err(Status::failed_precondition(format!(
                "process '{name}' is already running",
            )));
        }
        try_spawn_and_watch(proc, handles)
            .map_err(|e| Status::internal(format!("failed to start '{name}': {e:#}")))?;
        proc.record_config_gate_met();
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
