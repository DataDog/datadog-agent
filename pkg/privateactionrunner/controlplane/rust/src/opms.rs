// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! OPMS (On-Prem Management Service) HTTP client.
//!
//! Mirrors pkg/privateactionrunner/opms/opms.go — same endpoints, same JWT
//! auth header, same request/response structures.

use anyhow::{Context, Result};
use p256::ecdsa::SigningKey;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::jwt::generate_jwt;

// ── Endpoint paths (identical to opms.go constants) ────────────────────────

const DEQUEUE_PATH: &str =
    "/api/v2/on-prem-management-service/workflow-tasks/dequeue";
const TASK_UPDATE_PATH: &str =
    "/api/v2/on-prem-management-service/workflow-tasks/publish-task-update";
const HEARTBEAT_PATH: &str =
    "/api/v2/on-prem-management-service/workflow-tasks/heartbeat";

const JWT_HEADER: &str = "X-Datadog-OnPrem-JWT";
const VERSION_HEADER: &str = "X-Datadog-OnPrem-Version";
const RUNNER_VERSION: &str = env!("CARGO_PKG_VERSION");

// ── Task structure ──────────────────────────────────────────────────────────

/// Minimal task fields the control plane needs for OPMS calls.
/// The full raw JSON is forwarded unchanged to the executor.
#[derive(Debug, Clone)]
pub struct Task {
    pub task_id: String,
    pub job_id: String,
    pub action_fqn: String, // "{bundle_id}.{name}"
    pub client: Value,      // actionsclientpb.Client — kept as JSON value
    pub raw: Vec<u8>,       // raw OPMS response bytes forwarded to executor
}

#[derive(Deserialize)]
struct RawTask {
    data: RawTaskData,
}

#[derive(Deserialize)]
struct RawTaskData {
    id: String,
    attributes: RawTaskAttributes,
}

#[derive(Deserialize)]
struct RawTaskAttributes {
    name: String,
    bundle_id: String,
    job_id: String,
    #[serde(default)]
    client: Value,
}

impl Task {
    fn from_bytes(raw: Vec<u8>) -> Result<Self> {
        let parsed: RawTask =
            serde_json::from_slice(&raw).context("failed to parse task JSON from OPMS")?;
        Ok(Task {
            task_id: parsed.data.id,
            job_id: parsed.data.attributes.job_id,
            action_fqn: format!(
                "{}.{}",
                parsed.data.attributes.bundle_id, parsed.data.attributes.name
            ),
            client: parsed.data.attributes.client,
            raw,
        })
    }
}

// ── Publish request structures (mirrors Go PublishTaskUpdateJSONRequest) ────

#[derive(Serialize)]
struct PublishRequest<'a> {
    data: PublishData<'a>,
}

#[derive(Serialize)]
struct PublishData<'a> {
    #[serde(rename = "type")]
    kind: &'a str,
    id: &'a str,
    attributes: PublishAttributes<'a>,
}

#[derive(Serialize)]
struct PublishAttributes<'a> {
    task_id: &'a str,
    client: &'a Value,
    action_fqn: &'a str,
    job_id: &'a str,
    payload: PublishPayload<'a>,
}

#[derive(Serialize)]
struct PublishPayload<'a> {
    #[serde(skip_serializing_if = "Option::is_none")]
    outputs: Option<&'a Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    branch: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error_code: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error_details: Option<&'a str>,
}

#[derive(Serialize)]
struct HeartbeatRequest<'a> {
    data: HeartbeatData<'a>,
}

#[derive(Serialize)]
struct HeartbeatData<'a> {
    #[serde(rename = "type")]
    kind: &'a str,
    id: &'a str,
    attributes: HeartbeatAttributes<'a>,
}

#[derive(Serialize)]
struct HeartbeatAttributes<'a> {
    task_id: &'a str,
    client: &'a Value,
    action_fqn: &'a str,
    job_id: &'a str,
}

// ── OPMSClient ──────────────────────────────────────────────────────────────

/// Cloneable OPMS client backed by reqwest.
/// Mirrors the Go opms.Client interface.
#[derive(Clone)]
pub struct OPMSClient {
    http: reqwest::Client,
    base_url: String,
    org_id: i64,
    runner_id: String,
    signing_key: std::sync::Arc<SigningKey>,
}

