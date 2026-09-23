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

#[derive(Debug, Default, Deserialize)]
pub struct Entity {
    pub platform: String,
    pub database_cluster_name: String,
    pub database_instance_name: String,
    #[serde(default)]
    pub database_host_name: String,
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
/// The provider mirrors each integration's own connection keys, so `ssl` is a
/// Postgres SSL-mode string (`disable`/`allow`/...) or the MySQL `ssl` object
/// (`{ ca, cert, key, check_hostname }`); see the `Ssl` enum.
#[derive(Debug, Default, Deserialize)]
pub struct Connection {
    /// Hostname, or a directory path for a Unix socket (e.g. `/var/run/postgresql`).
    pub host: String,
    /// MySQL Unix socket path (integration key `sock`). Takes precedence over `host`.
    #[serde(default)]
    pub sock: Option<String>,
    #[serde(default)]
    pub port: Option<u16>,
    pub dbname: String,
    #[serde(default)]
    pub username: String,
    #[serde(default)]
    pub password: String,
    #[serde(default = "default_application_name")]
    pub application_name: String,
    #[serde(default)]
    pub ssl: Ssl,
    #[serde(default)]
    pub ssl_root_cert: Option<String>,
    #[serde(default)]
    pub ssl_cert: Option<String>,
    #[serde(default)]
    pub ssl_key: Option<String>,
    #[serde(default)]
    pub ssl_password: Option<String>,
}

/// The integration `ssl` value: a Postgres SSL-mode string or the MySQL `ssl`
/// object. Deserialized untagged so a YAML string picks the Postgres variant and
/// a mapping picks the MySQL one, matching what each integration writes.
#[derive(Debug, Deserialize)]
#[serde(untagged)]
pub enum Ssl {
    Postgres(SslMode),
    Mysql(MySqlSsl),
}

impl Default for Ssl {
    fn default() -> Self {
        Ssl::Postgres(SslMode::default())
    }
}

impl Ssl {
    /// Postgres SSL mode; defaults for a (mis-typed) MySQL object.
    pub fn postgres_mode(&self) -> SslMode {
        match self {
            Ssl::Postgres(mode) => *mode,
            Ssl::Mysql(_) => SslMode::default(),
        }
    }

    /// MySQL TLS options, or `None` when TLS is not configured. Mirrors the
    /// MySQL integration, where an empty `ssl` block means no TLS.
    pub fn mysql(&self) -> Option<&MySqlSsl> {
        match self {
            Ssl::Mysql(ssl) if !ssl.is_empty() => Some(ssl),
            _ => None,
        }
    }
}

/// MySQL TLS options, mirroring the MySQL integration's `ssl` section.
#[derive(Debug, Default, Deserialize)]
pub struct MySqlSsl {
    pub ca: Option<String>,
    pub cert: Option<String>,
    pub key: Option<String>,
    pub check_hostname: Option<bool>,
}

impl MySqlSsl {
    /// True when no TLS field is set; such an `ssl` block means no TLS.
    fn is_empty(&self) -> bool {
        self.ca.is_none()
            && self.cert.is_none()
            && self.key.is_none()
            && self.check_hostname.is_none()
    }

    /// Whether to verify the server hostname (defaults to true, like PyMySQL).
    pub fn check_hostname(&self) -> bool {
        self.check_hostname.unwrap_or(true)
    }
}

#[derive(Debug, Default, Deserialize, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub enum SslMode {
    Disable,
    #[default]
    Allow,
    Prefer,
    Require,
    VerifyCa,
    VerifyFull,
}

fn default_application_name() -> String {
    "datadog-agent".to_string()
}

// TODO(dsec-163): add tests for the config deserialization.
