// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::BootstrapConfig;
use crate::executor::ExecutorDispatcher;
use crate::procmgr::ExecutorLifecycle;
use anyhow::{Context, Result, bail};
use std::time::Duration;

const STARTUP_TIMEOUT: Duration = Duration::from_secs(120);
const RPC_TIMEOUT: Duration = Duration::from_secs(5);
const RETRY_INTERVAL: Duration = Duration::from_secs(1);

pub async fn run_bootstrap(
    lifecycle: &impl ExecutorLifecycle,
    executor: &ExecutorDispatcher,
) -> Result<BootstrapConfig> {
    tokio::time::timeout(
        STARTUP_TIMEOUT,
        bootstrap_loop(lifecycle, executor, RETRY_INTERVAL),
    )
    .await
    .context("timed out waiting for executor configuration; check executor logs")?
}

fn retryable(error: &anyhow::Error) -> bool {
    error.downcast_ref::<tonic::Status>().is_some_and(|status| {
        matches!(
            status.code(),
            tonic::Code::Unavailable | tonic::Code::DeadlineExceeded | tonic::Code::Cancelled
        )
    })
}

async fn bootstrap_loop(
    lifecycle: &impl ExecutorLifecycle,
    executor: &ExecutorDispatcher,
    retry_interval: Duration,
) -> Result<BootstrapConfig> {
    loop {
        match lifecycle.ensure_started().await {
            Ok(()) => break,
            Err(error) if retryable(&error) => tokio::time::sleep(retry_interval).await,
            Err(error) => return Err(error),
        }
    }
    loop {
        match tokio::time::timeout(RPC_TIMEOUT, executor.control_plane_config()).await {
            Ok(Ok(snapshot)) => return Ok(snapshot),
            Ok(Err(error)) if !retryable(&error) => return Err(error),
            _ => {}
        }
        match lifecycle.has_exited().await {
            Ok(true) => {
                bail!("executor exited before configuration became available; check executor logs")
            }
            Err(error) if !retryable(&error) => return Err(error),
            _ => {}
        }
        tokio::time::sleep(retry_interval).await;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn retries_transport_failures_not_auth_or_protocol_errors() {
        for code in [
            tonic::Code::Unavailable,
            tonic::Code::DeadlineExceeded,
            tonic::Code::Cancelled,
        ] {
            assert!(retryable(&tonic::Status::new(code, "test").into()));
        }
        for code in [
            tonic::Code::PermissionDenied,
            tonic::Code::Unauthenticated,
            tonic::Code::Unimplemented,
            tonic::Code::InvalidArgument,
            tonic::Code::NotFound,
        ] {
            assert!(!retryable(&tonic::Status::new(code, "test").into()));
        }
        assert!(!retryable(&anyhow::anyhow!("invalid snapshot")));
    }
}
