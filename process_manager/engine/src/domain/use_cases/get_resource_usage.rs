use crate::domain::{
    ports::ProcessRepository, DomainError, GetResourceUsageCommand, GetResourceUsageResponse,
    ResourceUsage,
};
use async_trait::async_trait;
use std::sync::Arc;

/// Get Resource Usage use case
#[async_trait]
pub trait GetResourceUsage: Send + Sync {
    async fn execute(
        &self,
        command: GetResourceUsageCommand,
    ) -> Result<GetResourceUsageResponse, DomainError>;
}

/// Implementation of GetResourceUsage use case
pub struct GetResourceUsageUseCase {
    repository: Arc<dyn ProcessRepository>,
    resource_reader: Arc<dyn ResourceUsageReader>,
}

/// Trait for reading resource usage from system
#[async_trait]
pub trait ResourceUsageReader: Send + Sync {
    fn get_usage(&self, pid: Option<u32>) -> Option<ResourceUsage>;
}

impl GetResourceUsageUseCase {
    pub fn new(
        repository: Arc<dyn ProcessRepository>,
        resource_reader: Arc<dyn ResourceUsageReader>,
    ) -> Self {
        Self {
            repository,
            resource_reader,
        }
    }
}

#[async_trait]
impl GetResourceUsage for GetResourceUsageUseCase {
    async fn execute(
        &self,
        command: GetResourceUsageCommand,
    ) -> Result<GetResourceUsageResponse, DomainError> {
        // 1. Find the process (using convenience method that handles Option unwrapping)
        let process = self
            .repository
            .find_by_id_or_name(command.process_id.as_ref(), command.process_name.as_deref())
            .await?;

        // 2. Get resource usage from system (cgroups)
        let usage = self
            .resource_reader
            .get_usage(process.pid())
            .unwrap_or_else(ResourceUsage::empty);

        // 3. Check if any usage data is available
        if !usage.has_data() {
            return Err(DomainError::InvalidCommand(format!(
                "Resource usage not available for process '{}'. \
                This may occur if cgroups v2 is not enabled or the process is not running.",
                process.name()
            )));
        }

        // 4. Return usage and configured limits
        Ok(GetResourceUsageResponse {
            process_id: process.id(),
            process_name: process.name().to_string(),
            usage,
            limits: process.resource_limits().clone(),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::Process;
    use crate::infrastructure::InMemoryProcessRepository;

    struct MockResourceUsageReader {
        usage: Option<ResourceUsage>,
    }

    #[async_trait]
    impl ResourceUsageReader for MockResourceUsageReader {
        fn get_usage(&self, _pid: Option<u32>) -> Option<ResourceUsage> {
            self.usage.clone()
        }
    }

    #[tokio::test]
    async fn test_get_resource_usage_success() {
        let repository = Arc::new(InMemoryProcessRepository::new());
        let mock_usage = ResourceUsage {
            memory_current: Some(1024 * 1024),  // 1MB
            memory_peak: Some(2 * 1024 * 1024), // 2MB
            cpu_usage_usec: Some(1000000),      // 1 second
            cpu_user_usec: Some(500000),
            cpu_system_usec: Some(500000),
            pids_current: Some(5),
        };

        let resource_reader = Arc::new(MockResourceUsageReader {
            usage: Some(mock_usage.clone()),
        });

        let use_case = GetResourceUsageUseCase::new(repository.clone(), resource_reader);

        // Create a test process
        let mut process = Process::builder("test-process".to_string(), "/bin/sleep".to_string())
            .build()
            .expect("Failed to create process");
        process.mark_starting().expect("Failed to mark starting");
        process.mark_running(1234).expect("Failed to mark running");

        let process_id = process.id();
        repository
            .save(process)
            .await
            .expect("Failed to save process");

        // Get resource usage
        let command = GetResourceUsageCommand::from_id(process_id);
        let result = use_case.execute(command).await;

        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response.process_name, "test-process");
        assert_eq!(response.usage.memory_current, Some(1024 * 1024));
        assert_eq!(response.usage.pids_current, Some(5));
    }

    #[tokio::test]
    async fn test_get_resource_usage_not_available() {
        let repository = Arc::new(InMemoryProcessRepository::new());
        let resource_reader = Arc::new(MockResourceUsageReader { usage: None });

        let use_case = GetResourceUsageUseCase::new(repository.clone(), resource_reader);

        // Create a test process
        let process = Process::builder("test-process".to_string(), "/bin/sleep".to_string())
            .build()
            .expect("Failed to create process");

        let process_id = process.id();
        repository
            .save(process)
            .await
            .expect("Failed to save process");

        // Try to get resource usage (should fail gracefully)
        let command = GetResourceUsageCommand::from_id(process_id);
        let result = use_case.execute(command).await;

        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(matches!(err, DomainError::InvalidCommand(_)));
    }

    #[tokio::test]
    async fn test_get_resource_usage_process_not_found() {
        let repository = Arc::new(InMemoryProcessRepository::new());
        let resource_reader = Arc::new(MockResourceUsageReader { usage: None });

        let use_case = GetResourceUsageUseCase::new(repository, resource_reader);

        let command = GetResourceUsageCommand::from_name("non-existent".to_string());
        let result = use_case.execute(command).await;

        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(matches!(err, DomainError::ProcessNotFound(_)));
    }
}
