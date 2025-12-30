use serde::{Deserialize, Serialize};

/// Resource usage statistics for a running process
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ResourceUsage {
    /// Current memory usage in bytes
    pub memory_current: Option<u64>,
    /// Peak memory usage in bytes
    pub memory_peak: Option<u64>,
    /// Total CPU usage in microseconds
    pub cpu_usage_usec: Option<u64>,
    /// User CPU time in microseconds
    pub cpu_user_usec: Option<u64>,
    /// System CPU time in microseconds
    pub cpu_system_usec: Option<u64>,
    /// Current number of PIDs/threads
    pub pids_current: Option<u32>,
}

impl ResourceUsage {
    /// Create an empty ResourceUsage (all fields None)
    pub fn empty() -> Self {
        Self {
            memory_current: None,
            memory_peak: None,
            cpu_usage_usec: None,
            cpu_user_usec: None,
            cpu_system_usec: None,
            pids_current: None,
        }
    }

    /// Check if any usage data is available
    pub fn has_data(&self) -> bool {
        self.memory_current.is_some()
            || self.memory_peak.is_some()
            || self.cpu_usage_usec.is_some()
            || self.cpu_user_usec.is_some()
            || self.cpu_system_usec.is_some()
            || self.pids_current.is_some()
    }
}
