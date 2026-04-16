// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Binary IPC client for communicating with par-executor over UDS.
//!
//! Protocol (one connection per request, matches protocol.go on the Go side):
//!
//! Ping request:  [0x01]
//! Ping response: [0x01]
//!
//! Execute request:
//!   [0x02]                      frame type
//!   [u32 LE]                    raw task byte length
//!   [task_len bytes]            verbatim OPMS task JSON — no base64 encoding
//!   [u32 LE]                    timeout in seconds
//!
//! Execute response:
//!   [u8]                        0x00 = ok, 0x01 = error
//!   [u32 LE]                    payload length
//!   [payload_len bytes]         JSON
//!     ok    → raw action output JSON
//!     error → {"error_code": N, "error_details": "..."}
//!
//! This replaces the previous HTTP/1.1 + JSON + base64 transport.  For a 15 MB
//! task payload the old approach allocated ~35 MB (base64 + JSON decode rounds).
//! The binary protocol allocates exactly one buffer of task_len bytes — matching
//! the allocation profile of the original single-process PAR.

use std::time::Duration;

use anyhow::{Context, Result};
use serde::Deserialize;
use serde_json::Value;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::UnixStream;

const FRAME_PING: u8 = 0x01;
const FRAME_EXECUTE: u8 = 0x02;
const STATUS_OK: u8 = 0x00;
const STATUS_ERR: u8 = 0x01;

/// Decoded execute response from par-executor.
#[derive(Deserialize, Debug)]
pub struct ExecuteResponse {
    #[serde(default)]
    pub output: Option<Value>,
    #[serde(default)]
    pub error_code: i32,
    #[serde(default)]
    pub error_details: String,
}

#[derive(Deserialize)]
struct ErrorPayload {
    error_code: i32,
    error_details: String,
}

/// Thin client that dispatches one frame to par-executor over UDS.
/// Each call opens a fresh connection — matches the one-connection-per-request
/// model on the server side.
pub struct ExecutorClient {
    socket_path: String,
}

impl ExecutorClient {
    pub fn new(socket_path: impl Into<String>) -> Self {
        ExecutorClient {
            socket_path: socket_path.into(),
        }
    }

    /// Sends a ping frame. Returns true when par-executor responds with pong.
    pub async fn ping(&self) -> bool {
        self.do_ping().await.unwrap_or(false)
    }

    async fn do_ping(&self) -> Result<bool> {
        let mut stream = UnixStream::connect(&self.socket_path)
            .await
            .context("ping: connect")?;
        stream.write_all(&[FRAME_PING]).await.context("ping: write")?;
        let mut pong = [0u8; 1];
        stream.read_exact(&mut pong).await.context("ping: read pong")?;
        Ok(pong[0] == 0x01)
    }

    /// Dispatches one action to par-executor.
    ///
    /// `raw_task_bytes` are the verbatim bytes received from OPMS — no encoding
    /// is applied; they are written directly into the frame.
    pub async fn execute(
        &self,
        raw_task_bytes: &[u8],
        timeout: Duration,
    ) -> Result<ExecuteResponse> {
        // Hard deadline: task timeout + grace for par-control to publish failure.
        let deadline = timeout + Duration::from_secs(5);
        tokio::time::timeout(deadline, self.do_execute(raw_task_bytes, timeout))
            .await
            .context("executor /execute timed out")?
    }

    async fn do_execute(
        &self,
        raw_task_bytes: &[u8],
        timeout: Duration,
    ) -> Result<ExecuteResponse> {
        let mut stream = UnixStream::connect(&self.socket_path)
            .await
            .with_context(|| format!("execute: connect to {}", self.socket_path))?;

        // ── Write request ──────────────────────────────────────────────────
        // [0x02][u32 LE: task_len][raw task bytes][u32 LE: timeout_secs]
        stream.write_all(&[FRAME_EXECUTE]).await.context("execute: write frame type")?;

        let task_len = raw_task_bytes.len() as u32;
        stream.write_all(&task_len.to_le_bytes()).await.context("execute: write task_len")?;
        stream.write_all(raw_task_bytes).await.context("execute: write task bytes")?;

        let timeout_secs = timeout.as_secs() as u32;
        stream.write_all(&timeout_secs.to_le_bytes()).await.context("execute: write timeout")?;
        stream.flush().await.context("execute: flush")?;

        // ── Read response ──────────────────────────────────────────────────
        // [u8: status][u32 LE: payload_len][payload bytes]
        let mut status_buf = [0u8; 1];
        stream.read_exact(&mut status_buf).await.context("execute: read status")?;

        let mut len_buf = [0u8; 4];
        stream.read_exact(&mut len_buf).await.context("execute: read payload_len")?;
        let payload_len = u32::from_le_bytes(len_buf) as usize;

        let mut payload = vec![0u8; payload_len];
        stream.read_exact(&mut payload).await.context("execute: read payload")?;

        // ── Decode ─────────────────────────────────────────────────────────
        match status_buf[0] {
            STATUS_OK => {
                let output: Value =
                    serde_json::from_slice(&payload).context("execute: parse output JSON")?;
                Ok(ExecuteResponse {
                    output: Some(output),
                    error_code: 0,
                    error_details: String::new(),
                })
            }
            STATUS_ERR => {
                let err: ErrorPayload =
                    serde_json::from_slice(&payload).context("execute: parse error payload")?;
                Ok(ExecuteResponse {
                    output: None,
                    error_code: err.error_code,
                    error_details: err.error_details,
                })
            }
            other => anyhow::bail!("execute: unknown response status 0x{:02x}", other),
        }
    }
}
