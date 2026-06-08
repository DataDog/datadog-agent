// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Remote-config poller for system-probe-lite.
//!
//! Connects to the agent's AgentSecure gRPC endpoint, polls the AGENT_CONFIG
//! product, and reports when `dynamic_instrumentation_enabled` becomes true so
//! the caller can transition into a full system-probe. The connection (TLS with
//! the IPC certificate as the trust root, plus a Bearer auth token) mirrors how
//! the Go system-probe's rcclient connects to the same endpoint.
//!
//! Validation note: the exact TLS server-name matching against the IPC
//! certificate, and the precise ClientGetConfigsRequest fields the agent
//! requires to return AGENT_CONFIG, must be confirmed against a running agent.

use std::time::Duration;

use anyhow::{Context, Result, anyhow};
use log::{debug, info, warn};
use serde::Deserialize;
use tokio::time::sleep;
use tonic::transport::{Certificate, Channel, ClientTlsConfig};

use crate::rc::datadog::api::v1::agent_secure_client::AgentSecureClient;
use crate::rc::datadog::config::{
    Client, ClientAgent, ClientGetConfigsRequest, ClientGetConfigsResponse, ClientState,
};

const AGENT_CONFIG_PRODUCT: &str = "AGENT_CONFIG";
const POLL_INTERVAL: Duration = Duration::from_secs(5);

/// Connection parameters for the agent's remote-config gRPC endpoint, passed in
/// by the Go system-probe when it execs into system-probe-lite.
pub struct RcConfig {
    pub ipc_address: String,
    pub ipc_port: String,
    pub auth_token_path: String,
    pub ipc_cert_path: String,
}

impl RcConfig {
    /// Returns true if every parameter needed to poll remote config is present.
    pub fn is_complete(&self) -> bool {
        !self.ipc_address.is_empty()
            && !self.ipc_port.is_empty()
            && self.ipc_port != "-1"
            && !self.auth_token_path.is_empty()
            && !self.ipc_cert_path.is_empty()
    }
}

#[derive(Deserialize)]
struct AgentConfigFile {
    config: Option<AgentConfigContent>,
}

#[derive(Deserialize)]
struct AgentConfigContent {
    dynamic_instrumentation_enabled: Option<bool>,
}

/// run polls remote config until `dynamic_instrumentation_enabled` is true, then
/// calls `on_enabled` once and returns. Transient errors are logged and retried.
pub async fn run<F: FnOnce()>(cfg: RcConfig, on_enabled: F) {
    let auth_token = match std::fs::read_to_string(&cfg.auth_token_path) {
        Ok(token) => token.trim().to_string(),
        Err(e) => {
            warn!(
                "remote-config disabled: cannot read auth token {}: {e}",
                cfg.auth_token_path
            );
            return;
        }
    };
    let ca_pem = match std::fs::read(&cfg.ipc_cert_path) {
        Ok(bytes) => bytes,
        Err(e) => {
            warn!(
                "remote-config disabled: cannot read IPC cert {}: {e}",
                cfg.ipc_cert_path
            );
            return;
        }
    };
    let client_id = read_client_id();

    loop {
        match poll_once(&cfg, &auth_token, &ca_pem, &client_id).await {
            Ok(true) => {
                info!("remote config indicates dynamic instrumentation is enabled");
                on_enabled();
                return;
            }
            Ok(false) => {}
            Err(e) => debug!("remote-config poll failed (will retry): {e:#}"),
        }
        sleep(POLL_INTERVAL).await;
    }
}

async fn connect(cfg: &RcConfig, ca_pem: &[u8]) -> Result<AgentSecureClient<Channel>> {
    let tls = ClientTlsConfig::new()
        .ca_certificate(Certificate::from_pem(ca_pem))
        // The IPC certificate is issued for the agent's IPC address.
        .domain_name(cfg.ipc_address.clone());
    let channel = Channel::from_shared(format!("https://{}:{}", cfg.ipc_address, cfg.ipc_port))
        .context("invalid IPC endpoint")?
        .tls_config(tls)
        .context("invalid TLS config")?
        .connect_timeout(Duration::from_secs(2))
        .connect()
        .await
        .context("connect to agent remote-config endpoint")?;
    Ok(AgentSecureClient::new(channel))
}

async fn poll_once(
    cfg: &RcConfig,
    auth_token: &str,
    ca_pem: &[u8],
    client_id: &str,
) -> Result<bool> {
    let mut client = connect(cfg, ca_pem).await?;

    let message = ClientGetConfigsRequest {
        client: Some(Client {
            state: Some(ClientState {
                root_version: 1,
                ..Default::default()
            }),
            id: client_id.to_string(),
            products: vec![AGENT_CONFIG_PRODUCT.to_string()],
            is_agent: true,
            client_agent: Some(ClientAgent {
                name: "system-probe-lite".to_string(),
                version: env!("CARGO_PKG_VERSION").to_string(),
                ..Default::default()
            }),
            ..Default::default()
        }),
        cached_target_files: Vec::new(),
    };

    let mut request = tonic::Request::new(message);
    request.metadata_mut().insert(
        "authorization",
        format!("Bearer {auth_token}")
            .parse()
            .map_err(|e| anyhow!("invalid authorization metadata: {e}"))?,
    );

    let response = client.client_get_configs(request).await?.into_inner();
    Ok(is_dynamic_instrumentation_enabled(&response))
}

fn is_dynamic_instrumentation_enabled(response: &ClientGetConfigsResponse) -> bool {
    for path in &response.client_configs {
        if !path.contains("/AGENT_CONFIG/") {
            continue;
        }
        let Some(file) = response.target_files.iter().find(|f| &f.path == path) else {
            continue;
        };
        let parsed: AgentConfigFile = match serde_json::from_slice(&file.raw) {
            Ok(parsed) => parsed,
            Err(e) => {
                debug!("could not parse AGENT_CONFIG at {path}: {e}");
                continue;
            }
        };
        if parsed
            .config
            .and_then(|c| c.dynamic_instrumentation_enabled)
            == Some(true)
        {
            return true;
        }
    }
    false
}

fn read_client_id() -> String {
    std::fs::read_to_string("/proc/sys/kernel/random/uuid")
        .map(|s| s.trim().to_string())
        .unwrap_or_else(|_| "system-probe-lite".to_string())
}
