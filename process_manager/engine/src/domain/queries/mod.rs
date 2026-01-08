//! Domain Queries
//!
//! Query data structures following CQRS pattern.
//! Queries represent read operations and return data without side effects.

mod get_process_status;
mod list_processes;

pub use get_process_status::{GetProcessStatusQuery, ProcessStatusResponse};
pub use list_processes::ListProcessesResponse;
