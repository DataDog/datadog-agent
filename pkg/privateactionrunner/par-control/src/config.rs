// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::opms::TlsConfig;
use anyhow::{Context, Result, ensure};
use saluki_config::GenericConfiguration;
use std::collections::HashMap;
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
const INTERNAL_USE_DD_URL_FOR_OPMS: &str = "DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS";

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
    pub opms_no_proxy: Option<String>,
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

#[derive(serde::Deserialize)]
struct AgentParConfig {
    enabled: bool,
    split_enabled: bool,
    task_concurrency: usize,
    executor: AgentExecutorConfig,
    opms_extra_headers: HashMap<String, String>,
}

#[derive(serde::Deserialize)]
struct AgentExecutorConfig {
    socket_path: PathBuf,
}

#[derive(serde::Deserialize, Default)]
#[serde(default)]
struct AgentProxyConfig {
    http: String,
    https: String,
    no_proxy: Vec<String>,
}

#[derive(serde::Deserialize, Debug, Default, Clone)]
#[serde(default, deny_unknown_fields)]
pub struct BootstrapConfig {
    pub split_mode: bool,
    pub log_level: String,
    identity: BootstrapIdentity,
    agent_version: String,
    pub cmd_port: u16,
    pub auth_token_file_path: String,
    pub ipc_cert_file_path: String,
}

#[derive(serde::Deserialize, Debug, Default, Clone)]
#[serde(default, deny_unknown_fields)]
struct BootstrapIdentity {
    urn: String,
    private_key: String,
    org_id: i64,
    runner_id: String,
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

