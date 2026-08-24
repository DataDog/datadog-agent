// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! The par-control runtime configuration.
//!
//! Go is the configuration authority. `privateactionrunner
//! bootstrap-par-control` loads the canonical Agent configuration — local YAML,
//! environment, Fleet policy, secrets, endpoint and path resolution — and hands
//! back the resolved values. This module models what comes back and validates it
//! at the trust boundary; it deliberately does not re-derive anything.

use crate::identity::Identity;
use crate::opms::TlsConfig;
use anyhow::{Context, Result, bail};
use std::collections::HashMap;
use std::path::PathBuf;
use std::time::Duration;

/// How long the executor has to report readiness after par-control starts it.
/// Owned here because it describes a Rust-side wait, not Agent configuration.
const EXECUTOR_READY_TIMEOUT: Duration = Duration::from_secs(30);

pub const EXECUTOR_PROCESS_NAME: &str = "datadog-agent-action-executor";

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
    pub opms_proxy_url: Option<String>,
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

/// The configuration `bootstrap-par-control` emits.
///
/// Field names and duration units are a contract with
/// `cmd/privateactionrunner/subcommands/bootstrapparcontrol`.
#[derive(serde::Deserialize, Debug, Default, Clone)]
pub struct BootstrapConfig {
    /// Gates the control plane. False means the monolithic runner owns OPMS
    /// polling and par-control should exit successfully.
    #[serde(default)]
    pub split_mode: bool,
    #[serde(default)]
    pub log_level: String,

    #[serde(default)]
    identity: BootstrapIdentity,

    #[serde(default)]
    opms_base_url: String,
    #[serde(default)]
    opms_proxy_url: String,
    #[serde(default)]
    agent_version: String,
    #[serde(default)]
    modes: Vec<String>,
    #[serde(default)]
    task_concurrency: i64,
    #[serde(default)]
    executor_socket: String,
    #[serde(default)]
    ipc_cert_file_path: String,
    #[serde(default)]
    opms_extra_headers: HashMap<String, String>,

    #[serde(default)]
    tls: BootstrapTls,

    #[serde(default)]
    loop_interval_milliseconds: u64,
    #[serde(default)]
    heartbeat_interval_milliseconds: u64,
    #[serde(default)]
    health_check_interval_milliseconds: u64,
    #[serde(default)]
    opms_request_timeout_milliseconds: u64,
    #[serde(default)]
    min_backoff_milliseconds: u64,
    #[serde(default)]
    max_backoff_milliseconds: u64,
    #[serde(default)]
    wait_before_retry_milliseconds: u64,
    #[serde(default)]
    max_attempts: i64,
}

#[derive(serde::Deserialize, Debug, Default, Clone)]
struct BootstrapIdentity {
    #[serde(default)]
    urn: String,
    #[serde(default)]
    private_key: String,
    #[serde(default)]
    org_id: i64,
    #[serde(default)]
    runner_id: String,
}

#[derive(serde::Deserialize, Debug, Default, Clone)]
struct BootstrapTls {
    #[serde(default)]
    skip_ssl_validation: bool,
    #[serde(default)]
    min_tls_version: String,
}

/// Agent log levels, as emitted by the Go `log_level` setting.
pub fn parse_log_level(raw: &str) -> log::LevelFilter {
    match raw.trim().to_ascii_lowercase().as_str() {
        "trace" => log::LevelFilter::Trace,
        "debug" => log::LevelFilter::Debug,
        "warn" | "warning" => log::LevelFilter::Warn,
        "error" | "critical" => log::LevelFilter::Error,
        "off" => log::LevelFilter::Off,
        _ => log::LevelFilter::Info,
    }
}

impl BootstrapConfig {
    pub fn log_level(&self) -> log::LevelFilter {
        parse_log_level(&self.log_level)
    }

