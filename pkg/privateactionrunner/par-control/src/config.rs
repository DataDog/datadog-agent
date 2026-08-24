// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::opms::TlsConfig;
use std::collections::HashMap;
use std::path::PathBuf;
use std::time::Duration;

const EXECUTOR_READY_TIMEOUT: Duration = Duration::from_secs(30);
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
    pub opms_proxy_url: Option<String>,
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

#[derive(serde::Deserialize, Debug, Default, Clone)]
#[serde(default, deny_unknown_fields)]
pub struct BootstrapConfig {
    pub split_mode: bool,
    pub log_level: String,
    identity: BootstrapIdentity,
    opms_base_url: String,
    opms_proxy_url: String,
    agent_version: String,
    modes: Vec<String>,
    task_concurrency: usize,
    executor_socket: String,
    ipc_cert_file_path: String,
    opms_extra_headers: HashMap<String, String>,
    tls: BootstrapTls,
    loop_interval_milliseconds: u64,
    heartbeat_interval_milliseconds: u64,
    health_check_interval_milliseconds: u64,
    opms_request_timeout_milliseconds: u64,
    min_backoff_milliseconds: u64,
    max_backoff_milliseconds: u64,
    wait_before_retry_milliseconds: u64,
    max_attempts: u32,
}

#[derive(serde::Deserialize, Debug, Default, Clone)]
#[serde(default, deny_unknown_fields)]
struct BootstrapIdentity {
    urn: String,
    private_key: String,
    org_id: i64,
    runner_id: String,
}

#[derive(serde::Deserialize, Debug, Default, Clone)]
#[serde(default, deny_unknown_fields)]
struct BootstrapTls {
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

    pub fn into_config(self) -> Config {
        Config {
            opms_base_url: self.opms_base_url,
            task_concurrency: self.task_concurrency,
            executor_socket: self.executor_socket.into(),
            procmgr_socket: dd_procmgr_client::default_ipc_path(),
            executor_process_name: EXECUTOR_PROCESS_NAME.to_string(),
            loop_interval: Duration::from_millis(self.loop_interval_milliseconds),
            heartbeat_interval: Duration::from_millis(self.heartbeat_interval_milliseconds),
            health_check_interval: Duration::from_millis(self.health_check_interval_milliseconds),
            ready_timeout: EXECUTOR_READY_TIMEOUT,
            opms_request_timeout: Duration::from_millis(self.opms_request_timeout_milliseconds),
            opms_extra_headers: self.opms_extra_headers,
            opms_proxy_url: if self.opms_proxy_url.is_empty() {
                None
            } else {
                Some(self.opms_proxy_url)
            },
            tls: TlsConfig {
                skip_ssl_validation: self.tls.skip_ssl_validation,
                min_tls_version: self.tls.min_tls_version,
            },
            min_backoff: Duration::from_millis(self.min_backoff_milliseconds),
            max_backoff: Duration::from_millis(self.max_backoff_milliseconds),
            wait_before_retry: Duration::from_millis(self.wait_before_retry_milliseconds),
            max_attempts: self.max_attempts,
            runner_version: self.agent_version,
            modes: self.modes,
            ipc_cert_file: self.ipc_cert_file_path.into(),
            identity: Identity {
                urn: self.identity.urn,
                private_key: self.identity.private_key,
                org_id: self.identity.org_id,
                runner_id: self.identity.runner_id,
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const JSON: &str = r#"{
        "split_mode": true,
        "log_level": "debug",
        "identity": {"urn":"urn","private_key":"key","org_id":42,"runner_id":"runner"},
        "opms_base_url":"https://api.datadoghq.com",
        "task_concurrency":5,
        "loop_interval_milliseconds":1000,
        "heartbeat_interval_milliseconds":20000,
        "health_check_interval_milliseconds":30000,
        "opms_request_timeout_milliseconds":30000,
        "min_backoff_milliseconds":1000,
        "max_backoff_milliseconds":180000,
        "wait_before_retry_milliseconds":300000,
        "max_attempts":20
    }"#;

    #[test]
    fn maps_bootstrap_config() {
        let bootstrap: BootstrapConfig = serde_json::from_str(JSON).unwrap();
        assert!(bootstrap.split_mode);
        assert_eq!(bootstrap.log_level(), log::LevelFilter::Debug);

        let config = bootstrap.into_config();
        assert_eq!(config.task_concurrency, 5);
        assert_eq!(config.loop_interval, Duration::from_secs(1));
        assert_eq!(config.identity.org_id, 42);
    }

    #[test]
    fn rejects_unknown_fields() {
        let json = JSON.replace("\"split_mode\": true", "\"unknown\": true");
        assert!(serde_json::from_str::<BootstrapConfig>(&json).is_err());
    }
}
