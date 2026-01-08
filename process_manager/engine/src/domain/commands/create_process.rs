//! CreateProcess Command
//!
//! Command data structure for creating a new process.
//! This is a shared command used across adapters, use cases, and domain services.

use crate::domain::{
    HealthCheck, KillMode, PathCondition, ProcessId, ProcessType, ResourceLimits, RestartPolicy,
};
use std::collections::HashMap;

/// Start behavior for a process
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum StartBehavior {
    /// Process must be started manually (default)
    #[default]
    Manual,
    /// Process starts automatically when created
    Automatic,
}

/// Command to create a new process
#[derive(Debug, Clone, Default)]
pub struct CreateProcessCommand {
    // Required fields
    pub name: String,
    pub command: String,
    pub args: Vec<String>,

    pub description: Option<String>,

    // Optional configuration
    pub restart: Option<RestartPolicy>,
    pub restart_sec: Option<u64>,
    pub restart_max_delay_sec: Option<u64>,
    pub start_limit_interval_sec: Option<u64>,
    pub start_limit_burst: Option<u32>,
    pub working_dir: Option<String>,
    pub env: Option<HashMap<String, String>>,
    pub environment_file: Option<String>,
    pub pidfile: Option<String>,
    pub start_behavior: StartBehavior,

    // Output redirection
    pub stdout: Option<String>,
    pub stderr: Option<String>,

    // Timeouts
    pub timeout_start_sec: Option<u64>,
    pub timeout_stop_sec: Option<u64>,

    // Kill configuration
    pub kill_signal: Option<i32>,
    pub kill_mode: Option<KillMode>,

    // Exit status
    pub success_exit_status: Vec<i32>,

    // Hooks
    pub exec_start_pre: Vec<String>,
    pub exec_start_post: Vec<String>,
    pub exec_stop_post: Vec<String>,

    // User/Group
    pub user: Option<String>,
    pub group: Option<String>,

    // Dependencies
    pub after: Vec<String>,
    pub before: Vec<String>,
    pub requires: Vec<String>,
    pub wants: Vec<String>,
    pub binds_to: Vec<String>,
    pub conflicts: Vec<String>,

    // Process type
    pub process_type: Option<ProcessType>,

    // Health check
    pub health_check: Option<HealthCheck>,

    // Resource limits
    pub resource_limits: Option<ResourceLimits>,

    // Conditional execution
    pub condition_path_exists: Vec<PathCondition>,

    // Runtime directories
    pub runtime_directory: Vec<String>,

    // Ambient capabilities
    pub ambient_capabilities: Vec<String>,
}

/// Response from creating a process
#[derive(Debug, Clone)]
pub struct CreateProcessResponse {
    pub id: ProcessId,
    pub name: String,
}
