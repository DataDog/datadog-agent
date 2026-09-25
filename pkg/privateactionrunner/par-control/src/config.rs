// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::opms::{ProxyDecision, TlsConfig};
use crate::proto::executor as pb;
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

// Wrap the credential-bearing protobuf rather than using its generated Debug.
pub struct BootstrapConfig(pb::GetControlPlaneConfigResponse);

impl fmt::Debug for BootstrapConfig {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("BootstrapConfig")
            .field("split_mode", &self.0.split_mode)
            .field("log_level", &self.0.log_level)
            .finish_non_exhaustive()
    }
}

impl BootstrapConfig {
    pub fn new(response: pb::GetControlPlaneConfigResponse) -> Self {
        Self(response)
    }

    pub fn split_mode(&self) -> bool {
        self.0.split_mode
    }

    pub fn log_level(&self) -> log::LevelFilter {
        match self.0.log_level.trim().to_ascii_lowercase().as_str() {
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
        let snapshot = self.0;
        ensure!(
            snapshot.split_mode,
            "bootstrap configuration has split mode disabled"
        );
        let identity = snapshot
            .identity
            .context("bootstrap configuration is missing identity")?;
        for (name, value) in [
            ("log_level", snapshot.log_level.as_str()),
            ("identity.urn", identity.urn.as_str()),
            ("identity.private_key", identity.private_key.as_str()),
            ("identity.runner_id", identity.runner_id.as_str()),
        ] {
            ensure!(
                !value.is_empty(),
                "bootstrap configuration is missing {name}"
            );
        }
        ensure!(
            identity.org_id > 0,
            "bootstrap configuration is missing identity.org_id"
        );
        let runtime = snapshot
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
        ensure!(
            !runtime.min_tls_version.is_empty(),
            "bootstrap minimum TLS version is empty"
        );
        let opms_proxy = if runtime.opms_proxy_url.is_empty() {
            ProxyDecision::None
        } else {
            ProxyDecision::ViaProxy(runtime.opms_proxy_url)
        };
        Ok(Config {
            opms_base_url: runtime.opms_base_url,
            task_concurrency: runtime.task_concurrency as usize,
            executor_socket,
            procmgr_socket: dd_procmgr_client::ipc_path(),
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
                urn: identity.urn,
                private_key: identity.private_key,
                org_id: identity.org_id,
                runner_id: identity.runner_id,
            },
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn snapshot() -> pb::GetControlPlaneConfigResponse {
        pb::GetControlPlaneConfigResponse {
            split_mode: true,
            log_level: "debug".into(),
            identity: Some(pb::ControlPlaneIdentity {
                urn: "urn".into(),
                private_key: "secret-key".into(),
                org_id: 42,
                runner_id: "runner".into(),
            }),
            runtime: Some(pb::ControlPlaneRuntime {
                opms_base_url: "https://api.us3.datadoghq.com".into(),
                task_concurrency: 9,
                opms_extra_headers: HashMap::from([("X-Test".into(), "secret-header".into())]),
                opms_proxy_url: "http://user:secret-password@proxy:3128".into(),
                skip_ssl_validation: true,
                min_tls_version: "tlsv1.3".into(),
            }),
        }
    }

    fn config(snapshot: pb::GetControlPlaneConfigResponse) -> Result<Config> {
        BootstrapConfig::new(snapshot).into_config("/launch.sock".into(), "/launch/cert.pem".into())
    }

    #[test]
    fn uses_bootstrap_runtime_and_launch_paths() {
        let bootstrap = BootstrapConfig::new(snapshot());
        assert!(bootstrap.split_mode());
        assert_eq!(bootstrap.log_level(), log::LevelFilter::Debug);
        let config = config(snapshot()).unwrap();
        assert_eq!(config.opms_base_url, "https://api.us3.datadoghq.com");
        assert_eq!(
            config.opms_proxy,
            ProxyDecision::ViaProxy("http://user:secret-password@proxy:3128".into())
        );
        assert_eq!(config.task_concurrency, 9);
        assert_eq!(config.executor_socket, PathBuf::from("/launch.sock"));
        assert_eq!(config.ipc_cert_file, PathBuf::from("/launch/cert.pem"));
        assert_eq!(config.opms_extra_headers["X-Test"], "secret-header");
        assert!(config.tls.skip_ssl_validation);
        assert_eq!(config.tls.min_tls_version, "tlsv1.3");
        assert_eq!(config.runner_version, crate::agent_version());
        assert_eq!(config.identity.org_id, 42);
        assert_eq!(config.identity.private_key, "secret-key");
    }

    #[test]
    fn validates_snapshot_without_exposing_credentials() {
        let mut invalid = snapshot();
        invalid.runtime = None;
        assert!(
            config(invalid)
                .err()
                .unwrap()
                .to_string()
                .contains("missing runtime")
        );
        let mut invalid = snapshot();
        invalid.identity = None;
        assert!(
            config(invalid)
                .err()
                .unwrap()
                .to_string()
                .contains("missing identity")
        );
        for concurrency in [0, -1] {
            let mut invalid = snapshot();
            invalid.runtime.as_mut().unwrap().task_concurrency = concurrency;
            assert!(
                config(invalid)
                    .err()
                    .unwrap()
                    .to_string()
                    .contains("task_concurrency")
            );
        }
        let debug = format!("{:?}", BootstrapConfig::new(snapshot()));
        for secret in ["secret-key", "secret-header", "secret-password"] {
            assert!(!debug.contains(secret));
        }
    }

    #[test]
    fn preserves_go_proxy_bypass_decision() {
        let mut response = snapshot();
        response.runtime.as_mut().unwrap().opms_proxy_url.clear();
        assert_eq!(config(response).unwrap().opms_proxy, ProxyDecision::None);
    }

    #[test]
    fn rejects_empty_launch_paths() {
        for (socket, cert, flag) in [
            ("", "/cert.pem", "--executor-socket"),
            ("/executor.sock", "", "--ipc-cert-file"),
        ] {
            let error = BootstrapConfig::new(snapshot())
                .into_config(socket.into(), cert.into())
                .err()
                .unwrap();
            assert!(error.to_string().contains(flag));
        }
    }

    #[test]
    fn disabled_response_needs_no_identity() {
        let bootstrap = BootstrapConfig::new(pb::GetControlPlaneConfigResponse::default());
        assert!(!bootstrap.split_mode());
    }
}
