//! Postgres scan engine.

use std::time::Duration;

use anyhow::{Context, Result};
use postgres::{Config, NoTls, Row};
use serde_json::{Map, Value};

use crate::backend::ScanEngine;
use crate::config::SubTask;

pub struct PostgresEngine;
pub const ENGINE: PostgresEngine = PostgresEngine;

impl ScanEngine for PostgresEngine {
    fn name(&self) -> &'static str {
        "postgres"
    }

    fn run_scan(&self, sub_task: &SubTask) -> Result<Value> {
        let conn = &sub_task.connection;
        let timeout = sub_task.timeout_seconds;
        println!(
            "datasecurity: connecting to postgres host={} port={} dbname={} user={} timeout={}s",
            conn.host, conn.port, conn.dbname, conn.username, timeout
        );

        let mut config = Config::new();
        config
            .port(conn.port)
            .dbname(&conn.dbname)
            .user(&conn.username)
            .password(&conn.password)
            .application_name(&conn.application_name)
            // `0` means no statement timeout in postgres.
            .options(&format!("-c statement_timeout={}", timeout * 1000));
        // A host starting with `/` is a Unix socket directory, otherwise a TCP host.
        if conn.host.starts_with('/') {
            config.host_path(&conn.host);
        } else if !conn.host.is_empty() {
            config.host(&conn.host);
        }
        if timeout > 0 {
            config.connect_timeout(Duration::from_secs(timeout));
        }

        // TODO(dsec-156): add TLS support; connections are unencrypted for now.
        let mut client = config.connect(NoTls).context("connecting to postgres")?;

        let rows = client
            .query(sub_task.query.as_str(), &[])
            .context("running postgres query")?;

        Ok(rows_to_columns(&rows))
    }
}

/// Builds a `{ column: [string values] }` map from the query rows.
fn rows_to_columns(rows: &[Row]) -> Value {
    let Some(first) = rows.first() else {
        return Value::Object(Map::new());
    };

    let names: Vec<String> = first.columns().iter().map(|c| c.name().to_string()).collect();
    let mut columns: Map<String, Value> = names
        .iter()
        .map(|name| (name.clone(), Value::Array(Vec::new())))
        .collect();

    for row in rows {
        for (i, name) in names.iter().enumerate() {
            if let Some(Value::Array(values)) = columns.get_mut(name) {
                values.push(cell_to_value(row, i));
            }
        }
    }

    Value::Object(columns)
}

/// Renders a postgres cell as a string value (or null), so the scanner sees a
/// uniform `{ column: [string values] }` shape.
fn cell_to_value(row: &Row, index: usize) -> Value {
    fn string(v: Option<impl ToString>) -> Value {
        v.map(|n| Value::String(n.to_string())).unwrap_or(Value::Null)
    }

    if let Ok(v) = row.try_get::<_, Option<String>>(index) {
        return string(v);
    }
    if let Ok(v) = row.try_get::<_, Option<i64>>(index) {
        return string(v);
    }
    if let Ok(v) = row.try_get::<_, Option<i32>>(index) {
        return string(v);
    }
    if let Ok(v) = row.try_get::<_, Option<i16>>(index) {
        return string(v);
    }
    if let Ok(v) = row.try_get::<_, Option<f64>>(index) {
        return string(v);
    }
    if let Ok(v) = row.try_get::<_, Option<bool>>(index) {
        return string(v);
    }
    Value::Null
}
