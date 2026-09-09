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
    #[serde(default)]
    pub ssl: SslMode,
    /// libpq `sslcert`: path to the PEM client certificate chain.
    #[serde(default)]
    pub ssl_cert: Option<String>,
    /// libpq `sslkey`: path to the PEM client private key.
    #[serde(default)]
    pub ssl_key: Option<String>,
    /// libpq `sslpassword`: passphrase for an encrypted `ssl_key`.
    #[serde(default)]
    pub ssl_password: Option<String>,
    /// libpq `sslrootcert`: path to the PEM CA bundle. Defaults to the system trust store.
    #[serde(default)]
    pub ssl_root_cert: Option<String>,
}

/// libpq `sslmode`, from weakest to strongest. Defaults to `allow` like the
/// postgres integration. Unlike the integration an unknown value is an error
/// rather than a silent fallback, so a typo cannot weaken the connection.
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

fn default_port() -> u16 {
    5432
}

fn default_application_name() -> String {
    "datadog-agent".to_string()
}

// TODO(dsec-163): add tests for the config deserialization.
#[cfg(test)]
mod tests {
    use super::*;

    /// The connection the autodiscovery provider emits when the postgres instance
    /// configures no TLS: the omitted settings must land on the integration's defaults.
    #[test]
    fn connection_without_tls_settings_defaults_to_allow() {
        let conn: Connection = serde_json::from_str(
            r#"{"host":"db-host","port":5678,"dbname":"app","username":"datadog","password":"secret"}"#,
        )
        .unwrap();

        assert_eq!(conn.ssl, SslMode::Allow);
        assert_eq!(conn.ssl_cert, None);
        assert_eq!(conn.ssl_key, None);
        assert_eq!(conn.ssl_password, None);
        assert_eq!(conn.ssl_root_cert, None);
    }

    #[test]
    fn connection_reads_the_forwarded_tls_settings() {
        let conn: Connection = serde_json::from_str(
            r#"{"host":"db-host","port":5678,"dbname":"app","username":"datadog","password":"secret",
                "ssl":"verify-full","ssl_cert":"/client.crt","ssl_key":"/client.key",
                "ssl_password":"passphrase","ssl_root_cert":"/root.crt"}"#,
        )
        .unwrap();

        assert_eq!(conn.ssl, SslMode::VerifyFull);
        assert_eq!(conn.ssl_cert.as_deref(), Some("/client.crt"));
        assert_eq!(conn.ssl_key.as_deref(), Some("/client.key"));
        assert_eq!(conn.ssl_password.as_deref(), Some("passphrase"));
        assert_eq!(conn.ssl_root_cert.as_deref(), Some("/root.crt"));
    }

    /// Values match the postgres integration's `ssl` option (libpq `sslmode`).
    #[test]
    fn ssl_accepts_every_libpq_spelling() {
        let modes = [
            ("disable", SslMode::Disable),
            ("allow", SslMode::Allow),
            ("prefer", SslMode::Prefer),
            ("require", SslMode::Require),
            ("verify-ca", SslMode::VerifyCa),
            ("verify-full", SslMode::VerifyFull),
        ];
        for (spelling, want) in modes {
            let got: SslMode = serde_json::from_str(&format!("\"{spelling}\"")).unwrap();
            assert_eq!(got, want, "{spelling}");
        }
        assert!(serde_json::from_str::<SslMode>("\"verify_full\"").is_err());
    }
}
