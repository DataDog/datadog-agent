// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! HTTP/1.1 client over Unix Domain Socket for communicating with par-executor.
//!
//! This is the Rust equivalent of pkg/system-probe/api/client/client_unix.go —
//! an http.Client that dials a Unix socket instead of TCP.  The HTTP protocol,
//! JSON bodies, and endpoint paths are identical to what the Go executor exposes.

use std::time::Duration;

use anyhow::{Context, Result};
use base64::Engine;
use base64::engine::general_purpose::STANDARD;
use http_body_util::{BodyExt, Full};
use hyper::body::Bytes;
use hyper::{Method, Request, StatusCode};
use hyper_util::rt::TokioIo;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::net::UnixStream;

// ── Wire types (must match executor/impl/executor.go) ──────────────────────

#[derive(Serialize)]
struct ExecuteRequest<'a> {
    raw_task: &'a str,           // base64-encoded raw OPMS task bytes
    #[serde(skip_serializing_if = "Option::is_none")]
    timeout_seconds: Option<u32>,
}

#[derive(Deserialize, Debug)]
pub struct ExecuteResponse {
    #[serde(default)]
    pub output: Option<Value>,
    #[serde(default)]
    pub error_code: i32,
    #[serde(default)]
    pub error_details: String,
}

// ── ExecutorClient ──────────────────────────────────────────────────────────

/// Thin client that dispatches one action to par-executor over UDS.
///
/// Each call opens a fresh connection to the socket.  This matches
/// what system-probe's Go client does (http.Transport with DialContext over
/// "unix" network) and keeps the Rust side stateless.
pub struct ExecutorClient {
    socket_path: String,
}

impl ExecutorClient {
    pub fn new(socket_path: impl Into<String>) -> Self {
        ExecutorClient {
            socket_path: socket_path.into(),
        }
    }

    /// Calls GET /debug/ready on the executor.
    /// Returns Ok(true) when the executor is accepting requests.
    pub async fn is_ready(&self) -> bool {
        match self.get("/debug/ready").await {
            Ok(status) => status == StatusCode::OK,
            Err(_) => false,
        }
    }

    /// Calls GET /debug/health on the executor.
    /// Returns Ok(true) when the executor's HTTP server is responsive.
    pub async fn is_healthy(&self) -> bool {
        match self.get("/debug/health").await {
            Ok(status) => status == StatusCode::OK,
            Err(_) => false,
        }
    }

    /// Dispatches one action to the executor via POST /execute.
    /// `raw_task_bytes` are the verbatim bytes returned by OPMS DequeueTask.
    /// `timeout` is applied at the HTTP level (total /execute round trip).
    pub async fn execute(
        &self,
        raw_task_bytes: &[u8],
        timeout: Duration,
    ) -> Result<ExecuteResponse> {
        let raw_task_b64 = STANDARD.encode(raw_task_bytes);
        let timeout_secs = timeout.as_secs() as u32;

        let body_json = serde_json::to_vec(&ExecuteRequest {
            raw_task: &raw_task_b64,
            timeout_seconds: Some(timeout_secs),
        })
        .context("failed to serialize ExecuteRequest")?;

        let resp_bytes = tokio::time::timeout(
            timeout + Duration::from_secs(5), // small grace on top of task timeout
            self.post("/execute", body_json),
        )
        .await
        .context("executor /execute timed out")?
        .context("executor /execute request failed")?;

        serde_json::from_slice(&resp_bytes).context("failed to parse ExecuteResponse")
    }

    // ── Low-level helpers ────────────────────────────────────────────────────

    async fn get(&self, path: &str) -> Result<StatusCode> {
        let (mut sender, conn) = self.connect().await?;
        tokio::spawn(conn);

        let req = Request::builder()
            .method(Method::GET)
            // hyper over UDS requires a dummy host; the socket path is the real address.
            .uri(format!("http://par-executor{path}"))
            .body(Full::<Bytes>::new(Bytes::new()))
            .context("failed to build GET request")?;

        let resp = sender
            .send_request(req)
            .await
            .context("GET request failed")?;

        Ok(resp.status())
    }

    async fn post(&self, path: &str, body: Vec<u8>) -> Result<Vec<u8>> {
        let (mut sender, conn) = self.connect().await?;
        tokio::spawn(conn);

        let req = Request::builder()
            .method(Method::POST)
            .uri(format!("http://par-executor{path}"))
            .header("Content-Type", "application/json")
            .body(Full::new(Bytes::from(body)))
            .context("failed to build POST request")?;

        let resp = sender
            .send_request(req)
            .await
            .context("POST request failed")?;

        let status = resp.status();
        let bytes = resp
            .into_body()
            .collect()
            .await
            .context("failed to read response body")?
            .to_bytes();

        // The executor always returns an ExecuteResponse JSON body, even for
        // protocol errors (4xx). Return the body for the caller to parse so
        // error_code / error_details are propagated correctly.
        // A 5xx (executor crash) is the only case where the body might not
        // be valid JSON — surface that as an error.
        if status.is_server_error() {
            anyhow::bail!(
                "executor returned server error {}: {}",
                status,
                String::from_utf8_lossy(&bytes)
            );
        }

        Ok(bytes.to_vec())
    }

    async fn connect(
        &self,
    ) -> Result<(
        hyper::client::conn::http1::SendRequest<Full<Bytes>>,
        hyper::client::conn::http1::Connection<TokioIo<UnixStream>, Full<Bytes>>,
    )> {
        let stream = UnixStream::connect(&self.socket_path)
            .await
            .with_context(|| format!("failed to connect to executor socket: {}", self.socket_path))?;

        hyper::client::conn::http1::handshake(TokioIo::new(stream))
            .await
            .context("HTTP/1.1 handshake failed")
    }
}
