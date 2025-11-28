//! Conflict Service
//! Handles process conflict resolution (systemd Conflicts= behavior)

use crate::domain::ports::{ProcessExecutor, ProcessRepository};
use crate::domain::{DomainError, Process, ProcessState};
use std::collections::HashMap;
use std::sync::Arc;
use tracing::{info, warn};

/// Service for handling process conflicts
///
/// Implements systemd's bidirectional conflicts behavior:
/// - If process A declares conflicts with B, starting A stops B
/// - If process B declares conflicts with A, starting A stops B (bidirectional)
pub struct ConflictResolutionService {
    repository: Arc<dyn ProcessRepository>,
    executor: Arc<dyn ProcessExecutor>,
}

impl ConflictResolutionService {
    pub fn new(repository: Arc<dyn ProcessRepository>, executor: Arc<dyn ProcessExecutor>) -> Self {
        Self {
            repository,
            executor,
        }
    }

    /// Stop a process gracefully with timeout, escalating to SIGKILL if needed
    async fn graceful_stop(&self, process: &mut Process) -> Result<(), DomainError> {
        let pid = match process.pid() {
            Some(p) => p,
            None => return Ok(()), // No PID, nothing to kill
        };

        // Get timeout from process config (or default to 90s)
        let timeout_duration = if process.timeout_stop_sec() > 0 {
            std::time::Duration::from_secs(process.timeout_stop_sec())
        } else {
            std::time::Duration::from_secs(crate::domain::constants::DEFAULT_STOP_TIMEOUT_SEC)
        };

        // Mark as stopping
        process.mark_stopping()?;
        self.repository.save(process.clone()).await?;

        // Send SIGTERM
        let signal = 15; // SIGTERM
        if let Err(e) = self
            .executor
            .kill_with_mode(pid, signal, process.kill_mode())
            .await
        {
            warn!(
                process = %process.name(),
                pid = pid,
                error = %e,
                "Failed to send SIGTERM to conflicting process (may have already exited)"
            );
        }

        // Wait for graceful shutdown with timeout
        match tokio::time::timeout(timeout_duration, self.executor.wait_for_exit(pid)).await {
            Ok(Ok(_exit_code)) => {
                info!(
                    process = %process.name(),
                    pid = pid,
                    "Conflicting process stopped gracefully"
                );
            }
            Ok(Err(e)) => {
                warn!(
                    process = %process.name(),
                    pid = pid,
                    error = %e,
                    "Error waiting for conflicting process to exit"
                );
            }
            Err(_) => {
                warn!(
                    process = %process.name(),
                    pid = pid,
                    timeout_secs = timeout_duration.as_secs(),
                    "Conflicting process stop timed out, sending SIGKILL"
                );
                // Force kill with SIGKILL
                if let Err(e) = self
                    .executor
                    .kill_with_mode(pid, 9, crate::domain::KillMode::ProcessGroup)
                    .await
                {
                    warn!(
                        process = %process.name(),
                        pid = pid,
                        error = %e,
                        "Failed to send SIGKILL to conflicting process"
                    );
                }
            }
        }

        // Mark as stopped
        process.mark_stopped()?;
        self.repository.save(process.clone()).await?;

        Ok(())
    }

    /// Stop all processes that conflict with the given process
    ///
    /// This handles both forward and reverse conflicts:
    /// 1. Forward: processes that `process` declares conflicts with
    /// 2. Reverse: processes that declare conflicts with `process`
    ///
    /// # Arguments
    ///
    /// * `process` - The process being started
    /// * `all_processes` - Map of all processes by name for conflict lookup
    pub async fn stop_conflicting_processes(
        &self,
        process: &Process,
        all_processes: &HashMap<String, Process>,
    ) -> Result<(), DomainError> {
        let mut processes_to_stop = Vec::new();

        // Forward conflicts: processes this process declares conflicts with
        for conflict in process.conflicts() {
            if let Some(conflicting_process) = all_processes.get(conflict) {
                if conflicting_process.state() == ProcessState::Running {
                    processes_to_stop.push(conflicting_process.clone());
                }
            }
        }

        // Reverse conflicts: processes that declare conflicts with this process (bidirectional)
        let process_name = process.name().to_string();
        for (name, other_process) in all_processes {
            if other_process.state() == ProcessState::Running
                && other_process.conflicts().contains(&process_name)
                && !processes_to_stop.iter().any(|p| p.name() == name)
            {
                processes_to_stop.push(other_process.clone());
            }
        }

        // Stop all conflicting processes
        for conflicting in processes_to_stop {
            self.stop_process(&conflicting, process.name()).await?;
        }

        Ok(())
    }

    /// Stop a single conflicting process with graceful shutdown
    async fn stop_process(
        &self,
        conflicting: &Process,
        requesting_process_name: &str,
    ) -> Result<(), DomainError> {
        let conflicting_name = conflicting.name().to_string();

        info!(
            process = %requesting_process_name,
            conflicting_process = %conflicting_name,
            timeout_stop_sec = conflicting.timeout_stop_sec(),
            "Stopping conflicting process before start"
        );

        let mut conflicting = conflicting.clone();
        self.graceful_stop(&mut conflicting).await?;

        info!(
            process = %requesting_process_name,
            conflicting_process = %conflicting_name,
            "Conflicting process stopped"
        );

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::{MockRepository, SpawnConfig, SpawnResult};
    use async_trait::async_trait;

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
            Ok(true)
        }

        async fn wait_for_exit(&self, _pid: u32) -> Result<i32, DomainError> {
            Ok(0)
        }
    }

    #[tokio::test]
    async fn test_stop_conflicting_processes_forward() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let service = ConflictResolutionService::new(repo.clone(), executor);

        // Create two processes: A conflicts with B
        let process_a = Process::builder("a".to_string(), "/bin/a".to_string())
            .conflicts(vec!["b".to_string()])
            .build()
            .unwrap();

        let mut process_b = Process::builder("b".to_string(), "/bin/b".to_string())
            .build()
            .unwrap();
        process_b.mark_starting().unwrap();
        process_b.mark_running(1234).unwrap();
        repo.save(process_b.clone()).await.unwrap();

        let mut all_processes = HashMap::new();
        all_processes.insert("b".to_string(), process_b);

        // Stop conflicts
        service
            .stop_conflicting_processes(&process_a, &all_processes)
            .await
            .unwrap();

        // Verify B was stopped
        let stopped_b = repo.find_by_name("b").await.unwrap().unwrap();
        assert_eq!(stopped_b.state(), ProcessState::Stopped);
    }

    #[tokio::test]
    async fn test_stop_conflicting_processes_bidirectional() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let service = ConflictResolutionService::new(repo.clone(), executor);

        // Create two processes: B conflicts with A (reverse)
        let process_a = Process::builder("a".to_string(), "/bin/a".to_string())
            .build()
            .unwrap();

        let mut process_b = Process::builder("b".to_string(), "/bin/b".to_string())
            .conflicts(vec!["a".to_string()])
            .build()
            .unwrap();
        process_b.mark_starting().unwrap();
        process_b.mark_running(1234).unwrap();
        repo.save(process_b.clone()).await.unwrap();

        let mut all_processes = HashMap::new();
        all_processes.insert("b".to_string(), process_b);

        // Stop conflicts (should stop B even though A doesn't declare conflict)
        service
            .stop_conflicting_processes(&process_a, &all_processes)
            .await
            .unwrap();

        // Verify B was stopped (bidirectional behavior)
        let stopped_b = repo.find_by_name("b").await.unwrap().unwrap();
        assert_eq!(stopped_b.state(), ProcessState::Stopped);
    }
}
