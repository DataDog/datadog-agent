// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::catalog::ProcessCatalog;
use crate::process::{StopWaitContext, coalesced_run_stop_wait};
use crate::shutdown::ShutdownBudget;

struct StopWaitSnapshot {
    active: bool,
}

struct CatalogProcessStopWait<'a> {
    catalog: &'a ProcessCatalog,
    idx: usize,
}

async fn stop_wait_snapshot(catalog: &ProcessCatalog, idx: usize) -> StopWaitSnapshot {
    let procs = catalog.read_processes().await;
    StopWaitSnapshot {
        active: procs[idx].state().awaiting_stop(),
    }
}

impl StopWaitContext for CatalogProcessStopWait<'_> {
    async fn stop_wait_active(&mut self) -> bool {
        stop_wait_snapshot(self.catalog, self.idx).await.active
    }

    async fn await_stop_progress(&mut self) -> bool {
        let notify = {
            let procs = self.catalog.read_processes().await;
            procs[self.idx].stop_wait_notify()
        };
        let notified = notify.notified();
        if !stop_wait_snapshot(self.catalog, self.idx).await.active {
            return false;
        }
        notified.await;
        stop_wait_snapshot(self.catalog, self.idx).await.active
    }

    async fn plan_stop(&mut self, budget: ShutdownBudget) -> Option<crate::process::StopWaitPlan> {
        let mut procs = self.catalog.write_processes().await;
        procs[self.idx].plan_stop_wait(budget)
    }

    async fn finalize_stop(
        &mut self,
        owner: crate::process::StopWaitOwner,
        result: crate::process::StopWaitResult,
        budget: ShutdownBudget,
    ) -> bool {
        let mut procs = self.catalog.write_processes().await;
        procs[self.idx]
            .finalize_stop_wait(owner, result, budget)
            .await
    }
}

pub(in crate::manager) async fn wait_for_process_stop(
    catalog: &ProcessCatalog,
    idx: usize,
    budget: ShutdownBudget,
) {
    let mut ctx = CatalogProcessStopWait { catalog, idx };
    if let Some(owner) = coalesced_run_stop_wait(&mut ctx, budget).await {
        let mut procs = catalog.write_processes().await;
        procs[idx].complete_stop_wait(owner, budget).await;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{ProcessDefinition, StaticConfigLoader};
    use crate::shutdown::ShutdownBudget;
    use crate::state::ProcessState;
    use crate::test_helpers;
    use crate::uuid_gen::V4UuidGenerator;
    use std::sync::Arc;
    use std::time::{Duration, Instant};

    // Windows CI: sleep_cmd(60) is ping ~61s; stop().await can exceed the job budget.
    #[cfg(not(windows))]
    #[tokio::test]
    async fn await_stop_progress_returns_false_when_stop_finishes_before_wait() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let catalog = ProcessCatalog::load(
            &StaticConfigLoader::new(vec![ProcessDefinition {
                name: "svc".into(),
                config: test_helpers::make_config(cmd, args),
            }]),
            Arc::new(V4UuidGenerator),
        );

        {
            let mut procs = catalog.write_processes().await;
            procs[0].spawn().unwrap();
            procs[0].request_stop();
            procs[0].stop().await;
        }

        let mut ctx = CatalogProcessStopWait {
            catalog: &catalog,
            idx: 0,
        };
        let still_waiting =
            tokio::time::timeout(Duration::from_millis(100), ctx.await_stop_progress())
                .await
                .unwrap();

        assert!(!still_waiting);
    }

    #[cfg(not(windows))]
    #[tokio::test]
    async fn concurrent_wait_for_process_stop_does_not_hang() {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let catalog = Arc::new(ProcessCatalog::load(
            &StaticConfigLoader::new(vec![ProcessDefinition {
                name: "svc".into(),
                config: test_helpers::make_config(cmd, args),
            }]),
            Arc::new(V4UuidGenerator),
        ));

        {
            let mut procs = catalog.write_processes().await;
            procs[0].spawn().unwrap();
            procs[0].request_stop();
        }

        let budget = ShutdownBudget::unlimited(Instant::now());
        let catalog_a = Arc::clone(&catalog);
        let catalog_b = Arc::clone(&catalog);
        tokio::time::timeout(Duration::from_secs(5), async {
            tokio::join!(
                wait_for_process_stop(catalog_a.as_ref(), 0, budget),
                wait_for_process_stop(catalog_b.as_ref(), 0, budget),
            );
        })
        .await
        .unwrap();

        let procs = catalog.read_processes().await;
        assert_eq!(procs[0].state(), ProcessState::Stopped);
    }
}
