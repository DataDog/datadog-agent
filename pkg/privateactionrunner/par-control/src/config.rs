// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod env;

use self::env::ParControlEnvProvider;
use crate::identity::Identity;
use anyhow::{Context, Result, bail};
use saluki_config::ConfigurationLoader;
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::time::Duration;

const SKIP_TASK_VERIFICATION_ENV: &str = "DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION";
const HEALTH_CHECK_INTERVAL: Duration = Duration::from_secs(30);
const EXECUTOR_READY_TIMEOUT: Duration = Duration::from_secs(30);
const DEFAULT_MODE: &str = "pull";

pub const EXECUTOR_PROCESS_NAME: &str = "datadog-agent-action-executor";
pub const FLAVOR: &str = "private_action_runner";

/// Per-task heartbeat rate while a result stream is open.
const HEARTBEAT_INTERVAL: Duration = Duration::from_secs(20);

#[cfg(not(windows))]
pub const DEFAULT_EXECUTOR_SOCKET: &str = "/opt/datadog-agent/run/par-executor.sock";
#[cfg(windows)]
pub const DEFAULT_EXECUTOR_SOCKET: &str = r"\\.\pipe\dd-par-executor";

/// Agent version reported to OPMS in the `X-Datadog-OnPrem-Version` header.
pub const RUNNER_VERSION: &str = match option_env!("DD_AGENT_VERSION") {
    Some(v) => v,
    None => env!("CARGO_PKG_VERSION"),
};

#[derive(Clone)]
pub struct Config {
    pub opms_base_url: String,
    pub task_concurrency: usize,
    pub executor_socket: PathBuf,
    pub procmgr_socket: PathBuf,
    pub executor_process_name: String,
    pub loop_interval: Duration,
    pub heartbeat_interval: Duration,
    pub health_check_interval: Duration,
    pub ready_timeout: Duration,
    pub opms_request_timeout: Duration,
    pub opms_extra_headers: HashMap<String, String>,
    pub proxy: ProxyConfig,
    pub tls: TlsConfig,
    /// Mirrors the Go circuit breaker.
    pub min_backoff: Duration,
    pub max_backoff: Duration,
    pub wait_before_retry: Duration,
    pub max_attempts: u32,
    pub runner_version: String,
    pub modes: Vec<String>,
    pub ipc_cert_file: PathBuf,
    pub identity: Identity,
}

/// Proxy settings used for the OPMS request scheme.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct ProxyConfig {
    pub http: Option<String>,
    pub https: Option<String>,
    pub no_proxy: Vec<String>,
    pub no_proxy_nonexact_match: bool,
}

/// TLS settings used for OPMS connections.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct TlsConfig {
    pub skip_ssl_validation: bool,
    pub min_tls_version: String,
}

/// Minimal view of `datadog.yaml` — only the fields the control plane reads.
#[derive(serde::Deserialize, Default, Clone)]
struct RawConfig {
    site: Option<String>,
    log_level: Option<String>,
    dd_url: Option<String>,
    ipc_cert_file_path: Option<String>,
    auth_token_file_path: Option<String>,
    proxy: Option<RawProxy>,
    no_proxy_nonexact_match: Option<bool>,
    skip_ssl_validation: Option<bool>,
    min_tls_version: Option<String>,
    private_action_runner: Option<RawPar>,
}

#[derive(serde::Deserialize, Default, Clone)]
struct RawProxy {
    http: Option<String>,
    https: Option<String>,
    no_proxy: Option<Vec<String>>,
}

#[derive(serde::Deserialize, Default, Clone)]
struct RawPar {
    enabled: Option<bool>,
    split_enabled: Option<bool>,
    self_enroll: Option<bool>,
    urn: Option<String>,
    private_key: Option<String>,
    task_concurrency: Option<usize>,
    executor: Option<RawParExecutor>,
    identity_file_path: Option<String>,
    opms_extra_headers: Option<HashMap<String, String>>,
}

#[derive(serde::Deserialize, Default, Clone)]
struct RawParExecutor {
    socket_path: Option<String>,
}

impl Config {
    pub fn from_yaml_file(path: &std::path::Path) -> Result<Self> {
        Self::try_from_yaml_file(path)?
            .context("runner is not enrolled: no inline identity and no identity file found")
    }

    /// Like [`Config::from_yaml_file`] but returns `Ok(None)` when the config is
    /// otherwise valid yet no runner identity is present.
    pub fn try_from_yaml_file(path: &std::path::Path) -> Result<Option<Self>> {
        Self::try_from_yaml_file_with_env(path, |name| std::env::var(name).ok())
    }

    fn try_from_yaml_file_with_env(
        path: &Path,
        env: impl Fn(&str) -> Option<String>,
    ) -> Result<Option<Self>> {
        let raw = resolve_raw(path, &env)?;

        let identity = match Identity::from_file(&identity_file_path(&raw, path))? {
            Some(id) => Some(id),
            None => inline_identity(raw.private_action_runner.as_ref())?,
        };
        match identity {
            Some(identity) => Ok(Some(Self::build(
                raw,
                identity,
                path,
                env(SKIP_TASK_VERIFICATION_ENV).as_deref() == Some("true"),
            )?)),
            None => Ok(None),
        }
    }

