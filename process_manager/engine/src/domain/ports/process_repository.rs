//! Repository port for persisting processes
//! This is an interface - implementations are in infrastructure layer

use crate::domain::{DomainError, Process, ProcessId};
use async_trait::async_trait;

/// Repository port for process persistence
#[async_trait]
pub trait ProcessRepository: Send + Sync {
    /// Save a process (create or update)
    async fn save(&self, process: Process) -> Result<(), DomainError>;

    /// Find a process by ID
    async fn find_by_id(&self, id: &ProcessId) -> Result<Option<Process>, DomainError>;

    /// Find a process by name
    async fn find_by_name(&self, name: &str) -> Result<Option<Process>, DomainError>;

    /// List all processes
    async fn find_all(&self) -> Result<Vec<Process>, DomainError>;

    /// Delete a process
    async fn delete(&self, id: &ProcessId) -> Result<(), DomainError>;

    /// Check if a process with this name exists
    async fn exists_by_name(&self, name: &str) -> Result<bool, DomainError>;

    /// Find a process by either ID or name (convenience method)
    /// Returns error if neither or both are provided
    async fn find_by_id_or_name(
        &self,
        id: Option<&ProcessId>,
        name: Option<&str>,
    ) -> Result<Process, DomainError> {
        match (id, name) {
            (Some(id), None) => self
                .find_by_id(id)
                .await?
                .ok_or_else(|| DomainError::ProcessNotFound(id.to_string())),
            (None, Some(name)) => self
                .find_by_name(name)
                .await?
                .ok_or_else(|| DomainError::ProcessNotFound(name.to_string())),
            (Some(_), Some(_)) => Err(DomainError::InvalidCommand(
                "Cannot specify both process_id and process_name".to_string(),
            )),
            (None, None) => Err(DomainError::InvalidCommand(
                "Must specify either process_id or process_name".to_string(),
            )),
        }
    }
}

// Legacy struct for backward compatibility with existing use cases
#[derive(Debug, Clone)]
pub struct ProcessInfo {
    pub id: ProcessId,
    pub name: String,
}
