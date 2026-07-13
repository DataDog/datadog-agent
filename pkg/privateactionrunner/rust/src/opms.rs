// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! OPMS client: the only component that talks to the on-prem management service.
//! Modeled as a trait so orchestration tests use a fake HTTP OPMS (PRD testing
//! seam 2). The real implementation authenticates every request with an ES256
//! JWT via [`crate::jwt::JwtSigner`] and reproduces the request envelopes,
//! headers, and status/retry-after handling of the Go `opms.Client`.

use crate::config::FLAVOR;
use crate::jwt::{JWT_HEADER_NAME, JwtSigner};
use anyhow::{Context, Result, bail};
use std::sync::{Arc, Mutex};
use std::time::Duration;

const DEQUEUE_PATH: &str = "/api/v2/on-prem-management-service/workflow-tasks/dequeue";
const TASK_UPDATE_PATH: &str =
    "/api/v2/on-prem-management-service/workflow-tasks/publish-task-update";
const HEARTBEAT_PATH: &str = "/api/v2/on-prem-management-service/workflow-tasks/heartbeat";

const RETRY_AFTER_HEADER: &str = "X-Retry-After-Ms";
/// Cap on a server-requested retry-after (matches `maxRetryAfter` on the Go side).
const MAX_RETRY_AFTER: Duration = Duration::from_secs(120);

/// A dequeued task. The control plane keeps the raw bytes (forwarded verbatim to
/// the executor for signature verification) and parses only the unverified
/// routing fields it needs for heartbeat/publish addressing.
#[derive(Debug, Clone)]
pub struct Task {
    /// Raw task envelope exactly as returned by OPMS; forwarded to the executor.
    pub raw: Vec<u8>,
    pub task_id: String,
    pub job_id: String,
    pub action_fqn: String,
    /// The actions "client" enum value (kept as its numeric wire value).
    pub client: i32,
}

impl Task {
    /// Parse routing fields from a raw OPMS task envelope.
    pub fn from_raw(raw: Vec<u8>) -> Result<Self> {
        #[derive(serde::Deserialize)]
        struct Envelope {
            data: Data,
        }
        #[derive(serde::Deserialize)]
        struct Data {
            #[serde(default)]
            id: String,
            #[serde(default)]
            attributes: Attributes,
        }
        #[derive(serde::Deserialize, Default)]
        struct Attributes {
            #[serde(default)]
            name: String,
            #[serde(default)]
            bundle_id: String,
            #[serde(default)]
            job_id: String,
            #[serde(default)]
            client: i32,
        }

        let env: Envelope =
            serde_json::from_slice(&raw).context("failed to parse dequeued task envelope")?;
        let action_fqn = format!(
            "{}.{}",
            env.data.attributes.bundle_id, env.data.attributes.name
        );
        Ok(Task {
            raw,
            task_id: env.data.id,
            job_id: env.data.attributes.job_id,
            action_fqn,
            client: env.data.attributes.client,
        })
    }
}

/// Result of a dequeue: an optional task plus a server-requested poll delay.
#[derive(Debug, Clone, Default)]
pub struct Dequeued {
    pub task: Option<Task>,
    pub retry_after: Option<Duration>,
}

/// Terminal outcome of an action, ready to publish to OPMS.
#[derive(Debug, Clone)]
pub enum Outcome {
    /// JSON-encoded success output (as produced by the executor).
    Success { output_json: Vec<u8> },
    /// Structured failure (verification, credential, allowlist, timeout, crash...).
    Failure {
        error_code: i32,
        message: String,
        external_message: String,
    },
}

/// The OPMS operations the control plane performs. Async trait (edition 2024);
/// implementors must be `Send + Sync` so the orchestrator can share and spawn.
pub trait Opms: Send + Sync {
    /// Dequeue one task (with any server-requested retry delay).
    fn dequeue(&self) -> impl std::future::Future<Output = Result<Dequeued>> + Send;

    /// Publish a task's terminal outcome (success or failure).
    fn publish(
        &self,
        task: &Task,
        outcome: &Outcome,
    ) -> impl std::future::Future<Output = Result<()>> + Send;

    /// Send a heartbeat to keep the task's lease alive.
    fn heartbeat(&self, task: &Task) -> impl std::future::Future<Output = Result<()>> + Send;
}

/// Build the JSON:API dequeue request body (client mode: `type` + attributes, no id),
/// matching `DequeueJSONRequest` marshaled with `jsonapi.MarshalClientMode()`.
fn dequeue_body(runner_started_at: &str, last_task_received_at: Option<&str>) -> Vec<u8> {
    let mut attributes = serde_json::Map::new();
    attributes.insert(
        "runner_started_at".into(),
        serde_json::Value::String(runner_started_at.to_string()),
    );
    if let Some(last) = last_task_received_at {
        attributes.insert(
            "last_task_received_at".into(),
            serde_json::Value::String(last.to_string()),
        );
    }
    let body = serde_json::json!({ "data": { "type": "dequeue", "attributes": attributes } });
    serde_json::to_vec(&body).expect("serializing dequeue body")
}

