use super::{ProcessManager, queue_restart, try_spawn_and_watch};
use crate::command::ReloadResult;
use crate::config::ProcessDefinition;
use crate::process::{ManagedProcess, ProcessOrigin};
use crate::state::ProcessState;
use log::{info, warn};
use super::supervisor::RuntimeHandles;
use tonic::Status;

pub(super) struct ReloadConfigApplyResult {
    pub added: Vec<String>,
    pub modified: Vec<String>,
    pub modified_running: Vec<String>,
    pub unchanged: Vec<String>,
}

pub(super) fn detach_removed_config_processes(
    procs: &mut Vec<ManagedProcess>,
    new_names: &std::collections::HashSet<&str>,
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

fn should_start_after_config_reload(proc: &ManagedProcess, was_running: bool) -> bool {
    if was_running {
        if proc.config().auto_start {
            proc.may_auto_start()
        } else {
            proc.start_conditions_met()
        }
    } else if proc.state() == ProcessState::Failed && proc.config().auto_start {
        proc.may_auto_start()
    } else {
        false
    }
}

async fn restart_after_config_change(
    proc: &mut ManagedProcess,
    handles: &RuntimeHandles,
    was_running: bool,
) {
    if was_running {
        proc.stop().await;
    }

    if should_start_after_config_reload(proc, was_running) {
        info!("[{}] starting after config reload", proc.name());
        if let Err(e) = try_spawn_and_watch(proc, handles) {
            warn!(
                "[{}] failed to start after config reload: {e:#}",
                proc.name()
            );
            queue_restart(proc, handles);
        }
    } else {
        info!("[{}] skipping start after config reload", proc.name());
    }

    proc.record_config_gate_met();
}

async fn reconcile_process_after_reload(
    proc: &mut ManagedProcess,
    handles: &RuntimeHandles,
    was_running: bool,
    config_changed: bool,
) {
    if proc.origin() != ProcessOrigin::Config {
        return;
    }

    if config_changed {
        restart_after_config_change(proc, handles, was_running).await;
        return;
    }

    let want_start = proc.may_auto_start();
    let start_conditions_was = proc.last_start_conditions_met();

    if proc.is_running() && should_stop_running_after_reload(proc, start_conditions_was) {
        info!("[{}] start conditions no longer met, stopping", proc.name());
        proc.stop().await;
    } else if !proc.is_running() && want_start && start_conditions_was == Some(false) {
        info!("[{}] start conditions now met, starting", proc.name());
        if let Err(e) = try_spawn_and_watch(proc, handles) {
            warn!(
                "[{}] failed to start after start conditions opened: {e:#}",
                proc.name()
            );
            queue_restart(proc, handles);
        }
    }

    proc.record_config_gate_met();
}

impl ProcessManager {
    pub(crate) async fn handle_reload_config(
        &self,
        handles: &RuntimeHandles,
    ) -> Result<ReloadResult, Status> {
        let new_configs = self.config_loader.load();

        let removed = self
            .remove_and_stop_dropped_config_processes(&new_configs)
            .await;

        let apply = self
            .apply_reloaded_config_processes(new_configs, handles)
            .await;

        self.reconcile_processes_after_reload(&apply, handles)
            .await;

        self.update_startup_order().await;
        Ok(ReloadResult {
            added: apply.added,
            removed,
            modified: apply.modified,
            unchanged: apply.unchanged,
        })
    }

    async fn remove_and_stop_dropped_config_processes(
        &self,
        new_configs: &[ProcessDefinition],
    ) -> Vec<String> {
        let new_names: std::collections::HashSet<&str> =
            new_configs.iter().map(|c| c.name.as_str()).collect();
        let (removed, mut procs_to_stop) = {
            let mut procs = self.processes.write().await;
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
        handles: &RuntimeHandles,
    ) -> ReloadConfigApplyResult {
        let mut procs = self.processes.write().await;
        let mut added = Vec::new();
        let mut modified = Vec::new();
        let mut modified_running = Vec::new();
        let mut unchanged = Vec::new();
        for np in new_configs {
            if let Some(existing) = procs.iter_mut().find(|p| p.name() == np.name) {
                if existing.config() != &np.config {
                    info!("[{}] config changed, updating", np.name);
                    if existing.is_running() {
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
                if proc.may_auto_start()
                    && let Err(e) = try_spawn_and_watch(&mut proc, handles)
                {
                    warn!("[{}] failed to start: {e:#}", np.name);
                    queue_restart(&mut proc, handles);
                }
                proc.record_config_gate_met();
                added.push(np.name);
                procs.push(proc);
            }
        }
        ReloadConfigApplyResult {
            added,
            modified,
            modified_running,
            unchanged,
        }
    }

    async fn reconcile_processes_after_reload(
        &self,
        apply: &ReloadConfigApplyResult,
        handles: &RuntimeHandles,
    ) {
        let stopped_for_config_update: std::collections::HashSet<String> =
            apply.modified_running.iter().cloned().collect();
        let modified_names: std::collections::HashSet<String> =
            apply.modified.iter().cloned().collect();
        let mut procs = self.processes.write().await;
        for proc in procs.iter_mut() {
            reconcile_process_after_reload(
                proc,
                handles,
                stopped_for_config_update.contains(proc.name()),
                modified_names.contains(proc.name()),
            )
            .await;
        }
    }
}
