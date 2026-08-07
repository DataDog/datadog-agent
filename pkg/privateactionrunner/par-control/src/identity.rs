// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Runner identity: the persisted ECDSA P-256 key plus the URN it was enrolled
//! under. The control plane is a thin consumer of identity — all enrollment and
//! key generation stay in the Go code (bootstrap is a later slice); Rust only
//! reads what Go persisted.

use anyhow::{Context, Result, bail};
use std::path::Path;

/// Default identity filename the Go enrollment writes next to the config, matching
/// `defaultIdentityFileName` on the Go side.
pub const DEFAULT_IDENTITY_FILE_NAME: &str = "privateactionrunner_private_identity.json";

/// A parsed runner URN of the form
/// `urn:dd:apps:on-prem-runner:<region>:<orgId>:<runnerId>`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RunnerUrn {
    pub region: String,
    pub org_id: i64,
    pub runner_id: String,
}

impl RunnerUrn {
    /// Parse a runner URN, mirroring `util.ParseRunnerURN` on the Go side.
    pub fn parse(urn: &str) -> Result<Self> {
        let parts: Vec<&str> = urn.split(':').collect();
        if parts.len() != 7 {
            bail!("invalid URN format: {urn}");
        }
        let org_id = parts[5]
            .parse::<i64>()
            .with_context(|| format!("invalid orgId in URN: {}", parts[5]))?;
        Ok(RunnerUrn {
            region: parts[4].to_string(),
            org_id,
            runner_id: parts[6].to_string(),
        })
    }
}

#[derive(Clone)]
pub struct Identity {
    pub urn: String,
    pub org_id: i64,
    pub runner_id: String,
    /// The persisted private key exactly as Go stored it: `base64url(JSON JWK)`.
    pub private_key: String,
}

impl Identity {
    pub fn new(urn: String, private_key: String) -> Result<Self> {
        if private_key.trim().is_empty() {
            bail!("runner private key is empty; the runner is not enrolled");
        }
        let parsed = RunnerUrn::parse(&urn)?;
        Ok(Identity {
            urn,
            org_id: parsed.org_id,
            runner_id: parsed.runner_id,
            private_key,
        })
    }

    /// Load identity from the JSON file the Go enrollment persists
    /// (`{private_key, urn, ...}`). Returns `Ok(None)` if the file does not exist,
    /// so callers can fall back to bootstrap/enroll.
    pub fn from_file(path: &Path) -> Result<Option<Self>> {
        if !path.exists() {
            return Ok(None);
        }
        #[derive(serde::Deserialize)]
        struct PersistedIdentity {
            #[serde(default)]
            private_key: String,
            #[serde(default)]
            urn: String,
        }
        let data = std::fs::read(path)
            .with_context(|| format!("reading identity file {}", path.display()))?;
        let persisted: PersistedIdentity = serde_json::from_slice(&data)
            .with_context(|| format!("parsing identity file {}", path.display()))?;
        if persisted.urn.is_empty() || persisted.private_key.is_empty() {
            bail!(
                "identity file {} is missing urn or private_key",
                path.display()
            );
        }
        Ok(Some(Identity::new(persisted.urn, persisted.private_key)?))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_valid_urn() {
        let urn = "urn:dd:apps:on-prem-runner:us1:12345:runner-abc";
        let parsed = RunnerUrn::parse(urn).unwrap();
        assert_eq!(parsed.region, "us1");
        assert_eq!(parsed.org_id, 12345);
        assert_eq!(parsed.runner_id, "runner-abc");
    }

    #[test]
    fn rejects_malformed_urn() {
        assert!(RunnerUrn::parse("urn:dd:apps:on-prem-runner:us1:12345").is_err());
        assert!(RunnerUrn::parse("urn:dd:apps:on-prem-runner:us1:not-a-number:r").is_err());
    }

    #[test]
    fn reads_identity_from_file() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join(DEFAULT_IDENTITY_FILE_NAME);
        std::fs::write(
            &path,
            r#"{"private_key":"enc-key","urn":"urn:dd:apps:on-prem-runner:us1:9:r9","hostname":"h"}"#,
        )
        .unwrap();

        let id = Identity::from_file(&path).unwrap().unwrap();
        assert_eq!(id.org_id, 9);
        assert_eq!(id.runner_id, "r9");
        assert_eq!(id.private_key, "enc-key");
    }

    #[test]
    fn from_file_returns_none_when_absent() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("missing.json");
        assert!(Identity::from_file(&path).unwrap().is_none());
    }

    #[test]
    fn identity_requires_a_key() {
        let urn = "urn:dd:apps:on-prem-runner:us1:1:r".to_string();
        assert!(Identity::new(urn.clone(), "   ".to_string()).is_err());
        assert!(Identity::new(urn, "encoded-jwk-key".to_string()).is_ok());
    }
}
