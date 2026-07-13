// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Configuration for par-control: the OPMS endpoint, concurrency, socket paths,
//! and timing. Loaded from the agent `datadog.yaml` (only the fields the control
//! plane needs are deserialized) plus the persisted runner identity.

use crate::identity::Identity;
use anyhow::{Context, Result, bail};
use std::path::{Path, PathBuf};
use std::time::Duration;

/// Default local sockets. The executor socket must match the Go executor's
/// `executor.DefaultSocketPath`; the procmgr socket matches dd-procmgrd.
pub const DEFAULT_EXECUTOR_SOCKET: &str = "/opt/datadog-agent/run/par-executor.sock";
pub const DEFAULT_PROCMGR_SOCKET: &str = "/var/run/datadog-procmgrd/dd-procmgrd.sock";
/// Process-definition name the executor is registered under with the manager.
pub const DEFAULT_EXECUTOR_PROCESS_NAME: &str = "par-executor";

/// PAR flavor string sent to OPMS (matches `flavor.PrivateActionRunner` on the Go side).
pub const FLAVOR: &str = "private_action_runner";

/// Runner version reported to OPMS. Set at build time; falls back to the crate version.
pub const RUNNER_VERSION: &str = env!("CARGO_PKG_VERSION");

/// Fully-resolved control-plane configuration.
#[derive(Clone)]
pub struct Config {
    /// Base URL for OPMS, e.g. `https://api.datadoghq.com`.
    pub opms_base_url: String,
    /// Max concurrent in-flight actions (the runner pool size).
    pub runner_pool_size: usize,
    /// Local socket the executor listens on.
    pub executor_socket: PathBuf,
    /// Local socket the process manager listens on.
    pub procmgr_socket: PathBuf,
    /// Process-definition name of the executor in the process manager.
    pub executor_process_name: String,
    /// How often to poll OPMS when idle.
    pub loop_interval: Duration,
    /// OPMS heartbeat cadence while an action's result stream is open.
    pub heartbeat_interval: Duration,
    /// Idle period with no in-flight work after which the executor is stopped.
    pub idle_timeout: Duration,
    /// How long to wait for the executor to report ready after starting it.
    pub ready_timeout: Duration,
    /// Per-request timeout for OPMS calls.
    pub opms_request_timeout: Duration,
    /// Dequeue retry backoff (mirrors the Go circuit breaker).
    pub min_backoff: Duration,
    pub max_backoff: Duration,
    pub wait_before_retry: Duration,
    pub max_attempts: u32,
    /// Runner version reported to OPMS.
    pub runner_version: String,
    /// Runner modes reported to OPMS (comma-joined in the header).
    pub modes: Vec<String>,
    /// Agent IPC certificate file used for mTLS to the executor (if configured).
    pub ipc_cert_file: Option<PathBuf>,
    /// The resolved runner identity.
    pub identity: Identity,
}

/// Minimal view of `datadog.yaml` — only the fields the control plane reads.
/// Unknown keys are ignored so the full agent config deserializes cleanly.
#[derive(serde::Deserialize, Default, Clone)]
struct RawConfig {
    site: Option<String>,
    /// Optional full DD URL override (used by e2e against a plaintext fake OPMS).
    dd_url: Option<String>,
    /// Agent IPC certificate file path (top-level agent config key).
    ipc_cert_file_path: Option<String>,
    private_action_runner: Option<RawPar>,
}

#[derive(serde::Deserialize, Default, Clone)]
struct RawPar {
    urn: Option<String>,
    private_key: Option<String>,
    runner_pool_size: Option<usize>,
    executor_socket_path: Option<String>,
    procmgr_socket_path: Option<String>,
    executor_process_name: Option<String>,
    identity_file_path: Option<String>,
    idle_timeout_seconds: Option<u64>,
    heartbeat_interval_seconds: Option<u64>,
    #[serde(default)]
    modes: Vec<String>,
}

