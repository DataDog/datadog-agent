// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::executor::Outcome;
use crate::jwt::{JWT_HEADER_NAME, JwtSigner};
use anyhow::{Context, Result, bail};
use reqwest::header::{HeaderMap, HeaderName, HeaderValue};
use saluki_tls::{ClientTLSConfigBuilder, TlsMinimumVersion};
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::Duration;

const FLAVOR: &str = "private_action_runner";
const DEQUEUE_PATH: &str = "/api/v2/on-prem-management-service/workflow-tasks/dequeue";
const TASK_UPDATE_PATH: &str =
    "/api/v2/on-prem-management-service/workflow-tasks/publish-task-update";
const HEARTBEAT_PATH: &str = "/api/v2/on-prem-management-service/workflow-tasks/heartbeat";
const HEALTH_CHECK_PATH: &str = "/api/v2/on-prem-management-service/runner/health-check";

const RETRY_AFTER_HEADER: &str = "X-Retry-After-Ms";
const SERVER_TIME_HEADER: &str = "X-Server-Time";
const MAX_RETRY_AFTER: Duration = Duration::from_secs(120);

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct TlsConfig {
    pub skip_ssl_validation: bool,
    pub min_tls_version: String,
}

/// A dequeued task.
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

#[derive(Debug, Clone, Default)]
pub struct Dequeued {
    pub task: Option<Task>,
    pub retry_after: Option<Duration>,
}

#[derive(Debug)]
pub struct DequeueError {
    error: anyhow::Error,
    pub retry_after: Option<Duration>,
}

impl DequeueError {
    pub(crate) fn new(error: anyhow::Error, retry_after: Option<Duration>) -> Self {
        Self { error, retry_after }
    }
}

impl std::fmt::Display for DequeueError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        self.error.fmt(f)
    }
}

