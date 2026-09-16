// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::BootstrapConfig;
use anyhow::{Context, Result};
use datadog_agent_commons::ipc::{
    client::RemoteAgentClient,
    config::{IpcAuthConfiguration, RemoteAgentClientConfiguration},
    session::{SessionId, SessionIdHandle},
};
use datadog_protos::agent::{ConfigSetting as AgentConfigSetting, ConfigSnapshot, config_event};
use futures::StreamExt as _;
use prost_types::value::Kind;
use saluki_config::{
    ConfigurationLoader, GenericConfiguration,
    dynamic::{ConfigSetting, ConfigUpdate, Provenance},
};
use serde_json::{Map, Value};
use std::time::Duration;
use tokio::sync::{mpsc, oneshot};

const RETRY_INTERVAL: Duration = Duration::from_secs(5);

pub async fn load(bootstrap: &BootstrapConfig) -> Result<(GenericConfiguration, bool)> {
    let ipc_config = ipc_config(bootstrap);
    let session_id = SessionIdHandle::empty();
    let (client, refresh_interval) = connect_and_register(&ipc_config, &session_id).await;
    tokio::spawn(maintain_registration(
        client.clone(),
        session_id.clone(),
        refresh_interval,
    ));

    let (sender, receiver) = mpsc::channel(100);
    let (provenance_sender, provenance_receiver) = oneshot::channel();
    tokio::spawn(stream_config(client, session_id, sender, provenance_sender));

    let config = ConfigurationLoader::default()
        .with_dynamic_configuration(receiver)
        .into_generic()
        .await?;
    config.ready().await;
    let dd_url_explicit = provenance_receiver
        .await
        .context("configuration stream closed before its initial snapshot")?;
    Ok((config, dd_url_explicit))
}

fn ipc_config(bootstrap: &BootstrapConfig) -> RemoteAgentClientConfiguration {
    RemoteAgentClientConfiguration {
        cmd_port: bootstrap.cmd_port,
        auth: IpcAuthConfiguration::new(
            bootstrap.auth_token_file_path.clone().into(),
            bootstrap.ipc_cert_file_path.clone().into(),
        ),
        grpc_max_message_size: 128 * 1024 * 1024,
        #[cfg(target_os = "linux")]
        vsock_cid: None,
    }
}

async fn connect_and_register(
    ipc_config: &RemoteAgentClientConfiguration,
    session_id: &SessionIdHandle,
) -> (RemoteAgentClient, Duration) {
    loop {
        let mut client = match RemoteAgentClient::connect(ipc_config).await {
            Ok(client) => client,
            Err(error) => {
                log::warn!("Core Agent connection failed: {error:#}");
                tokio::time::sleep(RETRY_INTERVAL).await;
                continue;
            }
        };

        match register(&mut client, session_id).await {
            Ok(refresh_interval) => return (client, refresh_interval),
            Err(error) => log::warn!("Core Agent registration failed: {error:#}"),
        }
        tokio::time::sleep(RETRY_INTERVAL).await;
    }
}

async fn register(
    client: &mut RemoteAgentClient,
    session_id: &SessionIdHandle,
) -> Result<Duration> {
    let response = client
        .register_remote_agent(
            std::process::id(),
            "Private Action Runner",
            "private_action_runner",
            "https://configstream-consumer/par-control",
            Vec::new(),
        )
        .await?
        .into_inner();

    session_id.update(Some(SessionId::new(&response.session_id)?));
    Ok(Duration::from_secs(
        response.recommended_refresh_interval_secs.max(1).into(),
    ))
}

async fn maintain_registration(
    mut client: RemoteAgentClient,
    session_id: SessionIdHandle,
    mut refresh_interval: Duration,
) {
    loop {
        tokio::time::sleep(refresh_interval).await;

        if let Some(current) = session_id.get()
            && client.refresh_remote_agent(&current).await.is_ok()
        {
            continue;
        }
        session_id.update(None);

        loop {
            match register(&mut client, &session_id).await {
                Ok(interval) => {
                    refresh_interval = interval;
                    break;
                }
                Err(error) => {
                    log::warn!("Core Agent registration failed: {error:#}");
                    tokio::time::sleep(RETRY_INTERVAL).await;
                }
            }
        }
    }
}

