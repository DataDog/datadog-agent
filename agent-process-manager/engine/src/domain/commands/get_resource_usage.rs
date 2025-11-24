//! GetResourceUsage Command

use crate::domain::{ProcessId, ResourceLimits, ResourceUsage};

/// Command to get resource usage for a process
#[derive(Debug, Clone)]
pub struct GetResourceUsageCommand {
    pub process_id: Option<ProcessId>,
    pub process_name: Option<String>,
}

impl GetResourceUsageCommand {
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

/// Response containing resource usage information
#[derive(Debug, Clone)]
pub struct GetResourceUsageResponse {
    pub process_id: ProcessId,
    pub process_name: String,
    pub usage: ResourceUsage,
    pub limits: ResourceLimits,
}
