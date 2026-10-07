// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#![allow(clippy::result_large_err)]

use crate::command::{Command, CreateResult, ReloadResult, StartResult, StopResult};
use crate::config::{self, ConfigLoader, InvalidConfigEntry, ProcessDefinition};
use crate::grpc;
use crate::ordering;
use crate::platform;
use crate::process::{ExitEvent, ManagedProcess, ProcessOrigin};
use crate::shutdown;
use crate::state::ProcessState;
use crate::uuid_gen::UuidGenerator;
use anyhow::Result;
use log::{debug, info, warn};
use std::path::PathBuf;
use std::sync::Arc;
use tokio::sync::{RwLock, mpsc, oneshot};
use tonic::Status;

/// Catalog stub for a `processes.d` file that did not produce a `ProcessConfig`.
#[derive(Debug, Clone)]
pub(crate) struct InvalidProcess {
    pub uuid: String,
    pub name: String,
    pub path: PathBuf,
    pub error: String,
}

impl InvalidProcess {
    fn from_entry(entry: InvalidConfigEntry, uuid: String) -> Self {
        Self {
            uuid,
            name: entry.name,
            path: entry.path,
            error: entry.error,
        }
    }
}

#[derive(Clone)]
pub struct ProcessManager {
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
    invalid: Arc<RwLock<Vec<InvalidProcess>>>,
    /// Indices into the `processes` Vec in dependency-resolved startup order.
    /// Recomputed on config reload so that indices stay in sync with the Vec.
    startup_order: Arc<RwLock<Vec<usize>>>,
    config_loader: Arc<dyn ConfigLoader>,
    uuid_gen: Arc<dyn UuidGenerator>,
}

