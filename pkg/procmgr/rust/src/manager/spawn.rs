// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::{PendingRestart, RuntimeContext, enqueue_pending_restart};
use crate::config::ProcessConfig;
use crate::platform;
use crate::process::{ManagedChildSpawn, ManagedProcess};
use anyhow::Result;
use log::{info, warn};
use std::sync::Arc;
use tokio::sync::RwLock;

pub(in crate::manager) enum SpawnKind {
    BootAutoStart,
    CreateAutoStart,
    Manual,
    Restart(PendingRestart),
}

impl SpawnKind {
    fn allowed(&self, proc: &ManagedProcess) -> bool {
        match self {
            Self::BootAutoStart | Self::CreateAutoStart => {
                proc.may_auto_start() && !proc.is_running()
            }
            Self::Manual => !proc.is_running(),
            Self::Restart(pending) => pending_restart_still_valid(proc, pending),
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

fn pending_restart_skip_reason(
    proc: &ManagedProcess,
    pending: &PendingRestart,
) -> Option<PendingRestartSkip> {
    if proc.uuid() != pending.uuid || proc.spawn_seq() != pending.spawn_seq {
        Some(PendingRestartSkip::StaleSpawnSeq)
    } else if proc.is_running() {
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
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
    idx: usize,
    ctx: &RuntimeContext,
    kind: SpawnKind,
) -> Result<()> {
    if !ctx.lifecycle.spawns_allowed() {
        return Ok(());
    }

    let snapshot = {
        let procs = processes.read().await;
        let Some(proc) = procs.get(idx) else {
            return Ok(());
        };
        if !kind.allowed(proc) {
            return Ok(());
        }
        (proc.name().to_owned(), proc.config().clone())
    };

    if !ctx.lifecycle.spawns_allowed() {
        return Ok(());
    }

    let (name, config) = snapshot;
    let spawn_name = name.clone();
    let spawn_result =
        tokio::task::spawn_blocking(move || spawn_managed_child_sync(&spawn_name, &config))
            .await
            .map_err(|e| anyhow::anyhow!("spawn worker join failed: {e}"))?;

    commit_spawn(processes, idx, &name, spawn_result, ctx, &kind).await
}

pub(in crate::manager) fn spawn_process_background(
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
    idx: usize,
    ctx: RuntimeContext,
    kind: SpawnKind,
) {
    tokio::spawn(async move {
        let _ = spawn_process(processes, idx, &ctx, kind).await;
    });
}

fn spawn_managed_child_sync(name: &str, config: &ProcessConfig) -> Result<ManagedChildSpawn> {
    #[cfg(windows)]
    let _console_guard = platform::console_lock();
    platform::spawn_managed_child(name, config)
}

async fn commit_spawn(
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
    idx: usize,
    name: &str,
    spawn_result: Result<ManagedChildSpawn>,
    ctx: &RuntimeContext,
    kind: &SpawnKind,
) -> Result<()> {
    let mut procs = processes.write().await;
    let Some(proc) = procs.get_mut(idx) else {
        abort_uncommitted(spawn_result, name).await;
        return Ok(());
    };

    if !ctx.lifecycle.spawns_allowed() || !kind.allowed(proc) {
        abort_uncommitted(spawn_result, name).await;
        return Ok(());
    }

    match spawn_result {
        Ok(outcome) => proc
            .spawn_and_watch_from_outcome(outcome, ctx.exit_tx.clone())
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
