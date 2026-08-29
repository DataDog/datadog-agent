use super::catalog::ProcessCatalog;
use super::spawn::{SpawnKind, spawn_process};
use super::{ProcessManager, RuntimeContext};
use crate::command::ReloadResult;
use crate::config::ProcessDefinition;
use crate::process::{ManagedProcess, ProcessOrigin};
use crate::state::ProcessState;
use log::{info, warn};
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use tonic::Status;

/// Pre-reload snapshot for a process whose config file changed.
pub(super) struct ModifiedReloadContext {
    was_running: bool,
    auto_start_before: bool,
}

impl ModifiedReloadContext {
    fn capture(proc: &ManagedProcess) -> Self {
        Self {
            was_running: proc.is_running(),
            auto_start_before: proc.config().auto_start,
        }
    }

    /// True when reload did not toggle `auto_start`, so a failed process may
    /// inherit a pre-reload crash retry. Skips bootstrap failures that reload
    /// disables via `auto_start: true` → `false`.
    fn allows_failed_manual_retry(&self, proc: &ManagedProcess) -> bool {
        self.auto_start_before == proc.config().auto_start
    }
}

pub(super) struct ReloadConfigApplyResult {
    pub added: Vec<String>,
    pub modified: HashMap<String, ModifiedReloadContext>,
    pub unchanged: Vec<String>,
    added_start: Vec<usize>,
}

pub(super) fn detach_removed_config_processes(
    procs: &mut Vec<ManagedProcess>,
    new_names: &HashSet<&str>,
) -> (Vec<String>, Vec<ManagedProcess>) {
    let mut removed = Vec::new();
    let mut procs_to_stop = Vec::new();
    let mut i = 0;
    while i < procs.len() {
        if procs[i].origin() == ProcessOrigin::Config && !new_names.contains(procs[i].name()) {
            let proc = procs.remove(i);
            info!("[{}] config removed, stopping", proc.name());
            removed.push(proc.name().to_owned());
            procs_to_stop.push(proc);
        } else {
            i += 1;
        }
    }
    (removed, procs_to_stop)
}

fn should_stop_running_after_reload(
    proc: &ManagedProcess,
    start_conditions_was: Option<bool>,
) -> bool {
    if !proc.config().auto_start {
        return !proc.start_conditions_met() && start_conditions_was == Some(true);
    }
    !proc.start_conditions_met()
}

fn should_resume_after_running_reload(proc: &ManagedProcess) -> bool {
    if proc.config().auto_start {
        proc.may_auto_start()
    } else {
        proc.start_conditions_met()
    }
}

fn should_retry_failed_after_reload(proc: &ManagedProcess, ctx: &ModifiedReloadContext) -> bool {
    if proc.config().auto_start {
        return proc.may_auto_start();
    }
    ctx.allows_failed_manual_retry(proc) && proc.should_complete_pending_restart()
}

fn should_start_after_config_reload(proc: &ManagedProcess, ctx: &ModifiedReloadContext) -> bool {
    if ctx.was_running {
        return should_resume_after_running_reload(proc);
    }
    if proc.state() == ProcessState::Failed {
        return should_retry_failed_after_reload(proc, ctx);
    }
    false
}

fn reload_spawn_kind(proc: &ManagedProcess) -> SpawnKind {
    if proc.may_auto_start() {
        SpawnKind::CreateAutoStart
    } else {
        SpawnKind::Manual
    }
}

async fn spawn_reload_process(
    catalog: Arc<ProcessCatalog>,
    idx: usize,
    ctx: &RuntimeContext,
    kind: SpawnKind,
    failure_context: &str,
) {
    if let Err(e) = spawn_process(catalog, idx, ctx, kind, None).await {
        warn!("[idx={idx}] {failure_context}: {e:#}");
    }
}

async fn restart_after_config_change(
    proc: &mut ManagedProcess,
    modified_ctx: &ModifiedReloadContext,
) -> Option<SpawnKind> {
    if modified_ctx.was_running {
        proc.stop().await;
    }

    let kind = if should_start_after_config_reload(proc, modified_ctx) {
        info!("[{}] starting after config reload", proc.name());
        Some(reload_spawn_kind(proc))
    } else {
        info!("[{}] skipping start after config reload", proc.name());
        None
    };

    proc.record_config_gate_met();
    kind
}

