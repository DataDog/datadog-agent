//! DeleteProcess Command

use crate::domain::ProcessId;

/// Command to delete a process
#[derive(Debug, Clone)]
pub struct DeleteProcessCommand {
    pub process_id: Option<ProcessId>,
    pub process_name: Option<String>,
    pub force: bool, // If true, stop running process before deleting
}

impl DeleteProcessCommand {
    /// Create command from process ID
    pub fn from_id(process_id: ProcessId, force: bool) -> Self {
        Self {
            process_id: Some(process_id),
            process_name: None,
            force,
        }
    }

    /// Create command from process name
    pub fn from_name(process_name: String, force: bool) -> Self {
        Self {
            process_id: None,
            process_name: Some(process_name),
            force,
        }
    }
}

/// Response from deleting a process
#[derive(Debug, Clone)]
pub struct DeleteProcessResponse {
    pub process_id: ProcessId,
}
