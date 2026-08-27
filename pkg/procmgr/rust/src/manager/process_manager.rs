use super::{
    ExitEvent, PendingRestart, RuntimeHandles, Supervisor, enqueue_pending_restart,
    find_index_by_name, resolve_index, try_auto_start,
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

    pub(in crate::manager) async fn auto_start_all(&self, handles: &RuntimeHandles) {
        let order = self.startup_order.read().await;
        let mut procs = self.processes.write().await;
        for &idx in order.iter() {
            try_auto_start(&mut procs[idx], handles);
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

    pub(in crate::manager) async fn handle_exit(&self, event: ExitEvent, handles: &RuntimeHandles) {
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
                Instant::now(),
            ))
            .await;
            enqueue_pending_restart(proc, handles);
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
        if pending.spawn_seq != proc.spawn_seq() {
            info!(
                "[{name}] ignoring stale queued restart (spawn_seq {} != {})",
                pending.spawn_seq,
                proc.spawn_seq()
            );
            return;
        }
        if proc.is_running() {
            info!("[{name}] already running, skipping queued restart");
            return;
        }
        if !proc.should_complete_pending_restart() {
            info!("[{name}] not restarting: policy or start conditions not met");
            return;
        }
        if let Err(e) = proc.spawn_and_watch(handles.exit_tx.clone()) {
            warn!("[{name}] restart failed: {e:#}");
            enqueue_pending_restart(proc, handles);
        }
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
            try_auto_start(procs.last_mut().unwrap(), handles);
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
        proc.spawn_and_watch(handles.exit_tx.clone())
            .map_err(|e| Status::internal(format!("failed to start '{name}': {e:#}")))?;
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
