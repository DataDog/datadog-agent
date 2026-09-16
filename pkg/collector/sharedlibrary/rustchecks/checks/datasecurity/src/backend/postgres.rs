//! Postgres scan engine.

use anyhow::{Context, Result, bail};
use postgres::types::Type;
use postgres::{Client, Config, NoTls, Row, Statement};

use crate::backend::{ScanData, ScanEngine, ScannedColumn};
use crate::config::SubTask;

pub struct PostgresEngine;
pub const ENGINE: PostgresEngine = PostgresEngine;

impl ScanEngine for PostgresEngine {
    fn name(&self) -> &'static str {
        "postgres"
    }

    fn fetch_data(&self, sub_task: &SubTask) -> Result<ScanData> {
        // WARNING: do not modify the `prepare`/`query` calls nor share the connection
        // unless you know what you are doing.
        //
        // We get a "free" security layer from two properties held together:
        //   - a single connection per query, forced read-only via
        //     `default_transaction_read_only=on` (a shared connection could have
        //     that flipped off before a write query runs);
        //   - a single statement via `prepare`/`query`, which rejects
        //     multi-statement input and so blocks piggy-backed writes.
        // Weakening either one removes the guarantee that scanning stays read-only.
        let mut client = connect(sub_task)?;
        let stmt = client
            .prepare(sub_task.query.as_str())
            .context("preparing postgres query")?;

        let rows = client.query(&stmt, &[]).context("running postgres query")?;

        Ok(rows_to_scan_data(&stmt, &rows))
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
        .options(&format!(
            "-c statement_timeout={} -c default_transaction_read_only=on",
            timeout.as_millis()
        ));
    // A host starting with `/` is a Unix socket directory, otherwise a TCP host.
    if conn.host.starts_with('/') {
        config.host_path(&conn.host);
    } else {
        config.host(&conn.host);
    }

    // TODO(dsec-156): add TLS support; connections are unencrypted for now.
    config.connect(NoTls).context("connecting to postgres")
}

/// Turns query rows into scanned columns plus one `ScanRow` per result row.
/// Empty results still report column metadata from `stmt`.
fn rows_to_scan_data(stmt: &Statement, rows: &[Row]) -> ScanData {
    let (indices, scanned_columns) = columns_from_stmt(stmt);
    let rows = rows
        .iter()
        .map(|row| indices.iter().map(|&i| cell(row, i)).collect())
        .collect();
    ScanData {
        scanned_columns,
        rows,
    }
}

fn columns_from_stmt(stmt: &Statement) -> (Vec<usize>, Vec<ScannedColumn>) {
    let mut indices = Vec::new();
    let mut scanned_columns = Vec::new();
    for (i, column) in stmt.columns().iter().enumerate() {
        if !is_supported_type(column.type_()) {
            continue;
        }
        indices.push(i);
        scanned_columns.push(ScannedColumn {
            name: column.name().to_string(),
            data_type: column.type_().name().to_string(),
        });
    }
    (indices, scanned_columns)
}

/// Postgres string/text types the scanner can read directly.
/// TODO(dsec-160): add support for other postgres types (integers, floats, booleans, etc.).
fn is_supported_type(ty: &Type) -> bool {
    matches!(*ty, Type::TEXT | Type::VARCHAR | Type::BPCHAR | Type::NAME)
}

/// Reads a string cell (`None` when the value is NULL).
/// TODO(dsec-160): add support for other postgres types (integers, floats, booleans, etc.).
fn cell(row: &Row, index: usize) -> Option<String> {
    row.try_get::<_, Option<String>>(index).ok().flatten()
}

// TODO(dsec-266): add tests for the postgres engine.
