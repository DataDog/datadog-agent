//! UpdateProcess Command
//!
//! Command to update process configuration with hot-update support

use crate::domain::{HealthCheck, KillMode, ProcessId, ResourceLimits, RestartPolicy};
use std::collections::HashMap;

/// Command to update a process
#[derive(Debug, Clone, Default)]
pub struct UpdateProcessCommand {
    pub process_id: Option<ProcessId>,
    pub process_name: Option<String>,

    // Hot-update fields (no restart required)
    pub restart_policy: Option<RestartPolicy>,
    pub timeout_stop_sec: Option<u64>,
    pub restart_sec: Option<u64>,
    pub restart_max_delay: Option<u64>,
    pub resource_limits: Option<ResourceLimits>,
    pub health_check: Option<HealthCheck>,
    pub success_exit_status: Option<Vec<i32>>,

    // Restart-required fields
    pub env: Option<HashMap<String, String>>,
    pub environment_file: Option<String>,
    pub working_dir: Option<String>,
    pub user: Option<String>,
    pub group: Option<String>,
    pub runtime_directory: Option<Vec<String>>,
    pub ambient_capabilities: Option<Vec<String>>,
    pub kill_mode: Option<KillMode>,
    pub kill_signal: Option<i32>,
    pub pidfile: Option<String>,

    // Flags
    pub restart_process: bool,
    pub dry_run: bool,
}

impl UpdateProcessCommand {
    /// Create command to update by ID
    pub fn from_id(process_id: ProcessId) -> Self {
        Self {
            process_id: Some(process_id),
            process_name: None,
            ..Default::default()
        }
    }

    /// Create command to update by name
    pub fn from_name(name: String) -> Self {
        Self {
            process_id: None,
            process_name: Some(name),
            ..Default::default()
        }
    }
}

/// Response from updating a process
#[derive(Debug, Clone)]
pub struct UpdateProcessResponse {
    pub process_id: ProcessId,
    pub updated_fields: Vec<String>,
    pub restart_required_fields: Vec<String>,
    pub process_restarted: bool,
}
