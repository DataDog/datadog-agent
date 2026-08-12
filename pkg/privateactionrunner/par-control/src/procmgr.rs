// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Executor lifecycle through the dd-procmgrd gRPC API.

use crate::proto::procmgr;
use crate::proto::procmgr::process_manager_client::ProcessManagerClient;
use crate::transport;
use anyhow::{Context, Result, bail};
use std::path::{Path, PathBuf};
use std::time::Duration;
use tonic::transport::Channel;

const EXECUTOR_PROCESS_NAME: &str = "datadog-agent-action-executor";
const STATE_POLL_INTERVAL: Duration = Duration::from_secs(1);

/// Bound RPCs because dd-procmgrd serializes commands through one loop.
const PROCMGR_RPC_TIMEOUT: Duration = Duration::from_secs(10);

pub fn default_socket_path() -> PathBuf {
    dd_procmgr_client::ipc_path()
}

pub struct ProcmgrLifecycle {
    client: ProcessManagerClient<Channel>,
    process_name: String,
    rpc_timeout: Duration,
    poll_interval: Duration,
}

impl ProcmgrLifecycle {
    pub fn from_env() -> Self {
        Self::with_socket(&default_socket_path(), EXECUTOR_PROCESS_NAME.to_string())
    }

    pub fn with_socket(socket: &Path, process_name: String) -> Self {
        Self {
            client: ProcessManagerClient::new(transport::connect_lazy(socket)),
            process_name,
            rpc_timeout: PROCMGR_RPC_TIMEOUT,
            poll_interval: STATE_POLL_INTERVAL,
        }
    }

    #[cfg(all(test, unix))]
    fn with_rpc_timeout(mut self, timeout: Duration) -> Self {
        self.rpc_timeout = timeout;
        self
    }

    #[cfg(all(test, unix))]
    fn with_poll_interval(mut self, interval: Duration) -> Self {
        self.poll_interval = interval;
        self
    }

    pub async fn ensure_started(&self) -> Result<()> {
        log::info!(
            "starting executor {:?} through dd-procmgrd",
            self.process_name
        );
        let result = self
            .rpc("Start", |mut client, request| async move {
                client
                    .start(procmgr::StartRequest {
                        name_or_uuid: request,
                    })
                    .await
            })
            .await;
        match result {
            Ok(_) => Ok(()),
            // par-control can restart while its sibling executor remains alive.
            Err(RpcError::Status(status)) if status.code() == tonic::Code::FailedPrecondition => {
                log::info!("executor {:?} is already running", self.process_name);
                Ok(())
            }
            Err(error) => Err(error).with_context(|| {
                format!("process-manager Start failed for {:?}", self.process_name)
            }),
        }
    }

    /// Stop the executor. Do not call this while par-control itself is stopping:
    /// dd-procmgrd may hold the process lock while waiting for par-control to exit.
    pub async fn stop(&self) -> Result<()> {
        let result = self
            .rpc("Stop", |mut client, request| async move {
                client
                    .stop(procmgr::StopRequest {
                        name_or_uuid: request,
                    })
                    .await
            })
            .await;
        match result {
            Ok(_) => Ok(()),
            Err(RpcError::Status(status)) if status.code() == tonic::Code::FailedPrecondition => {
                Ok(())
            }
            Err(error) => Err(error).with_context(|| {
                format!("process-manager Stop failed for {:?}", self.process_name)
            }),
        }
    }

    /// Watch until the executor fails. Clean idle exits and explicit stops are expected.
    pub async fn wait_for_failure(&self) -> Result<procmgr::ProcessState> {
        let mut last_reported = None;
        loop {
            let state = self.describe_state().await?.with_context(|| {
                format!(
                    "process-manager lost the definition for {:?}",
                    self.process_name
                )
            })?;
            if last_reported != Some(state) {
                log::debug!("executor {:?} is {state:?}", self.process_name);
                last_reported = Some(state);
            }
            match state {
                procmgr::ProcessState::Failed => return Ok(state),
                procmgr::ProcessState::Unknown => {
                    bail!(
                        "process-manager reports an unknown state for {:?}",
                        self.process_name
                    )
                }
                _ => {}
            }
            tokio::time::sleep(self.poll_interval).await;
        }
    }

    pub async fn describe_state(&self) -> Result<Option<procmgr::ProcessState>> {
        let response = self
            .rpc("Describe", |mut client, request| async move {
                client
                    .describe(procmgr::DescribeRequest {
                        name_or_uuid: request,
                    })
                    .await
            })
            .await
            .with_context(|| {
                format!(
                    "process-manager Describe failed for {:?}",
                    self.process_name
                )
            })?
            .into_inner();
        response
            .detail
            .map(|detail| {
                procmgr::ProcessState::try_from(detail.state)
                    .context("process-manager returned an unknown process state")
            })
            .transpose()
    }

