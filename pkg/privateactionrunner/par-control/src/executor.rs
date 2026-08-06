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

/// Public task-signing key cached by the control plane between executor runs.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SigningKey {
    pub id: String,
    pub key_type: String,
    pub key: Vec<u8>,
}

/// Dispatch operations against the executor. A trait so the orchestrator can be
/// tested without a real executor gRPC server.
pub trait Dispatcher: Send + Sync + 'static {
    fn health(&self) -> impl std::future::Future<Output = Result<Health>> + Send;
    fn sync_keys(
        &self,
        keys: Vec<SigningKey>,
    ) -> impl std::future::Future<Output = Result<Vec<SigningKey>>> + Send;
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
    /// the socket only exists after the control plane starts the executor).
    ///
    /// `ipc_cert_file` secures the channel with mTLS via the agent IPC cert, read
    /// at connection time rather than here (see [`transport::connect_lazy_tls`]).
    /// `None` dials a plaintext socket, which only a test executor accepts — the
    /// real one requires a CA-signed client cert.
    pub fn new(socket: &Path, ipc_cert_file: Option<&Path>) -> Self {
        let channel = match ipc_cert_file {
            Some(cert) => transport::connect_lazy_tls(socket, cert),
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

    async fn sync_keys(&self, keys: Vec<SigningKey>) -> Result<Vec<SigningKey>> {
        let mut client = self.client.clone();
        let response = client
            .sync_keys(pb::SyncKeysRequest {
                keys: keys
                    .into_iter()
                    .map(|key| pb::SigningKey {
                        id: key.id,
                        key_type: key.key_type,
                        key: key.key,
                    })
                    .collect(),
            })
            .await
            .context("executor SyncKeys failed")?
            .into_inner();
        Ok(response
            .keys
            .into_iter()
            .map(|key| SigningKey {
                id: key.id,
                key_type: key.key_type,
                key: key.key,
            })
            .collect())
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
