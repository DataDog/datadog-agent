//! ListProcesses use case
//! Query to retrieve all registered processes

use crate::domain::ports::ProcessRepository;
#[cfg(test)]
use crate::domain::Process;
use crate::domain::{DomainError, ListProcessesResponse};
use async_trait::async_trait;
use std::sync::Arc;

/// Use case for listing all processes
#[async_trait]
pub trait ListProcesses: Send + Sync {
    async fn execute(&self) -> Result<ListProcessesResponse, DomainError>;
}

/// Implementation of ListProcesses use case
pub struct ListProcessesUseCase {
    repository: Arc<dyn ProcessRepository>,
}

impl ListProcessesUseCase {
    pub fn new(repository: Arc<dyn ProcessRepository>) -> Self {
        Self { repository }
    }
}

#[async_trait]
impl ListProcesses for ListProcessesUseCase {
    async fn execute(&self) -> Result<ListProcessesResponse, DomainError> {
        let processes = self.repository.find_all().await?;
        Ok(ListProcessesResponse { processes })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::MockRepository;

    #[tokio::test]
    async fn test_list_empty() {
        let repo = Arc::new(MockRepository::new());
        let use_case = ListProcessesUseCase::new(repo);

        let result = use_case.execute().await.unwrap();
        assert_eq!(result.processes.len(), 0);
    }

    #[tokio::test]
    async fn test_list_with_processes() {
        let repo = Arc::new(MockRepository::new());

        // Add some test processes
        let process1 = Process::builder("process-1".to_string(), "/bin/app1".to_string())
            .build()
            .unwrap();
        let process2 = Process::builder("process-2".to_string(), "/bin/app2".to_string())
            .build()
            .unwrap();

        repo.save(process1).await.unwrap();
        repo.save(process2).await.unwrap();

        let use_case = ListProcessesUseCase::new(repo);
        let result = use_case.execute().await.unwrap();

        assert_eq!(result.processes.len(), 2);

        // Verify names are present (order may vary)
        let names: Vec<&str> = result.processes.iter().map(|p| p.name()).collect();
        assert!(names.contains(&"process-1"));
        assert!(names.contains(&"process-2"));
    }

    #[tokio::test]
    async fn test_list_many_processes() {
        let repo = Arc::new(MockRepository::new());

        // Add multiple processes
        for i in 0..10 {
            let process = Process::builder(format!("process-{}", i), "/bin/app".to_string())
                .build()
                .unwrap();
            repo.save(process).await.unwrap();
        }

        let use_case = ListProcessesUseCase::new(repo);
        let result = use_case.execute().await.unwrap();

        assert_eq!(result.processes.len(), 10);
    }
}
