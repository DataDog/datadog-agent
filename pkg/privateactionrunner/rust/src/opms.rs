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
use http_body_util::{BodyExt, Full};
// Re-export rather than a direct `bytes` dependency (which is not a workspace
// dependency); same approach as pkg/discovery/module/rust.
use hyper::Request;
use hyper::body::Bytes;
use hyper_util::rt::TokioIo;
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::net::TcpStream;

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

/// Where an OPMS base URL points: the pieces needed to open a connection.
#[derive(Debug, Clone, PartialEq, Eq)]
struct Endpoint {
    tls: bool,
    host: String,
    port: u16,
}

impl Endpoint {
    /// Parse an OPMS base URL (`https://api.datad0g.com`, `http://127.0.0.1:8080`).
    ///
    /// Hand-rolled rather than pulling in a URL crate: the input is always an
    /// agent-configured origin with no path, query, or userinfo.
    fn parse(base_url: &str) -> Result<Self> {
        let (tls, rest) = match base_url.split_once("://") {
            Some(("https", rest)) => (true, rest),
            Some(("http", rest)) => (false, rest),
            Some((scheme, _)) => bail!("unsupported OPMS URL scheme {scheme:?} in {base_url:?}"),
            None => bail!("OPMS URL {base_url:?} has no scheme"),
        };
        let authority = rest.trim_end_matches('/');
        if authority.is_empty() {
            bail!("OPMS URL {base_url:?} has no host");
        }
        // Split host:port from the right so IPv6 literals are not mangled.
        let (host, port) = match authority.rsplit_once(':') {
            Some((host, port)) if !host.is_empty() && !host.contains(']') => (
                host,
                port.parse::<u16>()
                    .with_context(|| format!("invalid port in OPMS URL {base_url:?}"))?,
            ),
            _ => (authority, if tls { 443 } else { 80 }),
        };
        Ok(Endpoint {
            tls,
            host: host.to_string(),
            port,
        })
    }

    /// The `Host` header value: the port is omitted when it is the scheme default.
    fn host_header(&self) -> String {
        let default_port = if self.tls { 443 } else { 80 };
        if self.port == default_port {
            self.host.clone()
        } else {
            format!("{}:{}", self.host, self.port)
        }
    }
}

/// Real OPMS client, speaking HTTP/1.1 over hyper with TLS from `native-tls`
/// (the agent's own OpenSSL, same stack as the control<->executor channel in
/// `tls.rs`). Server certificates are verified against the system trust store,
/// which `native-tls` locates at runtime via `openssl-probe`.
///
/// A plaintext `http://` base URL is honored for e2e against a fake OPMS.
pub struct HttpOpms {
    endpoint: Endpoint,
    /// Built once and reused: constructing a connector re-reads the trust store.
    /// `None` for a plaintext endpoint.
    tls: Option<tokio_native_tls::TlsConnector>,
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
    ) -> Result<Self> {
        let endpoint = Endpoint::parse(&base_url)?;
        let tls = if endpoint.tls {
            let connector = native_tls::TlsConnector::new()
                .context("building the OPMS TLS connector (no usable TLS backend?)")?;
            Some(tokio_native_tls::TlsConnector::from(connector))
        } else {
            None
        };
        Ok(HttpOpms {
            endpoint,
            tls,
            signer,
            timeout,
            runner_version,
            modes,
            runner_started_at: now_rfc3339(),
            last_task_received_at: Mutex::new(None),
        })
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

    /// POST `body` to `path` with the standard headers + a fresh JWT. Returns the
    /// raw response (status/retry-after/body) without treating a non-2xx status as
    /// an error, so callers can decide (matching Go).
    ///
    /// One connection per request: OPMS calls are at most a few per second and a
    /// pool would add reconnect/staleness handling for no measurable gain.
    async fn post(&self, path: &str, body: Vec<u8>) -> Result<HttpResponse> {
        let jwt = self
            .signer
            .sign()
            .context("failed to sign OPMS request JWT")?;

        let mut builder = Request::builder()
            .method(hyper::Method::POST)
            // HTTP/1.1 origin-form: the target is the path, and the authority
            // travels in the Host header.
            .uri(path)
            .header(hyper::header::HOST, self.endpoint.host_header());
        for (name, value) in self.headers(jwt) {
            builder = builder.header(name, value);
        }
        let request = builder
            .body(Full::new(Bytes::from(body)))
            .context("building the OPMS request")?;

        // One timeout over connect + TLS + request + response: a hung connect is
        // as harmful as a hung read, and the Go client bounds the whole call too.
        tokio::time::timeout(self.timeout, self.send(request))
            .await
            .with_context(|| format!("OPMS request to {path} timed out after {:?}", self.timeout))?
    }

    /// Connect (optionally wrapping in TLS) and exchange one request/response.
    async fn send(&self, request: Request<Full<Bytes>>) -> Result<HttpResponse> {
        let tcp = TcpStream::connect((self.endpoint.host.as_str(), self.endpoint.port))
            .await
            .with_context(|| {
                format!(
                    "connecting to OPMS at {}:{}",
                    self.endpoint.host, self.endpoint.port
                )
            })?;
        // Nagle off: these are small request/response pairs where latency matters
        // more than packet efficiency.
        let _ = tcp.set_nodelay(true);

        match &self.tls {
            Some(connector) => {
                let stream = connector
                    .connect(&self.endpoint.host, tcp)
                    .await
                    .with_context(|| {
                        format!("TLS handshake with OPMS at {}", self.endpoint.host)
                    })?;
                exchange(TokioIo::new(stream), request).await
            }
            None => exchange(TokioIo::new(tcp), request).await,
        }
    }
}

