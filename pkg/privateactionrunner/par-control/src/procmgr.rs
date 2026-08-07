// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Executor lifecycle through the existing `dd-procmgrd` gRPC API.

use crate::proto::procmgr;
use crate::proto::procmgr::process_manager_client::ProcessManagerClient;
use crate::transport;
use anyhow::{Context, Result};
use std::path::Path;
use std::time::Duration;
use tonic::transport::Channel;

/// Bounds each process-manager RPC.
///
/// dd-procmgrd serializes every command through a single loop
/// (`ProcessManager::run`), so one wedged operation stalls all callers. These
/// RPCs sit on the dispatch path via `ensure_ready`, after a task has been
/// leased from OPMS but before heartbeats start, so an unbounded call would hang
/// the action with nothing reporting it as alive and no outcome ever published.
const PROCMGR_RPC_TIMEOUT: Duration = Duration::from_secs(10);

/// Executor lifecycle operations the orchestrator relies on. A trait so the
/// orchestrator can be tested without a real process manager.
pub trait ExecutorLifecycle: Send + Sync + 'static {
    fn ensure_started(&self) -> impl std::future::Future<Output = Result<()>> + Send;
    fn is_running(&self) -> impl std::future::Future<Output = Result<bool>> + Send;
    /// For fail-and-report: exited/crashed/failed.
    fn has_exited(&self) -> impl std::future::Future<Output = Result<bool>> + Send;
    fn stop(&self) -> impl std::future::Future<Output = Result<()>> + Send;
}

/// [`ExecutorLifecycle`] backed by dd-procmgrd.
#[derive(Clone)]
pub struct ProcmgrLifecycle {
    client: ProcessManagerClient<Channel>,
    process_name: String,
    rpc_timeout: Duration,
}

impl ProcmgrLifecycle {
    /// Build a client for the process manager on its Unix socket (connects lazily).
    pub fn new(socket: &Path, process_name: String) -> Self {
        ProcmgrLifecycle {
            client: ProcessManagerClient::new(transport::connect_lazy(socket)),
            process_name,
            rpc_timeout: PROCMGR_RPC_TIMEOUT,
        }
    }

    /// Run one RPC under [`PROCMGR_RPC_TIMEOUT`], turning a silent daemon into a
    /// reportable error instead of an indefinite wait.
    async fn rpc<F, Fut, T>(&self, name: &str, call: F) -> Result<T>
    where
        F: FnOnce(ProcessManagerClient<Channel>, String) -> Fut,
        Fut: std::future::Future<Output = std::result::Result<T, tonic::Status>>,
    {
        match tokio::time::timeout(
            self.rpc_timeout,
            call(self.client.clone(), self.process_name.clone()),
        )
        .await
        {
            Ok(Ok(response)) => Ok(response),
            Ok(Err(status)) => Err(status.into()),
            Err(_) => Err(anyhow::anyhow!(
                "process-manager {name} did not respond within {:?} for process {:?}",
                self.rpc_timeout,
                self.process_name
            )),
        }
    }

    async fn describe_state(&self) -> Result<Option<i32>> {
        let resp = self
            .rpc("Describe", |mut client, name| async move {
                client
                    .describe(procmgr::DescribeRequest { name_or_uuid: name })
                    .await
            })
            .await
            .with_context(|| {
                format!(
                    "process-manager Describe failed for process {:?}",
                    self.process_name
                )
            })?
            .into_inner();
        Ok(resp.detail.map(|d| d.state))
    }
}

impl ExecutorLifecycle for ProcmgrLifecycle {
    async fn ensure_started(&self) -> Result<()> {
        if self.is_running().await? {
            return Ok(());
        }
        log::info!(
            "starting the on-demand executor {:?} via the process manager",
            self.process_name
        );
        let result = self
            .rpc("Start", |mut client, name| async move {
                client
                    .start(procmgr::StartRequest { name_or_uuid: name })
                    .await
            })
            .await;
        match result {
            Ok(_) => Ok(()),
            // dd-procmgrd rejects Start for anything it considers alive, which
            // includes Starting and Stopping, not just Running. The liveness
            // check above can therefore lose a race with a process that is
            // already coming up - including one started by a previous
            // par-control, since the two are siblings under dd-procmgrd rather
            // than parent and child. Treating that as an error would fail the
            // task that triggered the start for no reason.
            Err(error) => match error.downcast_ref::<tonic::Status>() {
                Some(status) if status.code() == tonic::Code::FailedPrecondition => {
                    log::info!(
                        "executor {:?} is already alive; adopting it",
                        self.process_name
                    );
                    Ok(())
                }
                _ => Err(error).with_context(|| {
                    format!(
                        "process-manager Start failed for process {:?}",
                        self.process_name
                    )
                }),
            },
        }
    }

