// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use serde::Serialize;
use std::collections::HashMap;

#[derive(Debug, Default, Serialize)]
pub struct UST {
    pub service: Option<String>,
    pub env: Option<String>,
    pub version: Option<String>,
}

impl UST {
    /// Creates a UST struct from environment variables.
    /// Extracts DD_SERVICE, DD_ENV, and DD_VERSION from the provided environment map.
    pub fn from_envs(envs: &HashMap<String, String>) -> Self {
        UST {
            service: envs.get("DD_SERVICE").cloned(),
            env: envs.get("DD_ENV").cloned(),
            version: envs.get("DD_VERSION").cloned(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ust_from_envs_all_fields_present() {
        let mut envs = HashMap::new();
        envs.insert("DD_SERVICE".to_string(), "my-service".to_string());
        envs.insert("DD_ENV".to_string(), "production".to_string());
        envs.insert("DD_VERSION".to_string(), "1.2.3".to_string());

        let ust = UST::from_envs(&envs);

        assert_eq!(ust.service, Some("my-service".to_string()));
        assert_eq!(ust.env, Some("production".to_string()));
        assert_eq!(ust.version, Some("1.2.3".to_string()));
    }

    #[test]
    fn test_ust_from_envs_partial_fields() {
        let mut envs = HashMap::new();
        envs.insert("DD_SERVICE".to_string(), "my-service".to_string());
        // DD_ENV and DD_VERSION are missing

        let ust = UST::from_envs(&envs);

        assert_eq!(ust.service, Some("my-service".to_string()));
        assert_eq!(ust.env, None);
        assert_eq!(ust.version, None);
    }
}
