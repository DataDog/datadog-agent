//! Postgres scan engine.

use anyhow::{Context, Result, bail};
use postgres::types::Type;
use postgres::{Client, Config, NoTls, Row};
use serde_json::{Map, Value};

use crate::backend::{ScanData, ScanEngine, ScannedColumn};
use crate::config::SubTask;

pub struct PostgresEngine;
pub const ENGINE: PostgresEngine = PostgresEngine;

impl ScanEngine for PostgresEngine {
    fn name(&self) -> &'static str {
        "postgres"
    }

    fn fetch_data(&self, sub_task: &SubTask) -> Result<ScanData> {
        // TODO(dsec-161): prevent reinitializing the connection for each sub task;
        // reuse a pooled/cached connection across sub tasks sharing the same target.
        let mut client = connect(sub_task)?;

        let rows = client
            .query(sub_task.query.as_str(), &[])
            .context("running postgres query")?;

        Ok(rows_to_scan_data(&rows))
    }
}

/// Opens a postgres connection for the sub task using its connection settings.
fn connect(sub_task: &SubTask) -> Result<Client> {
    let conn = &sub_task.connection;
    if conn.host.is_empty() {
        bail!("postgres connection host is required");
    }
    let timeout = sub_task.timeout;

    let mut config = Config::new();
    config
        .port(conn.port)
        .dbname(&conn.dbname)
        .user(&conn.username)
        .password(&conn.password)
        .application_name(&conn.application_name)
        .connect_timeout(timeout)
        .options(&format!("-c statement_timeout={}", timeout.as_millis()));
    // A host starting with `/` is a Unix socket directory, otherwise a TCP host.
    if conn.host.starts_with('/') {
        config.host_path(&conn.host);
    } else {
        config.host(&conn.host);
    }

    // TODO(dsec-156): add TLS support; connections are unencrypted for now.
    config.connect(NoTls).context("connecting to postgres")
}

/// Turns query rows into the scanner input plus scan metadata. The values are a
/// column-oriented map, e.g.
/// `{ "email": ["a@b.com", "c@d.com"], "name": ["alice", "bob"] }`, and the
/// metadata reports the scanned columns (name + Postgres type) and row count.
fn rows_to_scan_data(rows: &[Row]) -> ScanData {
    let scanned_row_count = rows.len() as i64;

    // TODO(dsec-229): add column metadata when the query returns no rows.
    let Some(first) = rows.first() else {
        return ScanData::default();
    };

    // Keep only supported columns (the scanner reads strings) and collect each
    // one's values across all rows alongside its name and Postgres type.
    let mut columns = Map::new();
    let mut scanned_columns = Vec::new();
    for (i, column) in first.columns().iter().enumerate() {
        if !is_supported_type(column.type_()) {
            continue;
        }
        let values: Vec<Value> = rows.iter().map(|row| cell_to_value(row, i)).collect();
        columns.insert(column.name().to_string(), Value::Array(values));
        scanned_columns.push(ScannedColumn {
            name: column.name().to_string(),
            data_type: column.type_().name().to_string(),
        });
    }

    ScanData {
        columns: Value::Object(columns),
        scanned_columns,
        scanned_row_count,
    }
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