    pub fn into_config(self, agent: &GenericConfiguration) -> Result<Config> {
        ensure!(
            self.split_mode,
            "bootstrap configuration has split mode disabled"
        );
        for (name, value) in [
            ("log_level", self.log_level.as_str()),
            ("identity.urn", self.identity.urn.as_str()),
            ("identity.private_key", self.identity.private_key.as_str()),
            ("identity.runner_id", self.identity.runner_id.as_str()),
            ("agent_version", self.agent_version.as_str()),
            ("ipc_cert_file_path", self.ipc_cert_file_path.as_str()),
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
        ensure!(
            self.cmd_port > 0,
            "bootstrap configuration is missing cmd_port"
        );

        let par: AgentParConfig = agent
            .get_typed("private_action_runner")
            .context("invalid private_action_runner configuration from the Core Agent")?;
        ensure!(par.enabled, "private_action_runner.enabled is disabled");
        ensure!(
            par.split_enabled,
            "private_action_runner.split_enabled is disabled"
        );
        ensure!(
            par.task_concurrency > 0,
            "private_action_runner.task_concurrency must be greater than zero"
        );
        ensure!(
            !par.executor.socket_path.as_os_str().is_empty(),
            "private_action_runner.executor.socket_path is empty"
        );

        let opms_base_url = opms_base_url(
            agent,
            std::env::var(INTERNAL_USE_DD_URL_FOR_OPMS).as_deref() == Ok("true"),
        )?;
        let (opms_proxy_url, opms_no_proxy) = proxy_for(agent, &opms_base_url)?;
        let min_tls_version: String = agent
            .get_typed("min_tls_version")
            .context("invalid min_tls_version configuration from the Core Agent")?;
        ensure!(!min_tls_version.is_empty(), "min_tls_version is empty");

        Ok(Config {
            opms_base_url,
            task_concurrency: par.task_concurrency,
            executor_socket: par.executor.socket_path,
            procmgr_socket: dd_procmgr_client::ipc_path(),
            executor_process_name: EXECUTOR_PROCESS_NAME.to_string(),
            loop_interval: LOOP_INTERVAL,
            heartbeat_interval: HEARTBEAT_INTERVAL,
            health_check_interval: HEALTH_CHECK_INTERVAL,
            ready_timeout: EXECUTOR_READY_TIMEOUT,
            opms_request_timeout: OPMS_REQUEST_TIMEOUT,
            opms_extra_headers: par.opms_extra_headers,
            opms_proxy_url,
            opms_no_proxy,
            tls: TlsConfig {
                skip_ssl_validation: agent
                    .get_typed("skip_ssl_validation")
                    .context("invalid skip_ssl_validation configuration from the Core Agent")?,
                min_tls_version,
            },
            min_backoff: MIN_BACKOFF,
            max_backoff: MAX_BACKOFF,
            wait_before_retry: WAIT_BEFORE_RETRY,
            max_attempts: MAX_ATTEMPTS,
            runner_version: self.agent_version,
            modes: vec!["pull".to_string()],
            ipc_cert_file: self.ipc_cert_file_path.into(),
            identity: Identity {
                urn: self.identity.urn,
                private_key: self.identity.private_key,
                org_id: self.identity.org_id,
                runner_id: self.identity.runner_id,
            },
        })
    }
}

fn opms_base_url(agent: &GenericConfiguration, use_dd_url: bool) -> Result<String> {
    if use_dd_url {
        let dd_url: String = agent
            .get_typed("dd_url")
            .context("invalid dd_url configuration from the Core Agent")?;
        let parsed = reqwest::Url::parse(&dd_url).context("invalid dd_url")?;
        ensure!(
            matches!(parsed.scheme(), "http" | "https"),
            "dd_url must use HTTP or HTTPS"
        );
        ensure!(parsed.host_str().is_some(), "dd_url has no host");
        return Ok(parsed.origin().ascii_serialization());
    }

    let site: String = agent
        .get_typed("site")
        .context("invalid site configuration from the Core Agent")?;
    let site = site.trim();
    ensure!(!site.is_empty(), "site is empty");
    Ok(format!("https://api.{site}"))
}

fn proxy_for(
    agent: &GenericConfiguration,
    target: &str,
) -> Result<(Option<String>, Option<String>)> {
    let proxy: AgentProxyConfig = agent
        .get_typed("proxy")
        .context("invalid proxy configuration from the Core Agent")?;
    let target = reqwest::Url::parse(target).context("invalid OPMS URL")?;
    let proxy_url = match target.scheme() {
        "http" => proxy.http,
        "https" => proxy.https,
        scheme => anyhow::bail!("unsupported OPMS URL scheme {scheme}"),
    };
    if proxy_url.is_empty() {
        return Ok((None, None));
    }

    let nonexact: bool = agent
        .get_typed("no_proxy_nonexact_match")
        .context("invalid no_proxy_nonexact_match configuration from the Core Agent")?;
    if nonexact {
        return Ok((Some(proxy_url), Some(proxy.no_proxy.join(","))));
    }

    let host = match target.port() {
        Some(port) => format!("{}:{port}", target.host_str().unwrap_or_default()),
        None => target.host_str().unwrap_or_default().to_string(),
    };
    if proxy.no_proxy.iter().any(|entry| entry == &host) {
        Ok((None, None))
    } else {
        Ok((Some(proxy_url), None))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use saluki_config::{
        ConfigurationLoader,
        dynamic::{ConfigSetting, ConfigUpdate},
    };
    use serde_json::json;
    use tokio::sync::mpsc;

    const JSON: &str = r#"{
        "split_mode": true,
        "log_level": "debug",
        "identity": {"urn":"urn","private_key":"key","org_id":42,"runner_id":"runner"},
        "agent_version":"7.76.0",
        "cmd_port":5001,
        "auth_token_file_path":"/etc/datadog-agent/auth_token",
        "ipc_cert_file_path":"/etc/datadog-agent/auth/cert.pem"
    }"#;

    async fn agent_config(task_concurrency: usize) -> GenericConfiguration {
        agent_config_with_proxy(task_concurrency, json!([]), false).await
    }

    async fn agent_config_with_proxy(
        task_concurrency: usize,
        no_proxy: serde_json::Value,
        nonexact: bool,
    ) -> GenericConfiguration {
        let settings = [
            ConfigSetting::explicit("private_action_runner.enabled", json!(true)),
            ConfigSetting::explicit("private_action_runner.split_enabled", json!(true)),
            ConfigSetting::explicit(
                "private_action_runner.task_concurrency",
                json!(task_concurrency),
            ),
            ConfigSetting::explicit(
                "private_action_runner.executor.socket_path",
                json!("/from-agent.sock"),
            ),
            ConfigSetting::explicit(
                "private_action_runner.opms_extra_headers",
                json!({"X-Test": "agent"}),
            ),
            ConfigSetting::explicit("site", json!("datadoghq.com")),
            ConfigSetting::explicit("dd_url", json!("http://fakeintake:8080/path")),
            ConfigSetting::explicit(
                "proxy",
                json!({"http": "", "https": "http://proxy:3128", "no_proxy": no_proxy}),
            ),
            ConfigSetting::explicit("no_proxy_nonexact_match", json!(nonexact)),
            ConfigSetting::explicit("skip_ssl_validation", json!(true)),
            ConfigSetting::explicit("min_tls_version", json!("tlsv1.3")),
        ];
        let (sender, receiver) = mpsc::channel(1);
        sender.send(ConfigUpdate::snapshot(settings)).await.unwrap();

        let config = ConfigurationLoader::default()
            .with_dynamic_configuration(receiver)
            .into_generic()
            .await
            .unwrap();
        config.ready().await;
        config
    }

    #[tokio::test]
    async fn combines_bootstrap_and_core_agent_configuration() {
        let bootstrap: BootstrapConfig = serde_json::from_str(JSON).unwrap();
        assert!(bootstrap.split_mode);
        assert_eq!(bootstrap.log_level(), log::LevelFilter::Debug);

        let config = bootstrap.into_config(&agent_config(9).await).unwrap();

        assert_eq!(config.opms_base_url, "https://api.datadoghq.com");
        assert_eq!(config.opms_proxy_url.as_deref(), Some("http://proxy:3128"));
        assert_eq!(config.task_concurrency, 9);
        assert_eq!(config.executor_socket, PathBuf::from("/from-agent.sock"));
        assert_eq!(config.opms_extra_headers["X-Test"], "agent");
        assert!(config.tls.skip_ssl_validation);
        assert_eq!(config.tls.min_tls_version, "tlsv1.3");
        assert_eq!(config.loop_interval, Duration::from_secs(1));
        assert_eq!(config.identity.org_id, 42);
    }

    #[tokio::test]
    async fn rejects_invalid_core_agent_configuration() {
        let bootstrap: BootstrapConfig = serde_json::from_str(JSON).unwrap();
        let error = bootstrap
            .into_config(&agent_config(0).await)
            .err()
            .unwrap()
            .to_string();

        assert!(error.contains("task_concurrency"));
    }

    #[tokio::test]
    async fn applies_proxy_bypass_modes() {
        let exact = agent_config_with_proxy(5, json!(["api.datadoghq.com"]), false).await;
        assert_eq!(
            proxy_for(&exact, "https://api.datadoghq.com").unwrap(),
            (None, None)
        );

        let nonexact = agent_config_with_proxy(5, json!(["datadoghq.com"]), true).await;
        assert_eq!(
            proxy_for(&nonexact, "https://api.datadoghq.com").unwrap(),
            (
                Some("http://proxy:3128".to_string()),
                Some("datadoghq.com".to_string())
            )
        );
    }

    #[tokio::test]
    async fn uses_dd_url_for_internal_tests() {
        assert_eq!(
            opms_base_url(&agent_config(5).await, true).unwrap(),
            "http://fakeintake:8080"
        );
    }

    #[test]
    fn rejects_unknown_bootstrap_fields() {
        let json = JSON.replace("\"split_mode\": true", "\"unknown\": true");
        assert!(serde_json::from_str::<BootstrapConfig>(&json).is_err());
    }
}
