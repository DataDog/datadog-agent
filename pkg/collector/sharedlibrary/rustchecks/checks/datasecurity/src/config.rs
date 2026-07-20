use anyhow::{Context, Result};
use core::AgentCheck;
use serde::Deserialize;
use serde_json::Value;

impl CheckConfig {
    /// Reads the check instance config into a `CheckConfig`.
    pub fn from_instance(check: &AgentCheck) -> Result<Self> {
        Ok(Self {
            task_id: check.instance.get("task_id").unwrap_or_default(),
            scan_data: check
                .instance
                .get("scan_data")
                .context("reading scan_data from instance config")?,
        })
    }
}


/// Instance configuration for the datasecurity check.
///
/// Kept in its own module so the deserialization surface of the check instance
/// config stays readable and easy to grow as the check gains real RC tasks.
#[derive(Debug, Default, Deserialize)]
pub struct CheckConfig {
    pub task_id: String,
    pub scan_data: Vec<SubTask>,
}

/// A single scan sub task.
#[derive(Debug, Deserialize)]
pub struct SubTask {
    pub sub_task_id: String,
    // TODO(DSEC-139): remove placeholder response once the postgres backend lands.
    pub placeholder_response: Value,
}
