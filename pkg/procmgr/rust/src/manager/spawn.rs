// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::catalog::ProcessCatalog;
use super::{PendingRestart, RuntimeContext, enqueue_pending_restart};
use crate::config::ProcessConfig;
use crate::platform;
use crate::process::{ManagedChildSpawn, ManagedProcess};
use crate::state::ProcessState;
use anyhow::Result;
use log::{info, warn};
use std::sync::Arc;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(in crate::manager) struct SpawnCommitSnapshot {
    pub uuid: String,
    pub pid: Option<u32>,
    pub state: ProcessState,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(in crate::manager) enum SpawnProcessOutcome {
    Committed(SpawnCommitSnapshot),
    NotStarted,
}

pub(in crate::manager) enum SpawnKind {
    BootAutoStart,
    CreateAutoStart,
    Manual,
    Restart(PendingRestart),
}

impl SpawnKind {
    fn may_begin_spawn(&self, proc: &ManagedProcess) -> bool {
        if proc.state().is_alive() {
            return false;
        }
        match self {
            Self::BootAutoStart | Self::CreateAutoStart => proc.may_auto_start(),
            Self::Manual => true,
            Self::Restart(pending) => pending_restart_still_valid(proc, pending),
        }
    }

    fn may_commit_spawn(&self, proc: &ManagedProcess) -> bool {
        if proc.state() != ProcessState::Starting {
            return false;
        }
        match self {
            Self::BootAutoStart | Self::CreateAutoStart => proc.may_auto_start(),
            Self::Manual => true,
            Self::Restart(pending) => pending_restart_matches(proc, pending),
        }
    }

    fn retry_on_failure(&self) -> bool {
        !matches!(self, Self::Manual)
    }
}

pub(in crate::manager) fn log_skipped_pending_restart(
    proc: &ManagedProcess,
    pending: &PendingRestart,
) {
    let Some(reason) = pending_restart_skip_reason(proc, pending) else {
        return;
    };
    let name = proc.name();
    match reason {
        PendingRestartSkip::StaleSpawnSeq => info!(
            "[{name}] ignoring stale queued restart (spawn_seq {} != {})",
            pending.spawn_seq,
            proc.spawn_seq()
        ),
        PendingRestartSkip::AlreadyRunning => {
            info!("[{name}] already running, skipping queued restart");
        }
        PendingRestartSkip::PolicyOrConditions => {
            info!("[{name}] not restarting: policy or start conditions not met");
        }
    }
}

enum PendingRestartSkip {
    StaleSpawnSeq,
    AlreadyRunning,
    PolicyOrConditions,
}

fn pending_restart_matches(proc: &ManagedProcess, pending: &PendingRestart) -> bool {
    proc.uuid() == pending.uuid && proc.spawn_seq() == pending.spawn_seq
}

fn pending_restart_skip_reason(
    proc: &ManagedProcess,
    pending: &PendingRestart,
) -> Option<PendingRestartSkip> {
    if !pending_restart_matches(proc, pending) {
        Some(PendingRestartSkip::StaleSpawnSeq)
    } else if matches!(proc.state(), ProcessState::Running | ProcessState::Stopping) {
        Some(PendingRestartSkip::AlreadyRunning)
    } else if !proc.should_complete_pending_restart() {
        Some(PendingRestartSkip::PolicyOrConditions)
    } else {
        None
    }
}

pub(in crate::manager) fn pending_restart_still_valid(
    proc: &ManagedProcess,
    pending: &PendingRestart,
) -> bool {
    pending_restart_skip_reason(proc, pending).is_none()
}

pub(in crate::manager) async fn spawn_process(
    catalog: Arc<ProcessCatalog>,
    idx: usize,
    ctx: &RuntimeContext,
    kind: SpawnKind,
) -> Result<SpawnProcessOutcome> {
    if !ctx.lifecycle.spawns_allowed() {
        return Ok(SpawnProcessOutcome::NotStarted);
    }

    let snapshot = {
        let mut procs = catalog.write_processes().await;
        let Some(proc) = procs.get_mut(idx) else {
            return Ok(SpawnProcessOutcome::NotStarted);
        };
        if !kind.may_begin_spawn(proc) {
            return Ok(SpawnProcessOutcome::NotStarted);
        }
        proc.begin_spawn_reservation()
            .map_err(|e| anyhow::anyhow!("{e:#}"))?;
        (proc.name().to_owned(), proc.config().clone())
    };

    if !ctx.lifecycle.spawns_allowed() {
        cancel_spawn_reservation(&catalog, idx).await;
        return Ok(SpawnProcessOutcome::NotStarted);
    }

    let (name, config) = snapshot;
    let spawn_name = name.clone();
    let spawn_result =
        tokio::task::spawn_blocking(move || spawn_managed_child_sync(&spawn_name, &config))
            .await
            .map_err(|e| anyhow::anyhow!("spawn worker join failed: {e}"))?;

    commit_spawn(catalog, idx, &name, spawn_result, ctx, &kind).await
}

pub(in crate::manager) fn spawn_process_background(
    catalog: Arc<ProcessCatalog>,
    idx: usize,
    ctx: RuntimeContext,
    kind: SpawnKind,
) {
    tokio::spawn(async move {
        let _ = spawn_process(catalog, idx, &ctx, kind).await;
    });
}

fn spawn_managed_child_sync(name: &str, config: &ProcessConfig) -> Result<ManagedChildSpawn> {
    #[cfg(all(test, unix))]
    wait_for_test_spawn_gate();
    #[cfg(windows)]
    let _console_guard = platform::console_lock();
    platform::spawn_managed_child(name, config)
}

async fn cancel_spawn_reservation(catalog: &ProcessCatalog, idx: usize) {
    let mut procs = catalog.write_processes().await;
    if let Some(proc) = procs.get_mut(idx) {
        proc.cancel_spawn_reservation();
    }
}

async fn commit_spawn(
    catalog: Arc<ProcessCatalog>,
    idx: usize,
    name: &str,
    spawn_result: Result<ManagedChildSpawn>,
    ctx: &RuntimeContext,
    kind: &SpawnKind,
) -> Result<SpawnProcessOutcome> {
    let mut procs = catalog.write_processes().await;
    let Some(proc) = procs.get_mut(idx) else {
        abort_uncommitted(spawn_result, name).await;
        return Ok(SpawnProcessOutcome::NotStarted);
    };

    if !ctx.lifecycle.spawns_allowed() || !kind.may_commit_spawn(proc) {
        proc.cancel_spawn_reservation();
        abort_uncommitted(spawn_result, name).await;
        return Ok(SpawnProcessOutcome::NotStarted);
    }

    match spawn_result {
        Ok(outcome) => proc
            .commit_spawn_from_outcome(outcome, ctx.exit_tx.clone())
            .map(|()| {
                SpawnProcessOutcome::Committed(SpawnCommitSnapshot {
                    uuid: proc.uuid().to_owned(),
                    pid: proc.pid(),
                    state: proc.state(),
                })
            })
            .map_err(|e| {
                warn!("[{name}] spawn failed: {e:#}");
                if kind.retry_on_failure() && ctx.lifecycle.spawns_allowed() {
                    enqueue_pending_restart(proc, ctx);
                }
                e
            }),
        Err(e) => {
            warn!("[{name}] spawn failed: {e:#}");
            proc.mark_spawn_failed();
            if kind.retry_on_failure() && ctx.lifecycle.spawns_allowed() {
                enqueue_pending_restart(proc, ctx);
            }
            Err(e)
        }
    }
}

async fn abort_uncommitted(spawn_result: Result<ManagedChildSpawn>, name: &str) {
    if let Ok(outcome) = spawn_result {
        outcome.abort(name).await;
    }
}

#[cfg(all(test, unix))]
static SPAWN_GATE: std::sync::OnceLock<(std::sync::Mutex<bool>, std::sync::Condvar)> =
    std::sync::OnceLock::new();

#[cfg(all(test, unix))]
fn spawn_gate() -> &'static (std::sync::Mutex<bool>, std::sync::Condvar) {
    SPAWN_GATE.get_or_init(|| (std::sync::Mutex::new(true), std::sync::Condvar::new()))
}

#[cfg(all(test, unix))]
pub(in crate::manager) struct SpawnGateGuard;

#[cfg(all(test, unix))]
impl Drop for SpawnGateGuard {
    fn drop(&mut self) {
        open_spawn_gate_for_test();
    }
}

#[cfg(all(test, unix))]
pub(in crate::manager) fn close_spawn_gate_for_test() -> SpawnGateGuard {
    *spawn_gate().0.lock().unwrap() = false;
    SpawnGateGuard
}

#[cfg(all(test, unix))]
pub(in crate::manager) fn open_spawn_gate_for_test() {
    *spawn_gate().0.lock().unwrap() = true;
    spawn_gate().1.notify_all();
}

#[cfg(all(test, unix))]
fn wait_for_test_spawn_gate() {
    let (lock, cv) = spawn_gate();
    let mut open = lock.lock().unwrap();
    while !*open {
        open = cv.wait(open).unwrap();
    }
}

#[cfg(all(test, unix))]
pub(in crate::manager) fn reset_spawn_gate_for_test() {
    open_spawn_gate_for_test();
}
