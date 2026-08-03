// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! OPMS client: the only component that talks to the on-prem management service.
//! Modeled as a trait so orchestration tests use a fake HTTP OPMS (PRD testing
//! seam 2). The real implementation authenticates every request with an ES256
//! JWT via [`crate::jwt::JwtSigner`] and reproduces the request envelopes,
//! headers, and status/retry-after handling of the Go `opms.Client`.

use crate::config::{FLAVOR, ProxyConfig, TlsConfig};
use crate::jwt::{JWT_HEADER_NAME, JwtSigner};
use anyhow::{Context, Result, bail};
use reqwest::header::{HeaderMap, HeaderName, HeaderValue};
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::Duration;

const DEQUEUE_PATH: &str = "/api/v2/on-prem-management-service/workflow-tasks/dequeue";
const TASK_UPDATE_PATH: &str =
    "/api/v2/on-prem-management-service/workflow-tasks/publish-task-update";
const HEARTBEAT_PATH: &str = "/api/v2/on-prem-management-service/workflow-tasks/heartbeat";
const HEALTH_CHECK_PATH: &str = "/api/v2/on-prem-management-service/runner/health-check";

const RETRY_AFTER_HEADER: &str = "X-Retry-After-Ms";
const SERVER_TIME_HEADER: &str = "X-Server-Time";
/// Cap on a server-requested retry-after (matches `maxRetryAfter` on the Go side).
const MAX_RETRY_AFTER: Duration = Duration::from_secs(120);

/// A dequeued task. The control plane keeps the raw bytes (forwarded verbatim to
/// the executor for signature verification) and parses only the unverified
/// routing fields it needs for heartbeat/publish addressing.
#[derive(Debug, Clone)]
pub struct Task {
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

/// Result of a terminal-outcome publication.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PublishResult {
    Published,
    /// OPMS rejected the update with a non-retryable client response.
    Rejected {
        status: u16,
        detail: String,
    },
}

/// Result of a heartbeat.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HeartbeatResult {
    Alive,
    /// OPMS no longer knows the job, so further heartbeats are pointless.
    NotFound,
}

/// Outcome of a runner health check. A non-200 status is *not* an error here:
/// the Go client also returns the parsed header data on rejection so the loop
/// can still honor a server-requested pacing hint.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct HealthCheck {
    pub status: u16,
    /// `X-Server-Time`, kept verbatim for logging (Go only logs it).
    pub server_time: Option<String>,
    pub retry_after: Option<Duration>,
    /// Bounded body quote; empty when the check succeeded.
    pub detail: String,
}

impl HealthCheck {
    pub fn ok(&self) -> bool {
        self.status == 200
    }
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
    ) -> impl std::future::Future<Output = Result<PublishResult>> + Send;

    /// Send a heartbeat to keep the task's lease alive.
    fn heartbeat(
        &self,
        task: &Task,
    ) -> impl std::future::Future<Output = Result<HeartbeatResult>> + Send;

    /// Report runner liveness to OPMS, independently of task flow. This is the
    /// only signal a runner with no work emits, so the control plane must send it
    /// exactly like the Go `CommonRunner` health-check loop does.
    fn health_check(&self) -> impl std::future::Future<Output = Result<HealthCheck>> + Send;
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
    server_time: Option<String>,
    body: Vec<u8>,
}

/// How much of an error response body to quote in an error message. OPMS returns
/// a JSON:API `errors` array on rejection, and without it a wire-envelope
/// mismatch surfaces as a bare "status 400" with nothing to act on. Bounded so a
/// misconfigured endpoint answering with an HTML page cannot flood the log.
const MAX_ERROR_BODY: usize = 512;

impl HttpResponse {
    /// A bounded, lossy rendering of the body for error messages.
    fn error_detail(&self) -> String {
        if self.body.is_empty() {
            return "<empty body>".to_string();
        }
        let truncated = self.body.len() > MAX_ERROR_BODY;
        let text = String::from_utf8_lossy(&self.body[..self.body.len().min(MAX_ERROR_BODY)]);
        if truncated {
            format!("{text}... [{} bytes total]", self.body.len())
        } else {
            text.into_owned()
        }
    }
}

