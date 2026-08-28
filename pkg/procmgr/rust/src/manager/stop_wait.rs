// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::catalog::ProcessCatalog;
use crate::process::{StopWaitContext, run_stop_wait};
use crate::shutdown::ShutdownBudget;
use crate::state::ProcessState;

struct CatalogProcessStopWait<'a> {
    catalog: &'a ProcessCatalog,
    idx: usize,
}

impl StopWaitContext for CatalogProcessStopWait<'_> {
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
    if matches!(
        catalog.read_processes().await[idx].state(),
        ProcessState::Running | ProcessState::Stopping
    ) {
        let mut ctx = CatalogProcessStopWait { catalog, idx };
        run_stop_wait(&mut ctx, budget).await;
    }

    let mut procs = catalog.write_processes().await;
    procs[idx].complete_stop_wait(budget).await;
}