async fn reconcile_process_after_reload(
    proc: &mut ManagedProcess,
    modified_ctx: Option<&ModifiedReloadContext>,
) -> Option<SpawnKind> {
    if proc.origin() != ProcessOrigin::Config {
        return None;
    }

    if let Some(modified) = modified_ctx {
        return restart_after_config_change(proc, modified).await;
    }

    let want_start = proc.may_auto_start();
    let start_conditions_was = proc.last_start_conditions_met();

    if proc.is_running() && should_stop_running_after_reload(proc, start_conditions_was) {
        info!("[{}] start conditions no longer met, stopping", proc.name());
        proc.stop().await;
    } else if !proc.is_running() && want_start && start_conditions_was == Some(false) {
        info!("[{}] start conditions now met, starting", proc.name());
        proc.record_config_gate_met();
        return Some(SpawnKind::CreateAutoStart);
    }

    proc.record_config_gate_met();
    None
}

impl ProcessManager {
    pub(crate) async fn handle_reload_config(
        &self,
        ctx: &RuntimeContext,
    ) -> Result<ReloadResult, Status> {
        crate::config_gate::clear_secret_caches();

        let new_configs = self.catalog.config_loader().load();

        let removed = self
            .remove_and_stop_dropped_config_processes(&new_configs)
            .await;

        let apply = self.apply_reloaded_config_processes(new_configs).await;
        for idx in apply.added_start.iter().copied() {
            spawn_reload_process(
                Arc::clone(&self.catalog),
                idx,
                ctx,
                SpawnKind::CreateAutoStart,
                "failed to start",
            )
            .await;
        }

        self.reconcile_processes_after_reload(&apply, ctx).await;

        self.catalog.update_startup_order().await;
        Ok(ReloadResult {
            added: apply.added,
            removed,
            modified: apply.modified.keys().cloned().collect(),
            unchanged: apply.unchanged,
        })
    }

    async fn remove_and_stop_dropped_config_processes(
        &self,
        new_configs: &[ProcessDefinition],
    ) -> Vec<String> {
        let new_names: HashSet<&str> = new_configs.iter().map(|c| c.name.as_str()).collect();
        let (removed, mut procs_to_stop) = {
            let mut procs = self.catalog.write_processes().await;
            detach_removed_config_processes(&mut procs, &new_names)
        };
        for proc in &mut procs_to_stop {
            proc.stop().await;
        }
        removed
    }

    async fn apply_reloaded_config_processes(
        &self,
        new_configs: Vec<ProcessDefinition>,
    ) -> ReloadConfigApplyResult {
        let mut procs = self.catalog.write_processes().await;
        let mut added = Vec::new();
        let mut added_start = Vec::new();
        let mut modified = HashMap::new();
        let mut unchanged = Vec::new();
        for np in new_configs {
            if let Some(existing) = procs.iter_mut().find(|p| p.name() == np.name) {
                if existing.config() != &np.config {
                    info!("[{}] config changed, updating", np.name);
                    modified.insert(np.name.clone(), ModifiedReloadContext::capture(existing));
                    existing.set_config(np.config);
                } else {
                    unchanged.push(np.name);
                }
            } else {
                info!("[{}] new config found, adding", np.name);
                let mut proc = ManagedProcess::new_config(
                    np.name.clone(),
                    self.catalog.generate_uuid(),
                    np.config,
                );
                if proc.may_auto_start() {
                    added_start.push(procs.len());
                }
                proc.record_config_gate_met();
                added.push(np.name);
                procs.push(proc);
            }
        }
        ReloadConfigApplyResult {
            added,
            modified,
            unchanged,
            added_start,
        }
    }

    async fn reconcile_processes_after_reload(
        &self,
        apply: &ReloadConfigApplyResult,
        ctx: &RuntimeContext,
    ) {
        let mut to_start = Vec::new();
        {
            let mut procs = self.catalog.write_processes().await;
            for (idx, proc) in procs.iter_mut().enumerate() {
                if let Some(kind) =
                    reconcile_process_after_reload(proc, apply.modified.get(proc.name())).await
                {
                    to_start.push((idx, kind));
                }
            }
        }
        for (idx, kind) in to_start {
            spawn_reload_process(
                Arc::clone(&self.catalog),
                idx,
                ctx,
                kind,
                "failed to start after config reload",
            )
            .await;
        }
    }
}