const POOL_IDLE_TIMEOUT: Duration = Duration::from_secs(45);
const MAX_IDLE_CONNECTIONS_PER_HOST: usize = 5;

/// Real OPMS client backed by reqwest's pooled Hyper transport and native-tls.
/// It preserves the Agent's proxy routing, including CONNECT tunnels, proxy
/// authentication, and no-proxy exclusions. A plaintext base URL remains
/// restricted to the verification-bypass E2E mode by configuration loading.
pub struct HttpOpms {
    base_url: reqwest::Url,
    client: reqwest::Client,
    signer: Arc<dyn JwtSigner>,
    runner_version: String,
    modes: Vec<String>,
    extra_headers: HashMap<String, String>,
    runner_started_at: String,
    last_task_received_at: Mutex<Option<String>>,
}

pub struct HttpOpmsConfig {
    pub runner_version: String,
    pub modes: Vec<String>,
    pub timeout: Duration,
    pub proxy: ProxyConfig,
    pub tls: TlsConfig,
    pub extra_headers: HashMap<String, String>,
}

impl HttpOpms {
    pub fn new(
        base_url: String,
        signer: Arc<dyn JwtSigner>,
        config: HttpOpmsConfig,
    ) -> Result<Self> {
        Self::new_with_builder(base_url, signer, config, reqwest::Client::builder())
    }

    fn new_with_builder(
        base_url: String,
        signer: Arc<dyn JwtSigner>,
        options: HttpOpmsConfig,
        builder: reqwest::ClientBuilder,
    ) -> Result<Self> {
        let base_url = parse_base_url(&base_url)?;
        let mut builder = builder
            .timeout(options.timeout)
            .pool_idle_timeout(POOL_IDLE_TIMEOUT)
            .pool_max_idle_per_host(MAX_IDLE_CONNECTIONS_PER_HOST)
            .http1_only()
            .danger_accept_invalid_certs(options.tls.skip_ssl_validation)
            .min_tls_version(min_tls_version(&options.tls.min_tls_version))
            // We already merged the Agent's YAML and environment proxy settings;
            // do not let reqwest independently re-read process environment.
            .no_proxy();
        if let Some(proxy) = proxy_for_base_url(&base_url, &options.proxy)? {
            builder = builder.proxy(proxy);
        }
        let client = builder.build().context("building the OPMS HTTP client")?;
        Ok(Self {
            base_url,
            client,
            signer,
            runner_version: options.runner_version,
            modes: options.modes,
            extra_headers: options.extra_headers,
            runner_started_at: now_rfc3339(),
            last_task_received_at: Mutex::new(None),
        })
    }

    fn headers(&self, jwt: String) -> Result<HeaderMap> {
        let fixed = [
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
        ];
        let mut headers = HeaderMap::new();
        for (name, value) in fixed {
            headers.insert(
                HeaderName::from_bytes(name.as_bytes()).context("invalid OPMS header name")?,
                HeaderValue::from_str(&value).context("invalid OPMS header value")?,
            );
        }
        // Match Go's precedence: operator-provided headers are applied last and
        // may intentionally override a standard routing/diagnostic header.
        for (name, value) in &self.extra_headers {
            headers.insert(
                HeaderName::from_bytes(name.as_bytes())
                    .with_context(|| format!("invalid OPMS extra header name {name:?}"))?,
                HeaderValue::from_str(value)
                    .with_context(|| format!("invalid value for OPMS extra header {name:?}"))?,
            );
        }
        Ok(headers)
    }

    /// POST `body` to `path` with the standard headers + a fresh JWT.
    async fn post(&self, path: &str, body: Vec<u8>) -> Result<HttpResponse> {
        self.request(reqwest::Method::POST, path, Some(body)).await
    }

