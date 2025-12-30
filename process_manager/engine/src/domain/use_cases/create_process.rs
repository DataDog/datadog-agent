//! CreateProcess use case
//! Handles creation and registration of new processes

use crate::domain::services::ProcessCreationService;
use crate::domain::{CreateProcessCommand, CreateProcessResponse, DomainError};
use async_trait::async_trait;
use std::sync::Arc;

/// Use case for creating a new process
#[async_trait]
pub trait CreateProcess: Send + Sync {
    async fn execute(
        &self,
        command: CreateProcessCommand,
    ) -> Result<CreateProcessResponse, DomainError>;
}

/// Implementation of CreateProcess use case
pub struct CreateProcessUseCase {
    creation_service: Arc<ProcessCreationService>,
}

impl CreateProcessUseCase {
    pub fn new(creation_service: Arc<ProcessCreationService>) -> Self {
        Self { creation_service }
    }
}

#[async_trait]
impl CreateProcess for CreateProcessUseCase {
    async fn execute(
        &self,
        command: CreateProcessCommand,
    ) -> Result<CreateProcessResponse, DomainError> {
        // Delegate to creation service
        let (id, name) = self.creation_service.create_from_command(command).await?;

        Ok(CreateProcessResponse { id, name })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::{MockRepository, ProcessRepository};

    #[tokio::test]
    async fn test_create_valid_process() {
        let repo = Arc::new(MockRepository::new());
        let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
        let use_case = CreateProcessUseCase::new(creation_service);

        let command = CreateProcessCommand {
            name: "my-service".to_string(),
            command: "/usr/bin/myapp".to_string(),
            args: vec![],
            ..Default::default()
        };

        let result = use_case.execute(command).await;
        assert!(result.is_ok());

        let response = result.unwrap();
        assert_eq!(response.name, "my-service");

        // Verify it was saved
        let found = repo.find_by_id(&response.id).await.unwrap();
        assert!(found.is_some());
        assert_eq!(found.unwrap().name(), "my-service");
    }

    #[tokio::test]
    async fn test_reject_duplicate_name() {
        let repo = Arc::new(MockRepository::new());
        let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
        let use_case = CreateProcessUseCase::new(creation_service);

        let command = CreateProcessCommand {
            name: "duplicate".to_string(),
            command: "/usr/bin/app".to_string(),
            args: vec![],
            ..Default::default()
        };

        // Create first process
        let result1 = use_case.execute(command.clone()).await;
        assert!(result1.is_ok());

        // Try to create with same name
        let result2 = use_case.execute(command).await;
        assert!(result2.is_err());

        match result2.unwrap_err() {
            DomainError::DuplicateProcess(name) => assert_eq!(name, "duplicate"),
            _ => panic!("Expected DuplicateProcess error"),
        }
    }

    #[tokio::test]
    async fn test_validate_configuration() {
        let repo = Arc::new(MockRepository::new());
        let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
        let use_case = CreateProcessUseCase::new(creation_service);

        // Empty name
        let result = use_case
            .execute(CreateProcessCommand {
                name: "".to_string(),
                command: "/usr/bin/app".to_string(),
                args: vec![],
                ..Default::default()
            })
            .await;
        assert!(matches!(result, Err(DomainError::InvalidName(_))));

        // Name with whitespace
        let result = use_case
            .execute(CreateProcessCommand {
                name: "my service".to_string(),
                command: "/usr/bin/app".to_string(),
                args: vec![],
                ..Default::default()
            })
            .await;
        assert!(matches!(result, Err(DomainError::InvalidName(_))));

        // Empty command
        let result = use_case
            .execute(CreateProcessCommand {
                name: "valid-name".to_string(),
                command: "".to_string(),
                args: vec![],
                ..Default::default()
            })
            .await;
        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
    }
}