    /// Validate the bootstrap output and build the runtime configuration.
    ///
    /// These are trust-boundary checks on another process's output, not a second
    /// configuration authority: nothing here re-derives a value Go resolved.
    pub fn into_config(self) -> Result<Config> {
        if !self.split_mode {
            bail!("cannot build a par-control configuration while split mode is disabled");
        }

        let identity = Identity::new(
            self.identity.urn,
            self.identity.private_key,
            self.identity.org_id,
            self.identity.runner_id,
        )?;

        if self.opms_base_url.is_empty() {
            bail!("bootstrap returned an empty OPMS base URL");
        }
        // Parsed once here so a malformed URL fails startup instead of every
        // request.
        reqwest::Url::parse(&self.opms_base_url)
            .context("bootstrap returned an invalid OPMS URL")?;

        let task_concurrency = usize::try_from(self.task_concurrency)
            .ok()
            .filter(|value| *value > 0)
            .context("bootstrap returned a non-positive task concurrency")?;

        if self.executor_socket.is_empty() {
            bail!("bootstrap returned an empty executor socket path");
        }
        if self.ipc_cert_file_path.is_empty() {
            bail!("bootstrap returned an empty IPC certificate path");
        }

        let max_attempts =
            u32::try_from(self.max_attempts).context("bootstrap returned invalid max attempts")?;

        Ok(Config {
            opms_base_url: self.opms_base_url,
            task_concurrency,
            executor_socket: PathBuf::from(self.executor_socket),
            procmgr_socket: dd_procmgr_client::default_ipc_path(),
            executor_process_name: EXECUTOR_PROCESS_NAME.to_string(),
            loop_interval: duration_ms("loop_interval", self.loop_interval_milliseconds)?,
            heartbeat_interval: duration_ms(
                "heartbeat_interval",
                self.heartbeat_interval_milliseconds,
            )?,
            health_check_interval: duration_ms(
                "health_check_interval",
                self.health_check_interval_milliseconds,
            )?,
            ready_timeout: EXECUTOR_READY_TIMEOUT,
            opms_request_timeout: duration_ms(
                "opms_request_timeout",
                self.opms_request_timeout_milliseconds,
            )?,
            opms_extra_headers: self.opms_extra_headers,
            opms_proxy_url: non_empty(self.opms_proxy_url),
            tls: TlsConfig {
                skip_ssl_validation: self.tls.skip_ssl_validation,
                min_tls_version: self.tls.min_tls_version,
            },
            min_backoff: duration_ms("min_backoff", self.min_backoff_milliseconds)?,
            max_backoff: duration_ms("max_backoff", self.max_backoff_milliseconds)?,
            wait_before_retry: duration_ms(
                "wait_before_retry",
                self.wait_before_retry_milliseconds,
            )?,
            max_attempts,
            runner_version: self.agent_version,
            modes: self.modes,
            ipc_cert_file: PathBuf::from(self.ipc_cert_file_path),
            identity,
        })
    }
}

fn duration_ms(field: &str, milliseconds: u64) -> Result<Duration> {
    if milliseconds == 0 {
        bail!("bootstrap returned a zero {field}");
    }
    Ok(Duration::from_millis(milliseconds))
}

