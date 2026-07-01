use std::collections::HashMap;
use std::time::Instant;

use anyhow::{Context, Result};
use postgres::{Client, NoTls, Row};
use serde_json::Value;

use crate::payload::PostgresConnection;

pub struct QueryResult {
    pub columns: HashMap<String, Vec<Value>>,
    pub duration_s: f64,
    pub dbname: String,
    pub host: String,
}

/// Connects to postgres, runs `query`, and returns column-oriented data matching
/// the Go datasecurity component (`map[string][]interface{}`).
pub fn run_postgres_query(conn: &PostgresConnection, query: &str) -> Result<QueryResult> {
    let conn_str = format!(
        "host={} port={} dbname={} user={} password={} sslmode=disable",
        conn.host, conn.port, conn.dbname, conn.username, conn.password
    );

    let mut client = Client::connect(&conn_str, NoTls).context("connecting to postgres")?;

    let start = Instant::now();
    let rows = client
        .query(query, &[])
        .context("running postgres query")?;

    let columns = rows_to_columns(&rows)?;
    let duration_s = start.elapsed().as_secs_f64();

    Ok(QueryResult {
        columns,
        duration_s,
        dbname: conn.dbname.clone(),
        host: conn.host.clone(),
    })
}

fn rows_to_columns(rows: &[Row]) -> Result<HashMap<String, Vec<Value>>> {
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

fn row_to_json(row: &Row, index: usize) -> Result<Value> {
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
