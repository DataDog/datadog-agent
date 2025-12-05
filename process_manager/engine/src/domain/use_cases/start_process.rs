//! StartProcess use case
//! Handles starting a process (spawning system process)

use crate::domain::ports::ProcessRepository;
use crate::domain::services::{ConflictResolutionService, ProcessLifecycleService};
#[cfg(test)]
use crate::domain::ProcessId;
use crate::domain::{DomainError, ProcessState, StartProcessCommand, StartProcessResponse};
use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::Arc;
use tracing::{debug, info, warn};

/// Use case for starting a process
#[async_trait]
pub trait StartProcess: Send + Sync {
    async fn execute(
        &self,
        command: StartProcessCommand,
    ) -> Result<StartProcessResponse, DomainError>;
}

/// Implementation of StartProcess use case
pub struct StartProcessUseCase {
    repository: Arc<dyn ProcessRepository>,
    conflict_service: Arc<ConflictResolutionService>,
    lifecycle_service: Arc<ProcessLifecycleService>,
}

impl StartProcessUseCase {
    pub fn new(
        repository: Arc<dyn ProcessRepository>,
        conflict_service: Arc<ConflictResolutionService>,
        lifecycle_service: Arc<ProcessLifecycleService>,
    ) -> Self {
        Self {
            repository,
            conflict_service,
            lifecycle_service,
        }
    }
}

