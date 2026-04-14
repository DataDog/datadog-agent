// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Reads the PAR-relevant section of datadog.yaml.
//!
//! Only the fields par-control needs are parsed; everything else is ignored.
//! This mirrors what pkg/privateactionrunner/adapters/config/transform.go does
//! but without the full DD config system.

use std::path::Path;

use anyhow::{Context, Result, bail};
use serde::Deserialize;
use std::time::Duration;

// ── datadog.yaml schema (partial) ──────────────────────────────────────────

#[derive(Debug, Deserialize, Default)]
struct DdYaml {
    /// e.g. "datadoghq.com" — used to compute api host when dd_url is absent.
    #[serde(default)]
    site: String,

    /// Overrides site-derived URL, e.g. "https://api.datadoghq.com".
    #[serde(default)]
    dd_url: String,

    #[serde(default)]
    private_action_runner: PARSection,
}

#[derive(Debug, Deserialize, Default)]
struct PARSection {
    #[serde(default)]
    private_key: String,

    #[serde(default)]
    urn: String,

    /// OPMS polling interval in seconds (default: 1).
    #[serde(default)]
    loop_interval_seconds: Option<u64>,

    /// Heartbeat interval in seconds (default: 20).
    #[serde(default)]
    heartbeat_interval_seconds: Option<u64>,

    /// Default task timeout in seconds (default: 60).
    #[serde(default)]
    task_timeout_seconds: Option<u32>,

    /// Executor idle timeout in seconds (default: 120).
    #[serde(default)]
    executor_idle_timeout_seconds: Option<u32>,

    /// Seconds to wait for executor readiness (default: 10).
    #[serde(default)]
    executor_start_timeout_seconds: Option<u32>,
}

// ── Parsed URN ─────────────────────────────────────────────────────────────

/// Extracted fields from the runner URN.
/// Format: urn:dd:apps:on-prem-runner:{region}:{org_id}:{runner_id}
#[derive(Debug, Clone)]
pub struct Urn {
    pub org_id: i64,
    pub runner_id: String,
}

fn parse_urn(urn: &str) -> Result<Urn> {
    let parts: Vec<&str> = urn.split(':').collect();
    if parts.len() != 7 {
        bail!("invalid URN format (expected 7 colon-separated parts): {urn}");
    }
    let org_id: i64 = parts[5]
        .parse()
        .with_context(|| format!("invalid org_id in URN: {}", parts[5]))?;
    Ok(Urn {
        org_id,
        runner_id: parts[6].to_string(),
    })
}

/// Compute the OPMS API host from the config.
/// Mirrors pkg/privateactionrunner/adapters/config/transform.go logic.
fn compute_api_host(site: &str, dd_url: &str) -> String {
    if !dd_url.is_empty() {
        // Strip scheme and trailing dots/slashes.
        return dd_url
            .trim_start_matches("https://")
            .trim_start_matches("http://")
            .trim_end_matches('/')
            .trim_end_matches('.')
            .to_string();
    }
    let site = if site.is_empty() { "datadoghq.com" } else { site };
    format!("api.{site}")
}

// ── Public parsed config ────────────────────────────────────────────────────

/// PAR values read from datadog.yaml.
#[derive(Debug, Clone)]
pub struct ParConfig {
    /// base64url-encoded JWK private key for JWT signing.
    pub private_key_b64: String,
    pub org_id: i64,
    pub runner_id: String,
    /// e.g. "api.datadoghq.com"
    pub dd_api_host: String,
    pub loop_interval: Duration,
    pub heartbeat_interval: Duration,
    pub task_timeout: Duration,
    pub executor_idle_timeout: u32,
    pub executor_start_timeout: Duration,
}

impl ParConfig {
    pub fn from_file(path: &Path) -> Result<Self> {
        let content = std::fs::read_to_string(path)
            .with_context(|| format!("failed to read config file: {}", path.display()))?;

        // serde_yaml tolerates extra keys so the full datadog.yaml is safe to parse.
        let dd: DdYaml = serde_yaml::from_str(&content)
            .with_context(|| format!("failed to parse config file: {}", path.display()))?;

        let par = &dd.private_action_runner;

        if par.private_key.is_empty() {
            bail!("private_action_runner.private_key is missing or empty in {}", path.display());
        }
        if par.urn.is_empty() {
            bail!("private_action_runner.urn is missing or empty in {}", path.display());
        }

        let urn = parse_urn(&par.urn)?;
        let dd_api_host = compute_api_host(&dd.site, &dd.dd_url);

        Ok(ParConfig {
            private_key_b64: par.private_key.clone(),
            org_id: urn.org_id,
            runner_id: urn.runner_id,
            dd_api_host,
            loop_interval: Duration::from_secs(par.loop_interval_seconds.unwrap_or(1)),
            heartbeat_interval: Duration::from_secs(par.heartbeat_interval_seconds.unwrap_or(20)),
            task_timeout: Duration::from_secs(u64::from(par.task_timeout_seconds.unwrap_or(60))),
            executor_idle_timeout: par.executor_idle_timeout_seconds.unwrap_or(120),
            executor_start_timeout: Duration::from_secs(u64::from(
                par.executor_start_timeout_seconds.unwrap_or(10),
            )),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_urn() {
        let u = parse_urn("urn:dd:apps:on-prem-runner:us1:12345:runner-abc").unwrap();
        assert_eq!(u.org_id, 12345);
        assert_eq!(u.runner_id, "runner-abc");
    }

    #[test]
    fn test_parse_urn_invalid() {
        assert!(parse_urn("urn:dd:apps:on-prem-runner").is_err());
    }

    #[test]
    fn test_compute_api_host_from_site() {
        assert_eq!(compute_api_host("datadoghq.eu", ""), "api.datadoghq.eu");
    }

    #[test]
    fn test_compute_api_host_default() {
        assert_eq!(compute_api_host("", ""), "api.datadoghq.com");
    }

    #[test]
    fn test_compute_api_host_from_dd_url() {
        assert_eq!(
            compute_api_host("datadoghq.com", "https://api.datadoghq.eu"),
            "api.datadoghq.eu"
        );
    }
}
