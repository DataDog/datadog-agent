// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Runner identity: the private key and URN.
//!
//! Go owns enrollment, key generation, persistence, and URN parsing. Rust
//! receives the resolved identity from `bootstrap-par-control` and only checks
//! that it is complete, so the org ID and runner ID have exactly one source of
//! truth.

use anyhow::{Result, bail};

#[derive(Clone)]
pub struct Identity {
    pub urn: String,
    pub org_id: i64,
    pub runner_id: String,
    pub private_key: String,
}

impl Identity {
    pub fn new(urn: String, private_key: String, org_id: i64, runner_id: String) -> Result<Self> {
        if urn.trim().is_empty() {
            bail!("runner URN is empty; the runner is not enrolled");
        }
        if private_key.trim().is_empty() {
            bail!("runner private key is empty; the runner is not enrolled");
        }
        if org_id <= 0 {
            bail!("runner organization ID {org_id} is not positive");
        }
        if runner_id.trim().is_empty() {
            bail!("runner ID is empty; the runner is not enrolled");
        }
        Ok(Identity {
            urn,
            org_id,
            runner_id,
            private_key,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const URN: &str = "urn:dd:apps:on-prem-runner:us1:42:runner-1";

    #[test]
    fn accepts_a_complete_identity() {
        let identity = Identity::new(
            URN.to_string(),
            "key".to_string(),
            42,
            "runner-1".to_string(),
        )
        .unwrap();

        assert_eq!(identity.urn, URN);
        assert_eq!(identity.org_id, 42);
        assert_eq!(identity.runner_id, "runner-1");
        assert_eq!(identity.private_key, "key");
    }

    #[test]
    fn rejects_incomplete_identities() {
        let cases = [
            ("", "key", 42, "runner-1"),
            ("   ", "key", 42, "runner-1"),
            (URN, "", 42, "runner-1"),
            (URN, "   ", 42, "runner-1"),
            (URN, "key", 0, "runner-1"),
            (URN, "key", -1, "runner-1"),
            (URN, "key", 42, ""),
            (URN, "key", 42, "  "),
        ];
        for (urn, key, org_id, runner_id) in cases {
            assert!(
                Identity::new(
                    urn.to_string(),
                    key.to_string(),
                    org_id,
                    runner_id.to_string()
                )
                .is_err(),
                "identity ({urn:?}, {key:?}, {org_id}, {runner_id:?}) should be rejected"
            );
        }
    }
}
