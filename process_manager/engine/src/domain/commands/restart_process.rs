//! RestartProcess Command

use crate::domain::ProcessId;

/// Command to restart a process
#[derive(Debug, Clone)]
pub struct RestartProcessCommand {
    pub process_id: Option<ProcessId>,
    pub process_name: Option<String>,
}

impl RestartProcessCommand {
    /// Create command from process ID
    pub fn from_id(process_id: ProcessId) -> Self {
        Self {
            process_id: Some(process_id),
            process_name: None,
        }
    }

    /// Create command from process name
    pub fn from_name(process_name: String) -> Self {
        Self {
            process_id: None,
            process_name: Some(process_name),
        }
    }
}

/// Response from restarting a process
#[derive(Debug, Clone)]
pub struct RestartProcessResponse {
    pub process_id: ProcessId,
    pub pid: u32,
}