impl std::error::Error for DequeueError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        Some(self.error.as_ref())
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PublishResult {
    Published,
    Rejected { status: u16, detail: String },
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HeartbeatResult {
    Alive,
    NotFound,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct HealthCheck {
    pub status: u16,
    pub server_time: Option<String>,
    pub retry_after: Option<Duration>,
    pub detail: String,
}

impl HealthCheck {
    pub fn ok(&self) -> bool {
        self.status == 200
    }
}

/// The OPMS operations the control plane performs.
pub trait Opms: Send + Sync {
    fn dequeue(
        &self,
    ) -> impl std::future::Future<Output = std::result::Result<Dequeued, DequeueError>> + Send;

    fn publish(
        &self,
        task: &Task,
        outcome: &Outcome,
    ) -> impl std::future::Future<Output = Result<PublishResult>> + Send;

    fn heartbeat(
        &self,
        task: &Task,
    ) -> impl std::future::Future<Output = Result<HeartbeatResult>> + Send;

    fn health_check(&self) -> impl std::future::Future<Output = Result<HealthCheck>> + Send;
}

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

fn is_containerized() -> bool {
    std::env::var_os("DOCKER_DD_AGENT").is_some()
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

impl HttpResponse {
    fn error_detail(&self) -> String {
        String::from_utf8_lossy(&self.body).into_owned()
    }
}

const POOL_IDLE_TIMEOUT: Duration = Duration::from_secs(45);
const MAX_IDLE_CONNECTIONS_PER_HOST: usize = 5;

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
    pub proxy_url: Option<String>,
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
        Self::new_with_builder_and_root_store(base_url, signer, options, builder, None)
    }

    fn new_with_builder_and_root_store(
        base_url: String,
        signer: Arc<dyn JwtSigner>,
        options: HttpOpmsConfig,
        builder: reqwest::ClientBuilder,
        root_cert_store: Option<rustls::RootCertStore>,
    ) -> Result<Self> {
        crate::tls::initialize_crypto_provider()?;
        let base_url = parse_base_url(&base_url)?;
        let mut tls_builder = ClientTLSConfigBuilder::new()
            .with_min_tls_version(min_tls_version(&options.tls.min_tls_version));
        if options.tls.skip_ssl_validation {
            tls_builder = tls_builder.danger_accept_invalid_certs();
        } else {
            let root_cert_store = root_cert_store
                .map(Ok)
                .unwrap_or_else(saluki_tls::load_platform_root_certificates_inner)
                .context("loading native roots for OPMS HTTPS")?;
            tls_builder = tls_builder.with_root_cert_store(root_cert_store);
        }
        let tls_config = tls_builder
            .build()
            .context("building the OPMS TLS configuration")?;
        let mut builder = builder
            .timeout(options.timeout)
            .pool_idle_timeout(POOL_IDLE_TIMEOUT)
            .pool_max_idle_per_host(MAX_IDLE_CONNECTIONS_PER_HOST)
            .http1_only()
            .use_preconfigured_tls(tls_config)
            // We already merged the Agent's YAML and environment proxy settings;
            // do not let reqwest independently re-read process environment.
            .no_proxy();
        if let Some(proxy_url) = options.proxy_url {
            builder = builder.proxy(
                reqwest::Proxy::all(proxy_url)
                    .map_err(|_| anyhow::anyhow!("invalid Agent proxy URL"))?,
            );
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

    async fn post(&self, path: &str, body: Vec<u8>) -> Result<HttpResponse> {
        self.request(reqwest::Method::POST, path, Some(body)).await
    }

    async fn get(&self, path: &str) -> Result<HttpResponse> {
        self.request(reqwest::Method::GET, path, None).await
    }

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

fn min_tls_version(raw: &str) -> TlsMinimumVersion {
    match raw.trim().to_ascii_lowercase().as_str() {
        "tlsv1.3" => TlsMinimumVersion::Tls13,
        // Rustls supports TLS 1.2 and 1.3. Accept legacy Agent values while
        // clamping them to Saluki's TLS 1.2 minimum.
        _ => TlsMinimumVersion::Tls12,
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

fn parse_dequeue_response(resp: HttpResponse) -> std::result::Result<Dequeued, DequeueError> {
    if resp.status != 200 {
        return Err(DequeueError::new(
            anyhow::anyhow!(
                "dequeue failed with status {}: {}",
                resp.status,
                resp.error_detail()
            ),
            resp.retry_after,
        ));
    }
    if resp.body.is_empty() {
        return Ok(Dequeued {
            task: None,
            retry_after: resp.retry_after,
        });
    }
    let retry_after = resp.retry_after;
    let task = Task::from_raw(resp.body).map_err(|error| DequeueError::new(error, retry_after))?;
    Ok(Dequeued {
        task: Some(task),
        retry_after,
    })
}

impl Opms for HttpOpms {
    async fn dequeue(&self) -> std::result::Result<Dequeued, DequeueError> {
        let last = self.last_task_received_at.lock().unwrap().clone();
        let body = dequeue_body(&self.runner_started_at, last.as_deref());
        let resp = self
            .post(DEQUEUE_PATH, body)
            .await
            .map_err(|error| DequeueError::new(error, None))?;
        let dequeued = parse_dequeue_response(resp)?;
        if dequeued.task.is_some() {
            *self.last_task_received_at.lock().unwrap() = Some(now_rfc3339());
        }
        Ok(dequeued)
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
        for value in ["", "not-a-version", "tlsv1.0", "tlsv1.1", "tlsv1.2"] {
            assert_eq!(min_tls_version(value), TlsMinimumVersion::Tls12);
        }
        assert_eq!(min_tls_version("TLSv1.3"), TlsMinimumVersion::Tls13);
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
    fn extra_headers_override_standard_headers() {
        let opms = HttpOpms::new(
            "http://localhost:8080".to_string(),
            Arc::new(crate::jwt::test_support::StaticSigner("jwt".into())),
            HttpOpmsConfig {
                runner_version: "7.83.0".into(),
                modes: vec!["pull".into()],
                timeout: Duration::from_secs(10),
                proxy_url: None,
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

    /// Health checks must preserve retry-after and server-time headers on non-200 responses.
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
                     {RETRY_AFTER_HEADER}: 86400000\r\n\r\n",
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
                    proxy_url: None,
                    tls: TlsConfig::default(),
                    extra_headers: HashMap::new(),
                },
            )
            .unwrap();

            let check = opms.health_check().await.expect("health check");
            assert_eq!(check.status, expected_status);
            assert_eq!(check.ok(), expected_status == 200);
            assert_eq!(check.server_time.as_deref(), Some("2026-02-03T04:05:06Z"));
            assert_eq!(check.retry_after, Some(MAX_RETRY_AFTER));
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

    #[test]
    fn dequeue_error_surfaces_server_pacing() {
        let error = parse_dequeue_response(HttpResponse {
            status: 429,
            retry_after: Some(Duration::from_secs(5)),
            server_time: None,
            body: br#"{"errors":["slow down"]}"#.to_vec(),
        })
        .unwrap_err();

        assert_eq!(error.retry_after, Some(Duration::from_secs(5)));
        assert!(error.to_string().contains("status 429"));
    }

    #[tokio::test]
    async fn uses_configured_https_proxy() {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let proxy_port = listener.local_addr().unwrap().port();
        let server = tokio::spawn(async move {
            use tokio::io::{AsyncReadExt, AsyncWriteExt};

            let (mut stream, _) = listener.accept().await.unwrap();
            let mut connect = Vec::new();
            let mut byte = [0_u8; 1];
            while !connect.ends_with(b"\r\n\r\n") {
                stream.read_exact(&mut byte).await.unwrap();
                connect.push(byte[0]);
            }
            stream
                .write_all(b"HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
                .await
                .unwrap();
            String::from_utf8_lossy(&connect).to_string()
        });

        let opms = HttpOpms::new_with_builder_and_root_store(
            "https://localhost".to_string(),
            Arc::new(crate::jwt::test_support::StaticSigner("jwt".into())),
            HttpOpmsConfig {
                runner_version: "7.83.0".into(),
                modes: vec!["pull".into()],
                timeout: Duration::from_secs(10),
                proxy_url: Some(format!(
                    "http://proxy-user:proxy-pass@127.0.0.1:{proxy_port}"
                )),
                tls: TlsConfig::default(),
                extra_headers: HashMap::new(),
            },
            reqwest::Client::builder(),
            Some(rustls::RootCertStore::empty()),
        )
        .unwrap();

        assert!(opms.dequeue().await.is_err());
        let connect = server.await.unwrap();
        assert!(connect.starts_with("CONNECT localhost:443 HTTP/1.1"));
        assert!(
            connect
                .to_ascii_lowercase()
                .contains("proxy-authorization: basic")
        );
    }

    #[test]
    fn heartbeat_omits_unspecified_client() {
        let mut task = sample_task();
        task.client = 0;
        let v: serde_json::Value = serde_json::from_slice(&heartbeat_body(&task)).unwrap();
        assert!(v["data"]["attributes"].get("client").is_none());
        assert_eq!(v["data"]["type"], "heartbeat");
        assert_eq!(v["data"]["id"], "t1");
    }
}
