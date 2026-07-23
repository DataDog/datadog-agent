use anyhow::{Context, Result};
use core::AgentCheck;
use serde::Deserialize;
use serde_json::Value;

use crate::scanning::ScanningRule;

impl CheckConfig {
    /// Reads the check instance config into a `CheckConfig`.
    pub fn from_instance(check: &AgentCheck) -> Result<Self> {
        Ok(Self {
            task_id: check.instance.get("task_id").unwrap_or_default(),
            // `scanning_rules` is common to every sub task; `scan_data` is the
            // list of sub tasks to run against it.
            scanning_rules: check
                .instance
                .get("scanning_rules")
                .context("failed to read scanning_rules from instance config")?,
            scan_data: check
                .instance
                .get("scan_data")
                .context("failed to read scan_data from instance config")?,
        })
    }
}

/// Instance configuration for the datasecurity check.
#[derive(Debug, Default, Deserialize)]
pub struct CheckConfig {
    pub task_id: String,
    pub scanning_rules: Vec<ScanningRule>,
    pub scan_data: Vec<SubTask>,
}

/// A single scan sub task.
#[derive(Debug, Deserialize)]
pub struct SubTask {
    pub sub_task_id: String,
    // TODO(DSEC-139): remove placeholder response once the postgres backend lands.
    pub placeholder_response: Value,
}
