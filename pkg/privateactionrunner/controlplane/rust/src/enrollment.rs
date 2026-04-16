// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Self-enrollment for par-control.
//!
//! Mirrors pkg/privateactionrunner/enrollment/enrollment.go and
//! pkg/privateactionrunner/opms/public_opms.go.
//!
//! Flow:
//!   1. Generate a fresh P-256 key pair.
//!   2. POST /api/unstable/on_prem_runners with DD-API-KEY auth.
//!   3. Parse the response to get runner_id + org_id.
//!   4. Build the URN: urn:dd:apps:on-prem-runner:{region}:{org_id}:{runner_id}
//!   5. Return (urn, private_key_b64) — caller uses them to start OPMS polling.
//!
//! Identity is kept in memory only for the POC.
//! TODO: persist to file / K8s secret so restarts don't re-enroll.

use anyhow::{Context, Result};
use base64::Engine;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use p256::ecdsa::SigningKey;
use p256::pkcs8::EncodePublicKey;
use rand_core::OsRng;
use serde::Deserialize;
use serde_json::{json, Value};

const ENROLL_PATH: &str = "/api/unstable/on_prem_runners";
const VERSION_HEADER: &str = "X-Datadog-OnPrem-Version";
const RUNNER_VERSION: &str = env!("CARGO_PKG_VERSION");

/// Result of a successful enrollment.
pub struct EnrollmentResult {
    pub urn: String,
    /// base64url-encoded JWK private key — same format load_signing_key expects.
    pub private_key_b64: String,
    pub signing_key: SigningKey,
}

/// Enroll this runner instance with the Datadog API.
/// Returns the URN and signing key to use for OPMS authentication.
pub async fn enroll(
    http: &reqwest::Client,
    api_key: &str,
    app_key: &str,
    dd_api_host: &str,
    site: &str,
    hostname: &str,
) -> Result<EnrollmentResult> {
    // 1. Generate P-256 key pair.
    let secret_key = p256::SecretKey::random(&mut OsRng);
    let signing_key = SigningKey::from(&secret_key);

    // Public key PEM for the enrollment request.
    let public_key_pem = secret_key
        .public_key()
        .to_public_key_pem(Default::default())
        .context("failed to encode public key as PEM")?;

    // Private key as base64url-encoded JWK (same format as Go's EcdsaToJWK).
    let private_key_b64 = secret_key_to_jwk_b64(&secret_key);

    // 2. Build runner name: <hostname>-<timestamp>
    let runner_name = format!("{}-{}", hostname, chrono_now_str());

    // 3. POST /api/unstable/on_prem_runners  (JSON:API format)
    let body = json!({
        "data": {
            "type": "createRunnerRequest",
            "attributes": {
                "runner_name": runner_name,
                "runner_modes": ["pull"],
                "public_key_pem": public_key_pem,
                "agent_hostname": hostname,
                "agent_flavor": "agent",
            }
        }
    });

    let url = format!("https://{}{}", dd_api_host, ENROLL_PATH);

    let mut req = http
        .post(&url)
        .header("Content-Type", "application/vnd.api+json")
        .header("Accept", "application/json")
        .header("DD-API-KEY", api_key)
        .header(VERSION_HEADER, RUNNER_VERSION)
        .json(&body);

    if !app_key.is_empty() {
        req = req.header("DD-APPLICATION-KEY", app_key);
    }

    let resp = req.send().await.context("enrollment HTTP request failed")?;

    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        anyhow::bail!("enrollment failed: HTTP {status}: {body}");
    }

    // 4. Parse JSON:API response.
    let resp_json: Value = resp.json().await.context("enrollment: reading response")?;
    let attrs = resp_json
        .pointer("/data/attributes")
        .ok_or_else(|| anyhow::anyhow!("enrollment response missing /data/attributes"))?;

    let runner_id = attrs["runner_id"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("enrollment response missing runner_id"))?;
    let org_id: i64 = attrs["org_id"]
        .as_i64()
        .ok_or_else(|| anyhow::anyhow!("enrollment response missing org_id"))?;

    // 5. Build URN — mirrors util.MakeRunnerURN in Go.
    let region = region_from_site(site);
    let urn = format!("urn:dd:apps:on-prem-runner:{}:{}:{}", region, org_id, runner_id);

    log::info!("par-control: enrolled successfully, URN={}", urn);

    Ok(EnrollmentResult {
        urn,
        private_key_b64,
        signing_key,
    })
}

/// Convert a P-256 secret key to the base64url-encoded JWK format that
/// load_signing_key() (jwt.rs) expects.
fn secret_key_to_jwk_b64(key: &p256::SecretKey) -> String {
    use p256::elliptic_curve::sec1::ToEncodedPoint;
    let d = URL_SAFE_NO_PAD.encode(key.to_bytes().as_slice());
    let point = key.public_key().to_encoded_point(false);
    let x = URL_SAFE_NO_PAD.encode(point.x().expect("x coordinate").as_slice());
    let y = URL_SAFE_NO_PAD.encode(point.y().expect("y coordinate").as_slice());

    let jwk = json!({
        "kty": "EC",
        "crv": "P-256",
        "alg": "ES256",
        "use": "sig",
        "x": x,
        "y": y,
        "d": d,
    });
    URL_SAFE_NO_PAD.encode(serde_json::to_string(&jwk).unwrap_or_default().as_bytes())
}

/// Derive a Datadog region string from a site, mirroring regions.GetRegionFromDDSite.
pub fn region_from_site(site: &str) -> &str {
    match site {
        "datadoghq.com" => "us1",
        "datadoghq.eu" => "eu1",
        "us3.datadoghq.com" => "us3",
        "us5.datadoghq.com" => "us5",
        "ap1.datadoghq.com" => "ap1",
        "ddog-gov.com" => "us1-fed",
        _ => "us1",
    }
}

/// Simple timestamp string for runner naming (no chrono dependency).
fn chrono_now_str() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let secs = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0);
    format!("{}", secs)
}

/// Deserialize the persisted identity JSON file.
/// Format matches Go's PersistedIdentity struct.
#[derive(Deserialize)]
pub struct PersistedIdentity {
    pub private_key: String,
    pub urn: String,
}