    /// Parse configuration from YAML contents with inline identity (test helper).
    #[cfg(test)]
    pub fn from_yaml_str(yaml: &str) -> Result<Self> {
        let raw = resolve_raw_str(yaml, &|_| None)?;
        let identity = inline_identity(raw.private_action_runner.as_ref())?
            .context("private_action_runner identity is not set")?;
        Self::build(raw, identity, Path::new("datadog.yaml"), false)
    }

    fn build(
        raw: RawConfig,
        identity: Identity,
        config_path: &Path,
        allow_insecure_opms: bool,
    ) -> Result<Self> {
        let par = raw.private_action_runner.clone().unwrap_or_default();
        let opms_base_url = resolve_opms_base_url(
            raw.site.as_deref(),
            raw.dd_url.as_deref(),
            allow_insecure_opms,
        )?;
        let task_concurrency = par.task_concurrency.unwrap_or(5);
        if task_concurrency == 0 {
            bail!("private_action_runner.task_concurrency must be greater than zero");
        }
        let proxy = resolved_proxy(&raw);
        let ipc_cert_file = ipc_cert_file_path(&raw, config_path);

        Ok(Config {
            opms_base_url,
            task_concurrency,
            executor_socket: PathBuf::from(
                par.executor
                    .and_then(|e| e.socket_path)
                    .filter(|s| !s.is_empty())
                    .unwrap_or_else(|| DEFAULT_EXECUTOR_SOCKET.to_string()),
            ),
            procmgr_socket: dd_procmgr_client::default_ipc_path(),
            executor_process_name: EXECUTOR_PROCESS_NAME.to_string(),
            loop_interval: Duration::from_secs(1),
            heartbeat_interval: HEARTBEAT_INTERVAL,
            health_check_interval: HEALTH_CHECK_INTERVAL,
            ready_timeout: EXECUTOR_READY_TIMEOUT,
            opms_request_timeout: Duration::from_secs(30),
            opms_extra_headers: par.opms_extra_headers.unwrap_or_default(),
            proxy,
            tls: TlsConfig {
                skip_ssl_validation: raw.skip_ssl_validation.unwrap_or(false),
                min_tls_version: raw.min_tls_version.unwrap_or_else(|| "tlsv1.2".to_string()),
            },
            // Backoff defaults mirror pkg/privateactionrunner/adapters/config/constants.go.
            min_backoff: Duration::from_secs(1),
            max_backoff: Duration::from_secs(180),
            wait_before_retry: Duration::from_secs(300),
            max_attempts: 20,
            runner_version: RUNNER_VERSION.to_string(),
            modes: vec![DEFAULT_MODE.to_string()],
            ipc_cert_file,
            identity,
        })
    }
}

/// Settings needed before identity bootstrap.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct LaunchGate {
    pub split_mode: bool,
    pub self_enroll: bool,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Launch {
    pub log_level: log::LevelFilter,
    pub gate: LaunchGate,
}

fn parse_log_level(raw: &str) -> log::LevelFilter {
    match raw.trim().to_ascii_lowercase().as_str() {
        "trace" => log::LevelFilter::Trace,
        "debug" => log::LevelFilter::Debug,
        "warn" | "warning" => log::LevelFilter::Warn,
        "error" | "critical" => log::LevelFilter::Error,
        "off" => log::LevelFilter::Off,
        _ => log::LevelFilter::Info,
    }
}

impl Launch {
    pub fn from_yaml_file(path: &Path) -> Result<Self> {
        Self::from_yaml_file_with_env(path, |name| std::env::var(name).ok())
    }

    #[cfg(test)]
    fn from_yaml_str(yaml: &str) -> Result<Self> {
        Self::from_yaml_str_with_env(yaml, |_| None)
    }

    #[cfg(test)]
    fn from_yaml_str_with_env(yaml: &str, env: impl Fn(&str) -> Option<String>) -> Result<Self> {
        let raw = resolve_raw_str(yaml, &env)?;
        Self::from_raw(raw)
    }

    fn from_yaml_file_with_env(path: &Path, env: impl Fn(&str) -> Option<String>) -> Result<Self> {
        let raw = resolve_raw(path, &env)?;
        Self::from_raw(raw)
    }

    fn from_raw(raw: RawConfig) -> Result<Self> {
        let log_level = raw
            .log_level
            .as_deref()
            .map(parse_log_level)
            .unwrap_or(log::LevelFilter::Info);
        let par = raw.private_action_runner.unwrap_or_default();
        Ok(Self {
            log_level,
            gate: LaunchGate {
                split_mode: par.enabled.unwrap_or(false) && par.split_enabled.unwrap_or(false),
                self_enroll: par.self_enroll.unwrap_or(true),
            },
        })
    }
}

