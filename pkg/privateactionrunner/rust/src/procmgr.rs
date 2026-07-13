// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Executor lifecycle, delegated to the existing Rust process manager
//! (`dd-procmgrd`) over its gRPC API. The control plane owns on-demand start and
//! graceful stop; the executor's process definition is registered out-of-band
//! (packaging is out of scope) with auto-start disabled and no auto-restart.

use crate::proto::procmgr;
use crate::proto::procmgr::process_manager_client::ProcessManagerClient;
use crate::transport;
use anyhow::{Context, Result};
use std::path::Path;
use tonic::transport::Channel;

// (constructor is synchronous; the channel connects lazily on first RPC)

/// Executor lifecycle operations the orchestrator relies on. A trait so the
/// orchestrator can be tested without a real process manager.
pub trait ExecutorLifecycle: Send + Sync + 'static {
    /// Start the executor if it is not already running.
    fn ensure_started(&self) -> impl std::future::Future<Output = Result<()>> + Send;
    /// Whether the executor process is currently running.
    fn is_running(&self) -> impl std::future::Future<Output = Result<bool>> + Send;
    /// Whether the executor process has exited/crashed/failed (for fail-and-report).
    fn has_exited(&self) -> impl std::future::Future<Output = Result<bool>> + Send;
    /// Gracefully stop the executor.
    fn stop(&self) -> impl std::future::Future<Output = Result<()>> + Send;
}

/// [`ExecutorLifecycle`] backed by dd-procmgrd.
#[derive(Clone)]
pub struct ProcmgrLifecycle {
    client: ProcessManagerClient<Channel>,
    process_name: String,
}

impl ProcmgrLifecycle {
    /// Build a client for the process manager on its Unix socket (connects lazily).
    pub fn new(socket: &Path, process_name: String) -> Self {
        ProcmgrLifecycle {
            client: ProcessManagerClient::new(transport::connect_lazy(socket)),
            process_name,
        }
    }

    async fn describe_state(&self) -> Result<Option<i32>> {
        let mut client = self.client.clone();
        let resp = client
            .describe(procmgr::DescribeRequest {
                name_or_uuid: self.process_name.clone(),
            })
            .await
            .context("process-manager Describe failed")?
            .into_inner();
        Ok(resp.detail.map(|d| d.state))
    }
}

impl ExecutorLifecycle for ProcmgrLifecycle {
    async fn ensure_started(&self) -> Result<()> {
        if self.is_running().await? {
            return Ok(());
        }
        let mut client = self.client.clone();
        client
            .start(procmgr::StartRequest {
                name_or_uuid: self.process_name.clone(),
            })
            .await
            .context("process-manager Start failed")?;
        Ok(())
    }

    async fn is_running(&self) -> Result<bool> {
        Ok(self.describe_state().await? == Some(procmgr::ProcessState::Running as i32))
    }

    async fn has_exited(&self) -> Result<bool> {
        match self.describe_state().await? {
            Some(state) => Ok(state == procmgr::ProcessState::Exited as i32
                || state == procmgr::ProcessState::Failed as i32),
            None => Ok(true),
        }
    }

    async fn stop(&self) -> Result<()> {
        let mut client = self.client.clone();
        client
            .stop(procmgr::StopRequest {
                name_or_uuid: self.process_name.clone(),
            })
            .await
            .context("process-manager Stop failed")?;
        Ok(())
    }
}
