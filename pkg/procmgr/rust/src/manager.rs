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
#[cfg(windows)]
use crate::process::OrphanedDeferredExitCleanup;
#[cfg(windows)]
use crate::process::finish_orphaned_deferred_exit_cleanup;
use crate::process::{ManagedProcess, ProcessOrigin};
use crate::shutdown;
use crate::state::ProcessState;
use crate::uuid_gen::UuidGenerator;
use anyhow::Result;
use log::{debug, info, warn};
use std::sync::Arc;
use tokio::sync::{RwLock, mpsc, oneshot};
use tonic::Status;

pub(crate) struct ExitEvent {
    pub name: String,
    pub pid: u32,
    pub status: std::process::ExitStatus,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PendingRestart {
    uuid: String,
    config_generation: u64,
}

#[derive(Clone)]
pub struct ProcessManager {
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
    startup_order: Arc<RwLock<Vec<usize>>>,
    config_loader: Arc<dyn ConfigLoader>,
    uuid_gen: Arc<dyn UuidGenerator>,
    #[cfg(windows)]
    orphaned_deferred_exit_cleanups: Arc<RwLock<Vec<OrphanedDeferredExitCleanup>>>,
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
            #[cfg(windows)]
            orphaned_deferred_exit_cleanups: Arc::new(RwLock::new(Vec::new())),
        }
    }

    async fn start_configured_processes(
        &self,
        exit_tx: &mpsc::Sender<ExitEvent>,
        restart_tx: &mpsc::Sender<PendingRestart>,
    ) {
        let order = self.startup_order.read().await;
        let mut procs = self.processes.write().await;
        for &idx in order.iter() {
            let proc = &mut procs[idx];
            if proc.may_auto_start()
                && let Err(e) = try_spawn_and_watch(proc, exit_tx)
            {
                warn!("{e:#}");
                queue_restart(proc, restart_tx);
            }
            proc.record_config_gate_met();
        }
    }

    pub async fn run(&self) -> Result<()> {
        let (cmd_tx, mut cmd_rx) = mpsc::channel::<Command>(64);
        let (grpc_shutdown_tx, grpc_shutdown_rx) = oneshot::channel::<()>();
        let grpc_handle = tokio::spawn(grpc::server::run(self.clone(), cmd_tx, grpc_shutdown_rx));

        let (exit_tx, mut exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<PendingRestart>(256);
        self.start_configured_processes(&exit_tx, &restart_tx).await;

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
                Some(pending) = restart_rx.recv() => {
                    self.complete_restart(pending, &exit_tx, &restart_tx).await;
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
                            let _ = reply.send(self.handle_reload_config(&exit_tx, &restart_tx).await);
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

    pub(crate) async fn handle_exit(
        &self,
        event: ExitEvent,
        restart_tx: &mpsc::Sender<PendingRestart>,
    ) {
        let mut procs = self.processes.write().await;
        let Some(proc) = procs.iter_mut().find(|p| p.name() == event.name) else {
            drop(procs);
            #[cfg(windows)]
            if self
                .complete_orphaned_late_exit_cleanup(&event.name, event.pid)
                .await
            {
                debug!(
                    "[{}] matched orphaned deferred Windows cleanup after late exit (pid {})",
                    event.name, event.pid
                );
                if self
                    .deferred_job_drain_pending(&event.name, event.pid)
                    .await
                {
                    self.spawn_deferred_job_drain(event.name.clone(), event.pid);
                }
                return;
            }
            warn!("exit event for unknown process '{}'", event.name);
            return;
        };

        if proc.pid() == Some(event.pid) && proc.state().is_alive() {
            info!("[{}] exited with {}", proc.name(), event.status);
            #[cfg(windows)]
            let drain_pid = proc.set_last_status(event.status);
            #[cfg(not(windows))]
            {
                proc.set_last_status(event.status);
            }
            queue_restart(proc, restart_tx);
            drop(procs);
            #[cfg(windows)]
            if let Some(pid) = drain_pid {
                self.spawn_deferred_job_drain(event.name.clone(), pid);
            }
            return;
        }

        #[cfg(windows)]
        if proc.complete_late_exit_cleanup(event.pid) {
            debug!(
                "[{}] matched deferred Windows cleanup after late exit (pid {})",
                proc.name(),
                event.pid
            );
            let needs_drain = proc.has_deferred_job_drain(event.pid);
            let process_name = proc.name().to_owned();
            drop(procs);
            if needs_drain {
                self.spawn_deferred_job_drain(process_name, event.pid);
            }
            return;
        }

        let name = proc.name().to_owned();
        let current_pid = proc.pid();
        let state = proc.state();
        drop(procs);

        #[cfg(windows)]
        if self
            .complete_orphaned_late_exit_cleanup(&name, event.pid)
            .await
        {
            debug!(
                "[{name}] matched orphaned deferred Windows cleanup after late exit (pid {})",
                event.pid
            );
            if self.deferred_job_drain_pending(&name, event.pid).await {
                self.spawn_deferred_job_drain(name, event.pid);
            }
            return;
        }

        debug!(
            "[{name}] ignoring stale exit event for pid {} (current pid: {current_pid:?}, state: {state})",
            event.pid
        );
    }

    pub(crate) async fn complete_restart(
        &self,
        pending: PendingRestart,
        exit_tx: &mpsc::Sender<ExitEvent>,
        restart_tx: &mpsc::Sender<PendingRestart>,
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
        if let Err(e) = try_spawn_and_watch(proc, exit_tx) {
            warn!("[{name}] restart failed: {e:#}");
            queue_restart(proc, restart_tx);
        }
        proc.record_config_gate_met();
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
                && let Err(e) = try_spawn_and_watch(proc, exit_tx)
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
        let name = proc.name().to_owned();

        if proc.is_running() {
            return Err(Status::failed_precondition(format!(
                "process '{name}' is already running",
            )));
        }
        try_spawn_and_watch(proc, exit_tx)
            .map_err(|e| Status::internal(format!("failed to start '{name}': {e:#}")))?;
        proc.record_config_gate_met();
        Ok(StartResult {
            uuid: proc.uuid().to_owned(),
            pid: proc.pid(),
            state: proc.state(),
        })
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
        #[cfg(windows)]
        let process_name = proc.name().to_owned();
        proc.request_stop();
        #[cfg(windows)]
        let drain_pid = proc.wait_for_stop().await;
        #[cfg(not(windows))]
        {
            proc.wait_for_stop().await;
        }
        let state = proc.state();
        drop(procs);
        #[cfg(windows)]
        if let Some(pid) = drain_pid {
            self.spawn_deferred_job_drain(process_name, pid);
        }
        Ok(StopResult { uuid, state })
    }

    pub(crate) async fn handle_reload_config(
        &self,
        exit_tx: &mpsc::Sender<ExitEvent>,
        restart_tx: &mpsc::Sender<PendingRestart>,
    ) -> Result<ReloadResult, Status> {
        crate::config_gate::clear_secret_caches();
        let new_configs = self.config_loader.load();
        let new_names: std::collections::HashSet<&str> =
            new_configs.iter().map(|c| c.name.as_str()).collect();

        let mut removed = Vec::new();
        let mut stopped_procs = Vec::new();
        #[cfg(windows)]
        let mut pending_job_drains = Vec::new();
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
            #[cfg(windows)]
            if let Some(pid) = proc.wait_for_stop().await {
                pending_job_drains.push((proc.name().to_owned(), pid));
            }
            #[cfg(not(windows))]
            {
                proc.wait_for_stop().await;
            }
        }
        #[cfg(windows)]
        self.adopt_deferred_exit_cleanups(&mut stopped_procs, &mut pending_job_drains)
            .await;

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
                    if proc.may_auto_start()
                        && let Err(e) = try_spawn_and_watch(&mut proc, exit_tx)
                    {
                        warn!("[{}] failed to start: {e:#}", np.name);
                        queue_restart(&mut proc, restart_tx);
                    }
                    proc.record_config_gate_met();
                    added.push(np.name);
                    procs.push(proc);
                }
            }
        }

        let stopped_for_config_update: std::collections::HashSet<String> =
            modified_running.into_iter().collect();
        let modified_names: std::collections::HashSet<String> = modified.iter().cloned().collect();
        {
            let mut procs = self.processes.write().await;
            for proc in procs.iter_mut() {
                let drain = reconcile_process_after_reload(
                    proc,
                    exit_tx,
                    restart_tx,
                    stopped_for_config_update.contains(proc.name()),
                    modified_names.contains(proc.name()),
                )
                .await;
                #[cfg(windows)]
                if let Some(drain) = drain {
                    pending_job_drains.push(drain);
                }
                #[cfg(not(windows))]
                let _ = drain;
            }
        }
        #[cfg(windows)]
        for (name, pid) in pending_job_drains {
            self.spawn_deferred_job_drain(name, pid);
        }

        self.update_startup_order().await;
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
        #[cfg(windows)]
        {
            let mut pending_job_drains = Vec::new();
            self.adopt_deferred_exit_cleanups(&mut procs, &mut pending_job_drains)
                .await;
            drop(procs);
            for (name, pid) in self.collect_orphaned_job_drains().await {
                self.await_deferred_job_drain(name, pid, Some(ManagedProcess::FORCE_KILL_TIMEOUT))
                    .await;
            }
            let mut orphaned = self.orphaned_deferred_exit_cleanups.write().await;
            for entry in orphaned.drain(..) {
                finish_orphaned_deferred_exit_cleanup(entry);
            }
        }
    }

    #[cfg(windows)]
    async fn collect_orphaned_job_drains(&self) -> Vec<(String, u32)> {
        let orphaned = self.orphaned_deferred_exit_cleanups.read().await;
        orphaned
            .iter()
            .filter(|entry| entry.job_object.is_some())
            .map(|entry| (entry.name.clone(), entry.pid))
            .collect()
    }

    #[cfg(windows)]
    async fn adopt_deferred_exit_cleanups(
        &self,
        procs: &mut [ManagedProcess],
        pending_job_drains: &mut Vec<(String, u32)>,
    ) {
        let mut orphaned = self.orphaned_deferred_exit_cleanups.write().await;
        for proc in procs {
            let name = proc.name().to_owned();
            for entry in proc.drain_deferred_exit_cleanups(&name) {
                if entry.job_object.is_some() {
                    pending_job_drains.push((entry.name.clone(), entry.pid));
                }
                orphaned.push(entry);
            }
        }
    }

    #[cfg(windows)]
    async fn await_deferred_job_drain(
        &self,
        name: String,
        pid: u32,
        max_duration: Option<std::time::Duration>,
    ) {
        use std::time::Instant;

        let started = Instant::now();
        let mut warned = false;
        loop {
            tokio::time::sleep(std::time::Duration::from_millis(100)).await;
            if self.try_finish_deferred_job_drain(&name, pid).await {
                debug!("[{name}] released deferred Windows profile after job drain (pid {pid})");
                return;
            }
            if !self.deferred_job_drain_pending(&name, pid).await {
                debug!("[{name}] deferred job drain finished elsewhere (pid {pid})");
                return;
            }
            if !warned && started.elapsed() >= ManagedProcess::FORCE_KILL_TIMEOUT {
                warn!(
                    "[{name}] deferred job drain still waiting for job members to exit (pid {pid})"
                );
                warned = true;
            }
            if max_duration.is_some_and(|max| started.elapsed() >= max) {
                warn!(
                    "[{name}] deferred job drain timed out during shutdown (pid {pid}); proceeding with cleanup"
                );
                return;
            }
        }
    }

    #[cfg(windows)]
    fn spawn_deferred_job_drain(&self, name: String, pid: u32) {
        let mgr = self.clone();
        tokio::spawn(async move {
            mgr.await_deferred_job_drain(name, pid, None).await;
        });
    }

    #[cfg(windows)]
    async fn deferred_job_drain_pending(&self, name: &str, pid: u32) -> bool {
        {
            let procs = self.processes.read().await;
            if let Some(proc) = procs.iter().find(|p| p.name() == name) {
                if proc.has_deferred_job_drain(pid) {
                    return true;
                }
            }
        }
        let orphaned = self.orphaned_deferred_exit_cleanups.read().await;
        orphaned
            .iter()
            .any(|entry| entry.name == name && entry.pid == pid && entry.job_object.is_some())
    }

    #[cfg(windows)]
    async fn try_finish_deferred_job_drain(&self, name: &str, pid: u32) -> bool {
        {
            let mut procs = self.processes.write().await;
            if let Some(proc) = procs.iter_mut().find(|p| p.name() == name) {
                if proc.try_finish_deferred_job_drain(pid) {
                    return true;
                }
            }
        }
        self.try_finish_orphaned_deferred_job_drain(name, pid).await
    }

    #[cfg(windows)]
    async fn try_finish_orphaned_deferred_job_drain(&self, name: &str, pid: u32) -> bool {
        let mut orphaned = self.orphaned_deferred_exit_cleanups.write().await;
        let Some(idx) = orphaned
            .iter()
            .position(|entry| entry.name == name && entry.pid == pid && entry.job_object.is_some())
        else {
            return false;
        };
        let job = orphaned[idx]
            .job_object
            .as_ref()
            .expect("deferred job drain entry must have a job");
        if job.may_have_active_members() {
            return false;
        }
        let entry = orphaned.remove(idx);
        finish_orphaned_deferred_exit_cleanup(entry);
        true
    }

    #[cfg(windows)]
    async fn complete_orphaned_late_exit_cleanup(&self, name: &str, pid: u32) -> bool {
        let mut orphaned = self.orphaned_deferred_exit_cleanups.write().await;
        let Some(idx) = orphaned
            .iter()
            .position(|entry| entry.name == name && entry.pid == pid)
        else {
            return false;
        };
        if orphaned[idx]
            .job_object
            .as_ref()
            .is_some_and(platform::JobObject::may_have_active_members)
        {
            return true;
        }
        let entry = orphaned.remove(idx);
        drop(orphaned);
        finish_orphaned_deferred_exit_cleanup(entry);
        true
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

fn find_index_by_name(procs: &[ManagedProcess], name: &str) -> Option<usize> {
    procs.iter().position(|p| p.name() == name)
}

fn resolve_index(procs: &[ManagedProcess], name_or_uuid: &str) -> Result<usize, Status> {
    if looks_like_uuid_prefix(name_or_uuid)
        && let Some(result) = resolve_by_uuid_prefix(procs, name_or_uuid)
    {
        return result;
    }
    find_index_by_name(procs, name_or_uuid)
        .ok_or_else(|| Status::not_found(format!("process '{name_or_uuid}' not found")))
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
    exit_tx: &mpsc::Sender<ExitEvent>,
    restart_tx: &mpsc::Sender<PendingRestart>,
    was_running: bool,
) -> Option<(String, u32)> {
    let pending_drain = if was_running {
        proc.wait_for_stop()
            .await
            .map(|pid| (proc.name().to_owned(), pid))
    } else {
        None
    };

    if should_start_after_config_reload(proc, was_running) {
        info!("[{}] starting after config reload", proc.name());
        if let Err(e) = try_spawn_and_watch(proc, exit_tx) {
            warn!(
                "[{}] failed to start after config reload: {e:#}",
                proc.name()
            );
            queue_restart(proc, restart_tx);
        }
    } else {
        info!("[{}] skipping start after config reload", proc.name());
    }

    proc.record_config_gate_met();
    pending_drain
}

async fn reconcile_process_after_reload(
    proc: &mut ManagedProcess,
    exit_tx: &mpsc::Sender<ExitEvent>,
    restart_tx: &mpsc::Sender<PendingRestart>,
    was_running: bool,
    config_changed: bool,
) -> Option<(String, u32)> {
    if proc.origin() != ProcessOrigin::Config {
        return None;
    }

    if config_changed {
        return restart_after_config_change(proc, exit_tx, restart_tx, was_running).await;
    }

    let want_start = proc.may_auto_start();
    let start_conditions_was = proc.last_start_conditions_met();
    let mut pending_drain = None;

    if proc.is_running() && should_stop_running_after_reload(proc, start_conditions_was) {
        info!("[{}] start conditions no longer met, stopping", proc.name());
        proc.request_stop();
        pending_drain = proc
            .wait_for_stop()
            .await
            .map(|pid| (proc.name().to_owned(), pid));
    } else if !proc.is_running() && want_start && start_conditions_was == Some(false) {
        info!("[{}] start conditions now met, starting", proc.name());
        if let Err(e) = try_spawn_and_watch(proc, exit_tx) {
            warn!(
                "[{}] failed to start after start conditions opened: {e:#}",
                proc.name()
            );
            queue_restart(proc, restart_tx);
        }
    }

    proc.record_config_gate_met();
    pending_drain
}

fn queue_restart(proc: &mut ManagedProcess, restart_tx: &mpsc::Sender<PendingRestart>) {
    if let Some(delay) = proc.schedule_restart() {
        let pending = PendingRestart {
            uuid: proc.uuid().to_owned(),
            config_generation: proc.config_generation(),
        };
        let tx = restart_tx.clone();
        tokio::spawn(async move {
            tokio::time::sleep(delay).await;
            let _ = tx.send(pending).await;
        });
    }
}

fn try_spawn_and_watch(proc: &mut ManagedProcess, exit_tx: &mpsc::Sender<ExitEvent>) -> Result<()> {
    proc.spawn()?;
    spawn_watcher(proc, exit_tx.clone());
    Ok(())
}

fn spawn_watcher(proc: &mut ManagedProcess, tx: mpsc::Sender<ExitEvent>) {
    if let Some(mut handle) = proc.take_handle() {
        let name = proc.name().to_owned();
        let pid = proc.pid().unwrap_or(0);
        let handle = tokio::spawn(async move {
            let status = match handle.wait().await {
                Ok(status) => status,
                Err(e) => {
                    warn!("[{name}] wait error: {e}, killing process");
                    let _ = handle.kill().await;
                    match handle.wait().await {
                        Ok(s) => s,
                        Err(e2) => {
                            warn!("[{name}] failed to reap after kill: {e2}");
                            return None;
                        }
                    }
                }
            };
            let _ = tx.try_send(ExitEvent {
                name: name.clone(),
                pid,
                status,
            });
            Some(status)
        });
        proc.set_watcher_handle(handle);
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{MutableConfigLoader, ProcessConfig, RestartPolicy, StaticConfigLoader};
    use crate::state::ProcessState;
    use crate::test_helpers;
    use crate::uuid_gen::{SequentialUuidGenerator, V4UuidGenerator};

    fn loader(defs: Vec<ProcessDefinition>) -> Arc<dyn ConfigLoader> {
        Arc::new(StaticConfigLoader::new(defs))
    }

    fn uuid_gen() -> Arc<dyn UuidGenerator> {
        Arc::new(V4UuidGenerator)
    }

    fn reload_test_channels() -> (mpsc::Sender<ExitEvent>, mpsc::Sender<PendingRestart>) {
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, _restart_rx) = mpsc::channel::<PendingRestart>(256);
        (exit_tx, restart_tx)
    }

    fn current_pending_restart(proc: &ManagedProcess) -> PendingRestart {
        PendingRestart {
            uuid: proc.uuid().to_owned(),
            config_generation: proc.config_generation(),
        }
    }

    fn sleep_def(name: &str) -> ProcessDefinition {
        sleep_def_secs(name, 60)
    }

    fn sleep_def_secs(name: &str, secs: u32) -> ProcessDefinition {
        let (cmd, args) = test_helpers::sleep_cmd(secs);
        ProcessDefinition {
            name: name.to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args,
                ..Default::default()
            },
        }
    }

    #[tokio::test]
    async fn test_spawn_failure_schedules_on_failure_restart() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(
            loader(vec![ProcessDefinition {
                name: "bad-spawn".to_string(),
                config: ProcessConfig {
                    command: "/nonexistent/dd-procmgr-spawn-fail".to_string(),
                    restart: RestartPolicy::OnFailure,
                    restart_sec: Some(0.05),
                    ..Default::default()
                },
            }]),
            uuid_gen(),
        );
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<PendingRestart>(256);

        mgr.start_configured_processes(&exit_tx, &restart_tx).await;

        assert!(!mgr.processes().await[0].is_running());
        let expected_uuid = mgr.processes().await[0].uuid().to_owned();
        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for restart after spawn failure");
        assert_eq!(
            pending.as_ref().map(|p| p.uuid.as_str()),
            Some(expected_uuid.as_str())
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_discards_crash_retry_after_config_change() -> anyhow::Result<()> {
        let (cmd, _) = test_helpers::sleep_cmd(60);
        let make_def = |secs: u32| ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args: test_helpers::sleep_cmd(secs).1,
                auto_start: true,
                restart: RestartPolicy::Always,
                restart_sec: Some(0.3),
                runtime_success_sec: Some(0),
                ..Default::default()
            },
        };
        let config_loader = Arc::new(MutableConfigLoader::new(vec![make_def(60)]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<PendingRestart>(256);

        mgr.handle_start("svc", &exit_tx).await?;
        let (pid, name) = {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
            (procs[0].pid().unwrap(), procs[0].name().to_owned())
        };

        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        test_helpers::cleanup_process(pid);
        mgr.handle_exit(
            ExitEvent {
                name,
                pid,
                status: test_helpers::exit_status(1),
            },
            &restart_tx,
        )
        .await;

        let stale_pending =
            tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
                .await
                .expect("timed out waiting for queued restart")
                .expect("expected queued restart event");

        config_loader.set(vec![make_def(120)]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert_ne!(
            mgr.processes().await[0].config_generation(),
            stale_pending.config_generation
        );
        assert!(
            mgr.processes().await[0].is_running(),
            "reload should start the process with fresh counters"
        );

        mgr.complete_restart(stale_pending, &exit_tx, &restart_tx)
            .await;
        let pid = mgr.processes().await[0].pid().unwrap();
        test_helpers::cleanup_process(pid);
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_discards_pending_retry_for_failed_auto_start_false() -> anyhow::Result<()>
    {
        let (cmd, _args) = test_helpers::sleep_cmd(60);
        let make_def = |secs: u32| ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args: test_helpers::sleep_cmd(secs).1,
                auto_start: false,
                restart: RestartPolicy::Always,
                restart_sec: Some(0.0),
                runtime_success_sec: Some(0),
                ..Default::default()
            },
        };
        let config_loader = Arc::new(MutableConfigLoader::new(vec![make_def(60)]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<PendingRestart>(256);

        mgr.handle_start("action-executor", &exit_tx).await?;
        let (pid, name) = {
            let procs = mgr.processes().await;
            (procs[0].pid().unwrap(), procs[0].name().to_owned())
        };

        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        test_helpers::cleanup_process(pid);
        mgr.handle_exit(
            ExitEvent {
                name,
                pid,
                status: test_helpers::exit_status(1),
            },
            &restart_tx,
        )
        .await;

        let stale_pending =
            tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
                .await
                .expect("timed out waiting for queued restart")
                .expect("expected queued restart event");

        config_loader.set(vec![make_def(90)]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(!mgr.processes().await[0].is_running());

        mgr.complete_restart(stale_pending, &exit_tx, &restart_tx)
            .await;
        assert!(
            !mgr.processes().await[0].is_running(),
            "config reload should discard pending crash retries for failed auto_start=false processes"
        );

        mgr.handle_start("action-executor", &exit_tx).await?;
        test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_queue_restart_retries_after_failed_respawn() -> anyhow::Result<()> {
        let (cmd, _args) = test_helpers::sleep_cmd(60);
        let make_def = |secs: u32| ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args: test_helpers::sleep_cmd(secs).1,
                auto_start: false,
                restart: RestartPolicy::Always,
                restart_sec: Some(0.0),
                runtime_success_sec: Some(0),
                ..Default::default()
            },
        };
        let config_loader = Arc::new(MutableConfigLoader::new(vec![make_def(60)]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<PendingRestart>(256);

        mgr.handle_start("action-executor", &exit_tx).await?;
        let (pid, name) = {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
            (procs[0].pid().unwrap(), procs[0].name().to_owned())
        };

        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        test_helpers::cleanup_process(pid);
        mgr.handle_exit(
            ExitEvent {
                name,
                pid,
                status: test_helpers::exit_status(1),
            },
            &restart_tx,
        )
        .await;

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for first queued restart")
            .expect("expected first queued restart event");

        {
            let mut procs = mgr.processes.write().await;
            procs[0].set_command_for_test("/nonexistent/dd-procmgr-failed-respawn".to_string());
        }

        mgr.complete_restart(pending, &exit_tx, &restart_tx).await;

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for second queued restart")
            .expect("expected second queued restart event");

        {
            let mut procs = mgr.processes.write().await;
            procs[0].set_command_for_test(cmd.to_string());
        }

        mgr.complete_restart(pending, &exit_tx, &restart_tx).await;
        assert!(mgr.processes().await[0].is_running());

        test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_discards_stale_bootstrap_retry_when_auto_start_disabled()
    -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-stale-retry".to_string(),
                restart: RestartPolicy::OnFailure,
                restart_sec: Some(0.2),
                auto_start: true,
                ..Default::default()
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<PendingRestart>(256);

        mgr.start_configured_processes(&exit_tx, &restart_tx).await;
        assert_eq!(mgr.processes().await[0].state(), ProcessState::Failed);

        config_loader.set(vec![ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-stale-retry".to_string(),
                restart: RestartPolicy::OnFailure,
                restart_sec: Some(0.2),
                auto_start: false,
                ..Default::default()
            },
        }]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for stale bootstrap retry")
            .expect("expected stale bootstrap retry event");
        mgr.complete_restart(pending, &exit_tx, &restart_tx).await;

        let procs = mgr.processes().await;
        assert!(!procs[0].is_running());
        assert_eq!(procs[0].state(), ProcessState::Failed);
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_discards_orphaned_retry_after_remove_and_readd() -> anyhow::Result<()> {
        let bad_def = ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-orphan-retry".to_string(),
                restart: RestartPolicy::OnFailure,
                restart_sec: Some(0.2),
                auto_start: true,
                ..Default::default()
            },
        };
        let auto_start_false_def = ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-orphan-retry".to_string(),
                restart: RestartPolicy::OnFailure,
                restart_sec: Some(0.2),
                auto_start: false,
                ..Default::default()
            },
        };
        let config_loader = Arc::new(MutableConfigLoader::new(vec![bad_def]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<PendingRestart>(256);

        mgr.start_configured_processes(&exit_tx, &restart_tx).await;
        let old_uuid = mgr.processes().await[0].uuid().to_owned();

        config_loader.set(vec![]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;

        config_loader.set(vec![auto_start_false_def]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert_ne!(mgr.processes().await[0].uuid(), old_uuid);

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for orphaned retry")
            .expect("expected orphaned retry event");
        assert_eq!(pending.uuid, old_uuid);

        mgr.complete_restart(pending, &exit_tx, &restart_tx).await;

        let procs = mgr.processes().await;
        assert!(!procs[0].is_running());
        assert_eq!(procs[0].state(), ProcessState::Created);
        Ok(())
    }

    #[tokio::test]
    async fn test_complete_restart_skips_already_running() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![sleep_def("svc")]), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, _restart_rx) = mpsc::channel::<PendingRestart>(256);

        mgr.handle_start("svc", &exit_tx).await?;
        let pending = {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
            current_pending_restart(&procs[0])
        };
        mgr.complete_restart(pending, &exit_tx, &restart_tx).await;

        let procs = mgr.processes().await;
        assert_eq!(procs.len(), 1);
        assert!(procs[0].is_running());

        test_helpers::cleanup_process(procs[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_complete_restart_honors_policy_for_auto_start_false() -> anyhow::Result<()> {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args,
                auto_start: false,
                restart: RestartPolicy::Always,
                restart_sec: Some(0.0),
                ..Default::default()
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, _restart_rx) = mpsc::channel::<PendingRestart>(256);

        mgr.handle_start("action-executor", &exit_tx).await?;
        {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
            assert!(!procs[0].may_auto_start());
            assert!(procs[0].start_conditions_met());
        }

        {
            let mut procs = mgr.processes.write().await;
            let (cmd, args) = test_helpers::false_cmd();
            let status = std::process::Command::new(cmd).args(args).status()?;
            procs[0].set_last_status(status);
        }
        assert!(!mgr.processes().await[0].is_running());

        let pending = {
            let procs = mgr.processes().await;
            current_pending_restart(&procs[0])
        };
        mgr.complete_restart(pending, &exit_tx, &restart_tx).await;
        assert!(
            mgr.processes().await[0].is_running(),
            "auto_start=false process should still restart when restart policy allows"
        );

        test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_complete_restart_skips_stale_retry_when_restart_policy_revoked()
    -> anyhow::Result<()> {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let always_def = ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args,
                auto_start: false,
                restart: RestartPolicy::Always,
                restart_sec: Some(0.3),
                ..Default::default()
            },
        };
        let never_def = ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: test_helpers::sleep_cmd(60).0.to_string(),
                args: test_helpers::sleep_cmd(60).1,
                auto_start: false,
                restart: RestartPolicy::Never,
                restart_sec: Some(0.3),
                ..Default::default()
            },
        };
        let config_loader = Arc::new(MutableConfigLoader::new(vec![always_def]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (restart_tx, mut restart_rx) = mpsc::channel::<PendingRestart>(256);

        mgr.handle_start("action-executor", &exit_tx).await?;
        let (pid, name) = {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
            (procs[0].pid().unwrap(), procs[0].name().to_owned())
        };

        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        test_helpers::cleanup_process(pid);
        mgr.handle_exit(
            ExitEvent {
                name,
                pid,
                status: test_helpers::exit_status(1),
            },
            &restart_tx,
        )
        .await;

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for queued restart")
            .expect("expected queued restart event");

        config_loader.set(vec![never_def]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert_ne!(
            mgr.processes().await[0].config_generation(),
            pending.config_generation
        );

        mgr.complete_restart(pending, &exit_tx, &restart_tx).await;
        assert!(
            !mgr.processes().await[0].is_running(),
            "stale crash retry must not respawn after restart policy is revoked"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_updates_modified_config() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.handle_start("svc-a", &exit_tx).await?;
        let old_pid = {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
            let expected_args = sleep_def("_").config.args;
            assert_eq!(procs[0].config().args, expected_args);
            procs[0].pid().unwrap()
        };

        // Reload with modified config (different args)
        config_loader.set(vec![sleep_def_secs("svc-a", 120)]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(result.modified.contains(&"svc-a".to_string()));
        assert!(result.added.is_empty());
        assert!(result.removed.is_empty());
        assert!(result.unchanged.is_empty());

        // Config should be updated and process restarted with a new PID
        let procs = mgr.processes().await;
        let expected_args = sleep_def_secs("_", 120).config.args;
        assert_eq!(procs[0].config().args, expected_args);
        assert!(
            procs[0].is_running(),
            "modified running process should be restarted"
        );
        assert_ne!(
            procs[0].pid().unwrap(),
            old_pid,
            "restarted process should have a different PID"
        );

        test_helpers::cleanup_process(procs[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_modified_not_running_stays_stopped() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        // Don't start svc-a — leave it in Created state
        config_loader.set(vec![sleep_def_secs("svc-a", 120)]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(result.modified.contains(&"svc-a".to_string()));

        let procs = mgr.processes().await;
        let expected_args = sleep_def_secs("_", 120).config.args;
        assert_eq!(procs[0].config().args, expected_args);
        assert!(
            !procs[0].is_running(),
            "non-running modified process should not be started"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_retries_failed_auto_start_after_config_change() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-reload-retry".to_string(),
                restart: RestartPolicy::Never,
                ..Default::default()
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.start_configured_processes(&exit_tx, &restart_tx).await;
        assert_eq!(mgr.processes().await[0].state(), ProcessState::Failed);

        config_loader.set(vec![sleep_def("svc-a")]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;

        let procs = mgr.processes().await;
        assert!(
            procs[0].is_running(),
            "failed config-managed auto-start process should retry after definition change"
        );
        test_helpers::cleanup_process(procs[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_modified_stopped_process_stays_stopped() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.handle_start("svc-a", &exit_tx).await?;
        mgr.handle_stop("svc-a").await?;
        assert_eq!(mgr.processes().await[0].state(), ProcessState::Stopped);

        config_loader.set(vec![sleep_def_secs("svc-a", 120)]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;

        let procs = mgr.processes().await;
        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert!(
            !procs[0].is_running(),
            "intentionally stopped process should not auto-retry after config change"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_unchanged_config_not_modified() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        // Reload with the exact same config
        config_loader.set(vec![sleep_def("svc-a")]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(result.unchanged.contains(&"svc-a".to_string()));
        assert!(result.modified.is_empty());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_restarts_running_auto_start_false_after_config_change()
    -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def("action-executor").config
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.handle_start("action-executor", &exit_tx).await?;
        let old_pid = mgr.processes().await[0].pid().unwrap();

        config_loader.set(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def_secs("action-executor", 120).config
            },
        }]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(result.modified.contains(&"action-executor".to_string()));

        let procs = mgr.processes().await;
        let expected_args = sleep_def_secs("_", 120).config.args;
        assert_eq!(procs[0].config().args, expected_args);
        assert!(
            procs[0].is_running(),
            "running auto_start=false process should restart after config change"
        );
        assert_ne!(procs[0].pid().unwrap(), old_pid);

        test_helpers::cleanup_process(procs[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_modified_auto_start_false_stopped_process_stays_stopped()
    -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def("action-executor").config
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.handle_start("action-executor", &exit_tx).await?;
        mgr.handle_stop("action-executor").await?;

        config_loader.set(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def_secs("action-executor", 120).config
            },
        }]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;

        let procs = mgr.processes().await;
        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert!(
            !procs[0].is_running(),
            "stopped auto_start=false process should not restart after config change"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_keeps_manually_started_auto_start_false_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def("action-executor").config
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.handle_start("action-executor", &exit_tx).await?;
        assert!(mgr.processes().await[0].is_running());

        config_loader.set(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def("action-executor").config
            },
        }]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(result.unchanged.contains(&"action-executor".to_string()));

        let procs = mgr.processes().await;
        assert!(
            procs[0].is_running(),
            "manually started auto_start=false process should survive unchanged reload"
        );
        test_helpers::cleanup_process(procs[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_stops_manually_started_auto_start_false_when_path_gate_closes()
    -> anyhow::Result<()> {
        let dir = tempfile::tempdir()?;
        let marker = dir.path().join("ready");
        std::fs::write(&marker, b"")?;
        let path_str = marker.to_str().unwrap().to_string();

        let def_with_path = |path: &str| ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                condition_path_exists: Some(path.to_string()),
                ..sleep_def("action-executor").config
            },
        };

        let config_loader = Arc::new(MutableConfigLoader::new(vec![def_with_path(&path_str)]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.handle_start("action-executor", &exit_tx).await?;
        assert!(mgr.processes().await[0].is_running());

        std::fs::remove_file(&marker)?;
        config_loader.set(vec![def_with_path(&path_str)]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(result.unchanged.contains(&"action-executor".to_string()));
        assert!(
            !mgr.processes().await[0].is_running(),
            "manually started auto_start=false process should stop when path gate closes"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_restarts_process_when_path_reappears() -> anyhow::Result<()> {
        let dir = tempfile::tempdir()?;
        let marker = dir.path().join("ready");
        std::fs::write(&marker, b"")?;
        let path_str = marker.to_str().unwrap().to_string();

        let def_with_path = |path: &str| ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                auto_start: true,
                condition_path_exists: Some(path.to_string()),
                ..sleep_def("svc-a").config
            },
        };

        let config_loader = Arc::new(MutableConfigLoader::new(vec![]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        config_loader.set(vec![def_with_path(&path_str)]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(result.added.contains(&"svc-a".to_string()));
        assert!(mgr.processes().await[0].is_running());

        std::fs::remove_file(&marker)?;
        config_loader.set(vec![def_with_path(&path_str)]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(result.unchanged.contains(&"svc-a".to_string()));
        assert!(
            !mgr.processes().await[0].is_running(),
            "process should stop when condition_path_exists is no longer met"
        );

        std::fs::write(&marker, b"")?;
        config_loader.set(vec![def_with_path(&path_str)]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
        assert!(
            mgr.processes().await[0].is_running(),
            "process should restart when condition_path_exists is met again"
        );

        test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
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
        let (exit_tx, restart_tx) = reload_test_channels();
        mgr.handle_create("runtime-svc".to_string(), config, &exit_tx)
            .await?;

        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
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
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.handle_start("svc-a", &exit_tx).await?;
        mgr.handle_start("svc-b", &exit_tx).await?;

        // Reload removes svc-b
        config_loader.set(vec![sleep_def("svc-a")]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
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
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.handle_start("svc-a", &exit_tx).await?;

        // Reload adds svc-b
        config_loader.set(vec![sleep_def("svc-a"), sleep_def("svc-b")]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
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
        let (exit_tx, restart_tx) = reload_test_channels();

        mgr.handle_start("svc-a", &exit_tx).await?;

        // Create a runtime process
        let (cmd, args) = test_helpers::sleep_cmd(60);
        mgr.handle_create(
            "runtime-svc".to_string(),
            ProcessConfig {
                command: cmd.to_string(),
                args,
                auto_start: false,
                ..Default::default()
            },
            &exit_tx,
        )
        .await?;
        mgr.handle_start("runtime-svc", &exit_tx).await?;

        // Reload removes svc-a but preserves runtime-svc
        config_loader.set(vec![]);
        let result = mgr.handle_reload_config(&exit_tx, &restart_tx).await?;
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
        let (cmd, args) = test_helpers::sleep_cmd(60);
        mgr.handle_create(
            "svc-b".to_string(),
            ProcessConfig {
                command: cmd.to_string(),
                args,
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
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        let (cmd, args) = test_helpers::sleep_cmd(60);
        mgr.handle_create(
            "auto-svc".to_string(),
            ProcessConfig {
                command: cmd.to_string(),
                args,
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
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let (cmd, args) = test_helpers::sleep_cmd(60);
        mgr.handle_create(
            "manual-svc".to_string(),
            ProcessConfig {
                command: cmd.to_string(),
                args,
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
        let (cmd, args) = test_helpers::sleep_cmd(60);
        mgr.handle_create(
            "cond-svc".to_string(),
            ProcessConfig {
                command: cmd.to_string(),
                args,
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
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, restart_tx) = reload_test_channels();

        {
            let order = mgr.startup_order.read().await;
            assert_eq!(*order, vec![0], "single process at index 0");
        }

        // Reload with a new process that has an after-dependency, which
        // forces a non-alphabetical order (svc-b before svc-api).
        let (cmd, args) = test_helpers::sleep_cmd(60);
        config_loader.set(vec![
            ProcessDefinition {
                name: "svc-api".to_string(),
                config: ProcessConfig {
                    command: cmd.to_string(),
                    args,
                    after: vec!["svc-b".to_string()],
                    ..Default::default()
                },
            },
            sleep_def("svc-b"),
        ]);
        mgr.handle_reload_config(&exit_tx, &restart_tx).await?;

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
        let mgr = ProcessManager::new(
            loader(vec![sleep_def("svc-a"), sleep_def("svc-b")]),
            uuid_gen,
        );
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);

        let err: Status = mgr.handle_start("aabbccdd", &exit_tx).await.unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
        assert!(
            err.message().contains("ambiguous"),
            "error should mention ambiguity: {}",
            err.message()
        );

        // A longer, unambiguous prefix should resolve correctly.
        mgr.handle_start("aabbccdd-1", &exit_tx)
            .await
            .expect("unambiguous prefix should resolve");

        // Clean up the spawned process.
        let _: Result<_, _> = mgr.handle_stop("aabbccdd-1").await;
    }
}
