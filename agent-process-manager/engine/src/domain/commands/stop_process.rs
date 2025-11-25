//! StopProcess Command

use crate::domain::ProcessId;

/// Command to stop a process
#[derive(Debug, Clone)]
pub struct StopProcessCommand {
    pub process_id: Option<ProcessId>,
    pub process_name: Option<String>,
    pub signal: Option<i32>, // Default: SIGTERM (15)
}

impl StopProcessCommand {
    /// Create command from process ID
    pub fn from_id(process_id: ProcessId) -> Self {
        Self {
            process_id: Some(process_id),
            process_name: None,
            signal: None,
        }
    }

    /// Create command from process name
    pub fn from_name(process_name: String) -> Self {
        Self {
            process_id: None,
            process_name: Some(process_name),
            signal: None,
        }
    }
}

/// Response from stopping a process
#[derive(Debug, Clone)]
pub struct StopProcessResponse {
    pub process_id: ProcessId,
    pub bound_processes_stopped: Vec<String>, // Names of processes stopped due to binds_to
}
