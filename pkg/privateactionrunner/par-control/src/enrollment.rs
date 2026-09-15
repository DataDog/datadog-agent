// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, ensure};
use datadog_agent_commons::ipc::config::RemoteAgentClientConfiguration;

#[derive(Clone, Debug, serde::Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Enrollment {
    pub urn: String,
    pub private_key: String,
    pub org_id: i64,
    pub runner_id: String,
    pub agent_version: String,
}

pub async fn ensure(ipc: &RemoteAgentClientConfiguration) -> Result<Enrollment> {
    let tls = datadog_agent_commons::ipc::tls::build_ipc_client_ipc_tls_config(
        ipc.auth.ipc_cert_file_path(),
    )
    .await
    .context("building Core Agent IPC TLS configuration")?;
    let token = std::fs::read_to_string(ipc.auth.auth_token_file_path())
        .context("reading Core Agent IPC auth token")?;
    let client = reqwest::Client::builder()
        .no_proxy()
        .use_preconfigured_tls(tls)
        .build()
        .context("building Core Agent IPC client")?;
    let response = client
        .post(format!(
            "https://127.0.0.1:{}/agent/private-action-runner/ensure-enrollment",
            ipc.cmd_port
        ))
        .bearer_auth(token)
        .send()
        .await
        .context("calling Core Agent enrollment endpoint")?;
    ensure!(
        response.status().is_success(),
        "Core Agent enrollment endpoint returned {}",
        response.status()
    );

    let body = response
        .bytes()
        .await
        .context("reading Core Agent enrollment response")?;
    decode_response(&body)
}

fn decode_response(body: &[u8]) -> Result<Enrollment> {
    let enrollment: Enrollment =
        serde_json::from_slice(body).context("decoding Core Agent enrollment response")?;
    for (name, value) in [
        ("urn", enrollment.urn.as_str()),
        ("private_key", enrollment.private_key.as_str()),
        ("runner_id", enrollment.runner_id.as_str()),
        ("agent_version", enrollment.agent_version.as_str()),
    ] {
        ensure!(!value.is_empty(), "enrollment response is missing {name}");
    }
    ensure!(
        enrollment.org_id > 0,
        "enrollment response is missing org_id"
    );
    Ok(enrollment)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn valid_body() -> serde_json::Value {
        json!({
            "urn": "urn",
            "private_key": "key",
            "org_id": 42,
            "runner_id": "runner",
            "agent_version": "7.83.0",
        })
    }

    #[test]
    fn decodes_a_valid_response() {
        let enrollment = decode_response(valid_body().to_string().as_bytes()).unwrap();

        assert_eq!(enrollment.urn, "urn");
        assert_eq!(enrollment.private_key, "key");
        assert_eq!(enrollment.org_id, 42);
        assert_eq!(enrollment.runner_id, "runner");
        assert_eq!(enrollment.agent_version, "7.83.0");
    }

    #[test]
    fn rejects_malformed_json() {
        let error = decode_response(b"not json").unwrap_err().to_string();
        assert!(error.contains("decoding"));
    }

    #[test]
    fn rejects_unknown_fields() {
        let mut body = valid_body();
        body["extra"] = json!("surprise");
        assert!(decode_response(body.to_string().as_bytes()).is_err());
    }

    #[test]
    fn rejects_missing_org_id() {
        let mut body = valid_body();
        body["org_id"] = json!(0);
        let error = decode_response(body.to_string().as_bytes())
            .unwrap_err()
            .to_string();
        assert!(error.contains("org_id"));
    }

    #[test]
    fn rejects_empty_string_fields() {
        for field in ["urn", "private_key", "runner_id", "agent_version"] {
            let mut body = valid_body();
            body[field] = json!("");
            let error = decode_response(body.to_string().as_bytes())
                .unwrap_err()
                .to_string();
            assert!(error.contains(field), "{field}: {error}");
        }
    }
}
