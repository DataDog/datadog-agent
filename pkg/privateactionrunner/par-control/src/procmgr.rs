// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
use dd_procmgr_client::proto as procmgr;
use dd_procmgr_client::proto::process_manager_client::ProcessManagerClient;
use std::path::Path;
use std::time::Duration;
use tonic::transport::Channel;

// Applied per request by the channel, so an unresponsive daemon fails with
// Code::Cancelled instead of hanging a dispatch.
const PROCMGR_RPC_TIMEOUT: Duration = Duration::from_secs(10);

/// Executor lifecycle operations the orchestrator relies on. A trait so the
/// orchestrator can be tested without a real process manager.
pub trait ExecutorLifecycle: Send + Sync + 'static {
    fn ensure_started(&self) -> impl std::future::Future<Output = Result<()>> + Send;
    /// For fail-and-report: exited/crashed/failed.
    fn has_exited(&self) -> impl std::future::Future<Output = Result<bool>> + Send;
}

#[derive(Clone)]
pub struct ProcmgrLifecycle {
    client: ProcessManagerClient<Channel>,
    process_name: String,
}

impl ProcmgrLifecycle {
    pub fn new(socket: &Path, process_name: String) -> Self {
        Self::with_rpc_timeout(socket, process_name, PROCMGR_RPC_TIMEOUT)
    }

    fn with_rpc_timeout(socket: &Path, process_name: String, rpc_timeout: Duration) -> Self {
        Self {
            client: ProcessManagerClient::new(dd_procmgr_client::connect_lazy_with_timeout(
                socket,
                rpc_timeout,
            )),
            process_name,
        }
    }

    pub async fn ensure_started(&self) -> Result<()> {
        log::debug!(
            "asking dd-procmgrd to start executor {:?}",
            self.process_name
        );
        let response = self
            .client
            .clone()
            .start(procmgr::StartRequest {
                name_or_uuid: self.process_name.clone(),
            })
            .await;
        match response {
            Ok(response) => {
                let response = response.into_inner();
                log::info!(
                    "started executor {:?} through dd-procmgrd (pid {})",
                    self.process_name,
                    response.pid
                );
                Ok(())
            }
            // Start uses FailedPrecondition for both "already running" and
            // InvalidConfig. Describe to tell those apart: only a live child
            // is the start-race that another caller already won. A Describe
            // that fails on its own terms surfaces as itself, so a daemon blip
            // stays retryable instead of reading as a precondition failure.
            Err(status) if status.code() == tonic::Code::FailedPrecondition => {
                if process_is_alive(self.describe_state().await?) {
                    Ok(())
                } else {
                    Err(status).with_context(|| {
                        format!("process-manager Start failed for {:?}", self.process_name)
                    })
                }
            }
            Err(status) => Err(status).with_context(|| {
                format!("process-manager Start failed for {:?}", self.process_name)
            }),
        }
    }

    /// Whether the executor exited, crashed, failed, or never had a loadable
    /// config. A missing definition is also gone.
    pub async fn has_exited(&self) -> Result<bool> {
        match self.describe_state().await? {
            None
            | Some(procmgr::ProcessState::Exited)
            | Some(procmgr::ProcessState::Crashed)
            | Some(procmgr::ProcessState::Failed)
            | Some(procmgr::ProcessState::InvalidConfig) => Ok(true),
            Some(procmgr::ProcessState::Unknown) => bail!(
                "process-manager reports an unknown state for {:?}",
                self.process_name
            ),
            Some(_) => Ok(false),
        }
    }

    async fn describe_state(&self) -> Result<Option<procmgr::ProcessState>> {
        let response = match self
            .client
            .clone()
            .describe(procmgr::DescribeRequest {
                name_or_uuid: self.process_name.clone(),
            })
            .await
        {
            Ok(response) => response,
            Err(status) if status.code() == tonic::Code::NotFound => return Ok(None),
            Err(status) => {
                return Err(status).with_context(|| {
                    format!(
                        "process-manager Describe failed for {:?}",
                        self.process_name
                    )
                });
            }
        }
        .into_inner();
        response
            .detail
            .map(|detail| {
                procmgr::ProcessState::try_from(detail.state)
                    .context("process-manager returned an unknown process state")
            })
            .transpose()
    }
}

fn process_is_alive(state: Option<procmgr::ProcessState>) -> bool {
    matches!(
        state,
        Some(procmgr::ProcessState::Starting)
            | Some(procmgr::ProcessState::Running)
            | Some(procmgr::ProcessState::Stopping)
    )
}

impl ExecutorLifecycle for ProcmgrLifecycle {
    async fn ensure_started(&self) -> Result<()> {
        ProcmgrLifecycle::ensure_started(self).await
    }

