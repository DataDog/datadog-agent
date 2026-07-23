// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! ES256 JWT minting for OPMS authentication. This is the only cryptography the
//! control plane performs: each OPMS request carries an `X-Datadog-OnPrem-JWT`
//! header with a short-lived (exp = iat + 60s) token signed by the runner key,
//! claims `{orgId, runnerId, iat, exp}`, mirroring `util.GeneratePARJWT` on the
//! Go side.

use anyhow::{Context, Result, bail};
use base64::Engine;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use p256::ecdsa::{Signature, SigningKey, signature::Signer};

/// HTTP header OPMS expects the signed JWT in.
pub const JWT_HEADER_NAME: &str = "X-Datadog-OnPrem-JWT";

/// Mints signed JWTs for OPMS requests. A trait so orchestration/OPMS tests can
/// inject a fake without real keys (PRD testing seam 2).
pub trait JwtSigner: Send + Sync {
    fn sign(&self) -> Result<String>;
}

/// Real ES256 signer backed by the runner's ECDSA P-256 key.
pub struct Es256Signer {
    org_id: i64,
    runner_id: String,
    key: SigningKey,
}

impl Es256Signer {
    /// Build a signer from the persisted runner key. The Go enrollment stores the
    /// key as `base64url(JSON JWK)` (see `util.Base64ToJWK`), so we base64url-decode
    /// it, parse the EC JWK, and build a P-256 signing key from the private scalar `d`.
    pub fn new(org_id: i64, runner_id: String, encoded_private_key: &str) -> Result<Self> {
        let key = parse_jwk_key(encoded_private_key)?;
        Ok(Es256Signer {
            org_id,
            runner_id,
            key,
        })
    }
}

fn parse_jwk_key(encoded: &str) -> Result<SigningKey> {
    #[derive(serde::Deserialize)]
    struct Jwk {
        kty: String,
        #[serde(default)]
        crv: String,
        /// Base64url-encoded private scalar.
        d: String,
    }

    let json = URL_SAFE_NO_PAD
        .decode(encoded.trim())
        .context("base64url-decoding the runner private key")?;
    let jwk: Jwk = serde_json::from_slice(&json).context("parsing the runner private key JWK")?;
    if jwk.kty != "EC" {
        bail!("unexpected runner key type {:?}, want EC", jwk.kty);
    }
    if !jwk.crv.is_empty() && jwk.crv != "P-256" {
        bail!("unexpected runner key curve {:?}, want P-256", jwk.crv);
    }
    let scalar = URL_SAFE_NO_PAD
        .decode(jwk.d)
        .context("decoding the EC private scalar")?;
    SigningKey::from_slice(&scalar).context("building the P-256 signing key")
}

impl JwtSigner for Es256Signer {
    fn sign(&self) -> Result<String> {
        let now = chrono::Utc::now().timestamp();
        let header = serde_json::json!({"alg": "ES256", "typ": "JWT", "cty": "JWT"});
        let claims = serde_json::json!({
            "orgId": self.org_id,
            "runnerId": self.runner_id,
            "iat": now,
            "exp": now + 60,
        });

        let signing_input = format!(
            "{}.{}",
            URL_SAFE_NO_PAD.encode(serde_json::to_vec(&header)?),
            URL_SAFE_NO_PAD.encode(serde_json::to_vec(&claims)?),
        );

        // p256's Signer hashes with SHA-256 (ES256) and yields a fixed-width
        // r||s signature, which is exactly the JWS ES256 encoding.
        let signature: Signature = self.key.sign(signing_input.as_bytes());
        let sig_b64 = URL_SAFE_NO_PAD.encode(signature.to_bytes());

        Ok(format!("{signing_input}.{sig_b64}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Deterministic fake for orchestration/OPMS tests.
    pub struct StaticSigner(pub String);
    impl JwtSigner for StaticSigner {
        fn sign(&self) -> Result<String> {
            Ok(self.0.clone())
        }
    }

    #[test]
    fn static_signer_returns_token() {
        let s = StaticSigner("fake.jwt.token".to_string());
        assert_eq!(s.sign().unwrap(), "fake.jwt.token");
    }

    #[test]
    fn signs_and_produces_three_segments() {
        // Fixed 32-byte scalar (valid P-256 key) encoded as a base64url JWK, the
        // same shape Go persists.
        let d = [0x11u8; 32];
        let jwk = format!(
            r#"{{"kty":"EC","crv":"P-256","d":"{}"}}"#,
            URL_SAFE_NO_PAD.encode(d)
        );
        let encoded = URL_SAFE_NO_PAD.encode(jwk);

        let signer = Es256Signer::new(7, "runner-x".to_string(), &encoded).unwrap();
        let token = signer.sign().unwrap();
        assert_eq!(token.split('.').count(), 3);
    }

    #[test]
    fn rejects_non_ec_key() {
        let encoded = URL_SAFE_NO_PAD.encode(r#"{"kty":"RSA","d":"AA"}"#);
        assert!(Es256Signer::new(1, "r".to_string(), &encoded).is_err());
    }
}
