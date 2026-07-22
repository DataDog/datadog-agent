//! Backend scan engines: run a sub task's query and return its column data.

use anyhow::{Context, Result};
use serde_json::Value;

use crate::config::SubTask;

#[cfg(feature = "engine-postgres")]
mod postgres;

/// A data-source engine that runs a sub task's query and returns the result as
/// a `{ column: [values] }` map ready for the scanner.
pub trait ScanEngine: Sync {
    /// Engine name, matched against the sub task platform.
    fn name(&self) -> &'static str;
    /// Runs the sub task's query and returns its columns.
    fn fetch_data(&self, sub_task: &SubTask) -> Result<Value>;
}

/// Compiled engines. Add a new engine here behind its `engine-*` feature.
fn engines() -> &'static [&'static dyn ScanEngine] {
    &[
        #[cfg(feature = "engine-postgres")]
        &postgres::ENGINE,
    ]
}

fn engine_for(platform: &str) -> Result<&'static dyn ScanEngine> {
    engines()
        .iter()
        .copied()
        .find(|engine| engine.name() == platform)
        .with_context(|| format!("unsupported platform {platform:?}"))
}

/// Runs the sub task on the engine selected by its platform.
pub fn fetch_data(sub_task: &SubTask) -> Result<Value> {
    engine_for(&sub_task.platform)?.fetch_data(sub_task)
}

#[cfg(all(test, feature = "engine-postgres"))]
mod tests {
    use super::engine_for;

    #[test]
    fn resolves_postgres_engine() {
        assert_eq!(engine_for("postgres").unwrap().name(), "postgres");
        assert!(engine_for("mysql").is_err());
    }
}
