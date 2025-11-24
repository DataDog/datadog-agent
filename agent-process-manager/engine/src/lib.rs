//! Process Manager Engine
//!
//! A library for managing system processes with support for:
//! - Process lifecycle management (create, start, stop)
//! - State tracking and monitoring
//! - Automatic restart policies (systemd-style)
//! - gRPC and REST APIs for cross-language integration

// Module declarations
pub mod constants;
pub mod types;

// Core architecture modules
pub mod adapters;
pub mod application;
pub mod domain;
pub mod infrastructure;

// Generated protobuf types
pub mod proto {
    pub mod process_manager {
        tonic::include_proto!("process_manager");

        // Include file descriptor for reflection
        pub const FILE_DESCRIPTOR_SET: &[u8] =
            tonic::include_file_descriptor_set!("proto_descriptor");
    }
}

// Re-export public types
pub use types::{
    HealthCheckConfig, HealthCheckType, HealthStatus, ProcessDetail, ProcessInfo, ProcessState,
    ResourceConfig, ResourceLimits, ResourceRequests, RestartPolicy,
};

use std::collections::HashMap;

/// Configuration for creating a new process
#[derive(Default, Clone)]
pub struct CreateProcessConfig {
    pub working_dir: Option<String>,
    pub env: HashMap<String, String>,
    pub environment_file: Option<String>,
    pub pidfile: Option<String>,
    pub restart_policy: Option<String>,
    pub restart_sec: Option<u64>,
    pub restart_max_delay: Option<u64>,
    pub start_limit_burst: Option<u32>,
    pub start_limit_interval: Option<u64>,
    pub stdout: Option<String>,
    pub stderr: Option<String>,
    pub timeout_start_sec: Option<u64>,
    pub timeout_stop_sec: Option<u64>,
    pub kill_signal: Option<String>,
    pub kill_mode: Option<String>,
    pub success_exit_status: Option<Vec<i32>>,
    pub exec_start_pre: Option<Vec<String>>,
    pub exec_start_post: Option<Vec<String>>,
    pub exec_stop_post: Option<Vec<String>>,
    pub user: Option<String>,
    pub group: Option<String>,
    // Dependencies (systemd-like)
    pub after: Option<Vec<String>>,
    pub before: Option<Vec<String>>,
    pub requires: Option<Vec<String>>,
    pub wants: Option<Vec<String>>,
    pub binds_to: Option<Vec<String>>,
    pub conflicts: Option<Vec<String>>,
    // Process type (systemd-like)
    pub process_type: Option<String>,
    // Health check (Docker/Kubernetes-style)
    pub health_check: Option<types::HealthCheckConfig>,
    // Resource limits (K8s-style)
    pub resources: Option<types::ResourceConfig>,
    // Conditional execution (systemd-like)
    pub condition_path_exists: Option<Vec<String>>,
    // Runtime directories (systemd-like)
    pub runtime_directory: Option<Vec<String>>,
    // Ambient capabilities (systemd-like, Linux-only)
    pub ambient_capabilities: Option<Vec<String>>,
    // Start behavior after creation
    pub start_behavior: crate::domain::StartBehavior,
}

/// Configuration for updating an existing process
#[derive(Default, Clone)]
pub struct UpdateProcessConfig {
    // Fields that can be updated without restart (hot updates)
    pub restart_policy: Option<String>,
    pub timeout_stop_sec: Option<u64>,
    pub restart_sec: Option<u64>,
    pub restart_max_delay: Option<u64>,
    pub resources: Option<types::ResourceConfig>,
    pub health_check: Option<types::HealthCheckConfig>,
    pub success_exit_status: Option<Vec<i32>>,

    // Fields that require restart to take effect
    pub env: Option<HashMap<String, String>>,
    pub environment_file: Option<String>,
    pub working_dir: Option<String>,
    pub user: Option<String>,
    pub group: Option<String>,
    pub runtime_directory: Option<Vec<String>>,
    pub ambient_capabilities: Option<Vec<String>>,
    pub kill_mode: Option<String>,
    pub kill_signal: Option<String>,
    pub pidfile: Option<String>,
}
