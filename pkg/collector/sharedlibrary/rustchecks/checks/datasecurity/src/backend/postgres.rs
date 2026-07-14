use anyhow::{Context, Result};

use crate::backend::{InstanceConnections, QueryResult, ScanEngine};
use crate::constants::RESOURCE_TYPE_RDS_INSTANCE;
use crate::payload::{PostgresConnection, PostgresScanData, ScanLocation, ScanTask};

pub struct PostgresEngine;

pub const ENGINE: PostgresEngine = PostgresEngine;

impl ScanEngine for PostgresEngine {
    fn name(&self) -> &'static str {
        "postgres"
    }

    fn run_scan(
        &self,
        task: &ScanTask,
        connections: &InstanceConnections<'_>,
    ) -> Result<QueryResult> {
        let scan_data = task
            .scan_data
            .postgres
            .as_ref()
            .context("no postgres scan_data in payload")?;
        let conn = connections
            .postgres
            .context("reading postgres connection from instance config")?;
        run_postgres_scan(conn, scan_data)
    }

    fn build_location(&self, result: &QueryResult) -> Result<(&'static str, ScanLocation)> {
        Ok((
            RESOURCE_TYPE_RDS_INSTANCE,
            ScanLocation {
                database: result.database.clone(),
                rds_table: crate::payload::RdsTable {
                    instance_arn: result.host.clone(),
                    database_name: result.database.clone(),
                    table_name: result.collection_or_table.clone(),
                },
            },
        ))
    }

    fn log_connection(&self, connections: &InstanceConnections<'_>) {
        if let Some(conn) = connections.postgres {
            println!(
                "datasecurity: connecting to postgres host={} port={} dbname={} user={}",
                conn.host, conn.port, conn.dbname, conn.username
            );
        }
    }
}

fn run_postgres_scan(
    conn: &PostgresConnection,
    scan_data: &PostgresScanData,
) -> Result<QueryResult> {
    use std::time::Instant;

    use postgres::{Client, NoTls};

    let conn_str = format!(
        "host={} port={} dbname={} user={} password={} sslmode=disable",
        conn.host, conn.port, conn.dbname, conn.username, conn.password
    );

    let mut client = Client::connect(&conn_str, NoTls).context("connecting to postgres")?;

    let start = Instant::now();
    let rows = client
        .query(&scan_data.query, &[])
        .context("running postgres query")?;

    let columns = rows_to_columns(&rows)?;
    let duration_s = start.elapsed().as_secs_f64();

    Ok(QueryResult {
        columns,
        duration_s,
        database: conn.dbname.clone(),
        host: conn.host.clone(),
        collection_or_table: scan_data.table.clone(),
    })
}

fn rows_to_columns(rows: &[postgres::Row]) -> Result<std::collections::HashMap<String, Vec<serde_json::Value>>> {
    use serde_json::Value;
    use std::collections::HashMap;

    if rows.is_empty() {
        return Ok(HashMap::new());
    }

    let names: Vec<String> = rows[0]
        .columns()
        .iter()
        .map(|col| col.name().to_string())
        .collect();

    let mut columns: HashMap<String, Vec<Value>> = names
        .iter()
        .map(|name| (name.clone(), Vec::new()))
        .collect();

    for row in rows {
        for (i, name) in names.iter().enumerate() {
            let value = row_to_json(row, i)?;
            columns.get_mut(name).expect("column initialized").push(value);
        }
    }

    Ok(columns)
}

fn row_to_json(row: &postgres::Row, index: usize) -> Result<serde_json::Value> {
    use serde_json::Value;

    if let Ok(v) = row.try_get::<_, Option<bool>>(index) {
        return Ok(v.map(Value::Bool).unwrap_or(Value::Null));
    }
    if let Ok(v) = row.try_get::<_, Option<i16>>(index) {
        return Ok(v.map(Value::from).unwrap_or(Value::Null));
    }
    if let Ok(v) = row.try_get::<_, Option<i32>>(index) {
        return Ok(v.map(Value::from).unwrap_or(Value::Null));
    }
    if let Ok(v) = row.try_get::<_, Option<i64>>(index) {
        return Ok(v.map(Value::from).unwrap_or(Value::Null));
    }
    if let Ok(v) = row.try_get::<_, Option<f32>>(index) {
        return Ok(v
            .and_then(|n| serde_json::Number::from_f64(f64::from(n)))
            .map(Value::Number)
            .unwrap_or(Value::Null));
    }
    if let Ok(v) = row.try_get::<_, Option<f64>>(index) {
        return Ok(v
            .and_then(|n| serde_json::Number::from_f64(n))
            .map(Value::Number)
            .unwrap_or(Value::Null));
    }
    if let Ok(v) = row.try_get::<_, Option<String>>(index) {
        return Ok(v.map(Value::String).unwrap_or(Value::Null));
    }
    if let Ok(v) = row.try_get::<_, Option<&str>>(index) {
        return Ok(v
            .map(|s| Value::String(s.to_string()))
            .unwrap_or(Value::Null));
    }

    Ok(Value::Null)
}
