// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! ES256 JWT generation, mirroring pkg/privateactionrunner/util/jwt.go.
//!
//! The Go code generates:
//!   header: {"alg":"ES256","typ":"JWT","cty":"JWT"}
//!   claims: {"orgId":<i64>, "runnerId":<str>, "iat":<unix>, "exp":<unix+60>}
//!
//! The private key is stored as a base64url-encoded JWK in datadog.yaml.
//! JWK format (EC P-256): {"kty":"EC","crv":"P-256","x":"...","y":"...","d":"..."}
//! The "d" field is the private scalar, base64url-encoded.

use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use base64::Engine;
use base64::engine::general_purpose::{URL_SAFE_NO_PAD, STANDARD};
use p256::ecdsa::{SigningKey, signature::Signer};
use serde_json::json;

/// Parses the base64url-encoded JWK and returns the P-256 signing key.
/// Called once at startup; the key is reused for every JWT.
pub fn load_signing_key(private_key_b64: &str) -> Result<SigningKey> {
    // The stored value is base64url (no padding) encoded JWK JSON.
    let jwk_bytes = URL_SAFE_NO_PAD
        .decode(private_key_b64)
        .or_else(|_| STANDARD.decode(private_key_b64))
        .context("failed to base64-decode private key")?;

    let jwk: serde_json::Value = serde_json::from_slice(&jwk_bytes)
        .context("failed to parse JWK JSON")?;

    let d_b64 = jwk["d"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("JWK missing 'd' field"))?;

    let d_bytes = URL_SAFE_NO_PAD
        .decode(d_b64)
        .context("failed to base64url-decode JWK 'd' field")?;

    SigningKey::from_slice(&d_bytes)
        .context("failed to construct P-256 signing key from 'd' scalar")
}

/// Generates a signed ES256 JWT, mirroring GeneratePARJWT in jwt.go.
///
/// Header:  {"alg":"ES256","typ":"JWT","cty":"JWT"}
/// Claims:  {"orgId":<org_id>, "runnerId":<runner_id>, "iat":<now>, "exp":<now+60>}
/// Signature: ES256 over "<b64url_header>.<b64url_claims>" in IEEE P1363 format.
pub fn generate_jwt(org_id: i64, runner_id: &str, key: &SigningKey) -> Result<String> {
    let header = URL_SAFE_NO_PAD.encode(
        serde_json::to_string(&json!({"alg":"ES256","typ":"JWT","cty":"JWT"}))
            .context("failed to serialize JWT header")?
            .as_bytes(),
    );

    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .context("system clock before UNIX epoch")?
        .as_secs() as i64;

    let claims_json = serde_json::to_string(&json!({
        "orgId": org_id,
        "runnerId": runner_id,
        "iat": now,
        "exp": now + 60,
    }))
    .context("failed to serialize JWT claims")?;

    let claims = URL_SAFE_NO_PAD.encode(claims_json.as_bytes());

    let message = format!("{header}.{claims}");
    // p256::ecdsa::Signature::to_bytes() returns the IEEE P1363 representation
    // (r || s, 64 bytes) which is exactly what RFC 7518 §3.4 requires for ES256.
    let signature: p256::ecdsa::Signature = key.sign(message.as_bytes());
    let sig_bytes: Vec<u8> = signature.to_bytes().to_vec();
    let sig = URL_SAFE_NO_PAD.encode(&sig_bytes);

    Ok(format!("{message}.{sig}"))
}
