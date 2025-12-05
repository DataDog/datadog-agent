//! DeleteProcess use case
//! Handles deletion of process definitions

use crate::domain::ports::{ProcessExecutor, ProcessRepository};
#[cfg(test)]
use crate::domain::ProcessId;
use crate::domain::{DeleteProcessCommand, DeleteProcessResponse, DomainError};
use async_trait::async_trait;
use std::sync::Arc;
use tracing::warn;

/// Use case for deleting a process
#[async_trait]
pub trait DeleteProcess: Send + Sync {
    async fn execute(
        &self,
        command: DeleteProcessCommand,
    ) -> Result<DeleteProcessResponse, DomainError>;
}

/// Implementation of DeleteProcess use case
pub struct DeleteProcessUseCase {
    repository: Arc<dyn ProcessRepository>,
    executor: Arc<dyn ProcessExecutor>,
}

impl DeleteProcessUseCase {
    pub fn new(repository: Arc<dyn ProcessRepository>, executor: Arc<dyn ProcessExecutor>) -> Self {
        Self {
            repository,
            executor,
        }
    }
}

#[async_trait]
impl DeleteProcess for DeleteProcessUseCase {
    async fn execute(
        &self,
        command: DeleteProcessCommand,
    ) -> Result<DeleteProcessResponse, DomainError> {
        // 1. Load the process from repository (by ID or name)
        let process = self
            .repository
            .find_by_id_or_name(command.process_id.as_ref(), command.process_name.as_deref())
            .await?;

        let process_id = process.id();
        let process_name = process.name().to_string();

        // 2. Check if process is running
        if process.is_running() {
            if command.force {
                // Force delete: stop the process first
                warn!(
                    process_id = %process_id,
                    process_name = %process_name,
                    "Force deleting running process - stopping first"
                );

                if let Some(pid) = process.pid() {
                    self.executor
                        .kill_with_mode(
                            pid,
                            15, // SIGTERM
                            process.kill_mode(),
                        )
                        .await?;
                }
            } else {
                // Not forced: reject the delete
                return Err(DomainError::InvalidStateTransition {
                    from: process.state().to_string(),
                    to: "deleted".to_string(),
                });
            }
        }

        // 3. Delete from repository
        self.repository.delete(&process_id).await?;

        // 4. Return response
        Ok(DeleteProcessResponse { process_id })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::{MockRepository, ProcessExecutor, SpawnConfig, SpawnResult};
    use crate::domain::Process;
    use async_trait::async_trait;

    // Mock executor for testing
    struct MockExecutor;

    #[async_trait]
    impl ProcessExecutor for MockExecutor {
        async fn spawn(&self, _config: SpawnConfig) -> Result<SpawnResult, DomainError> {
            Ok(SpawnResult {
                pid: 1234,
                exit_handle: None,
            })
        }

        async fn kill(&self, _pid: u32, _signal: i32) -> Result<(), DomainError> {
            Ok(())
        }

        async fn kill_with_mode(
            &self,
            _pid: u32,
            _signal: i32,
            _mode: crate::domain::KillMode,
        ) -> Result<(), DomainError> {
            Ok(())
        }

        async fn is_running(&self, _pid: u32) -> Result<bool, DomainError> {
            Ok(false)
        }

        async fn wait_for_exit(&self, _pid: u32) -> Result<i32, DomainError> {
            Ok(0)
        }
    }

    #[tokio::test]
    async fn test_delete_process_by_id() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let use_case = DeleteProcessUseCase::new(repo.clone(), executor);

        // Create a process
        let process = Process::builder("test-process".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Delete by ID
        let command = DeleteProcessCommand::from_id(process_id, false);
        let result = use_case.execute(command).await.unwrap();

        assert_eq!(result.process_id, process_id);

        // Verify it's gone
        let found = repo.find_by_id(&process_id).await.unwrap();
        assert!(found.is_none());
    }

    #[tokio::test]
    async fn test_delete_process_by_name() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let use_case = DeleteProcessUseCase::new(repo.clone(), executor);

        // Create a process
        let process = Process::builder("old-service".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Delete by name
        let command = DeleteProcessCommand::from_name("old-service".to_string(), false);
        let result = use_case.execute(command).await.unwrap();

        assert_eq!(result.process_id, process_id);

        // Verify it's gone
        let found = repo.find_by_name("old-service").await.unwrap();
        assert!(found.is_none());
    }

    #[tokio::test]
    async fn test_delete_nonexistent_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let use_case = DeleteProcessUseCase::new(repo, executor);

        let command = DeleteProcessCommand::from_id(ProcessId::generate(), false);
        let result = use_case.execute(command).await;

        assert!(matches!(result, Err(DomainError::ProcessNotFound(_))));
    }

    #[tokio::test]
    async fn test_cannot_delete_running_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let use_case = DeleteProcessUseCase::new(repo.clone(), executor);

        // Create and start a process
        let mut process = Process::builder("running".to_string(), "/bin/sleep".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_running(1234).unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Try to delete
        let command = DeleteProcessCommand::from_id(process_id, false);
        let result = use_case.execute(command).await;

        assert!(matches!(
            result,
            Err(DomainError::InvalidStateTransition { .. })
        ));

        // Verify it's still there
        let found = repo.find_by_id(&process_id).await.unwrap();
        assert!(found.is_some());
    }
}
