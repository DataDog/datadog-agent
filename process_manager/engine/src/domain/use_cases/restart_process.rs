//! RestartProcess use case
//! Handles restarting of processes (stop + start)

use crate::domain::ports::{ProcessExecutor, ProcessRepository};
use crate::domain::{DomainError, ProcessState, RestartProcessCommand, RestartProcessResponse};
use async_trait::async_trait;
use std::sync::Arc;

/// Use case for restarting a process
#[async_trait]
pub trait RestartProcess: Send + Sync {
    async fn execute(
        &self,
        command: RestartProcessCommand,
    ) -> Result<RestartProcessResponse, DomainError>;
}

/// Implementation of RestartProcess use case
pub struct RestartProcessUseCase {
    repository: Arc<dyn ProcessRepository>,
    executor: Arc<dyn ProcessExecutor>,
}

impl RestartProcessUseCase {
    pub fn new(repository: Arc<dyn ProcessRepository>, executor: Arc<dyn ProcessExecutor>) -> Self {
        Self {
            repository,
            executor,
        }
    }
}

#[async_trait]
impl RestartProcess for RestartProcessUseCase {
    async fn execute(
        &self,
        command: RestartProcessCommand,
    ) -> Result<RestartProcessResponse, DomainError> {
        // 1. Load the process from repository (by ID or name)
        let mut process = self
            .repository
            .find_by_id_or_name(command.process_id.as_ref(), command.process_name.as_deref())
            .await?;

        // 2. If process is running, stop it first
        if process.is_running() {
            let pid = process.pid().ok_or(DomainError::NotRunning)?;

            // Mark as stopping
            process.mark_stopping()?;
            self.repository.save(process.clone()).await?;

            // Kill the system process
            self.executor.kill(pid, 15).await?;

            // Mark as stopped
            process.mark_stopped()?;
            self.repository.save(process.clone()).await?;
        }

        // 3. Mark as restarting (transitional state)
        if matches!(process.state(), ProcessState::Failed | ProcessState::Exited) {
            process.mark_restarting()?;
            self.repository.save(process.clone()).await?;
        }

        // 4. Mark as starting
        process.mark_starting()?;
        self.repository.save(process.clone()).await?;

        // 5. Spawn the system process
        use crate::domain::ports::SpawnConfig;
        let config = SpawnConfig::from_process(&process);
        let result = self.executor.spawn(config).await?;

        // 6. Mark as running
        process.mark_running(result.pid)?;
        let process_id = process.id();
        self.repository.save(process).await?;

        // 7. Return response
        Ok(RestartProcessResponse {
            process_id,
            pid: result.pid,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::{MockRepository, ProcessExecutor, SpawnConfig, SpawnResult};
    use crate::domain::Process;
    use async_trait::async_trait;

    // Mock executor for testing
    struct MockExecutor {
        next_pid: u32,
    }

    impl MockExecutor {
        fn new() -> Self {
            Self { next_pid: 5678 }
        }
    }

    #[async_trait]
    impl ProcessExecutor for MockExecutor {
        async fn spawn(&self, _config: SpawnConfig) -> Result<SpawnResult, DomainError> {
            Ok(SpawnResult {
                pid: self.next_pid,
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
            Ok(true)
        }

        async fn wait_for_exit(&self, _pid: u32) -> Result<i32, DomainError> {
            Ok(0)
        }
    }

    #[tokio::test]
    async fn test_restart_stopped_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let use_case = RestartProcessUseCase::new(repo.clone(), executor);

        // Create a stopped process
        let mut process = Process::builder("service".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_running(1000).unwrap();
        process.mark_stopping().unwrap();
        process.mark_stopped().unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Restart
        let command = RestartProcessCommand::from_id(process_id);
        let result = use_case.execute(command).await.unwrap();

        assert_eq!(result.process_id, process_id);
        assert!(result.pid > 0);

        // Check state
        let process = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert_eq!(process.state(), ProcessState::Running);
    }

    #[tokio::test]
    async fn test_restart_running_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let use_case = RestartProcessUseCase::new(repo.clone(), executor);

        // Create a running process
        let mut process = Process::builder("service".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_running(1000).unwrap();
        let process_id = process.id();
        let old_pid = process.pid().unwrap();
        repo.save(process).await.unwrap();

        // Restart
        let command = RestartProcessCommand::from_id(process_id);
        let result = use_case.execute(command).await.unwrap();

        assert_eq!(result.process_id, process_id);
        assert_ne!(result.pid, old_pid); // New PID

        // Check state
        let process = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert_eq!(process.state(), ProcessState::Running);
        assert_eq!(process.pid(), Some(result.pid));
    }

    #[tokio::test]
    async fn test_restart_failed_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let use_case = RestartProcessUseCase::new(repo.clone(), executor);

        // Create a failed process
        let mut process = Process::builder("crasher".to_string(), "/bin/crash".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_exited(1).unwrap(); // Exit code 1 = failed
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Restart
        let command = RestartProcessCommand::from_name("crasher".to_string());
        let result = use_case.execute(command).await.unwrap();

        assert_eq!(result.process_id, process_id);

        // Check state
        let process = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert_eq!(process.state(), ProcessState::Running);
    }
}
