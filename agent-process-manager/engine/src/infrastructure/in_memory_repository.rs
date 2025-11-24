//! In-Memory Process Repository
//! Thread-safe implementation of ProcessRepository port

use crate::domain::{ports::ProcessRepository, DomainError, Process, ProcessId};
use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::{Arc, RwLock};
use tracing::{debug, info};

/// Thread-safe in-memory process repository
///
/// This adapter provides a production-ready in-memory storage solution
/// suitable for single-instance deployments
#[derive(Clone)]
pub struct InMemoryProcessRepository {
    processes: Arc<RwLock<HashMap<ProcessId, Process>>>,
}

impl InMemoryProcessRepository {
    pub fn new() -> Self {
        Self {
            processes: Arc::new(RwLock::new(HashMap::new())),
        }
    }
}

impl Default for InMemoryProcessRepository {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ProcessRepository for InMemoryProcessRepository {
    async fn save(&self, process: Process) -> Result<(), DomainError> {
        let process_id = process.id();
        let process_name = process.name().to_string();

        debug!(
            process_id = %process_id,
            process_name = %process_name,
            "Saving process to repository"
        );

        let mut processes = self.processes.write().unwrap();
        processes.insert(process_id, process);

        info!(
            process_id = %process_id,
            process_name = %process_name,
            total_processes = processes.len(),
            "Process saved successfully"
        );

        Ok(())
    }

    async fn find_by_id(&self, id: &ProcessId) -> Result<Option<Process>, DomainError> {
        let processes = self.processes.read().unwrap();
        Ok(processes.get(id).cloned())
    }

    async fn find_by_name(&self, name: &str) -> Result<Option<Process>, DomainError> {
        let processes = self.processes.read().unwrap();
        Ok(processes.values().find(|p| p.name() == name).cloned())
    }

    async fn find_all(&self) -> Result<Vec<Process>, DomainError> {
        let processes = self.processes.read().unwrap();
        Ok(processes.values().cloned().collect())
    }

    async fn delete(&self, id: &ProcessId) -> Result<(), DomainError> {
        debug!(process_id = %id, "Deleting process from repository");

        let mut processes = self.processes.write().unwrap();
        processes.remove(id);

        info!(
            process_id = %id,
            remaining_processes = processes.len(),
            "Process deleted successfully"
        );

        Ok(())
    }

    async fn exists_by_name(&self, name: &str) -> Result<bool, DomainError> {
        let processes = self.processes.read().unwrap();
        Ok(processes.values().any(|p| p.name() == name))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_save_and_find_by_id() {
        let repo = InMemoryProcessRepository::new();
        let process = Process::builder("test".to_string(), "/bin/test".to_string())
            .build()
            .unwrap();
        let process_id = process.id();

        // Save
        repo.save(process.clone()).await.unwrap();

        // Find
        let found = repo.find_by_id(&process_id).await.unwrap();
        assert!(found.is_some());
        assert_eq!(found.unwrap().name(), "test");
    }

    #[tokio::test]
    async fn test_find_by_name() {
        let repo = InMemoryProcessRepository::new();
        let process = Process::builder("nginx".to_string(), "/usr/bin/nginx".to_string())
            .build()
            .unwrap();

        repo.save(process).await.unwrap();

        let found = repo.find_by_name("nginx").await.unwrap();
        assert!(found.is_some());
        assert_eq!(found.unwrap().name(), "nginx");

        let not_found = repo.find_by_name("apache").await.unwrap();
        assert!(not_found.is_none());
    }

    #[tokio::test]
    async fn test_find_all() {
        let repo = InMemoryProcessRepository::new();

        let p1 = Process::builder("app1".to_string(), "/bin/app1".to_string())
            .build()
            .unwrap();
        let p2 = Process::builder("app2".to_string(), "/bin/app2".to_string())
            .build()
            .unwrap();
        let p3 = Process::builder("app3".to_string(), "/bin/app3".to_string())
            .build()
            .unwrap();

        repo.save(p1).await.unwrap();
        repo.save(p2).await.unwrap();
        repo.save(p3).await.unwrap();

        let all = repo.find_all().await.unwrap();
        assert_eq!(all.len(), 3);
    }

    #[tokio::test]
    async fn test_delete() {
        let repo = InMemoryProcessRepository::new();
        let process = Process::builder("temp".to_string(), "/bin/temp".to_string())
            .build()
            .unwrap();
        let process_id = process.id();

        repo.save(process).await.unwrap();
        assert!(repo.find_by_id(&process_id).await.unwrap().is_some());

        repo.delete(&process_id).await.unwrap();
        assert!(repo.find_by_id(&process_id).await.unwrap().is_none());
    }

    #[tokio::test]
    async fn test_exists_by_name() {
        let repo = InMemoryProcessRepository::new();
        let process = Process::builder("postgres".to_string(), "/usr/bin/postgres".to_string())
            .build()
            .unwrap();

        assert!(!repo.exists_by_name("postgres").await.unwrap());

        repo.save(process).await.unwrap();

        assert!(repo.exists_by_name("postgres").await.unwrap());
        assert!(!repo.exists_by_name("mysql").await.unwrap());
    }

    #[tokio::test]
    async fn test_update_process() {
        let repo = InMemoryProcessRepository::new();
        let mut process = Process::builder("updatable".to_string(), "/bin/old".to_string())
            .build()
            .unwrap();
        let process_id = process.id();

        // Save initial
        repo.save(process.clone()).await.unwrap();

        // Update
        process.update_command("/bin/new".to_string()).unwrap();
        repo.save(process).await.unwrap();

        // Verify update
        let found = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert_eq!(found.command(), "/bin/new");
    }

    #[tokio::test]
    async fn test_thread_safety() {
        let repo = InMemoryProcessRepository::new();
        let repo_clone1 = repo.clone();
        let repo_clone2 = repo.clone();

        // Spawn concurrent tasks
        let handle1 = tokio::spawn(async move {
            for i in 0..10 {
                let process = Process::builder(format!("proc-{}", i), "/bin/test".to_string())
                    .build()
                    .unwrap();
                repo_clone1.save(process).await.unwrap();
            }
        });

        let handle2 = tokio::spawn(async move {
            for i in 10..20 {
                let process = Process::builder(format!("proc-{}", i), "/bin/test".to_string())
                    .build()
                    .unwrap();
                repo_clone2.save(process).await.unwrap();
            }
        });

        handle1.await.unwrap();
        handle2.await.unwrap();

        // Verify all were saved
        let all = repo.find_all().await.unwrap();
        assert_eq!(all.len(), 20);
    }
}
