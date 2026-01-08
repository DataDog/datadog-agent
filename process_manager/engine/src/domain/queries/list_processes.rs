//! ListProcesses Query

use crate::domain::Process;

/// Response from listing processes
#[derive(Debug, Clone)]
pub struct ListProcessesResponse {
    pub processes: Vec<Process>,
}