fn non_empty(value: String) -> Option<String> {
    if value.is_empty() { None } else { Some(value) }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A complete payload, matching what bootstrap-par-control emits.
    fn full_json() -> String {
        serde_json::json!({
            "split_mode": true,
            "log_level": "debug",
            "identity": {
                "urn": "urn:dd:apps:on-prem-runner:us1:42:runner-1",
                "private_key": "encoded-jwk",
                "org_id": 42,
                "runner_id": "runner-1",
            },
            "opms_base_url": "https://api.datadoghq.com",
            "opms_proxy_url": "http://secure-proxy.example:8443",
            "agent_version": "7.83.0",
            "modes": ["pull"],
            "task_concurrency": 5,
            "executor_socket": "/opt/datadog-agent/run/par-executor.sock",
            "ipc_cert_file_path": "/etc/datadog-agent/ipc_cert.pem",
            "opms_extra_headers": {"X-Test-Routing": "canary"},
            "tls": {"skip_ssl_validation": true, "min_tls_version": "tlsv1.3"},
            "loop_interval_milliseconds": 1000,
            "heartbeat_interval_milliseconds": 20000,
            "health_check_interval_milliseconds": 30000,
            "opms_request_timeout_milliseconds": 30000,
            "min_backoff_milliseconds": 1000,
            "max_backoff_milliseconds": 180000,
            "wait_before_retry_milliseconds": 300000,
            "max_attempts": 20,
        })
        .to_string()
    }

    fn parse(json: &str) -> Result<Config> {
        serde_json::from_str::<BootstrapConfig>(json)
            .context("parse")?
            .into_config()
    }

    /// `Config` deliberately does not derive `Debug` — it holds the private key —
    /// so the error is extracted without `unwrap_err`'s `Debug` bound.
    fn parse_error(json: &str) -> String {
        match parse(json) {
            Ok(_) => panic!("expected a rejection"),
            Err(error) => format!("{error:#}"),
        }
    }

    #[test]
    fn builds_the_runtime_config_from_bootstrap_output() {
        let cfg = parse(&full_json()).unwrap();

        assert_eq!(cfg.opms_base_url, "https://api.datadoghq.com");
        assert_eq!(cfg.task_concurrency, 5);
        assert_eq!(cfg.identity.org_id, 42);
        assert_eq!(cfg.identity.runner_id, "runner-1");
        assert_eq!(cfg.identity.private_key, "encoded-jwk");
        assert_eq!(cfg.runner_version, "7.83.0");
        assert_eq!(cfg.modes, vec!["pull"]);
        assert_eq!(cfg.executor_process_name, EXECUTOR_PROCESS_NAME);
        assert_eq!(
            cfg.executor_socket,
            PathBuf::from("/opt/datadog-agent/run/par-executor.sock")
        );
        assert_eq!(
            cfg.ipc_cert_file,
            PathBuf::from("/etc/datadog-agent/ipc_cert.pem")
        );
        assert_eq!(cfg.procmgr_socket, dd_procmgr_client::default_ipc_path());
        assert_eq!(cfg.ready_timeout, EXECUTOR_READY_TIMEOUT);
        assert_eq!(
            cfg.opms_extra_headers.get("X-Test-Routing").unwrap(),
            "canary"
        );
        assert_eq!(
            cfg.opms_proxy_url.as_deref(),
            Some("http://secure-proxy.example:8443")
        );
        assert!(cfg.tls.skip_ssl_validation);
        assert_eq!(cfg.tls.min_tls_version, "tlsv1.3");
    }

    #[test]
    fn converts_durations_from_milliseconds() {
        let cfg = parse(&full_json()).unwrap();

        assert_eq!(cfg.loop_interval, Duration::from_secs(1));
        assert_eq!(cfg.heartbeat_interval, Duration::from_secs(20));
        assert_eq!(cfg.health_check_interval, Duration::from_secs(30));
        assert_eq!(cfg.opms_request_timeout, Duration::from_secs(30));
        assert_eq!(cfg.min_backoff, Duration::from_secs(1));
        assert_eq!(cfg.max_backoff, Duration::from_secs(180));
        assert_eq!(cfg.wait_before_retry, Duration::from_secs(300));
        assert_eq!(cfg.max_attempts, 20);
    }

    /// An omitted or zero duration would silently become a hot loop or an
    /// instant timeout.
    #[test]
    fn rejects_zero_durations() {
        for field in [
            "loop_interval_milliseconds",
            "heartbeat_interval_milliseconds",
            "health_check_interval_milliseconds",
            "opms_request_timeout_milliseconds",
            "min_backoff_milliseconds",
            "max_backoff_milliseconds",
            "wait_before_retry_milliseconds",
        ] {
            let mut value: serde_json::Value = serde_json::from_str(&full_json()).unwrap();
            value[field] = serde_json::json!(0);
            let error = parse_error(&value.to_string());
            assert!(
                error.contains(field.trim_end_matches("_milliseconds")),
                "{error}"
            );
        }
    }

    #[test]
    fn rejects_non_positive_concurrency() {
        for concurrency in [0, -1] {
            let mut value: serde_json::Value = serde_json::from_str(&full_json()).unwrap();
            value["task_concurrency"] = serde_json::json!(concurrency);
            let error = parse_error(&value.to_string());
            assert!(error.contains("task concurrency"), "{error}");
        }
    }

    #[test]
    fn rejects_incomplete_identity() {
        for (field, replacement) in [
            ("urn", serde_json::json!("")),
            ("private_key", serde_json::json!("")),
            ("private_key", serde_json::json!("   ")),
            ("org_id", serde_json::json!(0)),
            ("org_id", serde_json::json!(-1)),
            ("runner_id", serde_json::json!("")),
        ] {
            let mut value: serde_json::Value = serde_json::from_str(&full_json()).unwrap();
            value["identity"][field] = replacement.clone();
            assert!(
                parse(&value.to_string()).is_err(),
                "identity.{field} = {replacement} should be rejected"
            );
        }
    }

    #[test]
    fn rejects_an_invalid_opms_url() {
        for url in ["", "not-a-url", "://missing-scheme"] {
            let mut value: serde_json::Value = serde_json::from_str(&full_json()).unwrap();
            value["opms_base_url"] = serde_json::json!(url);
            assert!(
                parse(&value.to_string()).is_err(),
                "OPMS URL {url:?} should be rejected"
            );
        }
    }

    #[test]
    fn rejects_empty_paths() {
        for field in ["executor_socket", "ipc_cert_file_path"] {
            let mut value: serde_json::Value = serde_json::from_str(&full_json()).unwrap();
            value[field] = serde_json::json!("");
            let error = parse_error(&value.to_string());
            assert!(error.contains("empty"), "{error}");
        }
    }

    /// Go omits everything but the gate when split mode is off, so the runtime
    /// config must not be constructible from it.
    #[test]
    fn refuses_to_build_a_config_when_split_mode_is_disabled() {
        let gate: BootstrapConfig =
            serde_json::from_str(r#"{"split_mode":false,"log_level":"warn"}"#).unwrap();

        assert!(!gate.split_mode);
        assert_eq!(gate.log_level(), log::LevelFilter::Warn);
        assert!(gate.into_config().is_err());
    }

    #[test]
    fn empty_proxy_url_is_absent() {
        let mut value: serde_json::Value = serde_json::from_str(&full_json()).unwrap();
        value["opms_proxy_url"] = serde_json::json!("");
        let cfg = parse(&value.to_string()).unwrap();

        assert_eq!(cfg.opms_proxy_url, None);
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
            ("", log::LevelFilter::Info),
        ] {
            assert_eq!(parse_log_level(raw), want, "log_level: {raw}");
        }
    }
}
