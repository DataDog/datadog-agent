// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Client for the local control<->executor gRPC service. The orchestrator uses
//! this to gate dispatch on executor readiness (`Health`) and to run a single
//! action (`RunAction`), collapsing the server-streamed result into an
//! [`crate::opms::Outcome`] ready to publish.

use crate::opms::Outcome;
use crate::proto::executor as pb;
use crate::proto::executor::executor_client::ExecutorClient;
use crate::transport;
use anyhow::{Context, Result, bail};
use std::path::Path;
use tonic::transport::Channel;

/// Executor health snapshot used to gate dispatch.
#[derive(Debug, Clone)]
pub struct Health {
    pub ready: bool,
    pub active_actions: i32,
}

/// Dispatch operations against the executor. A trait so the orchestrator can be
/// tested without a real executor gRPC server.
pub trait Dispatcher: Send + Sync + 'static {
    /// Query executor readiness/liveness.
    fn health(&self) -> impl std::future::Future<Output = Result<Health>> + Send;
    /// Run one action (raw task bytes) and return its terminal outcome.
    fn run_action(&self, raw: Vec<u8>)
    -> impl std::future::Future<Output = Result<Outcome>> + Send;
}

/// [`Dispatcher`] backed by the executor gRPC service.
#[derive(Clone)]
pub struct ExecutorDispatcher {
    client: ExecutorClient<Channel>,
}

impl ExecutorDispatcher {
    /// Build a client for the executor on its Unix socket (connects lazily, since
    /// the socket only exists after the control plane starts the executor). When a
    /// TLS connector is supplied the channel is secured with mTLS via the agent IPC
    /// cert (slice 7); otherwise it dials a plaintext socket (e.g. for local tests).
    pub fn new(socket: &Path, tls: Option<tokio_native_tls::TlsConnector>) -> Self {
        let channel = match tls {
            Some(connector) => transport::connect_lazy_tls(socket, connector),
            None => transport::connect_lazy(socket),
        };
        ExecutorDispatcher {
            client: ExecutorClient::new(channel),
        }
    }
}

impl Dispatcher for ExecutorDispatcher {
    async fn health(&self) -> Result<Health> {
        let mut client = self.client.clone();
        let resp = client
            .health(pb::HealthRequest {})
            .await
            .context("executor Health failed")?
            .into_inner();
        Ok(Health {
            ready: resp.ready,
            active_actions: resp.active_actions,
        })
    }

    async fn run_action(&self, raw: Vec<u8>) -> Result<Outcome> {
        let mut client = self.client.clone();
        let mut stream = client
            .run_action(pb::RunActionRequest { task: raw })
            .await
            .context("executor RunAction failed")?
            .into_inner();

        // Drive the stream to its terminal ActionResult. Status events (if any)
        // are progress updates and are ignored here.
        let mut result: Option<pb::ActionResult> = None;
        while let Some(resp) = stream.message().await.context("RunAction stream error")? {
            if let Some(pb::run_action_response::Event::Result(r)) = resp.event {
                result = Some(r);
            }
        }

        let result = result.context("RunAction stream closed without a terminal result")?;
        match result.outcome {
            Some(pb::action_result::Outcome::Output(output_json)) => {
                Ok(Outcome::Success { output_json })
            }
            Some(pb::action_result::Outcome::Error(err)) => Ok(Outcome::Failure {
                error_code: err.error_code,
                message: err.message,
                external_message: err.external_message,
            }),
            None => bail!("RunAction result had no outcome"),
        }
    }
}
