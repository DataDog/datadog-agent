//! StartProcess Command

use crate::domain::ProcessId;

/// Command to start a process
#[derive(Debug, Clone)]
pub struct StartProcessCommand {
    pub process_id: Option<ProcessId>,
    pub process_name: Option<String>,
    /// Socket FDs for socket activation (passed to child process as FD 3, 4, 5...)
    pub listen_fds: Vec<i32>,
}

impl StartProcessCommand {
    /// Create command from process ID
    pub fn from_id(process_id: ProcessId) -> Self {
        Self {
            process_id: Some(process_id),
            process_name: None,
            listen_fds: Vec::new(),
        }
    }

    /// Create command from process name
    pub fn from_name(process_name: String) -> Self {
        Self {
            process_id: None,
            process_name: Some(process_name),
            listen_fds: Vec::new(),
        }
    }

    /// Create command with socket FDs for socket activation
    pub fn with_socket_fds(process_id: ProcessId, listen_fds: Vec<i32>) -> Self {
        Self {
            process_id: Some(process_id),
            process_name: None,
            listen_fds,
        }
    }

    /// Create command with socket FDs for socket activation (by name)
    pub fn from_name_with_fds(process_name: String, listen_fds: Vec<i32>) -> Self {
        Self {
            process_id: None,
            process_name: Some(process_name),
            listen_fds,
        }
    }
}

/// Response from starting a process
#[derive(Debug, Clone)]
pub struct StartProcessResponse {
    pub process_id: ProcessId,
    pub pid: u32,
}