    /// Alive in dd-procmgrd's sense (`ProcessState::is_alive`): a process that is
    /// still coming up or shutting down must not be started again.
    async fn is_running(&self) -> Result<bool> {
        Ok(matches!(
            self.describe_state().await?,
            Some(state)
                if state == procmgr::ProcessState::Running as i32
                    || state == procmgr::ProcessState::Starting as i32
                    || state == procmgr::ProcessState::Stopping as i32
        ))
    }

    async fn has_exited(&self) -> Result<bool> {
        match self.describe_state().await? {
            Some(state) => Ok(state == procmgr::ProcessState::Exited as i32
                || state == procmgr::ProcessState::Failed as i32),
            None => Ok(true),
        }
    }

    /// Stop the executor.
    ///
    /// Never call this from par-control's own shutdown path: dd-procmgrd
    /// serializes RPCs through a single loop that, by then, is either gone
    /// (daemon shutdown stops the gRPC server before stopping processes) or
    /// blocked holding the process write lock inside `handle_stop` waiting for
    /// par-control to exit. Either way the call deadlocks until `stop_timeout`
    /// expires and the job object kills us.
    async fn stop(&self) -> Result<()> {
        let result = self
            .rpc("Stop", |mut client, name| async move {
                client
                    .stop(procmgr::StopRequest { name_or_uuid: name })
                    .await
            })
            .await;
        match result {
            Ok(_) => Ok(()),
            // Already stopped, or stopped underneath us: nothing left to reap.
            Err(error) => match error.downcast_ref::<tonic::Status>() {
                Some(status) if status.code() == tonic::Code::FailedPrecondition => Ok(()),
                _ => Err(error).context("process-manager Stop failed"),
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{FakeProcmgr, serve_procmgr};
    use std::sync::Arc;
    use tonic::Status;

    const TEST_PROCESS_NAME: &str = "datadog-agent-action-executor";

    #[cfg(unix)]
    async fn lifecycle_for(fake: Arc<FakeProcmgr>) -> (ProcmgrLifecycle, tempfile::TempDir) {
        let (socket, dir) = serve_procmgr(fake).await;
        (
            ProcmgrLifecycle::new(&socket, TEST_PROCESS_NAME.to_string()),
            dir,
        )
    }

    /// dd-procmgrd treats Starting and Stopping as alive and rejects Start for
    /// them. If par-control asked anyway and surfaced the rejection, the task
    /// that triggered the start would fail with "executor unavailable" while the
    /// executor was in fact coming up fine.
    #[cfg(unix)]
    #[tokio::test]
    async fn ensure_started_adopts_a_process_that_is_still_coming_up() {
        for state in [
            procmgr::ProcessState::Starting,
            procmgr::ProcessState::Running,
            procmgr::ProcessState::Stopping,
        ] {
            let fake = FakeProcmgr::in_state(state);
            let (lifecycle, _dir) = lifecycle_for(Arc::clone(&fake)).await;

            lifecycle
                .ensure_started()
                .await
                .unwrap_or_else(|e| panic!("state {state:?} should be adopted: {e:#}"));
            assert!(
                fake.started().is_empty(),
                "state {state:?} is alive; Start must not be issued"
            );
        }
    }

    /// Even when the liveness check loses the race, the rejection is not an error.
    #[cfg(unix)]
    #[tokio::test]
    async fn ensure_started_tolerates_a_start_race() {
        let fake = FakeProcmgr::failing_start(
            procmgr::ProcessState::Exited,
            Status::failed_precondition("already running"),
        );
        let (lifecycle, _dir) = lifecycle_for(Arc::clone(&fake)).await;

        lifecycle.ensure_started().await.expect("race is not fatal");
        assert_eq!(fake.started().len(), 1);
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn ensure_started_propagates_other_failures() {
        let fake = FakeProcmgr::failing_start(
            procmgr::ProcessState::Exited,
            Status::not_found("no such process"),
        );
        let (lifecycle, _dir) = lifecycle_for(fake).await;

        let rendered = format!("{:#}", lifecycle.ensure_started().await.unwrap_err());
        assert!(rendered.contains("Start failed"), "{rendered}");
        assert!(rendered.contains("no such process"), "{rendered}");
    }

    /// These RPCs sit on the dispatch path, so a daemon that accepts connections
    /// but never answers must produce a reportable failure rather than hanging
    /// the action.
    #[cfg(unix)]
    #[tokio::test]
    async fn rpcs_time_out_against_an_unresponsive_daemon() {
        let (mut lifecycle, _dir) = lifecycle_for(FakeProcmgr::unresponsive()).await;
        lifecycle.rpc_timeout = Duration::from_millis(50);

        let rendered = format!("{:#}", lifecycle.is_running().await.unwrap_err());
        assert!(rendered.contains("did not respond within"), "{rendered}");
    }

    /// A missing definition means there is nothing to reap or report on.
    #[cfg(unix)]
    #[tokio::test]
    async fn reports_a_vanished_process_as_exited() {
        let (lifecycle, _dir) = lifecycle_for(FakeProcmgr::vanished()).await;

        assert!(lifecycle.has_exited().await.unwrap());
        assert!(!lifecycle.is_running().await.unwrap());
    }
}