    /// GET `path` with the standard headers + a fresh JWT.
    async fn get(&self, path: &str) -> Result<HttpResponse> {
        self.request(reqwest::Method::GET, path, None).await
    }

    /// Send a signed request. Returns the raw response (status/headers/body)
    /// without treating a non-2xx status as an error, so callers can decide
    /// (matching Go).
    async fn request(
        &self,
        method: reqwest::Method,
        path: &str,
        body: Option<Vec<u8>>,
    ) -> Result<HttpResponse> {
        let jwt = self
            .signer
            .sign()
            .context("failed to sign OPMS request JWT")?;
        let url = self
            .base_url
            .join(path)
            .with_context(|| format!("building OPMS request URL for {path}"))?;
        let mut request = self.client.request(method, url).headers(self.headers(jwt)?);
        if let Some(body) = body {
            request = request.body(body);
        }
        let response = request
            .send()
            .await
            .with_context(|| format!("OPMS request to {path} failed"))?;
        let status = response.status().as_u16();
        let retry_after = response
            .headers()
            .get(RETRY_AFTER_HEADER)
            .and_then(|value| value.to_str().ok())
            .and_then(|value| value.parse::<u64>().ok())
            .filter(|milliseconds| *milliseconds > 0)
            .map(|milliseconds| Duration::from_millis(milliseconds).min(MAX_RETRY_AFTER));
        let server_time = response
            .headers()
            .get(SERVER_TIME_HEADER)
            .and_then(|value| value.to_str().ok())
            .map(str::to_string);
        let body = response
            .bytes()
            .await
            .context("failed to read OPMS response body")?
            .to_vec();
        Ok(HttpResponse {
            status,
            retry_after,
            server_time,
            body,
        })
    }
}

fn min_tls_version(raw: &str) -> reqwest::tls::Version {
    match raw.trim().to_ascii_lowercase().as_str() {
        "tlsv1.0" => reqwest::tls::Version::TLS_1_0,
        "tlsv1.1" => reqwest::tls::Version::TLS_1_1,
        "tlsv1.3" => reqwest::tls::Version::TLS_1_3,
        // Match the Go transport: TLS 1.2 is both the explicit value and the
        // fallback for an empty or invalid setting.
        _ => reqwest::tls::Version::TLS_1_2,
    }
}

fn parse_base_url(raw: &str) -> Result<reqwest::Url> {
    let url = reqwest::Url::parse(raw).context("invalid OPMS URL")?;
    if !matches!(url.scheme(), "http" | "https") {
        bail!("unsupported OPMS URL scheme {:?}", url.scheme());
    }
    if url.username() != "" || url.password().is_some() {
        bail!("OPMS URL must not contain credentials");
    }
    if url.host_str().is_none() {
        bail!("OPMS URL has no host");
    }
    if url.path() != "/" || url.query().is_some() || url.fragment().is_some() {
        bail!("OPMS URL must be an origin without a path, query, or fragment");
    }
    Ok(url)
}

fn proxy_for_base_url(
    base_url: &reqwest::Url,
    config: &ProxyConfig,
) -> Result<Option<reqwest::Proxy>> {
    let raw_proxy = match base_url.scheme() {
        "https" => config.https.as_deref(),
        "http" => config.http.as_deref(),
        _ => None,
    };
    let Some(raw_proxy) = raw_proxy else {
        return Ok(None);
    };

    let authority = url_authority(base_url);
    if !config.no_proxy_nonexact_match && config.no_proxy.iter().any(|entry| entry == &authority) {
        return Ok(None);
    }

    let proxy = match base_url.scheme() {
        "https" => reqwest::Proxy::https(raw_proxy),
        "http" => reqwest::Proxy::http(raw_proxy),
        _ => unreachable!("base URL scheme was validated"),
    }
    .context("invalid Agent proxy URL")?;
    if config.no_proxy_nonexact_match {
        let no_proxy = reqwest::NoProxy::from_string(&config.no_proxy.join(","));
        Ok(Some(proxy.no_proxy(no_proxy)))
    } else {
        Ok(Some(proxy))
    }
}

