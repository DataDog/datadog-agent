//! Use Case Registry
//! Central composition root for all use cases (Dependency Injection container)

use crate::domain::ports::{ProcessExecutor, ProcessRepository};
use crate::domain::services::{
    ConflictResolutionService, ProcessCreationService, ProcessLifecycleService,
    ProcessSpawningService, ProcessSupervisionService,
};
use crate::domain::use_cases::{
    CreateProcess, CreateProcessUseCase, DeleteProcess, DeleteProcessUseCase, GetProcessStatus,
    GetProcessStatusUseCase, GetResourceUsage, GetResourceUsageUseCase, ListProcesses,
    ListProcessesUseCase, LoadConfig, LoadConfigUseCase, ResourceUsageReader, RestartProcess,
    RestartProcessUseCase, StartProcess, StartProcessUseCase, StopProcess, StopProcessUseCase,
    UpdateProcess, UpdateProcessUseCase,
};
use std::sync::Arc;

/// Registry for all application use cases
/// This is the composition root where dependencies are wired together
pub struct UseCaseRegistry {
    // Command use cases (modify state)
    create_process: Arc<dyn CreateProcess>,
    start_process: Arc<dyn StartProcess>,
    stop_process: Arc<dyn StopProcess>,
    restart_process: Arc<dyn RestartProcess>,
    update_process: Arc<dyn UpdateProcess>,
    delete_process: Arc<dyn DeleteProcess>,
    load_config: Arc<dyn LoadConfig>,

    // Query use cases (read state)
    list_processes: Arc<dyn ListProcesses>,
    get_process_status: Arc<dyn GetProcessStatus>,
    get_resource_usage: Arc<dyn GetResourceUsage>,

    // Domain services (exposed for infrastructure layer)
    spawn_service: Arc<ProcessSpawningService>,
}

impl UseCaseRegistry {
    /// Create a new registry with all use cases configured
    ///
    /// # Arguments
    ///
    /// * `repository` - Process storage adapter
    /// * `executor` - Process execution adapter (must also implement ResourceUsageReader)
    pub fn new<E>(repository: Arc<dyn ProcessRepository>, executor: Arc<E>) -> Self
    where
        E: ProcessExecutor + ResourceUsageReader + 'static,
    {
        Self::new_internal(repository, executor.clone(), executor, None)
    }

    /// Create a new registry with process supervisor coordination
    ///
    /// # Arguments
    ///
    /// * `repository` - Process storage adapter
    /// * `executor` - Process execution adapter (must also implement ResourceUsageReader)
    /// * `supervisor` - Process supervisor (coordinates exit monitoring + health monitoring)
    pub fn new_with_supervisor<E>(
        repository: Arc<dyn ProcessRepository>,
        executor: Arc<E>,
        supervisor: Arc<ProcessSupervisionService>,
    ) -> Self
    where
        E: ProcessExecutor + ResourceUsageReader + 'static,
    {
        Self::new_internal(repository, executor.clone(), executor, Some(supervisor))
    }

    /// Internal constructor that wires up all use cases
    fn new_internal(
        repository: Arc<dyn ProcessRepository>,
        executor: Arc<dyn ProcessExecutor>,
        resource_reader: Arc<dyn ResourceUsageReader>,
        supervisor: Option<Arc<ProcessSupervisionService>>,
    ) -> Self {
        // Wire up domain services
        let creation_service = Arc::new(ProcessCreationService::new(repository.clone()));

        let lifecycle_service = if let Some(ref supervisor) = supervisor {
            Arc::new(ProcessLifecycleService::with_supervisor(
                repository.clone(),
                executor.clone(),
                supervisor.clone(),
            ))
        } else {
            Arc::new(ProcessLifecycleService::new(
                repository.clone(),
                executor.clone(),
            ))
        };

        // Create conflict service
        let conflict_service = Arc::new(ConflictResolutionService::new(
            repository.clone(),
            executor.clone(),
        ));

        // Wire up command use cases
        let create_process = Arc::new(CreateProcessUseCase::new(creation_service.clone()));

        let start_process: Arc<dyn StartProcess> = Arc::new(StartProcessUseCase::new(
            repository.clone(),
            conflict_service,
            lifecycle_service.clone(),
        ));

        let stop_process: Arc<dyn StopProcess> = Arc::new(StopProcessUseCase::new(
            repository.clone(),
            executor.clone(),
        ));

        let restart_process = Arc::new(RestartProcessUseCase::new(
            repository.clone(),
            executor.clone(),
        ));

        let update_process = Arc::new(UpdateProcessUseCase::new(
            repository.clone(),
            stop_process.clone(),
            start_process.clone(),
        ));

        let delete_process = Arc::new(DeleteProcessUseCase::new(
            repository.clone(),
            executor.clone(),
        ));

        let load_config = Arc::new(LoadConfigUseCase::new(
            creation_service.clone(),
            start_process.clone(),
        ));

        // Wire up domain services (exposed for infrastructure layer)
        let spawn_service = if let Some(ref supervisor) = supervisor {
            Arc::new(ProcessSpawningService::new(
                repository.clone(),
                lifecycle_service.clone(),
                supervisor.clone(),
            ))
        } else {
            // If no supervisor, create a minimal one
            // (spawn service requires supervisor for proper coordination)
            Arc::new(ProcessSpawningService::new(
                repository.clone(),
                lifecycle_service.clone(),
                Arc::new(ProcessSupervisionService::new(
                    repository.clone(),
                    executor.clone(),
                )),
            ))
        };

        // Wire up query use cases
        let list_processes = Arc::new(ListProcessesUseCase::new(repository.clone()));
        let get_process_status = Arc::new(GetProcessStatusUseCase::new(repository.clone()));
        let get_resource_usage = Arc::new(GetResourceUsageUseCase::new(
            repository.clone(),
            resource_reader, // Separate trait object for resource usage reading
        ));

        Self {
            create_process,
            start_process,
            stop_process,
            restart_process,
            update_process,
            delete_process,
            load_config,
            list_processes,
            get_process_status,
            get_resource_usage,
            spawn_service,
        }
    }