impl ProcessManager {
    pub fn new(config_loader: Arc<dyn ConfigLoader>, uuid_gen: Arc<dyn UuidGenerator>) -> Self {
        let catalog = config_loader.load();
        let processes: Vec<ManagedProcess> = catalog
            .processes
            .into_iter()
            .map(|pd| ManagedProcess::new_config(pd.name, uuid_gen.generate(), pd.config))
            .collect();
        let invalid: Vec<InvalidProcess> = catalog
            .invalid
            .into_iter()
            .map(|entry| InvalidProcess::from_entry(entry, uuid_gen.generate()))
            .collect();
        let startup_result = recompute_startup_order(&processes);
        Self {
            processes: Arc::new(RwLock::new(processes)),
            invalid: Arc::new(RwLock::new(invalid)),
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

    pub(crate) async fn invalid_configs(
        &self,
    ) -> tokio::sync::RwLockReadGuard<'_, Vec<InvalidProcess>> {
        self.invalid.read().await
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
            proc.mark_restart_blocked_already_accounted();
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
            let invalid = self.invalid.read().await;
            if procs.iter().any(|p| p.name() == name) || invalid.iter().any(|p| p.name == name) {
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
        {
            let procs = self.processes.read().await;
            let invalid = self.invalid.read().await;
            if let Some(inv) = find_invalid(&invalid, &procs, name_or_uuid)? {
                return Err(Status::failed_precondition(format!(
                    "process '{}' has invalid config, cannot start: {}",
                    inv.name, inv.error
                )));
            }
        }
        let mut procs = self.processes.write().await;
        let invalid = self.invalid.read().await;
        let idx = resolve_index(&procs, &invalid, name_or_uuid)?;
        drop(invalid);
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

    /// Stop a process, holding the lock only to start and to record the stop.
    ///
    /// The wait in between runs for as long as `stop_timeout`, and `list`,
    /// `describe`, `status`, and `config` answer straight from the same lock
    /// rather than through this command channel, so holding it across the wait
    /// makes every read hang for the duration.
    pub(crate) async fn handle_stop(&self, name_or_uuid: &str) -> Result<StopResult, Status> {
        let (uuid, wait) = {
            let mut procs = self.processes.write().await;
            let invalid = self.invalid.read().await;
            if let Some(inv) = find_invalid(&invalid, &procs, name_or_uuid)? {
                return Err(Status::failed_precondition(format!(
                    "process '{}' has invalid config, cannot stop",
                    inv.name
                )));
            }
            let idx = resolve_index(&procs, &invalid, name_or_uuid)?;
            drop(invalid);
            let proc = &mut procs[idx];

            if !proc.is_running() {
                return Err(Status::failed_precondition(format!(
                    "process '{}' is not running",
                    proc.name()
                )));
            }
            proc.request_stop();
            (proc.uuid().to_owned(), proc.take_stop_wait())
        };

        if let Some(wait) = wait {
            wait.run().await;
        }

        // Re-resolved rather than kept as an index: nothing else may reorder
        // the list while the event loop is parked on this command, but only
        // the uuid says so at the point of use.
        let mut procs = self.processes.write().await;
        let invalid = self.invalid.read().await;
        let idx = resolve_index(&procs, &invalid, &uuid)?;
        drop(invalid);
        let proc = &mut procs[idx];
        proc.finish_stop();
        let state = proc.state();
        Ok(StopResult { uuid, state })
    }

    pub(crate) async fn handle_reload_config(
        &self,
        exit_tx: &mpsc::Sender<ExitEvent>,
    ) -> Result<ReloadResult, Status> {
        let catalog = self.config_loader.load();
        let valid_order: Vec<String> = catalog.processes.iter().map(|pd| pd.name.clone()).collect();
        let invalid_order: Vec<String> = catalog.invalid.iter().map(|e| e.name.clone()).collect();
        let mut valid: std::collections::HashMap<String, config::ProcessConfig> = catalog
            .processes
            .into_iter()
            .map(|pd| (pd.name, pd.config))
            .collect();
        let mut incoming_invalid: std::collections::HashMap<String, InvalidConfigEntry> = catalog
            .invalid
            .into_iter()
            .map(|entry| (entry.name.clone(), entry))
            .collect();
        let catalog_names: std::collections::HashSet<String> = valid
            .keys()
            .cloned()
            .chain(incoming_invalid.keys().cloned())
            .collect();

        let mut removed = Vec::new();
        let mut stopped_procs = Vec::new();
        let mut valid_to_invalid = Vec::new();
        {
            let mut procs = self.processes.write().await;
            let mut i = 0;
            while i < procs.len() {
                if procs[i].origin() == ProcessOrigin::Config
                    && !catalog_names.contains(procs[i].name())
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

        // A previously valid file that no longer parses: stop the child, then
        // keep the name as InvalidConfig instead of treating it as removed. A
        // live child holds on to its `ManagedProcess` until the stop below
        // finishes, so reads taken meanwhile report `Stopping` with its real pid
        // rather than a pid-0 InvalidConfig row for a workload still up.
        let mut stopping_to_invalid: Vec<(String, InvalidConfigEntry)> = Vec::new();
        {
            let mut procs = self.processes.write().await;
            let mut invalid = self.invalid.write().await;
            let mut i = 0;
            while i < procs.len() {
                if procs[i].origin() != ProcessOrigin::Config
                    || !incoming_invalid.contains_key(procs[i].name())
                {
                    i += 1;
                    continue;
                }
                let name = procs[i].name().to_owned();
                let entry = incoming_invalid
                    .get(&name)
                    .cloned()
                    .expect("name was present");
                info!("[{name}] config is no longer valid, stopping");
                if procs[i].is_running() {
                    procs[i].request_stop();
                    stopping_to_invalid.push((name.clone(), entry));
                    valid_to_invalid.push(name);
                    i += 1;
                    continue;
                }
                // Nothing to wait for, so the name can change hands right here.
                let proc = procs.remove(i);
                invalid.retain(|e| e.name != name);
                invalid.push(InvalidProcess::from_entry(entry, proc.uuid().to_owned()));
                valid_to_invalid.push(name);
            }
        }

        for proc in &mut stopped_procs {
            proc.wait_for_stop().await;
        }

        // The stub takes over the name once the child is down, keeping the uuid
        // so clients holding it can still describe the row. The lock goes back
        // between each one for the same reason as in `handle_stop`: reads must
        // not queue behind the wait.
        for (name, entry) in stopping_to_invalid {
            let wait = {
                let mut procs = self.processes.write().await;
                procs
                    .iter_mut()
                    .find(|p| p.name() == name)
                    .and_then(|proc| proc.take_stop_wait())
            };
            if let Some(wait) = wait {
                wait.run().await;
            }

            let mut procs = self.processes.write().await;
            let mut invalid = self.invalid.write().await;
            let Some(idx) = procs.iter().position(|p| p.name() == name) else {
                continue;
            };
            let mut proc = procs.remove(idx);
            proc.finish_stop();
            invalid.retain(|e| e.name != name);
            invalid.push(InvalidProcess::from_entry(entry, proc.uuid().to_owned()));
        }

        let mut added = Vec::new();
        let mut modified = valid_to_invalid;
        let mut modified_running: Vec<String> = Vec::new();
        let mut unchanged = Vec::new();

        {
            let mut procs = self.processes.write().await;
            let mut invalid = self.invalid.write().await;
            let mut i = 0;
            while i < invalid.len() {
                let name = invalid[i].name.clone();
                if let Some(config) = valid.remove(&name) {
                    let uuid = invalid.remove(i).uuid;
                    if procs.iter().any(|p| p.name() == name) {
                        warn!(
                            "[{name}] valid config not loaded as a new process: a process with this name already exists"
                        );
                        valid.insert(name, config);
                        continue;
                    }
                    info!("[{name}] config is valid again, loading");
                    let mut proc = ManagedProcess::new_config(name.clone(), uuid, config);
                    if proc.should_start()
                        && let Err(e) = proc.spawn(exit_tx.clone())
                    {
                        warn!("[{name}] failed to start: {e:#}");
                    }
                    procs.push(proc);
                    modified.push(name);
                } else if let Some(entry) = incoming_invalid.remove(&name) {
                    if modified.iter().any(|n| n == &name) {
                        i += 1;
                        continue;
                    }
                    if invalid[i].error != entry.error || invalid[i].path != entry.path {
                        invalid[i].error = entry.error;
                        invalid[i].path = entry.path;
                        modified.push(name);
                    } else {
                        unchanged.push(name);
                    }
                    i += 1;
                } else {
                    invalid.remove(i);
                    removed.push(name);
                }
            }
        }

        {
            let mut procs = self.processes.write().await;
            for name in &valid_order {
                let Some(config) = valid.remove(name) else {
                    continue;
                };
                if let Some(existing) = procs.iter_mut().find(|p| p.name() == name) {
                    if *existing.config() != config {
                        info!("[{name}] config changed, updating");
                        if existing.is_running() {
                            existing.request_stop();
                            modified_running.push(name.clone());
                        }
                        existing.set_config(config);
                        modified.push(name.clone());
                    } else {
                        unchanged.push(name.clone());
                    }
                } else {
                    info!("[{name}] new config found, adding");
                    let mut proc =
                        ManagedProcess::new_config(name.clone(), self.uuid_gen.generate(), config);
                    if proc.should_start()
                        && let Err(e) = proc.spawn(exit_tx.clone())
                    {
                        warn!("[{name}] failed to start: {e:#}");
                    }
                    added.push(name.clone());
                    procs.push(proc);
                }
            }
        }

        {
            let procs = self.processes.read().await;
            let mut invalid = self.invalid.write().await;
            for name in &invalid_order {
                let Some(entry) = incoming_invalid.remove(name) else {
                    continue;
                };
                if procs.iter().any(|p| p.name() == name) {
                    warn!(
                        "[{name}] invalid config ignored: a process with this name already exists"
                    );
                    continue;
                }
                info!("[{name}] invalid config, not starting");
                invalid.push(InvalidProcess::from_entry(entry, self.uuid_gen.generate()));
                added.push(name.clone());
            }
        }

        // Wait for modified processes that were running to stop, then restart
        // with the new config. The lock goes back between each one for the same
        // reason as in `handle_stop`: reads must not queue behind the wait.
        for name in &modified_running {
            let wait = {
                let mut procs = self.processes.write().await;
                let Some(proc) = procs.iter_mut().find(|p| p.name() == *name) else {
                    continue;
                };
                proc.take_stop_wait()
            };
            if let Some(wait) = wait {
                wait.run().await;
            }

            let mut procs = self.processes.write().await;
            let Some(proc) = procs.iter_mut().find(|p| p.name() == *name) else {
                continue;
            };
            proc.finish_stop();
            if !proc.may_respawn() {
                info!("[{name}] not restarting after reload: start conditions not met");
                proc.mark_restart_blocked_already_accounted();
                continue;
            }
            info!("[{name}] restarting with updated config");
            if let Err(e) = proc.spawn(exit_tx.clone()) {
                warn!("[{name}] failed to restart: {e:#}");
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
        // and the skip left it in `Exited`, `Crashed`, `Failed`, or `Stopped`.
        // Those states are also reached by a completed one-shot, a policy
        // mismatch, the burst limit, a failed spawn, and an operator stop, so
        // the guard keys off the recorded skip reason rather than the state.
        // `may_respawn`, not `should_start`: `auto_start` governs boot only, so
        // consulting it here would strand a manually started process forever.
        // The burst limit is re-checked only for an exit-time skip. That skip
        // records nothing, because the gate is checked before the limit, so it
        // can outlive a budget earlier crashes already exhausted. A backoff
        // re-check already recorded the restart the limit admitted, and the
        // reload of a running process is not a restart, so recovering either
        // must not consult the limit again.
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
                    ProcessState::Exited
                    | ProcessState::Crashed
                    | ProcessState::Failed
                    | ProcessState::Stopped => {
                        proc.restart_blocked_by_conditions()
                            && proc.may_respawn()
                            && !proc.recovered_restart_exceeds_burst()
                    }
                    _ => false,
                };
                if !eligible {
                    continue;
                }
                let name = proc.name().to_owned();
                info!("[{name}] start conditions now met after reload, starting");
                // After the burst check above, since an exit-time skip spends
                // from the budget here, and before `spawn`, which clears the
                // skip reason.
                proc.record_recovered_restart();
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

fn ambiguous_uuid_prefix(prefix: &str, matches: usize) -> Status {
    Status::invalid_argument(format!(
        "UUID prefix '{prefix}' is ambiguous ({matches} matches)"
    ))
}

/// Count UUID-prefix hits across the whole catalog (valid and invalid rows).
///
/// Returns `None` when the prefix matches nothing, so callers can fall through
/// to name lookup. A single hit yields that side's index; two or more hits
/// (including one on each side) are ambiguous.
fn resolve_uuid_prefix_across_catalog(
    procs: &[ManagedProcess],
    invalid: &[InvalidProcess],
    prefix: &str,
) -> Option<Result<(Option<usize>, Option<usize>), Status>> {
    let proc_matches: Vec<usize> = procs
        .iter()
        .enumerate()
        .filter(|(_, p)| p.uuid().starts_with(prefix))
        .map(|(i, _)| i)
        .collect();
    let inv_matches: Vec<usize> = invalid
        .iter()
        .enumerate()
        .filter(|(_, p)| p.uuid.starts_with(prefix))
        .map(|(i, _)| i)
        .collect();
    let total = proc_matches.len() + inv_matches.len();
    match total {
        0 => None,
        1 => Some(Ok((proc_matches.first().copied(), inv_matches.first().copied()))),
        _ => Some(Err(ambiguous_uuid_prefix(prefix, total))),
    }
}

pub(crate) fn find_invalid<'a>(
    invalid: &'a [InvalidProcess],
    procs: &[ManagedProcess],
    name_or_uuid: &str,
) -> Result<Option<&'a InvalidProcess>, Status> {
    if looks_like_uuid_prefix(name_or_uuid) {
        match resolve_uuid_prefix_across_catalog(procs, invalid, name_or_uuid) {
            Some(Ok((_, Some(i)))) => return Ok(Some(&invalid[i])),
            Some(Ok((_, None))) => {
                // Exactly one valid process matched; this is not an invalid hit.
                return Ok(None);
            }
            Some(Err(status)) => return Err(status),
            None => {}
        }
    }
    Ok(invalid.iter().find(|p| p.name == name_or_uuid))
}

fn resolve_index(
    procs: &[ManagedProcess],
    invalid: &[InvalidProcess],
    name_or_uuid: &str,
) -> Result<usize, Status> {
    if looks_like_uuid_prefix(name_or_uuid) {
        match resolve_uuid_prefix_across_catalog(procs, invalid, name_or_uuid) {
            Some(Ok((Some(i), None))) => return Ok(i),
            Some(Ok((None, Some(_)))) => {
                // Caller should have short-circuited via find_invalid; treat as missing
                // among managed processes so start/stop do not invent a row.
                return Err(Status::not_found(format!(
                    "process '{name_or_uuid}' not found"
                )));
            }
            Some(Err(status)) => return Err(status),
            None => {}
            Some(Ok(_)) => {
                // (None, None) and (Some, Some) are unreachable for total == 1 / Err.
                return Err(ambiguous_uuid_prefix(name_or_uuid, 2));
            }
        }
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
    use crate::config::{
        InvalidConfigEntry, LoadedCatalog, MutableConfigLoader, ProcessConfig, StaticConfigLoader,
    };
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

        let err = resolve_index(&procs, &[], "aabbccdd").unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
        assert!(
            err.message().contains("ambiguous"),
            "error should mention ambiguity: {}",
            err.message()
        );
        assert_eq!(resolve_index(&procs, &[], "aabbccdd-1").unwrap(), 0);
        assert_eq!(resolve_index(&procs, &[], "aabbccdd-2").unwrap(), 1);
    }

    #[test]
    fn test_uuid_prefix_ambiguous_across_valid_and_invalid() {
        let proc = ManagedProcess::new_config(
            "svc-a".to_string(),
            "aabbccdd-1111-0000-0000-000000000000".to_string(),
            test_helpers::make_config("true", vec![]),
        );
        let inv = InvalidProcess {
            uuid: "aabbccdd-2222-0000-0000-000000000000".to_string(),
            name: "svc-b".to_string(),
            path: PathBuf::from("/tmp/svc-b.yaml"),
            error: "parse failed".to_string(),
        };
        let procs = vec![proc];
        let invalid = vec![inv];

        let err = find_invalid(&invalid, &procs, "aabbccdd").unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
        assert!(
            err.message().contains("ambiguous"),
            "shared prefix across catalog sides must be ambiguous: {}",
            err.message()
        );
        assert!(
            err.message().contains("2 matches"),
            "error should count both sides: {}",
            err.message()
        );

        let err = resolve_index(&procs, &invalid, "aabbccdd").unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);

        assert_eq!(
            find_invalid(&invalid, &procs, "aabbccdd-2")
                .unwrap()
                .map(|e| e.name.as_str()),
            Some("svc-b")
        );
        assert_eq!(resolve_index(&procs, &invalid, "aabbccdd-1").unwrap(), 0);
    }

    /// A child that ignores the graceful stop, so the stop runs for the whole
    /// `stop_timeout` rather than returning at once. It creates `ready` once it
    /// is safe to signal.
    #[cfg(unix)]
    fn ignores_stop_def(
        name: &str,
        stop_timeout_secs: u64,
        ready: &std::path::Path,
    ) -> ProcessDefinition {
        let (cmd, args) = test_helpers::trap_term_sleep_ready(ready);
        let mut config = test_helpers::make_config(cmd, args);
        config.stop_timeout = Some(stop_timeout_secs);
        ProcessDefinition {
            name: name.to_string(),
            config,
        }
    }

    /// `list`, `describe`, `status`, and `config` read the process table
    /// directly rather than through the command channel, so a stop that holds
    /// the write lock while it waits takes all of them down with it: the daemon
    /// is alive and answers nothing (AGENTRUN-1507).
    ///
    /// Unix-only for want of a portable child, not because the lock scope is
    /// platform-specific: `handle_stop` is the same code everywhere. The test
    /// needs one that provably ignores its graceful stop, so the stop outlasts
    /// the reads taken around it.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_stop_leaves_reads_answerable() -> anyhow::Result<()> {
        use std::time::{Duration, Instant};

        let dir = tempfile::tempdir()?;
        let ready = dir.path().join("ready");
        let mgr = ProcessManager::new(
            loader(vec![ignores_stop_def("svc", 60, &ready)]),
            uuid_gen(),
        );
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        mgr.handle_start("svc", &exit_tx).await?;
        let pid = mgr.processes().await[0].pid().expect("spawned pid");
        test_helpers::wait_for_file(&ready, Duration::from_secs(10)).await;

        let stopping = tokio::spawn({
            let mgr = mgr.clone();
            async move { mgr.handle_stop("svc").await }
        });

        // Reads before the stop takes the lock still see Running, so poll for
        // the state the stop sets. Each read is bounded on its own: under the
        // old lock scope the first read after the stop started would hang for
        // the whole 60s timeout.
        let started = Instant::now();
        loop {
            let read = tokio::time::timeout(Duration::from_secs(5), mgr.processes()).await;
            let procs =
                read.map_err(|_| anyhow::anyhow!("a read queued behind an in-flight stop"))?;
            if procs[0].state() == ProcessState::Stopping {
                break;
            }
            assert!(
                started.elapsed() < Duration::from_secs(10),
                "a read taken mid-stop should report Stopping, got {}",
                procs[0].state()
            );
            drop(procs);
            tokio::time::sleep(Duration::from_millis(10)).await;
        }

        stopping.abort();
        test_helpers::cleanup_process(pid);
        Ok(())
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
    async fn test_invalid_config_is_catalogued_not_spawned() {
        let catalog = LoadedCatalog {
            processes: vec![],
            invalid: vec![InvalidConfigEntry {
                name: "broken".to_string(),
                path: PathBuf::from("/tmp/broken.yaml"),
                error: "parsing /tmp/broken.yaml: missing field `command`".to_string(),
            }],
        };
        let mgr = ProcessManager::new(
            Arc::new(StaticConfigLoader::with_catalog(catalog)),
            uuid_gen(),
        );
        assert!(mgr.processes().await.is_empty());
        let invalid = mgr.invalid_configs().await;
        assert_eq!(invalid.len(), 1);
        assert_eq!(invalid[0].name, "broken");
        assert!(!invalid[0].error.is_empty());
        assert!(!invalid[0].uuid.is_empty());
    }

    #[tokio::test]
    async fn test_start_and_stop_reject_invalid_config() {
        let catalog = LoadedCatalog {
            processes: vec![],
            invalid: vec![InvalidConfigEntry {
                name: "broken".to_string(),
                path: PathBuf::from("/tmp/broken.yaml"),
                error: "bad yaml".to_string(),
            }],
        };
        let mgr = ProcessManager::new(
            Arc::new(StaticConfigLoader::with_catalog(catalog)),
            uuid_gen(),
        );
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let start_err = mgr.handle_start("broken", &exit_tx).await.unwrap_err();
        assert_eq!(start_err.code(), tonic::Code::FailedPrecondition);
        assert!(start_err.message().contains("invalid config"));
        let stop_err = mgr.handle_stop("broken").await.unwrap_err();
        assert_eq!(stop_err.code(), tonic::Code::FailedPrecondition);
    }

    #[tokio::test]
    async fn test_reload_created_to_invalid_drops_managed_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                auto_start: false,
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let uuid = mgr.processes().await[0].uuid().to_owned();
        config_loader.set_catalog(LoadedCatalog {
            processes: vec![],
            invalid: vec![InvalidConfigEntry {
                name: "svc-a".to_string(),
                path: PathBuf::from("/tmp/svc-a.yaml"),
                error: "parse failed".to_string(),
            }],
        });
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert_eq!(result.modified, vec!["svc-a".to_string()]);
        assert!(mgr.processes().await.is_empty());
        let invalid = mgr.invalid_configs().await;
        assert_eq!(invalid[0].name, "svc-a");
        assert_eq!(
            invalid[0].uuid, uuid,
            "a uuid a client already holds must keep resolving across the transition"
        );
        Ok(())
    }

    /// The stub must not take over the name while the child is still shutting
    /// down: consumers read InvalidConfig as "nothing is running under it", so
    /// publishing it early reports a live workload as gone.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_reload_to_invalid_publishes_only_once_the_child_is_down() -> anyhow::Result<()> {
        use std::time::{Duration, Instant};

        let dir = tempfile::tempdir()?;
        let ready = dir.path().join("ready");
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ignores_stop_def(
            "svc", 60, &ready,
        )]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        mgr.handle_start("svc", &exit_tx).await?;
        let (pid, uuid) = {
            let procs = mgr.processes().await;
            (
                procs[0].pid().expect("spawned pid"),
                procs[0].uuid().to_owned(),
            )
        };
        test_helpers::wait_for_file(&ready, Duration::from_secs(10)).await;

        config_loader.set_catalog(LoadedCatalog {
            processes: vec![],
            invalid: vec![InvalidConfigEntry {
                name: "svc".to_string(),
                path: PathBuf::from("/tmp/svc.yaml"),
                error: "parse failed".to_string(),
            }],
        });
        let reloading = tokio::spawn({
            let mgr = mgr.clone();
            let exit_tx = exit_tx.clone();
            async move { mgr.handle_reload_config(&exit_tx).await }
        });

        // The child ignores its graceful stop, so the reload sits in the stop
        // wait long enough to read the state it publishes meanwhile.
        let started = Instant::now();
        loop {
            let procs = tokio::time::timeout(Duration::from_secs(5), mgr.processes())
                .await
                .map_err(|_| anyhow::anyhow!("a read queued behind an in-flight reload"))?;
            if procs[0].state() == ProcessState::Stopping {
                assert_eq!(procs[0].uuid(), uuid);
                assert!(
                    mgr.invalid_configs().await.is_empty(),
                    "InvalidConfig must wait for the child it replaces"
                );
                break;
            }
            assert!(
                started.elapsed() < Duration::from_secs(10),
                "a read taken mid-reload should report Stopping, got {}",
                procs[0].state()
            );
            drop(procs);
            tokio::time::sleep(Duration::from_millis(10)).await;
        }

        reloading.abort();
        test_helpers::cleanup_process(pid);
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_invalid_to_valid_loads_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![]));
        config_loader.set_catalog(LoadedCatalog {
            processes: vec![],
            invalid: vec![InvalidConfigEntry {
                name: "svc-a".to_string(),
                path: PathBuf::from("/tmp/svc-a.yaml"),
                error: "parse failed".to_string(),
            }],
        });
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        assert_eq!(mgr.invalid_configs().await.len(), 1);
        config_loader.set(vec![ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                auto_start: false,
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        }]);
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert_eq!(result.modified, vec!["svc-a".to_string()]);
        assert!(mgr.invalid_configs().await.is_empty());
        assert_eq!(mgr.processes().await[0].name(), "svc-a");
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_invalid_config_does_not_collide_with_runtime() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let mut config = test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS);
        config.auto_start = false;
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
        mgr.handle_create("svc".to_string(), config, &exit_tx)
            .await?;
        mgr.handle_start("svc", &exit_tx).await?;

        config_loader.set_catalog(LoadedCatalog {
            processes: vec![],
            invalid: vec![InvalidConfigEntry {
                name: "svc".to_string(),
                path: PathBuf::from("/tmp/svc.yaml"),
                error: "parse failed".to_string(),
            }],
        });
        mgr.handle_reload_config(&exit_tx).await?;
        assert!(
            mgr.invalid_configs().await.is_empty(),
            "invalid yaml must not hide a runtime process of the same name"
        );
        assert_eq!(mgr.processes().await.len(), 1);
        mgr.handle_stop("svc").await?;

        config_loader.set(vec![ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                auto_start: false,
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        }]);
        mgr.handle_reload_config(&exit_tx).await?;
        assert_eq!(
            mgr.processes().await.len(),
            1,
            "recovered yaml must not spawn a second process with the runtime name"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_adds_catalog_entries_in_load_order() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        config_loader.set_catalog(LoadedCatalog {
            processes: vec![
                ProcessDefinition {
                    name: "alpha".to_string(),
                    config: ProcessConfig {
                        auto_start: false,
                        command: "/bin/true".to_string(),
                        ..Default::default()
                    },
                },
                ProcessDefinition {
                    name: "bravo".to_string(),
                    config: ProcessConfig {
                        auto_start: false,
                        command: "/bin/true".to_string(),
                        ..Default::default()
                    },
                },
            ],
            invalid: vec![
                InvalidConfigEntry {
                    name: "charlie".to_string(),
                    path: PathBuf::from("/tmp/charlie.yaml"),
                    error: "parse failed".to_string(),
                },
                InvalidConfigEntry {
                    name: "delta".to_string(),
                    path: PathBuf::from("/tmp/delta.yaml"),
                    error: "parse failed".to_string(),
                },
            ],
        });
        let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(1);
        let result = mgr.handle_reload_config(&exit_tx).await?;
        assert_eq!(
            result.added,
            vec![
                "alpha".to_string(),
                "bravo".to_string(),
                "charlie".to_string(),
                "delta".to_string()
            ]
        );
        let procs = mgr.processes().await;
        let proc_names: Vec<String> = procs.iter().map(|p| p.name().to_owned()).collect();
        assert_eq!(proc_names, vec!["alpha", "bravo"]);
        let invalid = mgr.invalid_configs().await;
        let invalid_names: Vec<String> = invalid.iter().map(|p| p.name.clone()).collect();
        assert_eq!(invalid_names, vec!["charlie", "delta"]);
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

        /// Kills the child and reports an unsuccessful exit to the manager,
        /// which is what a dead child looks like from `run`'s point of view.
        ///
        /// The reported status is a plain non-zero exit, so these tests land in
        /// `Failed`, not `Crashed`. The gate accounting under test is the same
        /// for both, and keeping the synthetic status decoupled from how the
        /// child actually died is what makes the result identical on Windows.
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
            // skips without queueing a backoff or recording the restart.
            write_agent_yaml(dir.path(), false);
            crash(&mgr, "gated-svc", &restart_tx).await;
            assert!(
                restart_rx.try_recv().is_err(),
                "a closed gate must not queue a restart"
            );
            assert_stranded_by_gate(&mgr, ProcessState::Failed).await;
            assert_eq!(
                mgr.processes().await[0].restart_count(),
                0,
                "a skipped restart must not consume burst budget"
            );

            write_agent_yaml(dir.path(), true);
            let result = mgr.handle_reload_config(&exit_tx).await?;

            assert!(result.unchanged.contains(&"gated-svc".to_string()));
            assert!(
                mgr.processes().await[0].is_running(),
                "reload must restart a process the closed gate stranded at exit"
            );
            assert_eq!(
                mgr.processes().await[0].restart_count(),
                1,
                "recovering an exit-time skip owes the restart accounting the gate deferred"
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
            let (restart_tx, mut restart_rx) = mpsc::channel::<String>(8);

            mgr.start(&exit_tx).await;

            // The crash is handled with the gate still open, so `handle_restart`
            // records the restart and queues it. The gate then closes before the
            // backoff fires.
            crash(&mgr, "gated-svc", &restart_tx).await;
            assert_eq!(
                mgr.processes().await[0].restart_count(),
                1,
                "the exit-time decision recorded this restart"
            );
            write_agent_yaml(dir.path(), false);
            mgr.complete_restart("gated-svc", &exit_tx).await;
            assert_stranded_by_gate(&mgr, ProcessState::Failed).await;

            write_agent_yaml(dir.path(), true);
            mgr.handle_reload_config(&exit_tx).await?;

            assert!(
                mgr.processes().await[0].is_running(),
                "reload must restart a process whose gate closed during the backoff"
            );
            assert_eq!(
                mgr.processes().await[0].restart_count(),
                1,
                "this restart was already accounted for at exit, so recovery must not count it twice"
            );

            // The queued restart is dropped rather than delivered, which is the
            // condition this recovery exists to undo.
            let _ = restart_rx.try_recv();
            cleanup_first_process(&mgr).await;
            Ok(())
        }

        /// The restart that fills the burst window is recorded before the backoff
        /// runs. Closing the gate during that delay must not turn the admission
        /// into a refusal: the slot was spent on this restart, and `is_burst_limited`
        /// is true the moment it is recorded.
        #[tokio::test]
        async fn test_reload_restarts_backoff_skip_that_filled_the_burst() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let mut def = gated_on_failure_sleep_def("gated-svc", &yaml);
            def.config.start_limit_burst = Some(1);
            def.config.start_limit_interval_sec = Some(3600);
            let mgr = ProcessManager::new(loader(vec![def]), uuid_gen());
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
            let (restart_tx, mut restart_rx) = mpsc::channel::<String>(8);

            mgr.start(&exit_tx).await;

            crash(&mgr, "gated-svc", &restart_tx).await;
            assert_eq!(
                mgr.processes().await[0].restart_count(),
                1,
                "this crash is the one restart the burst allows, so it is recorded"
            );
            write_agent_yaml(dir.path(), false);
            mgr.complete_restart("gated-svc", &exit_tx).await;
            assert_stranded_by_gate(&mgr, ProcessState::Failed).await;

            write_agent_yaml(dir.path(), true);
            mgr.handle_reload_config(&exit_tx).await?;

            assert!(
                mgr.processes().await[0].is_running(),
                "recovery must spawn a restart the burst limit already admitted"
            );
            assert_eq!(
                mgr.processes().await[0].restart_count(),
                1,
                "the restart was recorded at exit, so recovery must not count it again"
            );

            let _ = restart_rx.try_recv();
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

        /// The gate check runs before the burst limit, so a crash behind a closed
        /// gate consumes no budget and records no restart. Recovery must therefore
        /// re-check the budget itself, or a gate flap hands back a restart the
        /// burst limit had already spent.
        #[tokio::test]
        async fn test_reload_does_not_bypass_burst_limit_after_gate_flap() -> anyhow::Result<()> {
            let (_env, dir) = gate_env();
            let yaml = write_agent_yaml(dir.path(), true);
            let mut def = gated_on_failure_sleep_def("crasher", &yaml);
            def.config.start_limit_burst = Some(1);
            def.config.start_limit_interval_sec = Some(3600);
            let mgr = ProcessManager::new(loader(vec![def]), uuid_gen());
            let (exit_tx, _exit_rx) = mpsc::channel::<ExitEvent>(256);
            let (restart_tx, _restart_rx) = mpsc::channel::<String>(8);

            // One crash with the gate open spends the whole budget.
            mgr.start(&exit_tx).await;
            crash(&mgr, "crasher", &restart_tx).await;
            assert_eq!(mgr.processes().await[0].restart_count(), 1);

            // The next crash finds the gate closed, so it is attributed to the
            // gate and leaves the spent budget untouched.
            mgr.handle_start("crasher", &exit_tx).await?;
            write_agent_yaml(dir.path(), false);
            crash(&mgr, "crasher", &restart_tx).await;
            assert_stranded_by_gate(&mgr, ProcessState::Failed).await;

            write_agent_yaml(dir.path(), true);
            mgr.handle_reload_config(&exit_tx).await?;

            assert!(
                !mgr.processes().await[0].is_running(),
                "recovery must not hand back a restart the burst limit already refused"
            );
            Ok(())
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
