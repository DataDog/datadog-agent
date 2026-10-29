use std::time::Duration;

use anyhow::{Context, Result};
use serde::{Deserialize, Deserializer};
use shlib_core::AgentCheck;

use crate::scanning::ScanningRule;

impl CheckConfig {
    /// Reads the check instance config into a `CheckConfig`.
    pub fn from_instance(check: &AgentCheck) -> Result<Self> {
        Ok(Self {
            task_id: check.instance.get("task_id").unwrap_or_default(),
            // `scanning_rules` is common to every sub task; `scan_data` is the
            // list of sub tasks to run against it.
            scanning_rules: check
                .instance
                .get("scanning_rules")
                .context("failed to read scanning_rules from instance config")?,
            scan_data: check
                .instance
                .get("scan_data")
                .context("failed to read scan_data from instance config")?,
        })
    }
}

/// Instance configuration for the datasecurity check.
///
/// No `Debug`: holds credentials (see [`Connection`]).
#[derive(Default, Deserialize)]
pub struct CheckConfig {
    pub task_id: String,
    pub scanning_rules: Vec<ScanningRule>,
    pub scan_data: Vec<SubTask>,
}

/// A single scan sub task: a query to run against one data source.
///
/// No `Debug`: embeds a [`Connection`].
#[derive(Default, Deserialize)]
pub struct SubTask {
    pub sub_task_id: String,
    pub connection: Connection,
    pub entity: Entity,
    /// SQL query whose result columns are scanned.
    pub query: String,
    /// Per-request connect/query timeout (`timeout_seconds`), must be > 0.
    #[serde(rename = "timeout_seconds", deserialize_with = "deserialize_timeout")]
    pub timeout: Duration,
}

#[derive(Debug, Default, Deserialize)]
pub struct Entity {
    pub platform: String,
    pub database_cluster_name: String,
    pub database_instance_name: String,
    pub database: String,
    pub schema: String,
    pub table: String,
}

/// Deserializes a timeout given in seconds into a `Duration`, rejecting zero so
/// we never issue a request without a timeout.
fn deserialize_timeout<'de, D>(deserializer: D) -> Result<Duration, D::Error>
where
    D: Deserializer<'de>,
{
    let seconds = u64::deserialize(deserializer)?;
    if seconds == 0 {
        return Err(serde::de::Error::custom(
            "timeout_seconds must be greater than 0",
        ));
    }
    Ok(Duration::from_secs(seconds))
}

/// Database connection parameters for a sub task.
///
/// No `Debug`: a derived one would print `password` and `ssl_password` in clear text.
#[derive(Default, Deserialize)]
pub struct Connection {
    /// Hostname, or a directory path for a Unix socket (e.g. `/var/run/postgresql`).
    pub host: String,
    #[serde(default = "default_port")]
    pub port: u16,
    pub dbname: String,
    #[serde(default)]
    pub username: String,
    #[serde(default)]
    pub password: String,
    #[serde(default = "default_application_name")]
    pub application_name: String,
    #[serde(default)]
    pub ssl: SslMode,
    #[serde(default)]
    pub ssl_root_cert: Option<String>,
    #[serde(default)]
    pub ssl_cert: Option<String>,
    #[serde(default)]
    pub ssl_key: Option<String>,
    #[serde(default)]
    pub ssl_password: Option<String>,
}

/// Same modes as the postgres integration's `ssl` option. Like the integration, an unknown value
/// falls back to `allow` (the default).
#[derive(Debug, Default, Deserialize, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub enum SslMode {
    Disable,
    Prefer,
    Require,
    VerifyCa,
    VerifyFull,
    #[default]
    #[serde(other)]
    Allow,
}

fn default_port() -> u16 {
    5432
}

fn default_application_name() -> String {
    "datadog-agent".to_string()
}

#[cfg(test)]
mod tests {
    use serde::de::IntoDeserializer;

    use super::*;

    fn ssl_mode(value: &str) -> SslMode {
        let de = IntoDeserializer::<serde::de::value::Error>::into_deserializer(value);
        SslMode::deserialize(de).unwrap()
    }

    #[test]
    fn ssl_mode_parses_known_values_and_defaults_to_allow() {
        assert_eq!(ssl_mode("disable"), SslMode::Disable);
        assert_eq!(ssl_mode("prefer"), SslMode::Prefer);
        assert_eq!(ssl_mode("require"), SslMode::Require);
        assert_eq!(ssl_mode("verify-ca"), SslMode::VerifyCa);
        assert_eq!(ssl_mode("verify-full"), SslMode::VerifyFull);
        assert_eq!(ssl_mode("allow"), SslMode::Allow);
        assert_eq!(ssl_mode("invalid_mode"), SslMode::Allow);
    }
}

// TODO(dsec-163): add more tests for the config deserialization.