/// Build the publish-task-update request body, matching `PublishTaskUpdateJSONRequest`.
fn publish_body(task: &Task, outcome: &Outcome) -> Result<Vec<u8>> {
    let (id, payload) = match outcome {
        Outcome::Success { output_json } => {
            let outputs: serde_json::Value =
                serde_json::from_slice(output_json).context("action output was not valid JSON")?;
            (
                "succeed_task",
                serde_json::json!({ "branch": "main", "outputs": outputs }),
            )
        }
        Outcome::Failure {
            error_code,
            message,
            external_message,
        } => (
            "fail_task",
            serde_json::json!({
                "error_code": error_code,
                "error_details": message,
                "api_error": external_message,
            }),
        ),
    };

    let mut attributes = serde_json::Map::new();
    attributes.insert("task_id".into(), task.task_id.clone().into());
    // `client` is omitted when zero, matching the Go `omitempty` tag.
    if task.client != 0 {
        attributes.insert("client".into(), task.client.into());
    }
    attributes.insert("action_fqn".into(), task.action_fqn.clone().into());
    attributes.insert("job_id".into(), task.job_id.clone().into());
    attributes.insert("payload".into(), payload);

    let body = serde_json::json!({
        "data": { "type": "taskUpdate", "id": id, "attributes": attributes }
    });
    serde_json::to_vec(&body).context("serializing publish body")
}

/// Build the heartbeat request body, matching `HeartbeatJSONRequest`.
fn heartbeat_body(task: &Task) -> Vec<u8> {
    let mut attributes = serde_json::Map::new();
    attributes.insert("task_id".into(), task.task_id.clone().into());
    if task.client != 0 {
        attributes.insert("client".into(), task.client.into());
    }
    attributes.insert("action_fqn".into(), task.action_fqn.clone().into());
    attributes.insert("job_id".into(), task.job_id.clone().into());

    let body = serde_json::json!({
        "data": { "type": "heartbeat", "id": task.task_id, "attributes": attributes }
    });
    serde_json::to_vec(&body).expect("serializing heartbeat body")
}

/// Map Rust's OS/arch names to the Go `runtime.GOOS`/`GOARCH` values OPMS expects.
fn go_platform() -> &'static str {
    match std::env::consts::OS {
        "macos" => "darwin",
        other => other,
    }
}
fn go_arch() -> &'static str {
    match std::env::consts::ARCH {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        other => other,
    }
}
/// Best-effort containerized detection (informational header only).
fn is_containerized() -> bool {
    std::path::Path::new("/.dockerenv").exists()
        || std::env::var_os("KUBERNETES_SERVICE_HOST").is_some()
}

fn now_rfc3339() -> String {
    chrono::Utc::now().to_rfc3339_opts(chrono::SecondsFormat::Secs, true)
}

struct HttpResponse {
    status: u16,
    retry_after: Option<Duration>,
    body: Vec<u8>,
}

/// Real HTTPS OPMS client.
///
/// Uses the workspace `ureq` client on a blocking thread pool. Real HTTPS
/// requires the `ureq` TLS feature enabled in the workspace `Cargo.toml`
/// (see README.md); against a plaintext `http://` OPMS it works feature-free.
pub struct HttpOpms {
    base_url: String,
    signer: Arc<dyn JwtSigner>,
    timeout: Duration,
    runner_version: String,
    modes: Vec<String>,
    runner_started_at: String,
    last_task_received_at: Mutex<Option<String>>,
}

impl HttpOpms {
    pub fn new(
        base_url: String,
        signer: Arc<dyn JwtSigner>,
        runner_version: String,
        modes: Vec<String>,
        timeout: Duration,
    ) -> Self {
        HttpOpms {
            base_url,
            signer,
            timeout,
            runner_version,
            modes,
            runner_started_at: now_rfc3339(),
            last_task_received_at: Mutex::new(None),
        }
    }

    fn headers(&self, jwt: String) -> Vec<(&'static str, String)> {
        vec![
            ("Accept", "application/json".to_string()),
            ("Content-Type", "application/json".to_string()),
            (JWT_HEADER_NAME, jwt),
            ("X-Datadog-OnPrem-Version", self.runner_version.clone()),
            ("X-Datadog-OnPrem-Modes", self.modes.join(",")),
            ("X-Datadog-OnPrem-Platform", go_platform().to_string()),
            ("X-Datadog-OnPrem-Architecture", go_arch().to_string()),
            ("X-Datadog-OnPrem-Flavor", FLAVOR.to_string()),
            (
                "X-Datadog-OnPrem-Containerized",
                is_containerized().to_string(),
            ),
        ]
    }

