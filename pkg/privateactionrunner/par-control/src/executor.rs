// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Client for the local control<->executor gRPC service. The orchestrator uses
//! this to gate dispatch on executor readiness (`Health`) and to run a single
//! action (`RunAction`), collapsing the server-streamed result into a terminal
//! [`Outcome`] that an upstream control-plane client can publish.

use crate::proto::executor as pb;
use crate::proto::executor::executor_client::ExecutorClient;
use crate::transport;
use anyhow::{Context, Result, bail};
use prost::Message;
use std::path::Path;
use tonic::transport::Channel;

/// Control<->executor protocol limit in bytes. Action inputs and outputs can
/// approach 15 MiB, so 20 MiB leaves protobuf headroom while still bounding
/// memory use. Keep this in sync with `maxMessageSize` in the Go executor.
const MAX_MESSAGE_SIZE: usize = 20 * 1024 * 1024;

/// Executor health snapshot used to gate dispatch.
#[derive(Debug, Clone)]
pub struct Health {
    pub ready: bool,
    pub active_actions: i32,
}

/// Terminal action outcome returned by the executor.
#[derive(Debug, Clone)]
pub enum Outcome {
    /// JSON-encoded success output produced by the executor.
    Success { output_json: Vec<u8> },
    /// Structured failure from verification or action execution.
    Failure {
        error_code: i32,
        message: String,
        external_message: String,
    },
}

/// Dispatch operations against the executor. A trait so the orchestrator can be
/// tested without a real executor gRPC server.
pub trait Dispatcher: Send + Sync + 'static {
    fn health(&self) -> impl std::future::Future<Output = Result<Health>> + Send;
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
            client: ExecutorClient::new(channel)
                .max_encoding_message_size(MAX_MESSAGE_SIZE)
                .max_decoding_message_size(MAX_MESSAGE_SIZE),
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
        let request = pb::RunActionRequest { task: raw };
        if request.encoded_len() > MAX_MESSAGE_SIZE {
            return Err(tonic::Status::resource_exhausted(format!(
                "RunAction request exceeds the {MAX_MESSAGE_SIZE}-byte protocol limit"
            ))
            .into());
        }
        let mut client = self.client.clone();
        let mut stream = client
            .run_action(request)
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

#[cfg(all(test, unix))]
mod tests {
    use super::*;
    use crate::proto::executor::executor_server::{Executor, ExecutorServer};
    use std::pin::Pin;
    use tokio_stream::Stream;
    use tokio_stream::wrappers::UnixListenerStream;
    use tonic::{Request, Response, Status};

    #[derive(Clone)]
    struct FakeExecutor {
        output: Vec<u8>,
    }

    #[tonic::async_trait]
    impl Executor for FakeExecutor {
        type RunActionStream =
            Pin<Box<dyn Stream<Item = std::result::Result<pb::RunActionResponse, Status>> + Send>>;

        async fn run_action(
            &self,
            _request: Request<pb::RunActionRequest>,
        ) -> std::result::Result<Response<Self::RunActionStream>, Status> {
            let response = pb::RunActionResponse {
                event: Some(pb::run_action_response::Event::Result(pb::ActionResult {
                    outcome: Some(pb::action_result::Outcome::Output(self.output.clone())),
                })),
            };
            Ok(Response::new(Box::pin(tokio_stream::once(Ok(response)))))
        }

        async fn health(
            &self,
            _request: Request<pb::HealthRequest>,
        ) -> std::result::Result<Response<pb::HealthResponse>, Status> {
            Ok(Response::new(pb::HealthResponse::default()))
        }
    }

    async fn test_dispatcher(output: Vec<u8>) -> (ExecutorDispatcher, tempfile::TempDir) {
        let dir = tempfile::tempdir().expect("tempdir");
        let socket = dir.path().join("executor.sock");
        let listener = tokio::net::UnixListener::bind(&socket).expect("bind executor socket");
        tokio::spawn(async move {
            let service = ExecutorServer::new(FakeExecutor { output })
                .max_decoding_message_size(MAX_MESSAGE_SIZE * 2)
                .max_encoding_message_size(MAX_MESSAGE_SIZE * 2);
            let _ = tonic::transport::Server::builder()
                .add_service(service)
                .serve_with_incoming(UnixListenerStream::new(listener))
                .await;
        });
        (ExecutorDispatcher::new(&socket, None), dir)
    }

    fn request_payload_with_encoded_size(target: usize) -> Vec<u8> {
        let mut payload = vec![b'a'; target];
        loop {
            let encoded = pb::RunActionRequest {
                task: payload.clone(),
            }
            .encoded_len();
            match encoded.cmp(&target) {
                std::cmp::Ordering::Equal => return payload,
                std::cmp::Ordering::Less => payload.resize(payload.len() + target - encoded, b'a'),
                std::cmp::Ordering::Greater => payload.truncate(payload.len() - (encoded - target)),
            }
        }
    }

    fn response_output_with_encoded_size(target: usize) -> Vec<u8> {
        let mut output = vec![b'a'; target];
        loop {
            let encoded = pb::RunActionResponse {
                event: Some(pb::run_action_response::Event::Result(pb::ActionResult {
                    outcome: Some(pb::action_result::Outcome::Output(output.clone())),
                })),
            }
            .encoded_len();
            match encoded.cmp(&target) {
                std::cmp::Ordering::Equal => return output,
                std::cmp::Ordering::Less => output.resize(output.len() + target - encoded, b'a'),
                std::cmp::Ordering::Greater => output.truncate(output.len() - (encoded - target)),
            }
        }
    }

    fn assert_status(error: &anyhow::Error, expected: tonic::Code) {
        let status = error
            .downcast_ref::<Status>()
            .unwrap_or_else(|| panic!("expected tonic status, got {error:#}"));
        assert_eq!(status.code(), expected, "unexpected status: {status}");
    }

    #[tokio::test]
    async fn enforces_request_encoding_limit() {
        let (dispatcher, _dir) = test_dispatcher(Vec::new()).await;
        let below = request_payload_with_encoded_size(MAX_MESSAGE_SIZE - 1);
        assert!(dispatcher.run_action(below).await.is_ok());

        let above = request_payload_with_encoded_size(MAX_MESSAGE_SIZE + 1);
        let error = dispatcher.run_action(above).await.unwrap_err();
        assert_status(&error, tonic::Code::ResourceExhausted);
    }

    #[tokio::test]
    async fn enforces_response_decoding_limit() {
        let below = response_output_with_encoded_size(MAX_MESSAGE_SIZE - 1);
        let (dispatcher, _dir) = test_dispatcher(below.clone()).await;
        match dispatcher.run_action(Vec::new()).await.unwrap() {
            Outcome::Success { output_json } => assert_eq!(output_json, below),
            Outcome::Failure { .. } => panic!("expected success"),
        }

        let above = response_output_with_encoded_size(MAX_MESSAGE_SIZE + 1);
        let (dispatcher, _dir) = test_dispatcher(above).await;
        let error = dispatcher.run_action(Vec::new()).await.unwrap_err();
        // Tonic reports an oversized decoded response as OutOfRange.
        assert_status(&error, tonic::Code::OutOfRange);
    }
}
