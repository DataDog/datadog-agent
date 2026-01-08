//! GetProcessStatus use case
//! Query to get detailed status of a single process

use crate::domain::ports::ProcessRepository;
use crate::domain::{DomainError, GetProcessStatusQuery, ProcessStatusResponse};
#[cfg(test)]
use crate::domain::{Process, ProcessId};
use async_trait::async_trait;
use std::sync::Arc;

/// Use case for getting process status
#[async_trait]
pub trait GetProcessStatus: Send + Sync {
    async fn execute(
        &self,
        query: GetProcessStatusQuery,
    ) -> Result<ProcessStatusResponse, DomainError>;
}

/// Implementation of GetProcessStatus use case
pub struct GetProcessStatusUseCase {
    repository: Arc<dyn ProcessRepository>,
}

impl GetProcessStatusUseCase {
    pub fn new(repository: Arc<dyn ProcessRepository>) -> Self {
        Self { repository }
    }
}

#[async_trait]
impl GetProcessStatus for GetProcessStatusUseCase {
    async fn execute(
        &self,
        query: GetProcessStatusQuery,
    ) -> Result<ProcessStatusResponse, DomainError> {
        let process = self
            .repository
            .find_by_id_or_name(query.process_id.as_ref(), query.process_name.as_deref())
            .await?;

        Ok(ProcessStatusResponse { process })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::{ports::MockRepository, ProcessState};

    #[tokio::test]
    async fn test_get_process_status_success() {
        let repo = Arc::new(MockRepository::new());
        let use_case = GetProcessStatusUseCase::new(repo.clone());

        // Create a process
        let process = Process::builder("test-process".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Get status by ID
        let query = GetProcessStatusQuery::from_id(process_id);
        let result = use_case.execute(query).await.unwrap();

        assert_eq!(result.process.id(), process_id);
        assert_eq!(result.process.name(), "test-process");
        assert_eq!(result.process.state(), ProcessState::Created);
    }

    #[tokio::test]
    async fn test_get_nonexistent_process_status() {
        let repo = Arc::new(MockRepository::new());
        let use_case = GetProcessStatusUseCase::new(repo);

        let query = GetProcessStatusQuery::from_id(ProcessId::generate());
        let result = use_case.execute(query).await;

        assert!(matches!(result, Err(DomainError::ProcessNotFound(_))));
    }

    #[tokio::test]
    async fn test_get_running_process_status() {
        let repo = Arc::new(MockRepository::new());
        let use_case = GetProcessStatusUseCase::new(repo.clone());

        // Create and start a process
        let mut process = Process::builder("running-process".to_string(), "/bin/sleep".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_running(9999).unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Get status by ID
        let query = GetProcessStatusQuery::from_id(process_id);
        let result = use_case.execute(query).await.unwrap();

        assert_eq!(result.process.state(), ProcessState::Running);
        assert_eq!(result.process.pid(), Some(9999));
        assert!(result.process.is_running());
    }

    #[tokio::test]
    async fn test_get_process_status_by_name() {
        let repo = Arc::new(MockRepository::new());
        let use_case = GetProcessStatusUseCase::new(repo.clone());

        // Create a process
        let process = Process::builder("postgres".to_string(), "/usr/bin/postgres".to_string())
            .build()
            .unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Get status by name
        let query = GetProcessStatusQuery::from_name("postgres".to_string());
        let result = use_case.execute(query).await.unwrap();

        assert_eq!(result.process.id(), process_id);
        assert_eq!(result.process.name(), "postgres");
    }
}
