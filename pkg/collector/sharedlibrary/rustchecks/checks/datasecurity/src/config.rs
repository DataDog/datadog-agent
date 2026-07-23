use std::time::Duration;

use anyhow::{Context, Result};
use core::AgentCheck;
use serde::{Deserialize, Deserializer};

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
#[derive(Debug, Default, Deserialize)]
pub struct CheckConfig {
    pub task_id: String,
    pub scanning_rules: Vec<ScanningRule>,
    pub scan_data: Vec<SubTask>,
}

/// A single scan sub task: a query to run against one data source.
#[derive(Debug, Default, Deserialize)]
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

/// TODO(dsec-140): add the other entity values (scan location) when needed,
/// e.g. database_cluster_name, database_instance_name, database, schema, table.
#[derive(Debug, Default, Deserialize)]
pub struct Entity {
    pub platform: String,
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
// TODO(dsec-156): add SSL/TLS support; only unencrypted connections for now.
#[derive(Debug, Default, Deserialize)]
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
}

fn default_port() -> u16 {
    5432
}

fn default_application_name() -> String {
    "datadog-agent".to_string()
}

// TODO(dsec-163): add tests for the config deserialization.