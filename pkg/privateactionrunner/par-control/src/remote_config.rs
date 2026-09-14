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
use tokio::sync::mpsc;

const RETRY_INTERVAL: Duration = Duration::from_secs(5);

pub async fn load(bootstrap: &BootstrapConfig) -> Result<GenericConfiguration> {
    let ipc_config = RemoteAgentClientConfiguration {
        cmd_port: bootstrap.cmd_port,
        auth: IpcAuthConfiguration::new(
            bootstrap.auth_token_file_path.clone().into(),
            bootstrap.ipc_cert_file_path.clone().into(),
        ),
        grpc_max_message_size: 128 * 1024 * 1024,
        #[cfg(target_os = "linux")]
        vsock_cid: None,
    };
    let mut client = RemoteAgentClient::connect(&ipc_config)
        .await
        .context("failed to connect to the Core Agent configuration stream")?;

    let session_id = SessionIdHandle::empty();
    let refresh_interval = register(&mut client, &session_id).await?;
    tokio::spawn(maintain_registration(
        client.clone(),
        session_id.clone(),
        refresh_interval,
    ));

    let (sender, receiver) = mpsc::channel(100);
    tokio::spawn(stream_config(client, session_id, sender));

    let config = ConfigurationLoader::default()
        .with_dynamic_configuration(receiver)
        .into_generic()
        .await?;
    config.ready().await;
    Ok(config)
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
) {
    loop {
        let current = session_id.wait_for_update().await;
        let mut stream = client.stream_config_events(&current);

        while let Some(result) = stream.next().await {
            let update = match result {
                Ok(event) => match event.event {
                    Some(config_event::Event::Snapshot(snapshot)) => {
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