async fn stream_config(
    mut client: RemoteAgentClient,
    session_id: SessionIdHandle,
    sender: mpsc::Sender<ConfigUpdate>,
    initial_provenance: oneshot::Sender<bool>,
) {
    let mut initial_provenance = Some(initial_provenance);
    loop {
        let current = session_id.wait_for_update().await;
        let mut stream = client.stream_config_events(&current);

        while let Some(result) = stream.next().await {
            let update = match result {
                Ok(event) => match event.event {
                    Some(config_event::Event::Snapshot(snapshot)) => {
                        if let Some(sender) = initial_provenance.take() {
                            let _ = sender.send(setting_is_explicit(&snapshot, "dd_url"));
                        }
                        Some(ConfigUpdate::Snapshot(snapshot_to_settings(&snapshot)))
                    }
                    Some(config_event::Event::Update(update)) => update
                        .setting
                        .as_ref()
                        .map(|setting| ConfigUpdate::Partial(setting_to_config_setting(setting))),
                    None => None,
                },
                Err(error) => {
                    log::warn!("Core Agent configuration stream failed: {error}");
                    break;
                }
            };

            if let Some(update) = update
                && sender.send(update).await.is_err()
            {
                return;
            }
        }

        tokio::time::sleep(RETRY_INTERVAL).await;
    }
}

fn setting_to_config_setting(setting: &AgentConfigSetting) -> ConfigSetting {
    let provenance = if matches!(setting.source.as_str(), "default" | "schema") {
        Provenance::Default
    } else {
        Provenance::Explicit
    };

    ConfigSetting::new(
        setting.key.clone(),
        proto_value_to_json(&setting.value),
        provenance,
    )
}

fn setting_is_explicit(snapshot: &ConfigSnapshot, key: &str) -> bool {
    snapshot.settings.iter().any(|setting| {
        setting.key == key && !matches!(setting.source.as_str(), "default" | "schema")
    })
}

fn snapshot_to_settings(snapshot: &ConfigSnapshot) -> Vec<ConfigSetting> {
    snapshot
        .settings
        .iter()
        .map(setting_to_config_setting)
        .collect()
}

fn proto_value_to_json(value: &Option<prost_types::Value>) -> Value {
    let Some(kind) = value.as_ref().and_then(|value| value.kind.as_ref()) else {
        return Value::Null;
    };

    match kind {
        Kind::NullValue(_) => Value::Null,
        Kind::NumberValue(number)
            if number.fract() == 0.0
                && *number >= i64::MIN as f64
                && *number <= i64::MAX as f64 =>
        {
            Value::from(*number as i64)
        }
        Kind::NumberValue(number) => Value::from(*number),
        Kind::StringValue(string) => Value::String(string.clone()),
        Kind::BoolValue(boolean) => Value::Bool(*boolean),
        Kind::StructValue(object) => Value::Object(
            object
                .fields
                .iter()
                .map(|(key, value)| (key.clone(), proto_value_to_json(&Some(value.clone()))))
                .collect::<Map<_, _>>(),
        ),
        Kind::ListValue(array) => Value::Array(
            array
                .values
                .iter()
                .map(|value| proto_value_to_json(&Some(value.clone())))
                .collect(),
        ),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolves_the_default_auth_token_path() {
        let bootstrap: BootstrapConfig =
            serde_json::from_str(r#"{"cmd_port":5001,"ipc_cert_file_path":"/tmp/ipc-cert.pem"}"#)
                .unwrap();

        assert!(
            !ipc_config(&bootstrap)
                .auth
                .auth_token_file_path()
                .as_os_str()
                .is_empty()
        );
    }

    #[test]
    fn detects_explicit_settings() {
        let snapshot = ConfigSnapshot {
            settings: vec![AgentConfigSetting {
                key: "dd_url".to_string(),
                source: "environment-variable".to_string(),
                value: None,
            }],
            ..Default::default()
        };

        assert!(setting_is_explicit(&snapshot, "dd_url"));
    }

    #[test]
    fn converts_integral_numbers_without_losing_their_type() {
        let value = Some(prost_types::Value {
            kind: Some(Kind::NumberValue(5.0)),
        });

        assert_eq!(proto_value_to_json(&value), Value::from(5));
    }

    #[test]
    fn maps_agent_defaults_to_default_provenance() {
        let setting = AgentConfigSetting {
            key: "private_action_runner.task_concurrency".to_string(),
            source: "default".to_string(),
            value: Some(prost_types::Value {
                kind: Some(Kind::NumberValue(5.0)),
            }),
        };

        let converted = setting_to_config_setting(&setting);
        assert_eq!(converted.key, "private_action_runner.task_concurrency");
        assert_eq!(converted.provenance, Provenance::Default);
    }
}
