use anyhow::{Context, Result};
use core::AgentCheck;
use serde::Deserialize;

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
    #[serde(default)]
    pub connection: Connection,
    #[serde(default)]
    pub entity: Entity,
    /// SQL query whose result columns are scanned.
    #[serde(default)]
    pub query: String,
    /// Connect and query timeout in seconds. `0` disables the timeout.
    #[serde(default = "default_timeout_seconds")]
    pub timeout_seconds: u64,
}

/// TODO(dsec-140): add the other entity values (scan location) when needed,
/// e.g. database_cluster_name, database_instance_name, database, schema, table.
#[derive(Debug, Default, Deserialize)]
pub struct Entity {
    /// Data-source platform, used to select the backend engine (e.g. `postgres`).
    #[serde(default)]
    pub platform: String,
}

fn default_timeout_seconds() -> u64 {
    30
}

/// Database connection parameters for a sub task.
// TODO(dsec-156): add SSL/TLS support; only unencrypted connections for now.
#[derive(Debug, Default, Deserialize)]
pub struct Connection {
    /// Hostname, or a directory path for a Unix socket (e.g. `/var/run/postgresql`).
    #[serde(default)]
    pub host: String,
    #[serde(default = "default_port")]
    pub port: u16,
    #[serde(default)]
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
