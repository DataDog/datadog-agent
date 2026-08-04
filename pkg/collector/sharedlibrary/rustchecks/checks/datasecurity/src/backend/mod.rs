//! Backend scan engines: run a sub task's query and return its column data.

use anyhow::{Context, Result};
use serde_json::Value;

use crate::config::SubTask;

#[cfg(feature = "engine-postgres")]
mod postgres;

/// One scanned column's name and its source data type (e.g. `text`, `varchar`).
#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct ScannedColumn {
    pub name: String,
    pub data_type: String,
}

/// The result of running a sub task's query: the `{ column: [values] }` map fed
/// to the scanner, plus metadata describing what was scanned.
#[derive(Debug, Default)]
pub struct ScanData {
    /// Column-oriented values consumed by the scanner.
    // TODO(dsec-173): return an `Event` (dd-sensitive-data-scanner) per backend
    // instead of a `Value`, to avoid the intermediate JSON map and its copies.
    pub columns: Value,
    /// The scanned columns (name + source data type), in query order.
    pub scanned_columns: Vec<ScannedColumn>,
    /// Number of rows returned by the query and scanned.
    pub scanned_row_count: i64,
}

/// A data-source engine that runs a sub task's query and returns the scanned
/// columns and their values ready for the scanner.
pub trait ScanEngine: Sync {
    /// Engine name, matched against the sub task platform.
    fn name(&self) -> &'static str;
    /// Runs the sub task's query and returns its columns and scan metadata.
    fn fetch_data(&self, sub_task: &SubTask) -> Result<ScanData>;
}

/// Compiled engines. Add a new engine here behind its `engine-*` feature.
fn engines() -> &'static [&'static dyn ScanEngine] {
    &[
        #[cfg(feature = "engine-postgres")]
        &postgres::ENGINE,
    ]
}

// TODO(dsec-174): use map lookup instead of linear scan.
fn engine_for(platform: &str) -> Result<&'static dyn ScanEngine> {
    engines()
        .iter()
        .copied()
        .find(|engine| engine.name() == platform)
        .with_context(|| format!("unsupported platform {platform:?}"))
}

/// Runs the sub task on the engine selected by its entity platform.
pub fn fetch_data(sub_task: &SubTask) -> Result<ScanData> {
    engine_for(&sub_task.entity.platform)?.fetch_data(sub_task)
}

#[cfg(all(test, feature = "engine-postgres"))]
mod tests {
    use super::engine_for;

    #[test]
    fn resolves_postgres_engine() {
        assert_eq!(engine_for("postgres").unwrap().name(), "postgres");
        assert!(engine_for("another_engine_not_register").is_err());
    }
}