/// Resolve precedence through Saluki: Fleet > environment > local YAML.
fn resolve_raw(path: &Path, env: &impl Fn(&str) -> Option<String>) -> Result<RawConfig> {
    let mut loader = ConfigurationLoader::default()
        .from_yaml(path)
        .with_context(|| format!("failed to load config file: {}", path.display()))?
        .add_providers([ParControlEnvProvider::new(env)?]);

    let bootstrap = loader.bootstrap_generic();
    let fleet_dir = bootstrap
        .try_get_typed::<String>("fleet_policies_dir")?
        .filter(|value| !value.is_empty())
        .or_else(crate::platform::fleet_policies_dir);
    if let Some(dir) = fleet_dir {
        loader = loader.try_from_yaml(Path::new(&dir).join("datadog.yaml"));
    }

    loader
        .into_typed()
        .context("failed to extract par-control configuration")
}

#[cfg(test)]
fn resolve_raw_str(yaml: &str, env: &impl Fn(&str) -> Option<String>) -> Result<RawConfig> {
    use std::io::Write;

    let mut file = tempfile::NamedTempFile::new().context("failed to create test config")?;
    file.write_all(yaml.as_bytes())?;
    resolve_raw(file.path(), env)
}

fn resolved_proxy(raw: &RawConfig) -> ProxyConfig {
    let proxy = raw.proxy.clone().unwrap_or_default();
    ProxyConfig {
        http: proxy.http.filter(|value| !value.is_empty()),
        https: proxy.https.filter(|value| !value.is_empty()),
        no_proxy: proxy.no_proxy.unwrap_or_default(),
        no_proxy_nonexact_match: raw.no_proxy_nonexact_match.unwrap_or(false),
    }
}

/// Resolve the production OPMS origin.
///
/// This intentionally does not reproduce the Agent config loader's FIPS endpoint,
/// proxy, or TLS transforms: Private Action Runner is not supported in FIPS
/// environments. FIPS support needs a separately designed and validated PAR
/// configuration path rather than a partial copy of those transforms here.
fn resolve_opms_base_url(
    site: Option<&str>,
    dd_url: Option<&str>,
    allow_insecure_opms: bool,
) -> Result<String> {
    if let Some(raw_url) = dd_url.filter(|url| !url.is_empty()) {
        let url = reqwest::Url::parse(raw_url).context("invalid dd_url")?;
        if url.scheme() == "http" {
            if !allow_insecure_opms {
                bail!("plaintext OPMS requires {SKIP_TASK_VERIFICATION_ENV}=true");
            }
            let host = url.host_str().context("dd_url has no host")?;
            let authority = match url.port() {
                Some(port) => format!("{host}:{port}"),
                None => host.to_string(),
            };
            return Ok(format!("http://{authority}"));
        }
        if url.scheme() != "https" {
            bail!("unsupported dd_url scheme {:?}", url.scheme());
        }
        let host = url.host_str().context("dd_url has no host")?;
        let site = extract_datadog_site(host).with_context(|| {
            format!("cannot derive a Datadog site from HTTPS dd_url {raw_url:?}")
        })?;
        return Ok(format!("https://api.{site}"));
    }

    let site = site.unwrap_or("datadoghq.com").trim();
    if site.is_empty() {
        bail!("site is empty and no dd_url override provided");
    }
    Ok(format!("https://api.{site}"))
}

fn extract_datadog_site(host: &str) -> Option<String> {
    let host = host.trim_end_matches('.').to_ascii_lowercase();
    let labels: Vec<&str> = host.split('.').collect();
    if labels.len() < 2 {
        return None;
    }
    let domain_start = labels.len() - 2;
    let recognized_domain = matches!(
        &labels[domain_start..],
        ["datadoghq" | "datad0g", "com" | "eu"] | ["ddog-gov", "com"]
    );
    if !recognized_domain {
        return None;
    }

    let start = domain_start.checked_sub(1).filter(|index| {
        let dc = labels[*index];
        let letters = dc.bytes().take_while(u8::is_ascii_lowercase).count();
        letters >= 2
            && dc[letters..].len() <= 2
            && !dc[letters..].is_empty()
            && dc[letters..].bytes().all(|byte| byte.is_ascii_digit())
    });
    Some(labels[start.unwrap_or(domain_start)..].join("."))
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
        (None, None) => Ok(None),
        (Some(_), None) => bail!("private_action_runner.private_key is required when urn is set"),
        (None, Some(_)) => bail!("private_action_runner.urn is required when private_key is set"),
    }
}

/// Default name of the agent IPC cert file, matching `defaultCertFileName` in
/// `pkg/api/security/cert/cert_getter.go`.
const DEFAULT_IPC_CERT_FILE_NAME: &str = "ipc_cert.pem";