    // ===== Command Use Cases =====

    /// Get the CreateProcess use case
    pub fn create_process(&self) -> Arc<dyn CreateProcess> {
        self.create_process.clone()
    }

    /// Get the StartProcess use case
    pub fn start_process(&self) -> Arc<dyn StartProcess> {
        self.start_process.clone()
    }

    /// Get the StopProcess use case
    pub fn stop_process(&self) -> Arc<dyn StopProcess> {
        self.stop_process.clone()
    }

    /// Get the RestartProcess use case
    pub fn restart_process(&self) -> Arc<dyn RestartProcess> {
        self.restart_process.clone()
    }

    /// Get the UpdateProcess use case
    pub fn update_process(&self) -> Arc<dyn UpdateProcess> {
        self.update_process.clone()
    }

    /// Get the DeleteProcess use case
    pub fn delete_process(&self) -> Arc<dyn DeleteProcess> {
        self.delete_process.clone()
    }

    /// Get the LoadConfig use case
    pub fn load_config(&self) -> Arc<dyn LoadConfig> {
        self.load_config.clone()
    }

    // ===== Query Use Cases =====

    /// Get the ListProcesses use case
    pub fn list_processes(&self) -> Arc<dyn ListProcesses> {
        self.list_processes.clone()
    }

    /// Get the GetProcessStatus use case
    pub fn get_process_status(&self) -> Arc<dyn GetProcessStatus> {
        self.get_process_status.clone()
    }

    /// Get the GetResourceUsage use case
    pub fn get_resource_usage(&self) -> Arc<dyn GetResourceUsage> {
        self.get_resource_usage.clone()
    }

    // ===== Domain Services (exposed for infrastructure layer) =====

    /// Get the ProcessSpawningService
    pub fn spawn_service(&self) -> Arc<ProcessSpawningService> {
        self.spawn_service.clone()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::{MockRepository, ProcessExecutor, SpawnConfig, SpawnResult};
    use crate::domain::{CreateProcessCommand, DomainError};

    // Mock executor for tests
    struct MockExecutor;

    #[async_trait::async_trait]
    impl ProcessExecutor for MockExecutor {
        async fn spawn(&self, _config: SpawnConfig) -> Result<SpawnResult, DomainError> {
            Ok(SpawnResult {
                pid: 12345,
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

    // Implement ResourceUsageReader for MockExecutor
    impl ResourceUsageReader for MockExecutor {
        fn get_usage(&self, _pid: Option<u32>) -> Option<crate::domain::ResourceUsage> {
            None // Mock returns no usage data
        }
    }

    #[tokio::test]
    async fn test_registry_creation() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let registry = UseCaseRegistry::new(repo, executor);

        // Should be able to access use cases through the registry
        let use_case = registry.create_process();

        // Verify it works
        let command = CreateProcessCommand {
            name: "test-from-registry".to_string(),
            command: "/usr/bin/test".to_string(),
            args: vec![],
            ..Default::default()
        };

        let result = use_case.execute(command).await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_registry_use_case_isolation() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let registry = UseCaseRegistry::new(repo, executor);

        // Create process through registry
        let create_use_case = registry.create_process();

        let command = CreateProcessCommand {
            name: "isolated-process".to_string(),
            command: "/bin/app".to_string(),
            args: vec![],
            ..Default::default()
        };

        let result = create_use_case.execute(command).await.unwrap();
        assert_eq!(result.name, "isolated-process");
    }

    #[tokio::test]
    async fn test_registry_cqrs_flow() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let registry = UseCaseRegistry::new(repo, executor);

        // Initially empty
        let list_result = registry.list_processes().execute().await.unwrap();
        assert_eq!(list_result.processes.len(), 0);

        // Create a process (Command)
        let create_command = CreateProcessCommand {
            name: "cqrs-test".to_string(),
            command: "/usr/bin/app".to_string(),
            args: vec![],
            ..Default::default()
        };
        registry
            .create_process()
            .execute(create_command)
            .await
            .unwrap();

        // List processes (Query)
        let list_result = registry.list_processes().execute().await.unwrap();
        assert_eq!(list_result.processes.len(), 1);
        assert_eq!(list_result.processes[0].name(), "cqrs-test");
    }

    #[tokio::test]
    async fn test_registry_all_use_cases_accessible() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let registry = UseCaseRegistry::new(repo, executor);

        // Verify all use cases are accessible
        let _ = registry.create_process();
        let _ = registry.start_process();
        let _ = registry.stop_process();
        let _ = registry.restart_process();
        let _ = registry.update_process();
        let _ = registry.delete_process();
        let _ = registry.list_processes();
        let _ = registry.get_process_status();
    }
}
