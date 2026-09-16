// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail, ensure};
use datadog_agent_commons::ipc::config::RemoteAgentClientConfiguration;
use std::fmt;

#[derive(Clone, PartialEq, Eq)]
pub struct RunnerIdentity {
    pub urn: String,
    pub private_key: String,
}

impl fmt::Debug for RunnerIdentity {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("RunnerIdentity")
            .field("urn", &self.urn)
            .field("private_key", &"<redacted>")
            .finish()
    }
}

#[derive(serde::Serialize)]
struct ResolveRequest {
    has_local_identity: bool,
}

#[derive(serde::Deserialize)]
#[serde(tag = "source", rename_all = "snake_case", deny_unknown_fields)]
enum ResolveResponse {
    Local,
    Provided { urn: String, private_key: String },
}

pub async fn resolve(
    ipc: &RemoteAgentClientConfiguration,
    configured_identity: Option<&RunnerIdentity>,
) -> Result<RunnerIdentity> {
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
    let request = serde_json::to_vec(&ResolveRequest {
        has_local_identity: configured_identity.is_some(),
    })
    .context("encoding Core Agent identity request")?;
    let response = client
        .post(format!(
            "{}agent/private-action-runner/resolve-identity",
            ipc.endpoint()
        ))
        .bearer_auth(token)
        .header(reqwest::header::CONTENT_TYPE, "application/json")
        .body(request)
        .send()
        .await
        .context("calling Core Agent identity endpoint")?;
    let status = response.status();
    if !status.is_success() {
        let detail = response.text().await.unwrap_or_default().trim().to_string();
        if detail.is_empty() {
            bail!("Core Agent identity endpoint returned {status}");
        }
        bail!("Core Agent identity endpoint returned {status}: {detail}");
    }

    let body = response
        .bytes()
        .await
        .context("reading Core Agent identity response")?;
    decode_response(&body, configured_identity)
}

fn decode_response(
    body: &[u8],
    configured_identity: Option<&RunnerIdentity>,
) -> Result<RunnerIdentity> {
    let response: ResolveResponse =
        serde_json::from_slice(body).context("decoding Core Agent identity response")?;
    let identity = match response {
        ResolveResponse::Local => {
            let configured = configured_identity
                .context("Core Agent selected a local identity that is not available")?;
            RunnerIdentity {
                urn: configured.urn.clone(),
                private_key: configured.private_key.clone(),
            }
        }
        ResolveResponse::Provided { urn, private_key } => RunnerIdentity { urn, private_key },
    };
    for (name, value) in [
        ("urn", identity.urn.as_str()),
        ("private_key", identity.private_key.as_str()),
    ] {
        ensure!(!value.is_empty(), "resolved identity is missing {name}");
    }
    Ok(identity)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn configured_identity() -> RunnerIdentity {
        RunnerIdentity {
            urn: "runner-urn".to_string(),
            private_key: "secret-key".to_string(),
        }
    }

    fn provided_body() -> serde_json::Value {
        json!({
            "source": "provided",
            "urn": "provided-urn",
            "private_key": "provided-key",
        })
    }

    #[test]
    fn serializes_only_the_local_identity_signal() {
        let identity = configured_identity();
        let encoded = serde_json::to_value(ResolveRequest {
            has_local_identity: true,
        })
        .unwrap();

        assert_eq!(encoded, json!({"has_local_identity": true}));
        assert!(!encoded.to_string().contains(&identity.urn));
        assert!(!encoded.to_string().contains("secret-key"));
        assert!(!format!("{identity:?}").contains("secret-key"));
    }

    #[test]
    fn uses_the_local_identity_when_selected() {
        let configured = configured_identity();
        let identity = decode_response(br#"{"source":"local"}"#, Some(&configured)).unwrap();

        assert_eq!(identity.urn, configured.urn);
        assert_eq!(identity.private_key, configured.private_key);
    }

    #[test]
    fn decodes_a_provided_identity() {
        let identity = decode_response(provided_body().to_string().as_bytes(), None).unwrap();

        assert_eq!(identity.urn, "provided-urn");
        assert_eq!(identity.private_key, "provided-key");
    }

    #[test]
    fn rejects_local_selection_without_a_local_identity() {
        let error = decode_response(br#"{"source":"local"}"#, None)
            .unwrap_err()
            .to_string();
        assert!(error.contains("not available"));
    }

    #[test]
    fn rejects_malformed_json() {
        let error = decode_response(b"not json", None).unwrap_err().to_string();
        assert!(error.contains("decoding"));
    }

    #[test]
    fn rejects_unknown_fields() {
        let mut body = provided_body();
        body["extra"] = json!("surprise");
        assert!(decode_response(body.to_string().as_bytes(), None).is_err());
    }

    #[test]
    fn rejects_empty_provided_identity_fields() {
        for field in ["urn", "private_key"] {
            let mut body = provided_body();
            body[field] = json!("");
            let error = decode_response(body.to_string().as_bytes(), None)
                .unwrap_err()
                .to_string();
            assert!(error.contains(field), "{field}: {error}");
        }
    }

    #[test]
    fn rejects_partial_local_identity_when_selected() {
        let configured = RunnerIdentity {
            urn: "runner-urn".to_string(),
            private_key: String::new(),
        };
        let error = decode_response(br#"{"source":"local"}"#, Some(&configured))
            .unwrap_err()
            .to_string();
        assert!(error.contains("private_key"));
    }
}