    async fn rpc<F, Fut, T>(&self, name: &str, call: F) -> std::result::Result<T, RpcError>
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
            Ok(Err(status)) => Err(RpcError::Status(status)),
            Err(_) => Err(RpcError::Timeout {
                rpc: name.to_string(),
                after: self.rpc_timeout,
            }),
        }
    }
}

#[derive(Debug)]
enum RpcError {
    Status(tonic::Status),
    Timeout { rpc: String, after: Duration },
}

impl std::fmt::Display for RpcError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            RpcError::Status(status) => write!(f, "{status}"),
            RpcError::Timeout { rpc, after } => {
                write!(f, "process-manager {rpc} did not respond within {after:?}")
            }
        }
    }
}

impl std::error::Error for RpcError {}

#[cfg(test)]
mod tests {
    use super::*;
    #[cfg(unix)]
    use crate::test_support::{FakeProcmgr, serve_procmgr};
    #[cfg(unix)]
    use tonic::Status;

    #[cfg(unix)]
    const TEST_PROCESS_NAME: &str = "datadog-agent-action-executor";

    #[cfg(unix)]
    async fn lifecycle_for(
        fake: std::sync::Arc<FakeProcmgr>,
    ) -> (ProcmgrLifecycle, tempfile::TempDir) {
        let (socket, dir) = serve_procmgr(fake).await;
        (
            ProcmgrLifecycle::with_socket(&socket, TEST_PROCESS_NAME.to_string()),
            dir,
        )
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn ensure_started_starts_the_executor_by_name() {
        let fake = FakeProcmgr::in_state(procmgr::ProcessState::Created);
        let (lifecycle, _dir) = lifecycle_for(std::sync::Arc::clone(&fake)).await;

        lifecycle.ensure_started().await.unwrap();

        assert_eq!(fake.started(), vec![TEST_PROCESS_NAME.to_string()]);
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn ensure_started_tolerates_an_already_running_executor() {
        let fake = FakeProcmgr::failing_start(
            procmgr::ProcessState::Created,
            Status::failed_precondition("process is already running"),
        );
        let (lifecycle, _dir) = lifecycle_for(fake).await;

        lifecycle
            .ensure_started()
            .await
            .expect("an already-running executor is not an error");
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn ensure_started_propagates_other_failures() {
        let fake = FakeProcmgr::failing_start(
            procmgr::ProcessState::Created,
            Status::not_found("process not found"),
        );
        let (lifecycle, _dir) = lifecycle_for(fake).await;

        let rendered = format!("{:#}", lifecycle.ensure_started().await.unwrap_err());
        assert!(rendered.contains("Start failed"), "{rendered}");
        assert!(rendered.contains("not found"), "{rendered}");
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn wait_for_failure_ignores_clean_exits_and_stops() {
        use procmgr::ProcessState::{Exited, Failed, Running, Stopped};
        let fake = FakeProcmgr::in_states(&[Running, Exited, Stopped, Running, Failed]);
        let (lifecycle, _dir) = lifecycle_for(fake).await;
        let lifecycle = lifecycle.with_poll_interval(Duration::from_millis(1));

        let state = tokio::time::timeout(Duration::from_secs(30), lifecycle.wait_for_failure())
            .await
            .expect("wait_for_failure should observe the Failed state")
            .unwrap();

        assert_eq!(state, Failed);
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn wait_for_failure_errors_when_the_definition_disappears() {
        let (lifecycle, _dir) = lifecycle_for(FakeProcmgr::vanished()).await;

        let rendered = format!("{:#}", lifecycle.wait_for_failure().await.unwrap_err());
        assert!(rendered.contains("lost the definition"), "{rendered}");
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn rpcs_time_out_against_an_unresponsive_daemon() {
        let (lifecycle, _dir) = lifecycle_for(FakeProcmgr::unresponsive()).await;
        let lifecycle = lifecycle.with_rpc_timeout(Duration::from_millis(50));

        let rendered = format!("{:#}", lifecycle.describe_state().await.unwrap_err());
        assert!(rendered.contains("did not respond within"), "{rendered}");
    }

    #[test]
    fn socket_path_prefers_the_daemon_environment_override() {
        let previous = std::env::var_os("DD_PM_SOCKET_PATH");
        unsafe { std::env::set_var("DD_PM_SOCKET_PATH", "/tmp/custom-procmgrd.sock") };
        assert_eq!(
            default_socket_path(),
            PathBuf::from("/tmp/custom-procmgrd.sock")
        );

        unsafe { std::env::remove_var("DD_PM_SOCKET_PATH") };

        if let Some(value) = previous {
            unsafe { std::env::set_var("DD_PM_SOCKET_PATH", value) };
        }
    }
}
