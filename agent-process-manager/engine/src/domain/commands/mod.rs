//! Domain Commands
//!
//! Command data structures following CQRS pattern.
//! Commands represent write operations and intent.

mod create_process;
mod delete_process;
mod get_resource_usage;
mod load_config;
mod restart_process;
mod start_process;
mod stop_process;
mod update_process;

pub use create_process::{CreateProcessCommand, CreateProcessResponse, StartBehavior};
pub use delete_process::{DeleteProcessCommand, DeleteProcessResponse};
pub use get_resource_usage::{GetResourceUsageCommand, GetResourceUsageResponse};
pub use load_config::{LoadConfigCommand, LoadConfigResponse};
pub use restart_process::{RestartProcessCommand, RestartProcessResponse};
pub use start_process::{StartProcessCommand, StartProcessResponse};
pub use stop_process::{StopProcessCommand, StopProcessResponse};
pub use update_process::{UpdateProcessCommand, UpdateProcessResponse};