fn url_authority(url: &reqwest::Url) -> String {
    let host = url.host_str().unwrap_or_default();
    let host = if host.contains(':') {
        format!("[{host}]")
    } else {
        host.to_string()
    };
    match url.port() {
        Some(port) => format!("{host}:{port}"),
        None => host,
    }
}

impl Opms for HttpOpms {
    async fn dequeue(&self) -> Result<Dequeued> {
        let last = self.last_task_received_at.lock().unwrap().clone();
        let body = dequeue_body(&self.runner_started_at, last.as_deref());
        let resp = self.post(DEQUEUE_PATH, body).await?;
        if resp.status != 200 {
            bail!(
                "dequeue failed with status {}: {}",
                resp.status,
                resp.error_detail()
            );
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

    async fn publish(&self, task: &Task, outcome: &Outcome) -> Result<PublishResult> {
        let body = publish_body(task, outcome)?;
        let resp = self.post(TASK_UPDATE_PATH, body).await?;
        match resp.status {
            200 => Ok(PublishResult::Published),
            408 | 425 | 429 | 500..=599 => bail!(
                "publishing task {} returned retryable status {}: {}",
                task.task_id,
                resp.status,
                resp.error_detail()
            ),
            status => Ok(PublishResult::Rejected {
                status,
                detail: resp.error_detail(),
            }),
        }
    }

    async fn heartbeat(&self, task: &Task) -> Result<HeartbeatResult> {
        let resp = self.post(HEARTBEAT_PATH, heartbeat_body(task)).await?;
        match resp.status {
            200 => Ok(HeartbeatResult::Alive),
            404 => Ok(HeartbeatResult::NotFound),
            _ => bail!(
                "heartbeat failed with status {}: {}",
                resp.status,
                resp.error_detail()
            ),
        }
    }

    async fn health_check(&self) -> Result<HealthCheck> {
        let resp = self.get(HEALTH_CHECK_PATH).await?;
        let detail = if resp.status == 200 {
            String::new()
        } else {
            resp.error_detail()
        };
        Ok(HealthCheck {
            status: resp.status,
            server_time: resp.server_time,
            retry_after: resp.retry_after,
            detail,
        })
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
    fn agent_tls_minimum_defaults_match_go() {
        assert_eq!(min_tls_version(""), reqwest::tls::Version::TLS_1_2);
        assert_eq!(
            min_tls_version("not-a-version"),
            reqwest::tls::Version::TLS_1_2
        );
        assert_eq!(min_tls_version("TLSv1.3"), reqwest::tls::Version::TLS_1_3);
    }

    #[test]
    fn parses_base_urls() {
        for url in [
            "https://api.datad0g.com",
            "https://api.datadoghq.com/",
            "http://127.0.0.1:8080",
            "http://fake-opms",
            "https://opms.internal:8443",
        ] {
            parse_base_url(url).unwrap_or_else(|error| panic!("{url}: {error}"));
        }

        for bad in [
            "api.datad0g.com",
            "ftp://host",
            "https://",
            "http://h:notaport",
            "https://api.datadoghq.com/path",
        ] {
            assert!(parse_base_url(bad).is_err(), "{bad} should not parse");
        }
    }

    #[test]
    fn url_authority_includes_only_explicit_ports() {
        assert_eq!(
            url_authority(&parse_base_url("https://api.datad0g.com").unwrap()),
            "api.datad0g.com"
        );
        assert_eq!(
            url_authority(&parse_base_url("http://127.0.0.1:8080").unwrap()),
            "127.0.0.1:8080"
        );
    }

    #[test]
    fn proxy_selection_preserves_agent_legacy_exact_no_proxy() {
        let base_url = parse_base_url("https://api.datadoghq.com").unwrap();
        let mut config = ProxyConfig {
            https: Some("http://proxy.example:3128".to_string()),
            no_proxy: vec!["api.datadoghq.com".to_string()],
            ..ProxyConfig::default()
        };
        assert!(proxy_for_base_url(&base_url, &config).unwrap().is_none());

        config.no_proxy = vec!["datadoghq.com".to_string()];
        assert!(proxy_for_base_url(&base_url, &config).unwrap().is_some());
        config.no_proxy_nonexact_match = true;
        // Reqwest receives the Agent's standard suffix/CIDR exclusion list.
        assert!(proxy_for_base_url(&base_url, &config).unwrap().is_some());
    }

    #[test]
    fn extra_headers_override_standard_headers() {
        let opms = HttpOpms::new(
            "http://localhost:8080".to_string(),
            Arc::new(crate::jwt::test_support::StaticSigner("jwt".into())),
            HttpOpmsConfig {
                runner_version: "7.83.0".into(),
                modes: vec!["pull".into()],
                timeout: Duration::from_secs(10),
                proxy: ProxyConfig::default(),
                tls: TlsConfig::default(),
                extra_headers: HashMap::from([
                    (
                        "X-Datadog-OnPrem-Version".to_string(),
                        "override".to_string(),
                    ),
                    ("X-Test-Routing".to_string(), "canary".to_string()),
                ]),
            },
        )
        .unwrap();
        let headers = opms.headers("jwt".to_string()).unwrap();
        assert_eq!(headers["X-Datadog-OnPrem-Version"], "override");
        assert_eq!(headers["X-Test-Routing"], "canary");
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
        assert_eq!(
            v["data"]["attributes"]["runner_started_at"],
            "2026-01-01T00:00:00Z"
        );
        assert!(
            v["data"]["attributes"]
                .get("last_task_received_at")
                .is_none()
        );
    }

    #[test]
    fn publish_success_body_matches_contract() {
        let body = publish_body(
            &sample_task(),
            &Outcome::Success {
                output_json: b"{\"k\":1}".to_vec(),
            },
        )
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
        let body = publish_body(
            &sample_task(),
            &Outcome::Failure {
                error_code: 5,
                message: "bad sig".into(),
                external_message: "nope".into(),
            },
        )
        .unwrap();
        let v: serde_json::Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(v["data"]["id"], "fail_task");
        assert_eq!(v["data"]["attributes"]["payload"]["error_code"], 5);
        assert_eq!(
            v["data"]["attributes"]["payload"]["error_details"],
            "bad sig"
        );
        assert_eq!(v["data"]["attributes"]["payload"]["api_error"], "nope");
    }

    /// Proves a TLS backend is actually compiled in and that the full client path
    /// (TCP -> native-tls handshake -> hyper HTTP/1.1 -> response) works, by
    /// talking to a local native-tls server.
    ///
    /// This exists because the crate shipped with *no* TLS provider compiled in:
    /// `ureq`'s native-tls code is gated on a feature we could not enable (it
    /// force-enables CDLA-licensed webpki roots), so every https:// request
    /// panicked inside a worker task, and the orchestrator dutifully logged
    /// "dequeue failed" and backed off forever. Nothing in the suite noticed,
    /// because every other test only checks JSON envelope shapes. Server-cert
    /// verification is necessarily relaxed here (the fixture CA is not in the
    /// system store); the point is the handshake and the request/response, not the
    /// trust decision.
    #[tokio::test]
    async fn round_trips_over_a_real_tls_connection() {
        let (cert_pem, key_pem) = crate::tls::test_support::generate_self_signed_cert();
        let identity = native_tls::Identity::from_pkcs8(
            &cert_pem,
            &crate::tls::test_support::to_pkcs8(&key_pem),
        )
        .expect("server identity");
        let acceptor = tokio_native_tls::TlsAcceptor::from(
            native_tls::TlsAcceptor::new(identity).expect("tls acceptor"),
        );

        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let port = listener.local_addr().unwrap().port();

        // Minimal HTTPS server: accept one connection, read the request head, and
        // answer with a retry-after header and a task envelope.
        let server = tokio::spawn(async move {
            let (tcp, _) = listener.accept().await.unwrap();
            let mut stream = acceptor.accept(tcp).await.expect("server handshake");
            use tokio::io::{AsyncReadExt, AsyncWriteExt};

            let mut request = Vec::new();
            let mut buf = [0u8; 1024];
            loop {
                let n = stream.read(&mut buf).await.unwrap();
                request.extend_from_slice(&buf[..n]);
                // The body length is known from the fixture, so stop once the head
                // plus a non-empty body have arrived.
                if n == 0
                    || (request.windows(4).any(|w| w == b"\r\n\r\n") && request.ends_with(b"}"))
                {
                    break;
                }
            }

            let body = br#"{"data":{"id":"task-tls","attributes":{"name":"do","bundle_id":"http","job_id":"job-tls"}}}"#;
            let head = format!(
                "HTTP/1.1 200 OK\r\nContent-Length: {}\r\n{}: 2500\r\n\r\n",
                body.len(),
                RETRY_AFTER_HEADER
            );
            stream.write_all(head.as_bytes()).await.unwrap();
            stream.write_all(body).await.unwrap();
            stream.flush().await.unwrap();
            String::from_utf8_lossy(&request).to_string()
        });

        let opms = HttpOpms::new_with_builder(
            format!("https://127.0.0.1:{port}"),
            Arc::new(crate::jwt::test_support::StaticSigner("jwt-abc".into())),
            HttpOpmsConfig {
                runner_version: "7.83.0".into(),
                modes: vec!["mode-a".into()],
                timeout: Duration::from_secs(10),
                proxy: ProxyConfig::default(),
                tls: TlsConfig::default(),
                extra_headers: HashMap::new(),
            },
            reqwest::Client::builder().add_root_certificate(
                reqwest::Certificate::from_pem(&cert_pem).expect("fixture root certificate"),
            ),
        )
        .expect("a TLS backend must be compiled in");

        let dequeued = opms.dequeue().await.expect("dequeue over TLS");
        let task = dequeued.task.expect("a task");
        assert_eq!(task.task_id, "task-tls");
        assert_eq!(task.action_fqn, "http.do");
        assert_eq!(dequeued.retry_after, Some(Duration::from_millis(2500)));

        // The request the server saw must carry origin-form target, Host, and the
        // OPMS auth/identity headers.
        let request = server.await.unwrap();
        assert!(
            request.starts_with(&format!("POST {DEQUEUE_PATH} HTTP/1.1")),
            "unexpected request line in: {request}"
        );
        assert!(
            request.contains(&format!("host: 127.0.0.1:{port}"))
                || request.contains(&format!("Host: 127.0.0.1:{port}")),
            "missing Host header in: {request}"
        );
        assert!(
            request.contains("jwt-abc"),
            "missing JWT header in: {request}"
        );
        assert!(
            request.contains("7.83.0"),
            "missing version header in: {request}"
        );
    }

    /// The health check is the only OPMS call an idle runner makes, so its wire
    /// shape is pinned here: a signed **GET** on the runner health-check path,
    /// with the server's pacing hint and server time surfaced to the caller. A
    /// rejection must still return the parsed headers (Go builds its
    /// `HealthCheckData` before returning the error), otherwise a 429 would be
    /// answered by hammering OPMS at the default interval.
    #[tokio::test]
    async fn health_check_is_a_signed_get_that_surfaces_server_pacing() {
        for (status_line, expected_status) in [("200 OK", 200_u16), ("429 Too Many Requests", 429)]
        {
            let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
            let port = listener.local_addr().unwrap().port();
            let server = tokio::spawn(async move {
                use tokio::io::{AsyncReadExt, AsyncWriteExt};
                let (mut stream, _) = listener.accept().await.unwrap();
                let mut request = Vec::new();
                let mut byte = [0_u8; 1];
                while !request.ends_with(b"\r\n\r\n") {
                    stream.read_exact(&mut byte).await.unwrap();
                    request.push(byte[0]);
                }
                let body = br#"{"errors":["slow down"]}"#;
                let head = format!(
                    "HTTP/1.1 {status_line}\r\nContent-Length: {}\r\n\
                     {SERVER_TIME_HEADER}: 2026-02-03T04:05:06Z\r\n\
                     {RETRY_AFTER_HEADER}: 45000\r\n\r\n",
                    body.len()
                );
                stream.write_all(head.as_bytes()).await.unwrap();
                stream.write_all(body).await.unwrap();
                stream.flush().await.unwrap();
                String::from_utf8_lossy(&request).to_string()
            });

            let opms = HttpOpms::new(
                format!("http://127.0.0.1:{port}"),
                Arc::new(crate::jwt::test_support::StaticSigner("jwt-health".into())),
                HttpOpmsConfig {
                    runner_version: "7.83.0".into(),
                    modes: vec!["pull".into()],
                    timeout: Duration::from_secs(10),
                    proxy: ProxyConfig::default(),
                    tls: TlsConfig::default(),
                    extra_headers: HashMap::new(),
                },
            )
            .unwrap();

            let check = opms.health_check().await.expect("health check");
            assert_eq!(check.status, expected_status);
            assert_eq!(check.ok(), expected_status == 200);
            assert_eq!(check.server_time.as_deref(), Some("2026-02-03T04:05:06Z"));
            assert_eq!(check.retry_after, Some(Duration::from_millis(45_000)));
            if expected_status == 200 {
                assert!(check.detail.is_empty());
            } else {
                assert!(check.detail.contains("slow down"), "{}", check.detail);
            }

            let request = server.await.unwrap();
            assert!(
                request.starts_with(&format!("GET {HEALTH_CHECK_PATH} HTTP/1.1")),
                "unexpected request line in: {request}"
            );
            assert!(
                request.contains("jwt-health"),
                "missing JWT header in: {request}"
            );
        }
    }

    /// A server-requested pacing hint must stay bounded, so a misconfigured or
    /// hostile OPMS cannot silence liveness reporting for hours.
    #[tokio::test]
    async fn health_check_retry_after_is_capped() {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let port = listener.local_addr().unwrap().port();
        let server = tokio::spawn(async move {
            use tokio::io::{AsyncReadExt, AsyncWriteExt};
            let (mut stream, _) = listener.accept().await.unwrap();
            let mut request = Vec::new();
            let mut byte = [0_u8; 1];
            while !request.ends_with(b"\r\n\r\n") {
                stream.read_exact(&mut byte).await.unwrap();
                request.push(byte[0]);
            }
            stream
                .write_all(
                    format!(
                        "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n{RETRY_AFTER_HEADER}: 86400000\r\n\r\n"
                    )
                    .as_bytes(),
                )
                .await
                .unwrap();
            stream.flush().await.unwrap();
        });

        let opms = HttpOpms::new(
            format!("http://127.0.0.1:{port}"),
            Arc::new(crate::jwt::test_support::StaticSigner("jwt".into())),
            HttpOpmsConfig {
                runner_version: "7.83.0".into(),
                modes: vec!["pull".into()],
                timeout: Duration::from_secs(10),
                proxy: ProxyConfig::default(),
                tls: TlsConfig::default(),
                extra_headers: HashMap::new(),
            },
        )
        .unwrap();
        let check = opms.health_check().await.unwrap();
        assert_eq!(check.retry_after, Some(MAX_RETRY_AFTER));
        server.await.unwrap();
    }

    /// Proves Agent HTTPS proxy configuration results in a CONNECT tunnel and
    /// that proxy credentials are kept on CONNECT rather than sent to OPMS.
    #[tokio::test]
    async fn routes_https_through_a_connect_proxy() {
        let (cert_pem, key_pem) = crate::tls::test_support::generate_self_signed_cert();
        let identity = native_tls::Identity::from_pkcs8(
            &cert_pem,
            &crate::tls::test_support::to_pkcs8(&key_pem),
        )
        .expect("server identity");
        let acceptor = tokio_native_tls::TlsAcceptor::from(
            native_tls::TlsAcceptor::new(identity).expect("tls acceptor"),
        );
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let proxy_port = listener.local_addr().unwrap().port();

        let server = tokio::spawn(async move {
            use tokio::io::{AsyncReadExt, AsyncWriteExt};

            let (mut tcp, _) = listener.accept().await.unwrap();
            let mut connect = Vec::new();
            let mut byte = [0_u8; 1];
            while !connect.ends_with(b"\r\n\r\n") {
                tcp.read_exact(&mut byte).await.unwrap();
                connect.push(byte[0]);
            }
            tcp.write_all(b"HTTP/1.1 200 Connection Established\r\n\r\n")
                .await
                .unwrap();

            // After CONNECT, reqwest establishes end-to-end TLS to the target.
            let mut stream = acceptor.accept(tcp).await.expect("target TLS handshake");
            let mut request = Vec::new();
            let mut buf = [0_u8; 1024];
            loop {
                let n = stream.read(&mut buf).await.unwrap();
                request.extend_from_slice(&buf[..n]);
                if n == 0
                    || (request.windows(4).any(|window| window == b"\r\n\r\n")
                        && request.ends_with(b"}"))
                {
                    break;
                }
            }
            stream
                .write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
                .await
                .unwrap();
            (
                String::from_utf8_lossy(&connect).to_string(),
                String::from_utf8_lossy(&request).to_string(),
            )
        });

        let proxy = ProxyConfig {
            https: Some(format!(
                "http://proxy-user:proxy-pass@127.0.0.1:{proxy_port}"
            )),
            ..ProxyConfig::default()
        };
        let opms = HttpOpms::new_with_builder(
            "https://localhost".to_string(),
            Arc::new(crate::jwt::test_support::StaticSigner(
                "jwt-through-proxy".into(),
            )),
            HttpOpmsConfig {
                runner_version: "7.83.0".into(),
                modes: vec!["pull".into()],
                timeout: Duration::from_secs(10),
                proxy,
                tls: TlsConfig::default(),
                extra_headers: HashMap::new(),
            },
            reqwest::Client::builder().add_root_certificate(
                reqwest::Certificate::from_pem(&cert_pem).expect("fixture root certificate"),
            ),
        )
        .unwrap();

        let dequeued = opms.dequeue().await.unwrap();
        assert!(dequeued.task.is_none());
        let (connect, request) = server.await.unwrap();
        assert!(connect.starts_with("CONNECT localhost:443 HTTP/1.1"));
        assert!(
            connect
                .to_ascii_lowercase()
                .contains("proxy-authorization: basic")
        );
        assert!(!request.to_ascii_lowercase().contains("proxy-authorization"));
        assert!(request.contains("jwt-through-proxy"));
    }

    /// A rejected OPMS request must carry the server's explanation, bounded, so a
    /// wire-envelope mismatch is diagnosable from the log alone.
    #[test]
    fn error_detail_is_quoted_and_bounded() {
        let resp = |body: Vec<u8>| HttpResponse {
            status: 400,
            retry_after: None,
            server_time: None,
            body,
        };

        assert_eq!(resp(vec![]).error_detail(), "<empty body>");
        assert_eq!(
            resp(br#"{"errors":["bad envelope"]}"#.to_vec()).error_detail(),
            r#"{"errors":["bad envelope"]}"#
        );

        let detail = resp(vec![b'x'; MAX_ERROR_BODY * 3]).error_detail();
        assert!(detail.starts_with(&"x".repeat(MAX_ERROR_BODY)));
        assert!(detail.ends_with(&format!("... [{} bytes total]", MAX_ERROR_BODY * 3)));
    }

    #[test]
    fn client_zero_is_omitted() {
        let mut task = sample_task();
        task.client = 0;
        let v: serde_json::Value = serde_json::from_slice(&heartbeat_body(&task)).unwrap();
        assert!(v["data"]["attributes"].get("client").is_none());
        assert_eq!(v["data"]["type"], "heartbeat");
        assert_eq!(v["data"]["id"], "t1");
    }
}
