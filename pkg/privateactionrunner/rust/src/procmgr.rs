// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Executor lifecycle through the dd-procmgrd gRPC API.

use crate::proto::procmgr;
use crate::proto::procmgr::process_manager_client::ProcessManagerClient;
use crate::transport;
use anyhow::{Context, Result};
use std::path::Path;
use std::time::Duration;
use tonic::transport::Channel;

#[cfg(not(windows))]
const PROCMGR_SOCKET: &str = "/var/run/datadog-procmgrd/dd-procmgrd.sock";
#[cfg(windows)]
const PROCMGR_SOCKET: &str = r"\\.\pipe\datadog-procmgrd";
const EXECUTOR_PROCESS_NAME: &str = "datadog-agent-action-executor";
const STATE_POLL_INTERVAL: Duration = Duration::from_secs(1);

pub struct ProcmgrLifecycle {
    client: ProcessManagerClient<Channel>,
    process_name: String,
}

impl ProcmgrLifecycle {
    pub fn new() -> Self {
        Self {
            client: ProcessManagerClient::new(transport::connect_lazy(Path::new(PROCMGR_SOCKET))),
            process_name: EXECUTOR_PROCESS_NAME.to_string(),
        }
    }

    pub async fn ensure_started(&self) -> Result<()> {
        log::info!(
            "starting executor {:?} through dd-procmgrd",
            self.process_name
        );
        let result = self
            .client
            .clone()
            .start(procmgr::StartRequest {
                name_or_uuid: self.process_name.clone(),
            })
            .await;
        match result {
            Ok(_) => Ok(()),
            // dd-procmgrd rejects Start when the executor is already alive.
            Err(status) if status.code() == tonic::Code::FailedPrecondition => Ok(()),
            Err(status) => Err(status).with_context(|| {
                format!("process-manager Start failed for {:?}", self.process_name)
            }),
        }
    }

    /// Wait until the executor reaches a terminal state. The caller treats this
    /// as an unexpected exit and lets dd-procmgrd restart par-control.
    pub async fn wait_for_exit(&self) -> Result<procmgr::ProcessState> {
        loop {
            let state = self.describe_state().await?.with_context(|| {
                format!(
                    "process-manager lost the definition for {:?}",
                    self.process_name
                )
            })?;
            if is_terminal(state) {
                return Ok(state);
            }
            tokio::time::sleep(STATE_POLL_INTERVAL).await;
        }
    }

    async fn describe_state(&self) -> Result<Option<procmgr::ProcessState>> {
        let response = self
            .client
            .clone()
            .describe(procmgr::DescribeRequest {
                name_or_uuid: self.process_name.clone(),
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
}

fn is_terminal(state: procmgr::ProcessState) -> bool {
    use procmgr::ProcessState::{Crashed, Exited, Failed, Stopped};
    matches!(state, Stopped | Crashed | Exited | Failed)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn identifies_terminal_states() {
        for state in [
            procmgr::ProcessState::Stopped,
            procmgr::ProcessState::Crashed,
            procmgr::ProcessState::Exited,
            procmgr::ProcessState::Failed,
        ] {
            assert!(is_terminal(state));
        }
        assert!(!is_terminal(procmgr::ProcessState::Running));
        assert!(!is_terminal(procmgr::ProcessState::Starting));
    }
}
