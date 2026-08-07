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
    use procmgr::process_manager_server::{ProcessManager, ProcessManagerServer};
    use std::sync::{Arc, Mutex};
    use tonic::{Request, Response, Status};

    const TEST_PROCESS_NAME: &str = "datadog-agent-action-executor";

    #[derive(Default)]
    struct FakeProcmgr {
        state: Mutex<Option<i32>>,
        start_result: Mutex<Option<Status>>,
        starts: Mutex<u32>,
        stops: Mutex<u32>,
        hang: bool,
    }

    /// Newtype so the trait impl has a local self type: under Bazel the generated
    /// bindings live in a foreign crate, so implementing a foreign trait for
    /// `Arc<FakeProcmgr>` would break the orphan rule even though it compiles
    /// under `cargo`, where `include_proto!` generates the trait locally.
    #[derive(Clone)]
    struct FakeService(Arc<FakeProcmgr>);

    #[tonic::async_trait]
    impl ProcessManager for FakeService {
        async fn describe(
            &self,
            _: Request<procmgr::DescribeRequest>,
        ) -> Result<Response<procmgr::DescribeResponse>, Status> {
            if self.0.hang {
                tokio::time::sleep(Duration::from_secs(3600)).await;
            }
            let state = *self.0.state.lock().unwrap();
            Ok(Response::new(procmgr::DescribeResponse {
                detail: state.map(|state| procmgr::ProcessDetail {
                    name: TEST_PROCESS_NAME.to_string(),
                    state,
                    ..Default::default()
                }),
            }))
        }

        async fn start(
            &self,
            _: Request<procmgr::StartRequest>,
        ) -> Result<Response<procmgr::StartResponse>, Status> {
            *self.0.starts.lock().unwrap() += 1;
            if let Some(status) = self.0.start_result.lock().unwrap().clone() {
                return Err(status);
            }
            Ok(Response::new(procmgr::StartResponse::default()))
        }

        async fn stop(
            &self,
            _: Request<procmgr::StopRequest>,
        ) -> Result<Response<procmgr::StopResponse>, Status> {
            *self.0.stops.lock().unwrap() += 1;
            Ok(Response::new(procmgr::StopResponse::default()))
        }

        async fn list(
            &self,
            _: Request<procmgr::ListRequest>,
        ) -> Result<Response<procmgr::ListResponse>, Status> {
            Err(Status::unimplemented("list"))
        }
        async fn get_status(
            &self,
            _: Request<procmgr::GetStatusRequest>,
        ) -> Result<Response<procmgr::GetStatusResponse>, Status> {
            Err(Status::unimplemented("get_status"))
        }
        async fn create(
            &self,
            _: Request<procmgr::CreateRequest>,
        ) -> Result<Response<procmgr::CreateResponse>, Status> {
            Err(Status::unimplemented("create"))
        }
        async fn reload_config(
            &self,
            _: Request<procmgr::ReloadConfigRequest>,
        ) -> Result<Response<procmgr::ReloadConfigResponse>, Status> {
            Err(Status::unimplemented("reload_config"))
        }
        async fn get_config(
            &self,
            _: Request<procmgr::GetConfigRequest>,
        ) -> Result<Response<procmgr::GetConfigResponse>, Status> {
            Err(Status::unimplemented("get_config"))
        }
    }

    #[cfg(unix)]
    async fn serve(fake: Arc<FakeProcmgr>) -> (ProcmgrLifecycle, tempfile::TempDir) {
        use tokio_stream::wrappers::UnixListenerStream;

        let dir = tempfile::tempdir().unwrap();
        let socket = dir.path().join("dd-procmgrd.sock");
        let listener = tokio::net::UnixListener::bind(&socket).unwrap();
        tokio::spawn(async move {
            let _ = tonic::transport::Server::builder()
                .add_service(ProcessManagerServer::new(FakeService(fake)))
                .serve_with_incoming(UnixListenerStream::new(listener))
                .await;
        });
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
            let fake = Arc::new(FakeProcmgr {
                state: Mutex::new(Some(state as i32)),
                ..Default::default()
            });
            let (lifecycle, _dir) = serve(Arc::clone(&fake)).await;

            lifecycle
                .ensure_started()
                .await
                .unwrap_or_else(|e| panic!("state {state:?} should be adopted: {e:#}"));
            assert_eq!(
                *fake.starts.lock().unwrap(),
                0,
                "state {state:?} is alive; Start must not be issued"
            );
        }
    }

    /// Even when the liveness check loses the race, the rejection is not an error.
    #[cfg(unix)]
    #[tokio::test]
    async fn ensure_started_tolerates_a_start_race() {
        let fake = Arc::new(FakeProcmgr {
            state: Mutex::new(Some(procmgr::ProcessState::Exited as i32)),
            start_result: Mutex::new(Some(Status::failed_precondition("already running"))),
            ..Default::default()
        });
        let (lifecycle, _dir) = serve(Arc::clone(&fake)).await;

        lifecycle.ensure_started().await.expect("race is not fatal");
        assert_eq!(*fake.starts.lock().unwrap(), 1);
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn ensure_started_propagates_other_failures() {
        let fake = Arc::new(FakeProcmgr {
            state: Mutex::new(Some(procmgr::ProcessState::Exited as i32)),
            start_result: Mutex::new(Some(Status::not_found("no such process"))),
            ..Default::default()
        });
        let (lifecycle, _dir) = serve(fake).await;

        let error = lifecycle.ensure_started().await.unwrap_err();
        let rendered = format!("{error:#}");
        assert!(rendered.contains("Start failed"), "{rendered}");
        assert!(rendered.contains("no such process"), "{rendered}");
    }

    /// These RPCs sit on the dispatch path, so a daemon that accepts connections
    /// but never answers must produce a reportable failure rather than hanging
    /// the action.
    #[cfg(unix)]
    #[tokio::test]
    async fn rpcs_time_out_against_an_unresponsive_daemon() {
        let fake = Arc::new(FakeProcmgr {
            hang: true,
            ..Default::default()
        });
        let (mut lifecycle, _dir) = serve(fake).await;
        lifecycle.rpc_timeout = Duration::from_millis(50);

        let error = lifecycle.is_running().await.unwrap_err();
        let rendered = format!("{error:#}");
        assert!(rendered.contains("did not respond within"), "{rendered}");
    }

    /// A missing definition means there is nothing to reap or report on.
    #[cfg(unix)]
    #[tokio::test]
    async fn reports_a_vanished_process_as_exited() {
        let fake = Arc::new(FakeProcmgr::default());
        let (lifecycle, _dir) = serve(fake).await;

        assert!(lifecycle.has_exited().await.unwrap());
        assert!(!lifecycle.is_running().await.unwrap());
    }
}
