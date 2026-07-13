use std::collections::HashMap;

use anyhow::Result;
use serde_json::Value;

use crate::payload::{PostgresConnection, ScanLocation, ScanTask};

#[cfg(any(feature = "engine-all", feature = "engine-postgres"))]
mod postgres;

pub struct QueryResult {
    pub columns: HashMap<String, Vec<Value>>,
    pub duration_s: f64,
    pub database: String,
    pub host: String,
    pub collection_or_table: String,
}

pub struct InstanceConnections<'a> {
    pub postgres: Option<&'a PostgresConnection>,
}

/// ScanEngine is the extension point for data-security scan engines in the Rust
/// check. Register new engines in `engines()` below.
pub trait ScanEngine: Sync {
    fn name(&self) -> &'static str;
    fn run_scan(&self, task: &ScanTask, connections: &InstanceConnections<'_>) -> Result<QueryResult>;
    fn build_location(&self, result: &QueryResult) -> Result<(&'static str, ScanLocation)>;
    fn log_connection(&self, connections: &InstanceConnections<'_>);
}

/// Single registry of compiled scan engines. Add a new engine here and behind
/// the appropriate `engine-*` Cargo feature.
fn engines() -> &'static [&'static dyn ScanEngine] {
    &[
        #[cfg(any(feature = "engine-all", feature = "engine-postgres"))]
        &postgres::ENGINE,
    ]
}

pub fn engine_for(name: &str) -> Result<&'static dyn ScanEngine> {
    engines()
        .iter()
        .copied()
        .find(|engine| engine.name() == name)
        .ok_or_else(|| {
            let available = engines()
                .iter()
                .map(|engine| engine.name())
                .collect::<Vec<_>>()
                .join(", ");
            anyhow::anyhow!("unsupported backend {name:?} (compiled engines: {available})")
        })
}

pub fn execute_scan(
    backend: &str,
    task: &ScanTask,
    connections: &InstanceConnections<'_>,
) -> Result<QueryResult> {
    engine_for(backend)?.run_scan(task, connections)
}

pub fn build_location(backend: &str, result: &QueryResult) -> Result<(&'static str, ScanLocation)> {
    engine_for(backend)?.build_location(result)
}

pub fn log_connection(backend: &str, connections: &InstanceConnections<'_>) -> Result<()> {
    match engine_for(backend) {
        Ok(engine) => {
            engine.log_connection(connections);
            Ok(())
        }
        Err(err) => {
            eprintln!("datasecurity: {err}");
            Ok(())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_features_compile_all_engines() {
        let names: Vec<_> = engines().iter().map(|engine| engine.name()).collect();
        assert!(!names.is_empty());
    }
}
