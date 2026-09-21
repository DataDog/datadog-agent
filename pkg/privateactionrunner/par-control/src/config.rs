// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::opms::{ProxyDecision, TlsConfig};
use anyhow::{Context, Result, ensure};
use std::collections::HashMap;
use std::fmt;
use std::path::PathBuf;
use std::time::Duration;

const EXECUTOR_READY_TIMEOUT: Duration = Duration::from_secs(30);
const LOOP_INTERVAL: Duration = Duration::from_secs(1);
const HEARTBEAT_INTERVAL: Duration = Duration::from_secs(20);
const HEALTH_CHECK_INTERVAL: Duration = Duration::from_secs(30);
const OPMS_REQUEST_TIMEOUT: Duration = Duration::from_secs(30);
const MIN_BACKOFF: Duration = Duration::from_secs(1);
const MAX_BACKOFF: Duration = Duration::from_secs(180);
const WAIT_BEFORE_RETRY: Duration = Duration::from_secs(300);
const MAX_ATTEMPTS: u32 = 20;
pub const EXECUTOR_PROCESS_NAME: &str = "datadog-agent-action-executor";

#[derive(Clone)]
pub struct Identity {
    pub urn: String,
    pub org_id: i64,
    pub runner_id: String,
    pub private_key: String,
}

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
    pub opms_proxy: ProxyDecision,
    pub tls: TlsConfig,
    pub min_backoff: Duration,
    pub max_backoff: Duration,
    pub wait_before_retry: Duration,
    pub max_attempts: u32,
    pub runner_version: String,
    pub modes: Vec<String>,
    pub ipc_cert_file: PathBuf,
    pub identity: Identity,
}

#[derive(serde::Deserialize, Default, Clone)]
#[serde(default, deny_unknown_fields)]
pub struct BootstrapConfig {
    pub split_mode: bool,
    pub log_level: String,
    identity: BootstrapIdentity,
    runtime: Option<BootstrapRuntimeConfig>,
}

impl fmt::Debug for BootstrapConfig {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("BootstrapConfig")
            .field("split_mode", &self.split_mode)
            .field("log_level", &self.log_level)
            .finish_non_exhaustive()
    }
}

#[derive(serde::Deserialize, Default, Clone)]
#[serde(default, deny_unknown_fields)]
struct BootstrapIdentity {
    urn: String,
    private_key: String,
    org_id: i64,
    runner_id: String,
}

#[derive(serde::Deserialize, Clone)]
#[serde(deny_unknown_fields)]
struct BootstrapRuntimeConfig {
    opms_base_url: String,
    task_concurrency: usize,
    opms_extra_headers: HashMap<String, String>,
    opms_proxy_url: String,
    skip_ssl_validation: bool,
    min_tls_version: String,
}

impl BootstrapConfig {
    pub fn log_level(&self) -> log::LevelFilter {
        match self.log_level.trim().to_ascii_lowercase().as_str() {
            "trace" => log::LevelFilter::Trace,
            "debug" => log::LevelFilter::Debug,
            "warn" | "warning" => log::LevelFilter::Warn,
            "error" | "critical" => log::LevelFilter::Error,
            "off" => log::LevelFilter::Off,
            _ => log::LevelFilter::Info,
        }
    }