#[async_trait]
impl StartProcess for StartProcessUseCase {
    async fn execute(
        &self,
        command: StartProcessCommand,
    ) -> Result<StartProcessResponse, DomainError> {
        // 1. Load the process from repository (by ID or name)
        let mut process = self
            .repository
            .find_by_id_or_name(command.process_id.as_ref(), command.process_name.as_deref())
            .await?;

        // 2. Check if process can be started
        if !process.can_start() {
            return Err(DomainError::InvalidStateTransition {
                from: process.state().to_string(),
                to: "starting".to_string(),
            });
        }

        // 3. Mark process as "starting" EARLY to prevent dependency cycles
        // This is critical: if A wants B and B binds_to A, when we recursively
        // start B, it will see A is already "starting" and won't try to start it again.
        process.mark_starting()?;
        self.repository.save(process.clone()).await?;

        // 4. Get all processes for dependency and conflict resolution
        let all_processes_vec = self.repository.find_all().await?;
        let all_processes: HashMap<String, _> = all_processes_vec
            .into_iter()
            .map(|p| (p.name().to_string(), p))
            .collect();

        // 5. Handle conflicts: Stop all conflicting processes (forward + bidirectional)
        self.conflict_service
            .stop_conflicting_processes(&process, &all_processes)
            .await?;

        // Auto-start hard dependencies first (requires, binds_to)
        // These MUST succeed or the process cannot start
        let hard_deps: Vec<String> = process
            .requires()
            .iter()
            .chain(process.binds_to().iter())
            .cloned()
            .collect();

        for dep_name in &hard_deps {
            if let Some(dep_process) = all_processes.get(dep_name) {
                if dep_process.state() != ProcessState::Running
                    && dep_process.state() != ProcessState::Starting
                {
                    info!(
                        process = %process.name(),
                        dependency = %dep_name,
                        "Auto-starting hard dependency"
                    );
                    let dep_command = StartProcessCommand::from_id(dep_process.id());
                    self.execute(dep_command).await?; // Hard dep failure stops parent
                }
            } else {
                return Err(DomainError::DependencyNotFound(dep_name.clone()));
            }
        }

        // Auto-start soft dependencies (wants)
        // These are best-effort - failures are logged but don't stop the parent
        for dep_name in process.wants() {
            if let Some(dep_process) = all_processes.get(dep_name) {
                if dep_process.state() != ProcessState::Running
                    && dep_process.state() != ProcessState::Starting
                {
                    info!(
                        process = %process.name(),
                        dependency = %dep_name,
                        "Auto-starting soft dependency (wants)"
                    );
                    let dep_command = StartProcessCommand::from_id(dep_process.id());
                    // Soft dependency - log error but continue
                    if let Err(e) = self.execute(dep_command).await {
                        warn!(
                            process = %process.name(),
                            dependency = %dep_name,
                            error = %e,
                            "Soft dependency failed to start, continuing anyway"
                        );
                    }
                }
            } else {
                warn!(
                    process = %process.name(),
                    dependency = %dep_name,
                    "Soft dependency not found, continuing"
                );
            }
        }

        debug!(
            process = %process.name(),
            "Dependency checks passed, checking filesystem conditions"
        );

        // 4. Check filesystem conditions (ConditionPathExists)
        use crate::domain::check_all_conditions;
        if !check_all_conditions(process.condition_path_exists()) {
            let process_name = process.name().to_string();
            warn!(
                process = %process_name,
                "Filesystem conditions not satisfied, refusing to start"
            );
            // Reset state back to created since we couldn't start
            process.reset_to_created();
            self.repository.save(process).await?;
            return Err(DomainError::InvalidCommand(format!(
                "Process '{}' cannot start: filesystem conditions not met",
                process_name
            )));
        }

        // 5. Check start limit (prevent restart thrashing)
        if process.is_start_limit_exceeded() {
            let state_str = process.state().to_string();
            warn!(
                process = %process.name(),
                burst = process.start_limit_burst(),
                interval = process.start_limit_interval_sec(),
                "Start limit exceeded, refusing to start"
            );
            // Reset state back to created since we couldn't start
            process.reset_to_created();
            self.repository.save(process).await?;
            return Err(DomainError::InvalidStateTransition {
                from: state_str,
                to: "starting (start limit exceeded)".to_string(),
            });
        }

        // 6. Record start time for start limit tracking
        process.record_start_time();
        self.repository.save(process.clone()).await?;

        // 8. Create runtime directories (if configured)
        if !process.runtime_directory().is_empty() {
            debug!(
                process = %process.name(),
                directories = ?process.runtime_directory(),
                "Creating runtime directories"
            );
            crate::domain::services::create_runtime_directories(
                process.runtime_directory(),
                process.user(),
                process.group(),
            )?;
        }

        // 7. Delegate to lifecycle service for spawn + register
        // This handles: pre-start hooks, spawn, mark running, reset failures,
        // register with supervisor, post-start hooks, increment run count
        self.lifecycle_service
            .spawn_and_register(
                &process.id(),
                command.listen_fds,
                command.fd_env_var_names,
                process.timeout_start_sec(),
            )
            .await?;

        // 8. Reload process to get updated state (including PID)
        let process = self.repository.find_by_id(&process.id()).await?.unwrap();

        // 9. Return response
        Ok(StartProcessResponse {
            process_id: process.id(),
            pid: process
                .pid()
                .expect("Process should have PID after spawning"),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::{MockRepository, ProcessExecutor, SpawnConfig, SpawnResult};
    use crate::domain::services::ProcessLifecycleService;
    use crate::domain::Process;
    use async_trait::async_trait;

    // Mock executor for testing
    struct MockExecutor {
        next_pid: u32,
    }

    impl MockExecutor {
        fn new() -> Self {
            Self { next_pid: 1234 }
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
    async fn test_start_process_by_id() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let lifecycle_service =
            Arc::new(ProcessLifecycleService::new(repo.clone(), executor.clone()));
        let conflict_service = Arc::new(ConflictResolutionService::new(
            repo.clone(),
            executor.clone(),
        ));
        let use_case = StartProcessUseCase::new(repo.clone(), conflict_service, lifecycle_service);

        // Create a process
        let process = Process::builder("test-process".to_string(), "/bin/sleep 10".to_string())
            .build()
            .unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Start the process by ID
        let command = StartProcessCommand::from_id(process_id);
        let result = use_case.execute(command).await.unwrap();

        assert_eq!(result.process_id, process_id);
        assert_eq!(result.pid, 1234);

        // Verify process is running in repository
        let updated = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert!(updated.is_running());
        assert_eq!(updated.pid(), Some(1234));
    }

    #[tokio::test]
    async fn test_start_process_by_name() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let lifecycle_service =
            Arc::new(ProcessLifecycleService::new(repo.clone(), executor.clone()));
        let conflict_service = Arc::new(ConflictResolutionService::new(
            repo.clone(),
            executor.clone(),
        ));
        let use_case = StartProcessUseCase::new(repo.clone(), conflict_service, lifecycle_service);

        // Create a process
        let process = Process::builder("my-service".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Start the process by name
        let command = StartProcessCommand::from_name("my-service".to_string());
        let result = use_case.execute(command).await.unwrap();

        assert_eq!(result.process_id, process_id);
        assert_eq!(result.pid, 1234);

        // Verify process is running in repository
        let updated = repo.find_by_name("my-service").await.unwrap().unwrap();
        assert!(updated.is_running());
    }

    #[tokio::test]
    async fn test_start_nonexistent_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let lifecycle_service =
            Arc::new(ProcessLifecycleService::new(repo.clone(), executor.clone()));
        let conflict_service = Arc::new(ConflictResolutionService::new(
            repo.clone(),
            executor.clone(),
        ));
        let use_case = StartProcessUseCase::new(repo, conflict_service, lifecycle_service);

        let command = StartProcessCommand::from_id(ProcessId::generate());
        let result = use_case.execute(command).await;

        assert!(matches!(result, Err(DomainError::ProcessNotFound(_))));
    }

    #[tokio::test]
    async fn test_start_already_running_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let lifecycle_service =
            Arc::new(ProcessLifecycleService::new(repo.clone(), executor.clone()));
        let conflict_service = Arc::new(ConflictResolutionService::new(
            repo.clone(),
            executor.clone(),
        ));
        let use_case = StartProcessUseCase::new(repo.clone(), conflict_service, lifecycle_service);

        // Create and start a process
        let mut process = Process::builder("test".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_running(9999).unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Try to start again
        let command = StartProcessCommand::from_id(process_id);
        let result = use_case.execute(command).await;

        assert!(matches!(
            result,
            Err(DomainError::InvalidStateTransition { .. })
        ));
    }
}