/// Drive one HTTP/1.1 request/response over an established stream.
///
/// Generic over the stream type so the TLS and plaintext paths share it without
/// boxing or an enum.
async fn exchange<I>(io: I, request: Request<Full<Bytes>>) -> Result<HttpResponse>
where
    I: hyper::rt::Read + hyper::rt::Write + Unpin + Send + 'static,
{
    let (mut sender, connection) = hyper::client::conn::http1::handshake(io)
        .await
        .context("HTTP handshake with OPMS failed")?;
    // The connection task pumps the socket; it ends when the response is complete
    // and the sender is dropped, so it needs no explicit shutdown.
    tokio::spawn(async move {
        if let Err(e) = connection.await {
            log::debug!("OPMS connection closed: {e}");
        }
    });

    let response = sender
        .send_request(request)
        .await
        .context("OPMS request failed")?;
    let status = response.status().as_u16();
    let retry_after = response
        .headers()
        .get(RETRY_AFTER_HEADER)
        .and_then(|v| v.to_str().ok())
        .and_then(|s| s.parse::<u64>().ok())
        .filter(|ms| *ms > 0)
        .map(|ms| Duration::from_millis(ms).min(MAX_RETRY_AFTER));
    let body = response
        .into_body()
        .collect()
        .await
        .context("failed to read OPMS response body")?
        .to_bytes()
        .to_vec();

    Ok(HttpResponse {
        status,
        retry_after,
        body,
    })
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
    fn parses_endpoints() {
        let cases = [
            ("https://api.datad0g.com", true, "api.datad0g.com", 443),
            ("https://api.datadoghq.com/", true, "api.datadoghq.com", 443),
            ("http://127.0.0.1:8080", false, "127.0.0.1", 8080),
            ("http://fake-opms", false, "fake-opms", 80),
            ("https://opms.internal:8443", true, "opms.internal", 8443),
        ];
        for (url, tls, host, port) in cases {
            let ep = Endpoint::parse(url).unwrap_or_else(|e| panic!("{url}: {e}"));
            assert_eq!(
                ep,
                Endpoint {
                    tls,
                    host: host.to_string(),
                    port
                },
                "{url}"
            );
        }

        for bad in [
            "api.datad0g.com",
            "ftp://host",
            "https://",
            "http://h:notaport",
        ] {
            assert!(Endpoint::parse(bad).is_err(), "{bad} should not parse");
        }
    }

    /// The `Host` header carries the port only when it is non-default, matching
    /// what every other HTTP client sends — OPMS routes on this header.
    #[test]
    fn host_header_omits_default_ports() {
        assert_eq!(
            Endpoint::parse("https://api.datad0g.com")
                .unwrap()
                .host_header(),
            "api.datad0g.com"
        );
        assert_eq!(
            Endpoint::parse("http://127.0.0.1:8080")
                .unwrap()
                .host_header(),
            "127.0.0.1:8080"
        );
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

        let mut opms = HttpOpms::new(
            format!("https://127.0.0.1:{port}"),
            Arc::new(crate::jwt::test_support::StaticSigner("jwt-abc".into())),
            "7.83.0".into(),
            vec!["mode-a".into()],
            Duration::from_secs(10),
        )
        .expect("a TLS backend must be compiled in");
        // The fixture CA is not in the system trust store.
        opms.tls = Some(tokio_native_tls::TlsConnector::from(
            native_tls::TlsConnector::builder()
                .danger_accept_invalid_certs(true)
                .build()
                .unwrap(),
        ));

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

    /// A rejected OPMS request must carry the server's explanation, bounded, so a
    /// wire-envelope mismatch is diagnosable from the log alone.
    #[test]
    fn error_detail_is_quoted_and_bounded() {
        let resp = |body: Vec<u8>| HttpResponse {
            status: 400,
            retry_after: None,
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
