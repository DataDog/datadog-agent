// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::spawn::spawn_process;
use super::{ProcessManager, RuntimeContext, spawn::SpawnKind};
use crate::platform;
#[cfg(windows)]
use crate::shutdown;
use log::{debug, info, warn};
use std::future::Future;
use std::pin::Pin;
#[cfg(windows)]
use std::time::Duration;

pub(in crate::manager) async fn run(
    manager: &ProcessManager,
    ctx: &RuntimeContext,
    mut shutdown: Pin<&mut impl Future<Output = ()>>,
) {
    let order = manager.catalog.startup_order().await.clone();
    if order.is_empty() {
        info!("startup: catalog is empty, nothing to auto-start");
        if platform::shutdown_requested() {
            ctx.lifecycle.begin_stopping();
        }
        return;
    }

    log_startup_plan(manager, &order).await;

    for (step, &idx) in order.iter().enumerate() {
        if ctx.lifecycle.is_stopping() || platform::shutdown_requested() {
            if platform::shutdown_requested() {
                ctx.lifecycle.begin_stopping();
            }
            info!("startup: service stopping, skipping remaining auto-starts");
            return;
        }

        let name = manager.catalog.read_processes().await[idx]
            .name()
            .to_owned();
        debug!(
            "startup: step {}/{} spawning '{name}' if configured",
            step + 1,
            order.len()
        );

        let catalog = manager.catalog.clone();
        let ctx_spawn = ctx.clone();
        let (spawn_done_tx, spawn_done_rx) = tokio::sync::oneshot::channel();
        let spawn_handle = tokio::spawn(async move {
            let result =
                spawn_process(catalog, idx, &ctx_spawn, SpawnKind::BootAutoStart, None).await;
            let _ = spawn_done_tx.send(());
            result
        });

        tokio::select! {
            biased;
            _ = shutdown.as_mut() => {
                ctx.lifecycle.begin_stopping();
                info!("startup: shutdown signaled, skipping remaining auto-starts");
                join_in_flight_spawn(ctx, spawn_handle).await;
                return;
            }
            _ = spawn_done_rx => {
                log_spawn_task_result(spawn_handle.await, &name);
            }
        }
    }

    info!("startup: auto-start complete");
}

async fn log_startup_plan(manager: &ProcessManager, order: &[usize]) {
    let procs = manager.catalog.read_processes().await;
    let names: Vec<&str> = order.iter().map(|&i| procs[i].name()).collect();
    info!(
        "startup: {} catalog processes, order: {}",
        procs.len(),
        names.join(" -> ")
    );
}

fn log_spawn_result(result: Result<super::spawn::SpawnProcessOutcome, anyhow::Error>, name: &str) {
    match result {
        Ok(_) => {}
        Err(e) => warn!("[{name}] auto-start failed: {e:#}"),
    }
}

fn log_spawn_task_result(
    result: Result<anyhow::Result<super::spawn::SpawnProcessOutcome>, tokio::task::JoinError>,
    name: &str,
) {
    match result {
        Ok(spawn_result) => log_spawn_result(spawn_result, name),
        Err(e) => warn!("[{name}] auto-start task failed: {e}"),
    }
}

#[cfg(windows)]
fn log_spawn_monitor_result(
    result: Result<anyhow::Result<super::spawn::SpawnProcessOutcome>, tokio::task::JoinError>,
) {
    match result {
        Ok(spawn_result) => {
            if let Err(e) = spawn_result {
                warn!("startup: in-flight auto-start failed: {e:#}");
            }
        }
        Err(e) => warn!("startup: in-flight auto-start task failed: {e}"),
    }
}

async fn join_in_flight_spawn(
    ctx: &RuntimeContext,
    handle: tokio::task::JoinHandle<anyhow::Result<super::spawn::SpawnProcessOutcome>>,
) {
    #[cfg(windows)]
    if let Some(cap) = shutdown::ShutdownBudget::remaining_service_stop_cap(Duration::from_secs(180))
    {
        if cap.is_zero() {
            warn!(
                "startup: SCM shutdown budget exhausted; deferring in-flight auto-start to supervisor teardown"
            );
            super::spawn::defer_spawn_join_handle(&ctx.background_spawns, handle);
            return;
        }

        let mut handle = handle;
        match tokio::time::timeout(cap, &mut handle).await {
            Ok(result) => log_spawn_monitor_result(result),
            Err(_) => {
                warn!(
                    "startup: timed out waiting for in-flight auto-start ({cap:?} left in SCM budget); deferring to supervisor teardown"
                );
                super::spawn::defer_spawn_join_handle(&ctx.background_spawns, handle);
            }
        }
        return;
    }

    let _ = ctx;
    match handle.await {
        Ok(spawn_result) => {
            if let Err(e) = spawn_result {
                warn!("startup: in-flight auto-start failed: {e:#}");
            }
        }
        Err(e) => warn!("startup: in-flight auto-start task failed: {e}"),
    }
}
