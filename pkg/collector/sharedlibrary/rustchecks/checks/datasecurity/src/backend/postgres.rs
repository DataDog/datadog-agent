//! Postgres scan engine.

use std::time::Duration;

use anyhow::{Context, Result};
use postgres::types::Type;
use postgres::{Client, Config, NoTls, Row};
use serde_json::{Map, Value};

use crate::backend::ScanEngine;
use crate::config::SubTask;

pub struct PostgresEngine;
pub const ENGINE: PostgresEngine = PostgresEngine;

impl ScanEngine for PostgresEngine {
    fn name(&self) -> &'static str {
        "postgres"
    }

    fn fetch_data(&self, sub_task: &SubTask) -> Result<Value> {
        // TODO(dsec-161): prevent reinitializing the connection for each sub task;
        // reuse a pooled/cached connection across sub tasks sharing the same target.
        let mut client = connect(sub_task)?;

        let rows = client
            .query(sub_task.query.as_str(), &[])
            .context("running postgres query")?;

        Ok(rows_to_columns(&rows))
    }
}

/// Opens a postgres connection for the sub task using its connection settings.
fn connect(sub_task: &SubTask) -> Result<Client> {
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
    config.connect(NoTls).context("connecting to postgres")
}

/// Turns query rows into a column-oriented map, e.g.
/// `{ "email": ["a@b.com", "c@d.com"], "name": ["alice", "bob"] }`.
fn rows_to_columns(rows: &[Row]) -> Value {
    let Some(first) = rows.first() else {
        return Value::Object(Map::new());
    };

    // Keep only supported columns (the scanner reads strings) and collect
    // each one's values across all rows.
    let columns: Map<String, Value> = first
        .columns()
        .iter()
        .enumerate()
        .filter(|(_, c)| is_supported_type(c.type_()))
        .map(|(i, c)| {
            let values: Vec<Value> = rows.iter().map(|row| cell_to_value(row, i)).collect();
            (c.name().to_string(), Value::Array(values))
        })
        .collect();

    Value::Object(columns)
}

/// Postgres string/text types the scanner can read directly.
/// TODO(dsec-160): add support for other postgres types (integers, floats, booleans, etc.).
fn is_supported_type(ty: &Type) -> bool {
    matches!(*ty, Type::TEXT | Type::VARCHAR | Type::BPCHAR | Type::NAME)
}

/// Renders a string cell as a JSON string (null when the value is NULL).
/// TODO(dsec-160): add support for other postgres types (integers, floats, booleans, etc.).
fn cell_to_value(row: &Row, index: usize) -> Value {
    match row.try_get::<_, Option<String>>(index) {
        Ok(Some(v)) => Value::String(v),
        _ => Value::Null,
    }
}

// TODO(dsec-161): add tests for the postgres engine.