impl Config {
    /// Load configuration from the given `datadog.yaml` path, erroring if the
    /// runner has no identity yet (use [`Config::try_from_yaml_file`] to detect
    /// the not-yet-enrolled case for bootstrap).
    pub fn from_yaml_file(path: &std::path::Path) -> Result<Self> {
        Self::try_from_yaml_file(path)?
            .context("runner is not enrolled: no inline identity and no identity file found")
    }

    /// Like [`Config::from_yaml_file`] but returns `Ok(None)` when the config is
    /// otherwise valid yet no runner identity is present, so the caller can run
    /// the Go one-shot enroll and retry.
    pub fn try_from_yaml_file(path: &std::path::Path) -> Result<Option<Self>> {
        let contents = std::fs::read_to_string(path)
            .with_context(|| format!("failed to read config file: {}", path.display()))?;
        let raw: RawConfig = serde_yaml::from_str(&contents).context("failed to parse datadog.yaml")?;

        // Resolve identity from inline config keys, else the persisted identity
        // file (explicit path, else next to datadog.yaml) that Go enrollment writes.
        let identity = match inline_identity(raw.private_action_runner.as_ref())? {
            Some(id) => Some(id),
            None => Identity::from_file(&identity_file_path(&raw, path))?,
        };
        match identity {
            Some(identity) => Ok(Some(Self::build(raw, identity)?)),
            None => Ok(None),
        }
    }

    /// Parse configuration from YAML contents with inline identity (test helper).
    pub fn from_yaml_str(yaml: &str) -> Result<Self> {
        let raw: RawConfig = serde_yaml::from_str(yaml).context("failed to parse datadog.yaml")?;
        let identity = inline_identity(raw.private_action_runner.as_ref())?
            .context("private_action_runner identity is not set")?;
        Self::build(raw, identity)
    }

    fn build(raw: RawConfig, identity: Identity) -> Result<Self> {
        let par = raw.private_action_runner.clone().unwrap_or_default();
        let opms_base_url = resolve_opms_base_url(raw.site.as_deref(), raw.dd_url.as_deref())?;

        Ok(Config {
            opms_base_url,
            runner_pool_size: par.runner_pool_size.filter(|n| *n > 0).unwrap_or(10),
            executor_socket: PathBuf::from(
                par.executor_socket_path
                    .unwrap_or_else(|| DEFAULT_EXECUTOR_SOCKET.to_string()),
            ),
            procmgr_socket: PathBuf::from(
                par.procmgr_socket_path
                    .unwrap_or_else(|| DEFAULT_PROCMGR_SOCKET.to_string()),
            ),
            executor_process_name: par
                .executor_process_name
                .unwrap_or_else(|| DEFAULT_EXECUTOR_PROCESS_NAME.to_string()),
            loop_interval: Duration::from_secs(1),
            heartbeat_interval: Duration::from_secs(par.heartbeat_interval_seconds.unwrap_or(20)),
            idle_timeout: Duration::from_secs(par.idle_timeout_seconds.unwrap_or(60)),
            ready_timeout: Duration::from_secs(10),
            opms_request_timeout: Duration::from_secs(30),
            // Backoff defaults mirror pkg/privateactionrunner/adapters/config/constants.go.
            min_backoff: Duration::from_secs(1),
            max_backoff: Duration::from_secs(180),
            wait_before_retry: Duration::from_secs(300),
            max_attempts: 20,
            runner_version: RUNNER_VERSION.to_string(),
            modes: par.modes,
            ipc_cert_file: raw
                .ipc_cert_file_path
                .filter(|s| !s.is_empty())
                .map(PathBuf::from),
            identity,
        })
    }
}

/// Resolve the OPMS base URL. Production uses `https://api.<site>`. A `dd_url`
/// starting with `http://` is honored verbatim so e2e tests can point at a
/// plaintext fake OPMS (mirrors the Go client's endpointURL behavior).
fn resolve_opms_base_url(site: Option<&str>, dd_url: Option<&str>) -> Result<String> {
    if let Some(url) = dd_url {
        if let Some(host) = url.strip_prefix("http://") {
            return Ok(format!("http://{}", host.trim_end_matches('/')));
        }
    }
    let site = site.unwrap_or("datadoghq.com");
    if site.is_empty() {
        bail!("site is empty and no dd_url override provided");
    }
    Ok(format!("https://api.{site}"))
}