    pub fn into_config(self, executor_socket: PathBuf, ipc_cert_file: PathBuf) -> Result<Config> {
        ensure!(
            !executor_socket.as_os_str().is_empty(),
            "--executor-socket is empty"
        );
        ensure!(
            !ipc_cert_file.as_os_str().is_empty(),
            "--ipc-cert-file is empty"
        );
        ensure!(
            self.split_mode,
            "bootstrap configuration has split mode disabled"
        );
        for (name, value) in [
            ("log_level", self.log_level.as_str()),
            ("identity.urn", self.identity.urn.as_str()),
            ("identity.private_key", self.identity.private_key.as_str()),
            ("identity.runner_id", self.identity.runner_id.as_str()),
        ] {
            ensure!(
                !value.is_empty(),
                "bootstrap configuration is missing {name}"
            );
        }
        ensure!(
            self.identity.org_id > 0,
            "bootstrap configuration is missing identity.org_id"
        );
        let runtime = self
            .runtime
            .context("bootstrap configuration is missing runtime")?;
        ensure!(
            runtime.task_concurrency > 0,
            "private_action_runner.task_concurrency must be greater than zero"
        );
        ensure!(
            !runtime.opms_base_url.is_empty(),
            "bootstrap OPMS URL is empty"
        );
        let opms_proxy = if runtime.opms_proxy_url.is_empty() {
            ProxyDecision::None
        } else {
            ProxyDecision::Direct(runtime.opms_proxy_url)
        };

        Ok(Config {
            opms_base_url: runtime.opms_base_url,
            task_concurrency: runtime.task_concurrency,
            executor_socket,
            procmgr_socket: dd_procmgr_client::ipc_path(),
            executor_process_name: EXECUTOR_PROCESS_NAME.to_string(),
            loop_interval: LOOP_INTERVAL,
            heartbeat_interval: HEARTBEAT_INTERVAL,
            health_check_interval: HEALTH_CHECK_INTERVAL,
            ready_timeout: EXECUTOR_READY_TIMEOUT,
            opms_request_timeout: OPMS_REQUEST_TIMEOUT,
            opms_extra_headers: runtime.opms_extra_headers,
            opms_proxy,
            tls: TlsConfig {
                skip_ssl_validation: runtime.skip_ssl_validation,
                min_tls_version: runtime.min_tls_version,
            },
            min_backoff: MIN_BACKOFF,
            max_backoff: MAX_BACKOFF,
            wait_before_retry: WAIT_BEFORE_RETRY,
            max_attempts: MAX_ATTEMPTS,
            runner_version: crate::agent_version().to_string(),
            modes: vec!["pull".to_string()],
            ipc_cert_file,
            identity: Identity {
                urn: self.identity.urn,
                private_key: self.identity.private_key,
                org_id: self.identity.org_id,
                runner_id: self.identity.runner_id,
            },
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    const JSON: &str = r#"{
        "split_mode": true,
        "log_level": "debug",
        "identity": {"urn":"urn","private_key":"secret-key","org_id":42,"runner_id":"runner"},
        "runtime": {
            "opms_base_url":"https://api.us3.datadoghq.com",
            "task_concurrency":9,
            "opms_extra_headers":{"X-Test":"secret-header"},
            "opms_proxy_url":"http://user:secret-password@proxy:3128",
            "skip_ssl_validation":true,
            "min_tls_version":"tlsv1.3"
        }
    }"#;

    #[test]
    fn uses_bootstrap_runtime_and_launch_paths() {
        let bootstrap: BootstrapConfig = serde_json::from_str(JSON).unwrap();
        assert!(bootstrap.split_mode);
        assert_eq!(bootstrap.log_level(), log::LevelFilter::Debug);

        let config = bootstrap
            .into_config("/launch.sock".into(), "/launch/cert.pem".into())
            .unwrap();

        assert_eq!(config.opms_base_url, "https://api.us3.datadoghq.com");
        assert_eq!(
            config.opms_proxy,
            ProxyDecision::Direct("http://user:secret-password@proxy:3128".to_string())
        );
        assert_eq!(config.task_concurrency, 9);
        assert_eq!(config.executor_socket, PathBuf::from("/launch.sock"));
        assert_eq!(config.ipc_cert_file, PathBuf::from("/launch/cert.pem"));
        assert_eq!(config.opms_extra_headers["X-Test"], "secret-header");
        assert!(config.tls.skip_ssl_validation);
        assert_eq!(config.tls.min_tls_version, "tlsv1.3");
        assert_eq!(config.loop_interval, Duration::from_secs(1));
        assert_eq!(config.runner_version, crate::agent_version());
        assert!(!config.runner_version.is_empty());
        assert_eq!(config.identity.org_id, 42);
        assert_eq!(config.identity.private_key, "secret-key");
    }

    #[test]
    fn rejects_missing_runtime_instead_of_falling_back_to_core_agent() {
        let mut payload: serde_json::Value = serde_json::from_str(JSON).unwrap();
        payload.as_object_mut().unwrap().remove("runtime");
        let bootstrap: BootstrapConfig = serde_json::from_value(payload).unwrap();
        let error = bootstrap
            .into_config("/launch.sock".into(), "/launch/cert.pem".into())
            .err()
            .unwrap()
            .to_string();
        assert!(error.contains("missing runtime"));
    }

    #[test]
    fn rejects_invalid_runtime_settings() {
        for (field, value, message) in [
            ("task_concurrency", json!(0), "task_concurrency"),
            ("opms_base_url", json!(""), "OPMS URL"),
        ] {
            let mut payload: serde_json::Value = serde_json::from_str(JSON).unwrap();
            payload["runtime"][field] = value;
            let bootstrap: BootstrapConfig = serde_json::from_value(payload).unwrap();
            let error = bootstrap
                .into_config("/launch.sock".into(), "/launch/cert.pem".into())
                .err()
                .unwrap()
                .to_string();
            assert!(error.contains(message), "{field}: {error}");
        }
    }

    #[test]
    fn preserves_go_proxy_bypass_decision() {
        let mut bootstrap: BootstrapConfig = serde_json::from_str(JSON).unwrap();
        bootstrap.runtime.as_mut().unwrap().opms_proxy_url.clear();
        assert_eq!(
            bootstrap
                .into_config("/launch.sock".into(), "/launch/cert.pem".into())
                .unwrap()
                .opms_proxy,
            ProxyDecision::None
        );
    }

    #[test]
    fn rejects_empty_launch_paths() {
        for (socket, cert, flag) in [
            ("", "/cert.pem", "--executor-socket"),
            ("/executor.sock", "", "--ipc-cert-file"),
        ] {
            let bootstrap: BootstrapConfig = serde_json::from_str(JSON).unwrap();
            let error = bootstrap
                .into_config(socket.into(), cert.into())
                .err()
                .unwrap();
            assert!(error.to_string().contains(flag));
        }
    }

    #[test]
    fn rejects_unknown_or_missing_runtime_fields() {
        let mut payload: serde_json::Value = serde_json::from_str(JSON).unwrap();
        payload["runtime"]["unknown"] = json!(true);
        assert!(serde_json::from_value::<BootstrapConfig>(payload).is_err());

        let mut payload: serde_json::Value = serde_json::from_str(JSON).unwrap();
        payload["runtime"]
            .as_object_mut()
            .unwrap()
            .remove("skip_ssl_validation");
        assert!(serde_json::from_value::<BootstrapConfig>(payload).is_err());
    }

    #[test]
    fn rejects_unknown_bootstrap_fields() {
        let json = JSON.replace("\"split_mode\": true", "\"unknown\": true");
        assert!(serde_json::from_str::<BootstrapConfig>(&json).is_err());
    }

    #[test]
    fn debug_does_not_expose_credentials() {
        let bootstrap: BootstrapConfig = serde_json::from_str(JSON).unwrap();
        let debug = format!("{bootstrap:?}");
        for secret in ["secret-key", "secret-header", "secret-password"] {
            assert!(!debug.contains(secret));
        }
    }
}