    async fn has_exited(&self) -> Result<bool> {
        ProcmgrLifecycle::has_exited(self).await
    }
}

#[cfg(all(test, unix))]
mod tests {
    use super::*;
    use crate::test_support::{FakeProcmgr, serve_procmgr};
    use std::sync::Arc;
    use tonic::Status;

    const TEST_PROCESS_NAME: &str = "datadog-agent-action-executor";

    async fn lifecycle_for(fake: Arc<FakeProcmgr>) -> (ProcmgrLifecycle, tempfile::TempDir) {
        let (socket, dir) = serve_procmgr(fake).await;
        (
            ProcmgrLifecycle::new(&socket, TEST_PROCESS_NAME.to_string()),
            dir,
        )
    }

    #[tokio::test]
    async fn ensure_started_starts_without_a_preflight_describe() {
        let fake = FakeProcmgr::in_state(procmgr::ProcessState::Created);
        let (lifecycle, _dir) = lifecycle_for(Arc::clone(&fake)).await;

        lifecycle.ensure_started().await.unwrap();

        assert_eq!(fake.started(), vec![TEST_PROCESS_NAME.to_string()]);
        assert_eq!(fake.describe_count(), 0);
    }

    #[tokio::test]
    async fn ensure_started_tolerates_a_start_race() {
        let fake = FakeProcmgr::failing_start(
            procmgr::ProcessState::Running,
            Status::failed_precondition("process is already running"),
        );
        let (lifecycle, _dir) = lifecycle_for(Arc::clone(&fake)).await;

        lifecycle.ensure_started().await.unwrap();

        assert_eq!(fake.started(), vec![TEST_PROCESS_NAME.to_string()]);
        assert_eq!(fake.describe_count(), 1);
    }

    #[tokio::test]
    async fn ensure_started_rejects_invalid_config() {
        let fake = FakeProcmgr::failing_start(
            procmgr::ProcessState::InvalidConfig,
            Status::failed_precondition("process has invalid config, cannot start"),
        );
        let (lifecycle, _dir) = lifecycle_for(Arc::clone(&fake)).await;

        let rendered = format!("{:#}", lifecycle.ensure_started().await.unwrap_err());
        assert!(rendered.contains("Start failed"), "{rendered}");
        assert!(rendered.contains("invalid config"), "{rendered}");
        assert_eq!(fake.describe_count(), 1);
    }

    /// `bootstrap` retries transport errors and gives up on preconditions, so a
    /// describe that could not answer must not be reported as the start race.
    #[tokio::test]
    async fn ensure_started_surfaces_a_failed_describe() {
        let fake = FakeProcmgr::failing_start_and_describe(
            Status::failed_precondition("process is already running"),
            Status::unavailable("daemon went away"),
        );
        let (lifecycle, _dir) = lifecycle_for(fake).await;

        let error = lifecycle.ensure_started().await.unwrap_err();
        let status = error
            .downcast_ref::<Status>()
            .expect("the describe failure should surface as a gRPC status");
        assert_eq!(status.code(), tonic::Code::Unavailable, "{status}");
    }

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

    #[tokio::test]
    async fn has_exited_handles_terminal_and_missing_processes() {
        for (fake, expected) in [
            (FakeProcmgr::in_state(procmgr::ProcessState::Running), false),
            (FakeProcmgr::in_state(procmgr::ProcessState::Stopped), false),
            (FakeProcmgr::in_state(procmgr::ProcessState::Exited), true),
            (FakeProcmgr::in_state(procmgr::ProcessState::Crashed), true),
            (FakeProcmgr::in_state(procmgr::ProcessState::Failed), true),
            (
                FakeProcmgr::in_state(procmgr::ProcessState::InvalidConfig),
                true,
            ),
            (FakeProcmgr::vanished(), true),
        ] {
            let (lifecycle, _dir) = lifecycle_for(fake).await;
            assert_eq!(lifecycle.has_exited().await.unwrap(), expected);
        }
    }

    #[tokio::test]
    async fn rpcs_time_out_against_an_unresponsive_daemon() {
        let (socket, _dir) = serve_procmgr(FakeProcmgr::unresponsive()).await;
        let lifecycle = ProcmgrLifecycle::with_rpc_timeout(
            &socket,
            TEST_PROCESS_NAME.to_string(),
            Duration::from_millis(50),
        );

        let error = lifecycle.ensure_started().await.unwrap_err();
        let status = error
            .downcast_ref::<Status>()
            .expect("the timeout should surface as a gRPC status");
        assert_eq!(status.code(), tonic::Code::Cancelled, "{status}");
    }
}