/// Resolve an agent-managed file path the way Go's `getCertFilepath` does, since
/// the explicit keys default to empty and so are unset on virtually every host:
///
/// 1. the explicit key,
/// 2. else next to `auth_token_file_path` (operators who move the auth token off
///    the config directory get these files moved with it),
/// 3. else next to `datadog.yaml`.
fn resolve_agent_file(
    explicit: Option<&str>,
    raw: &RawConfig,
    config_path: &Path,
    file_name: &str,
) -> PathBuf {
    if let Some(explicit) = explicit.filter(|value| !value.is_empty()) {
        return PathBuf::from(explicit);
    }
    raw.auth_token_file_path
        .as_deref()
        .filter(|value| !value.is_empty())
        .map_or(config_path, Path::new)
        .parent()
        .unwrap_or_else(|| Path::new("."))
        .join(file_name)
}

fn ipc_cert_file_path(raw: &RawConfig, config_path: &Path) -> PathBuf {
    resolve_agent_file(
        raw.ipc_cert_file_path.as_deref(),
        raw,
        config_path,
        DEFAULT_IPC_CERT_FILE_NAME,
    )
}

fn identity_file_path(raw: &RawConfig, config_path: &Path) -> PathBuf {
    resolve_agent_file(
        raw.private_action_runner
            .as_ref()
            .and_then(|par| par.identity_file_path.as_deref()),
        raw,
        config_path,
        crate::identity::DEFAULT_IDENTITY_FILE_NAME,
    )
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
        assert_eq!(cfg.task_concurrency, 5);
        assert_eq!(cfg.identity.org_id, 42);
        assert_eq!(cfg.identity.runner_id, "runner-1");
        assert_eq!(cfg.modes, vec!["pull"]);
        assert_eq!(cfg.executor_process_name, EXECUTOR_PROCESS_NAME);
        assert_eq!(cfg.executor_socket, PathBuf::from(DEFAULT_EXECUTOR_SOCKET));
        assert_eq!(cfg.procmgr_socket, dd_procmgr_client::default_ipc_path());
        assert_eq!(cfg.ready_timeout, EXECUTOR_READY_TIMEOUT);
        assert_eq!(cfg.ipc_cert_file, PathBuf::from("ipc_cert.pem"));
    }

    #[test]
    fn reads_nested_executor_socket_path() {
        let yaml = format!("{MIN_YAML}  executor:\n    socket_path: /tmp/custom-executor.sock\n");
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(
            cfg.executor_socket,
            PathBuf::from("/tmp/custom-executor.sock")
        );
    }

    #[test]
    #[cfg(bazel)]
    fn runner_version_is_the_injected_agent_version() {
        assert_eq!(RUNNER_VERSION, env!("DD_AGENT_VERSION"));
        assert_ne!(
            RUNNER_VERSION,
            env!("CARGO_PKG_VERSION"),
            "reported version must be the agent version, not the crate version"
        );
        let major = RUNNER_VERSION.split('.').next().unwrap_or_default();
        assert!(
            !major.is_empty() && major.chars().all(|c| c.is_ascii_digit()),
            "malformed agent version {RUNNER_VERSION:?}"
        );
        assert!(
            RUNNER_VERSION.split('.').count() >= 3,
            "agent version {RUNNER_VERSION:?} must have major.minor.patch"
        );
    }

    #[test]
    fn ignores_flat_executor_socket_path_key() {
        let yaml = format!("{MIN_YAML}  executor_socket_path: /tmp/wrong.sock\n");
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(cfg.executor_socket, PathBuf::from(DEFAULT_EXECUTOR_SOCKET));
    }

    /// `ipc_cert_file_path` defaults to empty in the agent and is unset on
    /// essentially every host, so par-control has to reproduce Go's resolution chain.
    #[test]
    fn resolves_ipc_cert_path_like_the_agent() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        let identity_yaml = "private_action_runner:\n  urn: \"urn:dd:apps:on-prem-runner:us1:42:r\"\n  private_key: \"k\"\n";

        // 1. Explicit setting wins.
        std::fs::write(
            &cfg_path,
            format!("ipc_cert_file_path: /custom/cert.pem\nauth_token_file_path: /tok/auth_token\n{identity_yaml}"),
        )
        .unwrap();
        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.ipc_cert_file, PathBuf::from("/custom/cert.pem"));

        // 2. Else next to auth_token_file_path.
        std::fs::write(
            &cfg_path,
            format!("auth_token_file_path: /tok/auth_token\n{identity_yaml}"),
        )
        .unwrap();
        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.ipc_cert_file, PathBuf::from("/tok/ipc_cert.pem"));

        // 3. Else next to datadog.yaml — the case that applies to a default host.
        std::fs::write(&cfg_path, identity_yaml).unwrap();
        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.ipc_cert_file, dir.path().join("ipc_cert.pem"));

        // An empty string is the agent's own default and must not be taken
        // literally as a path.
        std::fs::write(
            &cfg_path,
            format!("ipc_cert_file_path: \"\"\n{identity_yaml}"),
        )
        .unwrap();
        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.ipc_cert_file, dir.path().join("ipc_cert.pem"));
    }

    #[test]
    fn plaintext_opms_requires_test_bypass() {
        assert_eq!(
            resolve_opms_base_url(None, Some("http://fake-opms:8080"), true).unwrap(),
            "http://fake-opms:8080"
        );
        assert!(resolve_opms_base_url(None, Some("http://fake-opms:8080"), false).is_err());
    }

    #[test]
    fn https_dd_url_selects_the_matching_datadog_site() {
        assert_eq!(
            resolve_opms_base_url(
                Some("datadoghq.com"),
                Some("https://app.us3.datadoghq.com"),
                false,
            )
            .unwrap(),
            "https://api.us3.datadoghq.com"
        );
        assert_eq!(
            resolve_opms_base_url(None, Some("https://api.datad0g.eu."), false).unwrap(),
            "https://api.datad0g.eu"
        );
        assert!(resolve_opms_base_url(None, Some("https://custom.example.com"), false).is_err());
    }

    #[test]
    fn parses_proxy_and_tls_configuration() {
        let yaml = format!(
            "{MIN_YAML}proxy:\n  http: http://proxy.example:8080\n  https: http://secure-proxy.example:8443\n  no_proxy: [localhost, api.datadoghq.com]\nno_proxy_nonexact_match: true\nskip_ssl_validation: true\nmin_tls_version: tlsv1.3\n"
        );
        let cfg = Config::from_yaml_str(&yaml).unwrap();

        assert_eq!(cfg.proxy.http.as_deref(), Some("http://proxy.example:8080"));
        assert_eq!(
            cfg.proxy.https.as_deref(),
            Some("http://secure-proxy.example:8443")
        );
        assert_eq!(cfg.proxy.no_proxy, ["localhost", "api.datadoghq.com"]);
        assert!(cfg.proxy.no_proxy_nonexact_match);
        assert!(cfg.tls.skip_ssl_validation);
        assert_eq!(cfg.tls.min_tls_version, "tlsv1.3");
    }

    #[test]
    fn full_config_honors_environment_overrides() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(&cfg_path, MIN_YAML).unwrap();
        let env = |name: &str| match name {
            "DD_SITE" => Some("datadoghq.eu".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_URN" => {
                Some("urn:dd:apps:on-prem-runner:eu1:7:env-runner".to_string())
            }
            "DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY" => Some("env-key".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_TASK_CONCURRENCY" => Some("3".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_EXECUTOR_SOCKET_PATH" => {
                Some("/tmp/env-executor.sock".to_string())
            }
            "DD_PROXY_HTTPS" => Some("http://proxy.example:3128".to_string()),
            "DD_PROXY_NO_PROXY" => Some("localhost, 127.0.0.1".to_string()),
            "DD_NO_PROXY_NONEXACT_MATCH" => Some("true".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_OPMS_EXTRA_HEADERS" => {
                Some(r#"{"X-Test-Routing":"canary"}"#.to_string())
            }
            _ => None,
        };

        let cfg = Config::try_from_yaml_file_with_env(&cfg_path, env)
            .unwrap()
            .unwrap();
        assert_eq!(cfg.opms_base_url, "https://api.datadoghq.eu");
        assert_eq!(cfg.identity.org_id, 7);
        assert_eq!(cfg.identity.runner_id, "env-runner");
        assert_eq!(cfg.identity.private_key, "env-key");
        assert_eq!(cfg.task_concurrency, 3);
        assert_eq!(cfg.executor_socket, PathBuf::from("/tmp/env-executor.sock"));
        assert_eq!(
            cfg.proxy.https.as_deref(),
            Some("http://proxy.example:3128")
        );
        assert_eq!(cfg.proxy.no_proxy, ["localhost", "127.0.0.1"]);
        assert!(cfg.proxy.no_proxy_nonexact_match);
        assert_eq!(
            cfg.opms_extra_headers
                .get("X-Test-Routing")
                .map(String::as_str),
            Some("canary")
        );
    }

    #[test]
    fn full_config_uses_fleet_over_environment_over_local_precedence() {
        let dir = tempfile::tempdir().unwrap();
        let fleet_dir = dir.path().join("fleet");
        std::fs::create_dir(&fleet_dir).unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(
            &cfg_path,
            format!(
                "{MIN_YAML}  task_concurrency: 1\nproxy:\n  https: http://local-proxy\nmin_tls_version: tlsv1.1\n"
            ),
        )
        .unwrap();
        std::fs::write(
            fleet_dir.join("datadog.yaml"),
            r#"
site: us3.datadoghq.com
proxy:
  https: http://fleet-proxy
no_proxy_nonexact_match: false
min_tls_version: tlsv1.3
private_action_runner:
  task_concurrency: 3
  urn: urn:dd:apps:on-prem-runner:us3:3:fleet-runner
  private_key: fleet-key
  opms_extra_headers:
    X-Source: fleet
"#,
        )
        .unwrap();
        let fleet_dir = fleet_dir.to_string_lossy().into_owned();
        let cfg = Config::try_from_yaml_file_with_env(&cfg_path, |name| match name {
            "DD_FLEET_POLICIES_DIR" => Some(fleet_dir.clone()),
            "DD_SITE" => Some("datadoghq.eu".to_string()),
            "DD_PROXY_HTTPS" => Some("http://env-proxy".to_string()),
            "DD_NO_PROXY_NONEXACT_MATCH" => Some("true".to_string()),
            "DD_MIN_TLS_VERSION" => Some("tlsv1.2".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_TASK_CONCURRENCY" => Some("2".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_URN" => {
                Some("urn:dd:apps:on-prem-runner:eu1:2:env-runner".to_string())
            }
            "DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY" => Some("env-key".to_string()),
            _ => None,
        })
        .unwrap()
        .unwrap();

        assert_eq!(cfg.opms_base_url, "https://api.us3.datadoghq.com");
        assert_eq!(cfg.task_concurrency, 3);
        assert_eq!(cfg.proxy.https.as_deref(), Some("http://fleet-proxy"));
        assert!(!cfg.proxy.no_proxy_nonexact_match);
        assert_eq!(cfg.tls.min_tls_version, "tlsv1.3");
        assert_eq!(cfg.identity.runner_id, "fleet-runner");
        assert_eq!(cfg.identity.private_key, "fleet-key");
        assert_eq!(cfg.opms_extra_headers.get("X-Source").unwrap(), "fleet");
    }

    #[test]
    fn fleet_proxy_overrides_environment() {
        let dir = tempfile::tempdir().unwrap();
        let fleet_dir = dir.path().join("fleet");
        std::fs::create_dir(&fleet_dir).unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(
            &cfg_path,
            format!(
                "{MIN_YAML}proxy:\n  http: http://local-http\n  https: http://local-https\n  no_proxy:\n  - local.internal\n"
            ),
        )
        .unwrap();
        std::fs::write(
            fleet_dir.join("datadog.yaml"),
            "proxy:\n  http: http://fleet-http\n  https: http://fleet-https\n  no_proxy:\n  - fleet.internal\n",
        )
        .unwrap();
        let fleet_dir = fleet_dir.to_string_lossy().into_owned();

        // Fleet wins over local YAML when no proxy env var is set.
        let cfg = Config::try_from_yaml_file_with_env(&cfg_path, |name| match name {
            "DD_FLEET_POLICIES_DIR" => Some(fleet_dir.clone()),
            _ => None,
        })
        .unwrap()
        .unwrap();
        assert_eq!(cfg.proxy.http.as_deref(), Some("http://fleet-http"));
        assert_eq!(cfg.proxy.https.as_deref(), Some("http://fleet-https"));
        assert_eq!(
            cfg.proxy.no_proxy,
            vec!["local.internal".to_string(), "fleet.internal".to_string()]
        );

        // Fleet also wins over proxy environment variables.
        let cfg = Config::try_from_yaml_file_with_env(&cfg_path, |name| match name {
            "DD_FLEET_POLICIES_DIR" => Some(fleet_dir.clone()),
            "HTTP_PROXY" => Some("http://env-http".to_string()),
            "DD_PROXY_HTTPS" => Some("http://env-https".to_string()),
            "NO_PROXY" => Some("env.internal,other.internal".to_string()),
            _ => None,
        })
        .unwrap()
        .unwrap();
        assert_eq!(cfg.proxy.http.as_deref(), Some("http://fleet-http"));
        assert_eq!(cfg.proxy.https.as_deref(), Some("http://fleet-https"));
        assert_eq!(
            cfg.proxy.no_proxy,
            vec![
                "local.internal".to_string(),
                "env.internal".to_string(),
                "other.internal".to_string(),
                "fleet.internal".to_string(),
            ]
        );
    }

    #[test]
    fn figment_deep_merges_extra_headers() {
        let dir = tempfile::tempdir().unwrap();
        let fleet_dir = dir.path().join("fleet");
        std::fs::create_dir(&fleet_dir).unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(
            &cfg_path,
            format!("{MIN_YAML}  opms_extra_headers:\n    X-Local: local\n    X-Shared: local\n"),
        )
        .unwrap();
        std::fs::write(
            fleet_dir.join("datadog.yaml"),
            "private_action_runner:\n  opms_extra_headers:\n    X-Fleet: fleet\n    X-Shared: fleet\n",
        )
        .unwrap();
        let fleet_dir = fleet_dir.to_string_lossy().into_owned();
        let cfg = Config::try_from_yaml_file_with_env(&cfg_path, |name| {
            (name == "DD_FLEET_POLICIES_DIR").then(|| fleet_dir.clone())
        })
        .unwrap()
        .unwrap();

        assert_eq!(cfg.opms_extra_headers.get("X-Local").unwrap(), "local");
        assert_eq!(cfg.opms_extra_headers.get("X-Fleet").unwrap(), "fleet");
        assert_eq!(cfg.opms_extra_headers.get("X-Shared").unwrap(), "fleet");
    }

    #[test]
    fn discovers_fleet_directory_from_local_config() {
        let dir = tempfile::tempdir().unwrap();
        let fleet_dir = dir.path().join("fleet");
        std::fs::create_dir(&fleet_dir).unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(
            &cfg_path,
            format!("fleet_policies_dir: {}\n{MIN_YAML}", fleet_dir.display()),
        )
        .unwrap();
        std::fs::write(
            fleet_dir.join("datadog.yaml"),
            "private_action_runner:\n  task_concurrency: 9\n",
        )
        .unwrap();

        let cfg = Config::try_from_yaml_file_with_env(&cfg_path, |_| None)
            .unwrap()
            .unwrap();
        assert_eq!(cfg.task_concurrency, 9);
    }

    #[test]
    fn malformed_and_missing_fleet_files_are_ignored() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(&cfg_path, MIN_YAML).unwrap();

        for fleet_dir in [dir.path().join("missing"), dir.path().join("malformed")] {
            if fleet_dir.ends_with("malformed") {
                std::fs::create_dir(&fleet_dir).unwrap();
                std::fs::write(fleet_dir.join("datadog.yaml"), "not: [valid").unwrap();
            }
            let fleet_dir = fleet_dir.to_string_lossy().into_owned();
            let cfg = Config::try_from_yaml_file_with_env(&cfg_path, |name| {
                (name == "DD_FLEET_POLICIES_DIR").then(|| fleet_dir.clone())
            })
            .unwrap()
            .unwrap();
            assert_eq!(cfg.identity.runner_id, "runner-1");
        }
    }

    #[test]
    fn rejects_zero_concurrency() {
        let concurrency = format!("{MIN_YAML}  task_concurrency: 0\n");
        assert!(Config::from_yaml_str(&concurrency).is_err());
    }

    #[test]
    fn fails_without_identity() {
        assert!(Config::from_yaml_str("site: datadoghq.com\n").is_err());
    }

    #[test]
    fn resolves_identity_from_sibling_file_when_not_inline() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(
            &cfg_path,
            "site: datadoghq.com\nprivate_action_runner:\n  enabled: true\n",
        )
        .unwrap();
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
    fn persisted_identity_wins_over_inline_and_follows_auth_token_path() {
        let dir = tempfile::tempdir().unwrap();
        let config_dir = dir.path().join("config");
        let runtime_dir = dir.path().join("runtime");
        std::fs::create_dir(&config_dir).unwrap();
        std::fs::create_dir(&runtime_dir).unwrap();
        let cfg_path = config_dir.join("datadog.yaml");
        std::fs::write(
            &cfg_path,
            format!(
                "auth_token_file_path: {}\nprivate_action_runner:\n  urn: urn:dd:apps:on-prem-runner:us1:1:inline\n  private_key: inline-key\n",
                runtime_dir.join("auth_token").display()
            ),
        )
        .unwrap();
        std::fs::write(
            runtime_dir.join(crate::identity::DEFAULT_IDENTITY_FILE_NAME),
            r#"{"private_key":"persisted-key","urn":"urn:dd:apps:on-prem-runner:us1:2:persisted"}"#,
        )
        .unwrap();

        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.identity.org_id, 2);
        assert_eq!(cfg.identity.runner_id, "persisted");
        assert_eq!(cfg.identity.private_key, "persisted-key");

        let explicit = dir.path().join("explicit-identity.json");
        std::fs::write(
            &explicit,
            r#"{"private_key":"explicit-key","urn":"urn:dd:apps:on-prem-runner:us1:3:explicit"}"#,
        )
        .unwrap();
        std::fs::write(
            &cfg_path,
            format!(
                "auth_token_file_path: {}\nprivate_action_runner:\n  identity_file_path: {}\n  urn: urn:dd:apps:on-prem-runner:us1:1:inline\n  private_key: inline-key\n",
                runtime_dir.join("auth_token").display(),
                explicit.display()
            ),
        )
        .unwrap();
        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.identity.org_id, 3);
        assert_eq!(cfg.identity.runner_id, "explicit");
    }

    #[test]
    fn try_load_returns_none_when_no_identity_anywhere() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(&cfg_path, "site: datadoghq.com\n").unwrap();
        assert!(Config::try_from_yaml_file(&cfg_path).unwrap().is_none());
    }

    #[test]
    fn incomplete_inline_identity_is_rejected() {
        let error = Config::from_yaml_str(
            "private_action_runner:\n  urn: urn:dd:apps:on-prem-runner:us1:1:r\n",
        )
        .err()
        .unwrap()
        .to_string();
        assert!(error.contains("private_action_runner.private_key"));
    }

    #[test]
    fn split_mode_requires_enabled_and_split_enabled() {
        let cases = [
            ("", false),
            ("private_action_runner:\n  enabled: true\n", false),
            ("private_action_runner:\n  split_enabled: true\n", false),
            (
                "private_action_runner:\n  enabled: true\n  split_enabled: true\n",
                true,
            ),
            (
                "private_action_runner:\n  enabled: false\n  split_enabled: true\n",
                false,
            ),
        ];
        for (yaml, want) in cases {
            let launch = Launch::from_yaml_str(yaml).unwrap();
            assert_eq!(launch.gate.split_mode, want, "yaml: {yaml:?}");
        }
    }

    #[test]
    fn launch_gate_environment_overrides_yaml() {
        let env = |name: &str| match name {
            "DD_PRIVATE_ACTION_RUNNER_ENABLED" => Some("true".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED" => Some("1".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL" => Some("false".to_string()),
            "DD_LOG_LEVEL" => Some("trace".to_string()),
            _ => None,
        };
        let launch = Launch::from_yaml_str_with_env(
            "log_level: warn\nprivate_action_runner:\n  enabled: false\n  split_enabled: false\n",
            env,
        )
        .unwrap();
        assert!(launch.gate.split_mode);
        assert!(!launch.gate.self_enroll);
        assert_eq!(launch.log_level, log::LevelFilter::Trace);
    }

    #[test]
    fn empty_environment_overrides_fall_back_to_yaml() {
        let env = |name: &str| match name {
            "DD_PRIVATE_ACTION_RUNNER_ENABLED"
            | "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED"
            | "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL" => Some(String::new()),
            _ => None,
        };
        let launch = Launch::from_yaml_str_with_env(
            "private_action_runner:\n  enabled: true\n  split_enabled: true\n  self_enroll: false\n",
            env,
        )
        .unwrap();
        assert!(launch.gate.split_mode);
        assert!(!launch.gate.self_enroll);
    }

    #[test]
    fn fleet_policy_overrides_local_config_and_environment() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(
            dir.path().join("datadog.yaml"),
            "log_level: error\nprivate_action_runner:\n  enabled: true\n  split_enabled: true\n  self_enroll: false\n",
        )
        .unwrap();
        let fleet_dir = dir.path().to_string_lossy().into_owned();
        let launch = Launch::from_yaml_str_with_env(
            "log_level: debug\nprivate_action_runner:\n  enabled: false\n  split_enabled: false\n",
            |name| match name {
                "DD_FLEET_POLICIES_DIR" => Some(fleet_dir.clone()),
                "DD_PRIVATE_ACTION_RUNNER_ENABLED" | "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED" => {
                    Some("false".to_string())
                }
                "DD_LOG_LEVEL" => Some("trace".to_string()),
                _ => None,
            },
        )
        .unwrap();
        assert!(launch.gate.split_mode);
        assert!(!launch.gate.self_enroll);
        assert_eq!(launch.log_level, log::LevelFilter::Error);
    }

    #[test]
    fn launch_gate_accepts_agent_boolean_environment_values() {
        for (raw, expected) in [
            ("1", true),
            ("TRUE", true),
            ("True", true),
            ("0", false),
            ("f", false),
            ("False", false),
        ] {
            let launch = Launch::from_yaml_str_with_env("", |name| match name {
                "DD_PRIVATE_ACTION_RUNNER_ENABLED" => Some(raw.to_string()),
                "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED" => Some("true".to_string()),
                _ => None,
            })
            .unwrap();
            assert_eq!(launch.gate.split_mode, expected, "boolean value {raw:?}");
        }
    }

    #[test]
    fn launch_gate_rejects_invalid_environment_boolean() {
        let result = Launch::from_yaml_str_with_env("", |name| {
            (name == "DD_PRIVATE_ACTION_RUNNER_ENABLED").then(|| "sometimes".to_string())
        });
        assert!(result.is_err());
    }

    #[test]
    fn gate_resolves_without_identity() {
        let launch = Launch::from_yaml_str("site: datadoghq.com\n").unwrap();
        assert!(!launch.gate.split_mode);
        // Same default as Go's private_action_runner.self_enroll.
        assert!(launch.gate.self_enroll);
        assert_eq!(launch.log_level, log::LevelFilter::Info);
    }

    #[test]
    fn parses_agent_log_levels() {
        for (raw, want) in [
            ("debug", log::LevelFilter::Debug),
            ("TRACE", log::LevelFilter::Trace),
            ("warn", log::LevelFilter::Warn),
            ("warning", log::LevelFilter::Warn),
            ("error", log::LevelFilter::Error),
            ("critical", log::LevelFilter::Error),
            ("off", log::LevelFilter::Off),
            ("not-a-level", log::LevelFilter::Info),
        ] {
            let launch = Launch::from_yaml_str(&format!("log_level: {raw}\n")).unwrap();
            assert_eq!(launch.log_level, want, "log_level: {raw}");
        }
    }

    #[test]
    fn reports_a_missing_config_file() {
        let dir = tempfile::tempdir().unwrap();
        let error = Launch::from_yaml_file(&dir.path().join("missing.yaml")).unwrap_err();
        assert!(
            format!("{error:#}").contains("failed to load config file"),
            "{error:#}"
        );
    }

    #[test]
    fn overrides_pool_size_but_always_uses_pull_mode() {
        let yaml = format!("{MIN_YAML}  task_concurrency: 3\n  modes: [push]\n");
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(cfg.task_concurrency, 3);
        // Liveness reporting is a fixed contract with OPMS, not a knob.
        assert_eq!(cfg.health_check_interval, Duration::from_secs(30));
        // `modes` is intentionally not deserialized. Enrollment and the Go
        // monolith support pull mode only, so an undocumented YAML key cannot
        // make the runtime advertise a different capability.
        assert_eq!(cfg.modes, vec!["pull"]);
    }
}
