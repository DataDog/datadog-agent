// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::catalog::ProcessCatalog;
use crate::process::{StopWaitContext, coalesced_run_stop_wait};
use crate::shutdown::ShutdownBudget;
use std::sync::Arc;
use tokio::sync::Notify;

struct StopWaitSnapshot {
    active: bool,
    notify: Arc<Notify>,
}

struct CatalogProcessStopWait<'a> {
    catalog: &'a ProcessCatalog,
    idx: usize,
}

async fn stop_wait_snapshot(catalog: &ProcessCatalog, idx: usize) -> StopWaitSnapshot {
    let procs = catalog.read_processes().await;
    StopWaitSnapshot {
        active: procs[idx].state().awaiting_stop(),
        notify: procs[idx].stop_wait_notify(),
    }
}

impl StopWaitContext for CatalogProcessStopWait<'_> {
    async fn stop_wait_active(&mut self) -> bool {
        stop_wait_snapshot(self.catalog, self.idx).await.active
    }

    async fn await_stop_progress(&mut self) -> bool {
        let snapshot = stop_wait_snapshot(self.catalog, self.idx).await;
        if !snapshot.active {
            return false;
        }
        snapshot.notify.notified().await;
        stop_wait_snapshot(self.catalog, self.idx).await.active
    }

    async fn plan_stop(&mut self, budget: ShutdownBudget) -> Option<crate::process::StopWaitPlan> {
        let mut procs = self.catalog.write_processes().await;
        procs[self.idx].plan_stop_wait(budget)
    }

    async fn finalize_stop(
        &mut self,
        result: crate::process::StopWaitResult,
        budget: ShutdownBudget,
    ) -> bool {
        let mut procs = self.catalog.write_processes().await;
        procs[self.idx].finalize_stop_wait(result, budget).await
    }
}

pub(in crate::manager) async fn wait_for_process_stop(
    catalog: &ProcessCatalog,
    idx: usize,
    budget: ShutdownBudget,
) {
    let mut ctx = CatalogProcessStopWait { catalog, idx };
    if coalesced_run_stop_wait(&mut ctx, budget).await {
        let mut procs = catalog.write_processes().await;
        procs[idx].complete_stop_wait(budget).await;
    }
}
