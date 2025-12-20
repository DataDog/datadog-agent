//! GetProcessStatus Query

use crate::domain::{Process, ProcessId};

/// Query to get process status
#[derive(Debug, Clone)]
pub struct GetProcessStatusQuery {
    pub process_id: Option<ProcessId>,
    pub process_name: Option<String>,
}

impl GetProcessStatusQuery {
    /// Create query from process ID
    pub fn from_id(process_id: ProcessId) -> Self {
        Self {
            process_id: Some(process_id),
            process_name: None,
        }
    }

    /// Create query from process name
    pub fn from_name(process_name: String) -> Self {
        Self {
            process_id: None,
            process_name: Some(process_name),
        }
    }
}

/// Response with process status details
#[derive(Debug, Clone)]
pub struct ProcessStatusResponse {
    pub process: Process,
}
