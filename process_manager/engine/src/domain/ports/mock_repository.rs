//! Mock repository implementation for testing
//! This is a simple in-memory implementation for unit tests

use crate::domain::{DomainError, Process, ProcessId};
use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::{Arc, Mutex};

use super::ProcessRepository;

/// In-memory mock repository for testing
#[derive(Clone)]
pub struct MockRepository {
    storage: Arc<Mutex<HashMap<ProcessId, Process>>>,
}

impl MockRepository {
    /// Create a new empty mock repository
    pub fn new() -> Self {
        Self {
            storage: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    /// Get the current number of processes stored
    #[cfg(test)]
    pub fn len(&self) -> usize {
        self.storage.lock().unwrap().len()
    }

    /// Check if the repository is empty
    #[cfg(test)]
    pub fn is_empty(&self) -> bool {
        self.storage.lock().unwrap().is_empty()
    }
}

impl Default for MockRepository {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ProcessRepository for MockRepository {
    async fn save(&self, process: Process) -> Result<(), DomainError> {
        let mut storage = self.storage.lock().unwrap();
        storage.insert(process.id(), process);
        Ok(())
    }

    async fn find_by_id(&self, id: &ProcessId) -> Result<Option<Process>, DomainError> {
        let storage = self.storage.lock().unwrap();
        Ok(storage.get(id).cloned())
    }

    async fn find_by_name(&self, name: &str) -> Result<Option<Process>, DomainError> {
        let storage = self.storage.lock().unwrap();
        Ok(storage.values().find(|p| p.name() == name).cloned())
    }

    async fn find_all(&self) -> Result<Vec<Process>, DomainError> {
        let storage = self.storage.lock().unwrap();
        Ok(storage.values().cloned().collect())
    }

    async fn delete(&self, id: &ProcessId) -> Result<(), DomainError> {
        let mut storage = self.storage.lock().unwrap();
        storage.remove(id);
        Ok(())
    }

    async fn exists_by_name(&self, name: &str) -> Result<bool, DomainError> {
        let storage = self.storage.lock().unwrap();
        Ok(storage.values().any(|p| p.name() == name))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_save_and_find_by_id() {
        let repo = MockRepository::new();
        let process = Process::builder("test-process".to_string(), "/bin/echo".to_string())
            .build()
            .unwrap();
        let id = process.id();

        // Save process
        repo.save(process.clone()).await.unwrap();

        // Find by ID
        let found = repo.find_by_id(&id).await.unwrap();
        assert!(found.is_some());
        assert_eq!(found.unwrap().name(), "test-process");
    }

    #[tokio::test]
    async fn test_find_by_name() {
        let repo = MockRepository::new();
        let process = Process::builder("my-service".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        let id = process.id();

        repo.save(process).await.unwrap();

        // Find by name
        let found = repo.find_by_name("my-service").await.unwrap();
        assert!(found.is_some());
        assert_eq!(found.unwrap().id(), id);

        // Non-existent name
        let not_found = repo.find_by_name("other-service").await.unwrap();
        assert!(not_found.is_none());
    }

    #[tokio::test]
    async fn test_find_all_and_delete() {
        let repo = MockRepository::new();

        // Add multiple processes
        let process1 = Process::builder("process1".to_string(), "/bin/app1".to_string())
            .build()
            .unwrap();
        let process2 = Process::builder("process2".to_string(), "/bin/app2".to_string())
            .build()
            .unwrap();
        let id1 = process1.id();

        repo.save(process1).await.unwrap();
        repo.save(process2).await.unwrap();

        // Find all
        let all = repo.find_all().await.unwrap();
        assert_eq!(all.len(), 2);

        // Delete one
        repo.delete(&id1).await.unwrap();
        let all = repo.find_all().await.unwrap();
        assert_eq!(all.len(), 1);
        assert_eq!(all[0].name(), "process2");
    }

    #[tokio::test]
    async fn test_exists_by_name() {
        let repo = MockRepository::new();

        // Should not exist initially
        assert!(!repo.exists_by_name("test").await.unwrap());

        // Add process
        let process = Process::builder("test".to_string(), "/bin/test".to_string())
            .build()
            .unwrap();
        repo.save(process).await.unwrap();

        // Should exist now
        assert!(repo.exists_by_name("test").await.unwrap());
    }
}