impl OPMSClient {
    pub fn new(
        dd_api_host: &str,
        org_id: i64,
        runner_id: String,
        signing_key: SigningKey,
    ) -> Result<Self> {
        let http = reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(30))
            .build()
            .context("failed to build reqwest client")?;

        Ok(OPMSClient {
            http,
            base_url: format!("https://{dd_api_host}"),
            org_id,
            runner_id,
            signing_key: std::sync::Arc::new(signing_key),
        })
    }

    fn jwt(&self) -> Result<String> {
        generate_jwt(self.org_id, &self.runner_id, &self.signing_key)
    }

    fn url(&self, path: &str) -> String {
        format!("{}{}", self.base_url, path)
    }

    /// Dequeue a task from OPMS. Returns None when the queue is empty.
    /// Mirrors opms.DequeueTask.
    pub async fn dequeue_task(&self) -> Result<Option<Task>> {
        let jwt = self.jwt().context("JWT generation failed")?;
        let resp = self
            .http
            .post(self.url(DEQUEUE_PATH))
            .header(JWT_HEADER, jwt)
            .header(VERSION_HEADER, RUNNER_VERSION)
            .header("Accept", "application/json")
            .header("Content-Type", "application/json")
            .send()
            .await
            .context("dequeue_task HTTP request failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("dequeue_task failed: HTTP {status}: {body}");
        }

        let bytes = resp.bytes().await.context("dequeue_task: reading body")?;
        if bytes.is_empty() {
            return Ok(None); // empty body = no task available
        }

        let task = Task::from_bytes(bytes.to_vec())?;
        Ok(Some(task))
    }

    /// Publish a successful task result. Mirrors opms.PublishSuccess.
    pub async fn publish_success(&self, task: &Task, output: &Value) -> Result<()> {
        let req = PublishRequest {
            data: PublishData {
                kind: "taskUpdate",
                id: "succeed_task",
                attributes: PublishAttributes {
                    task_id: &task.task_id,
                    client: &task.client,
                    action_fqn: &task.action_fqn,
                    job_id: &task.job_id,
                    payload: PublishPayload {
                        outputs: Some(output),
                        branch: Some("main"),
                        error_code: None,
                        error_details: None,
                    },
                },
            },
        };
        self.post_update(&req).await
    }

    /// Publish a task failure. Mirrors opms.PublishFailure.
    pub async fn publish_failure(&self, task: &Task, error_code: i32, details: &str) -> Result<()> {
        let req = PublishRequest {
            data: PublishData {
                kind: "taskUpdate",
                id: "fail_task",
                attributes: PublishAttributes {
                    task_id: &task.task_id,
                    client: &task.client,
                    action_fqn: &task.action_fqn,
                    job_id: &task.job_id,
                    payload: PublishPayload {
                        outputs: None,
                        branch: None,
                        error_code: Some(error_code),
                        error_details: Some(details),
                    },
                },
            },
        };
        self.post_update(&req).await
    }

    /// Send a heartbeat to OPMS while a task is executing.
    /// Mirrors opms.Heartbeat.
    pub async fn heartbeat(&self, task: &Task) -> Result<()> {
        let jwt = self.jwt()?;
        let req = HeartbeatRequest {
            data: HeartbeatData {
                kind: "heartbeat",
                id: &task.task_id,
                attributes: HeartbeatAttributes {
                    task_id: &task.task_id,
                    client: &task.client,
                    action_fqn: &task.action_fqn,
                    job_id: &task.job_id,
                },
            },
        };
        let resp = self
            .http
            .post(self.url(HEARTBEAT_PATH))
            .header(JWT_HEADER, jwt)
            .header(VERSION_HEADER, RUNNER_VERSION)
            .json(&req)
            .send()
            .await
            .context("heartbeat HTTP request failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("heartbeat failed: HTTP {status}: {body}");
        }
        Ok(())
    }

    async fn post_update<T: Serialize>(&self, body: &T) -> Result<()> {
        let jwt = self.jwt()?;
        let resp = self
            .http
            .post(self.url(TASK_UPDATE_PATH))
            .header(JWT_HEADER, jwt)
            .header(VERSION_HEADER, RUNNER_VERSION)
            .json(body)
            .send()
            .await
            .context("publish_update HTTP request failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("publish_update failed: HTTP {status}: {body}");
        }
        Ok(())
    }
}