/// Identity from the inline `datadog.yaml` keys, if both are present.
fn inline_identity(par: Option<&RawPar>) -> Result<Option<Identity>> {
    let Some(par) = par else {
        return Ok(None);
    };
    let urn = par.urn.as_deref().filter(|s| !s.is_empty());
    let key = par.private_key.as_deref().filter(|s| !s.is_empty());
    match (urn, key) {
        (Some(urn), Some(key)) => Ok(Some(Identity::new(urn.to_string(), key.to_string())?)),
        _ => Ok(None),
    }
}

/// Path to the persisted identity file: the explicit `identity_file_path` if set,
/// else `<config dir>/privateactionrunner_private_identity.json` (matching Go's
/// default next to datadog.yaml).
fn identity_file_path(raw: &RawConfig, config_path: &Path) -> PathBuf {
    if let Some(explicit) = raw
        .private_action_runner
        .as_ref()
        .and_then(|p| p.identity_file_path.as_deref())
        .filter(|s| !s.is_empty())
    {
        return PathBuf::from(explicit);
    }
    let dir = config_path.parent().unwrap_or_else(|| Path::new("."));
    dir.join(crate::identity::DEFAULT_IDENTITY_FILE_NAME)
}

#[cfg(test)]
mod tests {
    use super::*;

    const MIN_YAML: &str = r#"
site: datadoghq.com
private_action_runner:
  urn: "urn:dd:apps:on-prem-runner:us1:42:runner-1"
  private_key: "-----BEGIN EC PRIVATE KEY-----\nabc\n-----END EC PRIVATE KEY-----"
"#;

    #[test]
    fn loads_minimal_config() {
        let cfg = Config::from_yaml_str(MIN_YAML).unwrap();
        assert_eq!(cfg.opms_base_url, "https://api.datadoghq.com");
        assert_eq!(cfg.runner_pool_size, 10);
        assert_eq!(cfg.identity.org_id, 42);
        assert_eq!(cfg.identity.runner_id, "runner-1");
        assert_eq!(cfg.executor_process_name, "par-executor");
    }

    #[test]
    fn honors_http_dd_url_override() {
        let yaml = format!("dd_url: \"http://fake-opms:8080\"\n{MIN_YAML}");
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(cfg.opms_base_url, "http://fake-opms:8080");
    }

    #[test]
    fn fails_without_identity() {
        assert!(Config::from_yaml_str("site: datadoghq.com\n").is_err());
    }

    #[test]
    fn resolves_identity_from_sibling_file_when_not_inline() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(&cfg_path, "site: datadoghq.com\nprivate_action_runner:\n  enabled: true\n").unwrap();
        std::fs::write(
            dir.path().join(crate::identity::DEFAULT_IDENTITY_FILE_NAME),
            r#"{"private_key":"enc","urn":"urn:dd:apps:on-prem-runner:us1:7:r7"}"#,
        )
        .unwrap();

        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.identity.org_id, 7);
        assert_eq!(cfg.identity.runner_id, "r7");
    }

    #[test]
    fn try_load_returns_none_when_no_identity_anywhere() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(&cfg_path, "site: datadoghq.com\n").unwrap();
        assert!(Config::try_from_yaml_file(&cfg_path).unwrap().is_none());
    }

    #[test]
    fn overrides_pool_size_and_intervals() {
        let yaml = format!(
            "{MIN_YAML}  runner_pool_size: 3\n  idle_timeout_seconds: 120\n  heartbeat_interval_seconds: 5\n"
        );
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(cfg.runner_pool_size, 3);
        assert_eq!(cfg.idle_timeout, Duration::from_secs(120));
        assert_eq!(cfg.heartbeat_interval, Duration::from_secs(5));
    }
}
