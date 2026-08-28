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
        let ctx = ctx.clone();
        let spawn_fut = spawn_process(catalog, idx, &ctx, SpawnKind::BootAutoStart);
        tokio::pin!(spawn_fut);

        tokio::select! {
            biased;
            _ = shutdown.as_mut() => {
                ctx.lifecycle.begin_stopping();
                info!("startup: shutdown signaled, skipping remaining auto-starts");
                join_in_flight_spawn(spawn_fut.as_mut()).await;
                return;
            }
            result = spawn_fut.as_mut() => {
                log_spawn_result(result, &name);
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

fn log_spawn_result(result: Result<(), anyhow::Error>, name: &str) {
    if let Err(e) = result {
        warn!("[{name}] auto-start failed: {e:#}");
    }
}

async fn join_in_flight_spawn<F>(mut spawn_fut: Pin<&mut F>)
where
    F: Future<Output = anyhow::Result<()>>,
{
    #[cfg(windows)]
    if let Some(signal_time) = platform::service_stop_signal_time() {
        let budget = shutdown::ShutdownBudget::service_stop(signal_time);
        let cap = budget.remaining_cap(Duration::from_secs(180));
        if cap.is_zero() {
            warn!("startup: SCM shutdown budget exhausted; not waiting for in-flight auto-start");
            return;
        }
        match tokio::time::timeout(cap, spawn_fut.as_mut()).await {
            Ok(Ok(())) => {}
            Ok(Err(e)) => warn!("startup: in-flight auto-start failed: {e:#}"),
            Err(_) => warn!(
                "startup: timed out waiting for in-flight auto-start ({cap:?} left in SCM budget)"
            ),
        }
        return;
    }

    if let Err(e) = spawn_fut.as_mut().await {
        warn!("startup: in-flight auto-start failed: {e:#}");
    }
}
