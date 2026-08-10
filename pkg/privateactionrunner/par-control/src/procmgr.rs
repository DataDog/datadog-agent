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

/// Bounds each process-manager RPC. dd-procmgrd serializes every command
/// through a single loop (`ProcessManager::run`), so one wedged operation stalls
/// all callers; without a deadline par-control would wait forever on a socket
/// that accepts connections but never answers, and would keep reporting itself
/// healthy while the executor is gone.
const PROCMGR_RPC_TIMEOUT: Duration = Duration::from_secs(10);

/// Resolve dd-procmgrd's listening endpoint through the shared client so the
/// daemon, CLI, and par-control follow the same platform and environment rules.
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
    /// Build a client for the executor process, resolving dd-procmgrd's socket
    /// from the environment exactly as the daemon does.
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
            // dd-procmgrd rejects Start when the executor is already alive. This
            // is the normal path when par-control is restarted underneath a
            // still-running executor, since the two are siblings under
            // dd-procmgrd rather than parent and child.
            Err(RpcError::Status(status)) if status.code() == tonic::Code::FailedPrecondition => {
                log::info!("executor {:?} is already running", self.process_name);
                Ok(())
            }
            Err(error) => Err(error).with_context(|| {
                format!("process-manager Start failed for {:?}", self.process_name)
            }),
        }
    }

    /// Stop the executor.
    ///
    /// Never call this from par-control's own shutdown path. dd-procmgrd
    /// serializes RPCs through the loop in `ProcessManager::run`, and that loop
    /// is either gone (daemon shutdown, which stops the gRPC server before
    /// stopping processes) or blocked holding the process write lock inside
    /// `handle_stop` waiting for par-control itself to exit. Either way the call
    /// deadlocks until `stop_timeout` expires and the job object kills us.
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
            // Already stopped: nothing to reap.
            Err(RpcError::Status(status)) if status.code() == tonic::Code::FailedPrecondition => {
                Ok(())
            }
            Err(error) => Err(error).with_context(|| {
                format!("process-manager Stop failed for {:?}", self.process_name)
            }),
        }
    }

    /// Watch the executor and return only when it has *failed*.
    ///
    /// A clean exit or an explicit stop is expected rather than fatal: the
    /// executor is on-demand, so it exits 0 once it has been idle and is started
    /// again on the next task. Treating that as an error would make par-control
    /// exit non-zero, and dd-procmgrd's `restart: on-failure` would then restart
    /// par-control, which would immediately re-spawn the executor and defeat idle
    /// shutdown entirely.
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
                // dd-procmgrd only reports Unknown if its own bookkeeping is
                // broken; there is no state to wait for.
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

    /// Issue one RPC under [`PROCMGR_RPC_TIMEOUT`].
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

    /// Start is rejected when the executor is already alive. par-control is
    /// restarted independently of the executor, so adopting it must not be an
    /// error.
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

    /// Any other Start failure must surface: a missing process definition means
    /// the packaging is broken and par-control cannot do its job.
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

    /// A clean exit is the idle-shutdown path and must not terminate the watch,
    /// otherwise par-control would exit non-zero, get restarted by
    /// dd-procmgrd's `restart: on-failure`, and immediately re-spawn the
    /// executor it just let go idle.
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

    /// A socket that accepts but never answers must not hang par-control
    /// forever; the RPC deadline turns it into a reportable error.
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
        // Guarded: `default_socket_path` reads process-wide state.
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