    /// POST `body` to `path` with the standard headers + a fresh JWT, on a blocking
    /// thread. Returns the raw response (status/retry-after/body) without treating
    /// a non-2xx status as an error, so callers can decide (matching Go).
    async fn post(&self, path: &str, body: Vec<u8>) -> Result<HttpResponse> {
        let url = format!("{}{}", self.base_url, path);
        let jwt = self.signer.sign().context("failed to sign OPMS request JWT")?;
        let headers = self.headers(jwt);
        let timeout = self.timeout;

        tokio::task::spawn_blocking(move || -> Result<HttpResponse> {
            let mut req = ureq::post(&url)
                .config()
                .http_status_as_error(false)
                .timeout_global(Some(timeout))
                .build();
            for (name, value) in &headers {
                req = req.header(*name, value);
            }

            let mut resp = req.send(&body[..]).context("OPMS request failed")?;
            let status = resp.status().as_u16();
            let retry_after = resp
                .headers()
                .get(RETRY_AFTER_HEADER)
                .and_then(|v| v.to_str().ok())
                .and_then(|s| s.parse::<u64>().ok())
                .filter(|ms| *ms > 0)
                .map(|ms| Duration::from_millis(ms).min(MAX_RETRY_AFTER));
            let bytes = resp
                .body_mut()
                .read_to_vec()
                .context("failed to read OPMS response body")?;
            Ok(HttpResponse {
                status,
                retry_after,
                body: bytes,
            })
        })
        .await
        .context("OPMS request task panicked")?
    }
}

impl Opms for HttpOpms {
    async fn dequeue(&self) -> Result<Dequeued> {
        let last = self.last_task_received_at.lock().unwrap().clone();
        let body = dequeue_body(&self.runner_started_at, last.as_deref());
        let resp = self.post(DEQUEUE_PATH, body).await?;
        if resp.status != 200 {
            bail!("dequeue failed with status {}", resp.status);
        }
        if resp.body.is_empty() {
            return Ok(Dequeued {
                task: None,
                retry_after: resp.retry_after,
            });
        }
        let task = Task::from_raw(resp.body)?;
        *self.last_task_received_at.lock().unwrap() = Some(now_rfc3339());
        Ok(Dequeued {
            task: Some(task),
            retry_after: resp.retry_after,
        })
    }

    async fn publish(&self, task: &Task, outcome: &Outcome) -> Result<()> {
        // The Go client does not gate publish on status; mirror that (best effort).
        let body = publish_body(task, outcome)?;
        self.post(TASK_UPDATE_PATH, body).await?;
        Ok(())
    }

    async fn heartbeat(&self, task: &Task) -> Result<()> {
        let resp = self.post(HEARTBEAT_PATH, heartbeat_body(task)).await?;
        if resp.status != 200 {
            bail!("heartbeat failed with status {}", resp.status);
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn sample_task() -> Task {
        Task {
            raw: vec![],
            task_id: "t1".into(),
            job_id: "j1".into(),
            action_fqn: "http.do".into(),
            client: 3,
        }
    }

    #[test]
    fn parses_routing_fields_from_raw_task() {
        let raw = br#"{"data":{"id":"task-9","attributes":{"name":"do","bundle_id":"http","job_id":"job-7","client":3}}}"#.to_vec();
        let task = Task::from_raw(raw).unwrap();
        assert_eq!(task.task_id, "task-9");
        assert_eq!(task.job_id, "job-7");
        assert_eq!(task.action_fqn, "http.do");
        assert_eq!(task.client, 3);
    }

    #[test]
    fn dequeue_body_is_client_mode_envelope() {
        let v: serde_json::Value =
            serde_json::from_slice(&dequeue_body("2026-01-01T00:00:00Z", None)).unwrap();
        assert_eq!(v["data"]["type"], "dequeue");
        assert!(v["data"].get("id").is_none());
        assert_eq!(v["data"]["attributes"]["runner_started_at"], "2026-01-01T00:00:00Z");
        assert!(v["data"]["attributes"].get("last_task_received_at").is_none());
    }

    #[test]
    fn publish_success_body_matches_contract() {
        let body = publish_body(&sample_task(), &Outcome::Success {
            output_json: b"{\"k\":1}".to_vec(),
        })
        .unwrap();
        let v: serde_json::Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(v["data"]["type"], "taskUpdate");
        assert_eq!(v["data"]["id"], "succeed_task");
        assert_eq!(v["data"]["attributes"]["task_id"], "t1");
        assert_eq!(v["data"]["attributes"]["client"], 3);
        assert_eq!(v["data"]["attributes"]["payload"]["branch"], "main");
        assert_eq!(v["data"]["attributes"]["payload"]["outputs"]["k"], 1);
    }

    #[test]
    fn publish_failure_body_carries_error_code() {
        let body = publish_body(&sample_task(), &Outcome::Failure {
            error_code: 5,
            message: "bad sig".into(),
            external_message: "nope".into(),
        })
        .unwrap();
        let v: serde_json::Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(v["data"]["id"], "fail_task");
        assert_eq!(v["data"]["attributes"]["payload"]["error_code"], 5);
        assert_eq!(v["data"]["attributes"]["payload"]["error_details"], "bad sig");
        assert_eq!(v["data"]["attributes"]["payload"]["api_error"], "nope");
    }

    #[test]
    fn client_zero_is_omitted() {
        let mut task = sample_task();
        task.client = 0;
        let v: serde_json::Value =
            serde_json::from_slice(&heartbeat_body(&task)).unwrap();
        assert!(v["data"]["attributes"].get("client").is_none());
        assert_eq!(v["data"]["type"], "heartbeat");
        assert_eq!(v["data"]["id"], "t1");
    }
}
