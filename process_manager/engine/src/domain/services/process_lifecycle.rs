//! Process Lifecycle Service
//!
//! Domain service that coordinates process spawning and registration with supervisor.
//! This service encapsulates the common logic used by both StartProcess and LoadConfig use cases.

use crate::domain::ports::{ProcessExecutor, ProcessRepository, SpawnConfig};
use crate::domain::services::{execute_hooks, ProcessSupervisionService};
use crate::domain::{DomainError, ProcessId};
use std::sync::Arc;
use tracing::{debug, info, warn};

/// Process Lifecycle Service
/// Handles spawning processes and registering them with the supervisor
pub struct ProcessLifecycleService {
    repository: Arc<dyn ProcessRepository>,
    executor: Arc<dyn ProcessExecutor>,
    supervisor: Option<Arc<ProcessSupervisionService>>,
}

impl ProcessLifecycleService {
    /// Create a new process lifecycle service
    pub fn new(repository: Arc<dyn ProcessRepository>, executor: Arc<dyn ProcessExecutor>) -> Self {
        Self {
            repository,
            executor,
            supervisor: None,
        }
    }

    /// Create with supervisor for coordinated lifecycle management
    pub fn with_supervisor(
        repository: Arc<dyn ProcessRepository>,
        executor: Arc<dyn ProcessExecutor>,
        supervisor: Arc<ProcessSupervisionService>,
    ) -> Self {
        Self {
            repository,
            executor,
            supervisor: Some(supervisor),
        }
    }

    /// Spawn a process and register it with the supervisor
    ///
    /// This is the common logic for starting a process:
    /// 1. Execute pre-start hooks (ExecStartPre)
    /// 2. Spawn the system process (with optional socket FDs and timeout)
    /// 3. Update domain entity state
    /// 4. Register with supervisor for monitoring
    /// 5. Execute post-start hooks (ExecStartPost)
    ///
    /// # Arguments
    /// * `process_id` - ID of the process to spawn
    /// * `listen_fds` - Optional socket FDs for socket activation
    /// * `timeout_sec` - Optional timeout in seconds (0 = no timeout)
    ///
    /// After successful completion, the process entity will be updated with the PID.
    /// Query the repository to get the updated process state.
    pub async fn spawn_and_register(
        &self,
        process_id: &ProcessId,
        listen_fds: Vec<i32>,
        timeout_sec: u64,
    ) -> Result<(), DomainError> {
        // 1. Load process from repository
        let mut process = self
            .repository
            .find_by_id(process_id)
            .await?
            .ok_or_else(|| DomainError::ProcessNotFound(process_id.to_string()))?;

        debug!(
            process_id = %process_id,
            process_name = %process.name(),
            "Spawning process"
        );

        // 2. Execute pre-start hooks (ExecStartPre)
        if let Err(e) =
            execute_hooks(process.exec_start_pre(), "ExecStartPre", process.name()).await
        {
            return Err(DomainError::InvalidCommand(format!(
                "Failed to execute pre-start hooks for '{}': {}",
                process.name(),
                e
            )));
        }

        // 3. Mark as starting
        process.mark_starting()?;
        self.repository.save(process.clone()).await?;

        // 3. Build spawn configuration
        let mut spawn_config = SpawnConfig::from_process(&process);

        // Add socket FDs for socket activation if provided
        if !listen_fds.is_empty() {
            debug!(
                process = %process.name(),
                num_fds = listen_fds.len(),
                "Starting process with socket activation FDs"
            );
            spawn_config.listen_fds = listen_fds;
        }

        // 4. Spawn the system process (with timeout if configured)
        let spawn_result = if timeout_sec > 0 {
            let timeout_duration = std::time::Duration::from_secs(timeout_sec);
            match tokio::time::timeout(timeout_duration, self.executor.spawn(spawn_config)).await {
                Ok(result) => result?,
                Err(_) => {
                    tracing::warn!(
                        process = %process.name(),
                        timeout = timeout_sec,
                        "Process start timed out"
                    );
                    return Err(DomainError::InvalidCommand(format!(
                        "Process start timed out after {} seconds",
                        timeout_sec
                    )));
                }
            }
        } else {
            // No timeout configured
            self.executor.spawn(spawn_config).await?
        };

        info!(
            process_id = %process_id,
            process_name = %process.name(),
            pid = spawn_result.pid,
            "Process spawned successfully"
        );

        // 5. Update process state to running
        process.increment_run_count(); // Track how many times process has been started
        process.mark_running(spawn_result.pid)?;
        // Note: Don't reset failures here - failures should only reset after
        // successful runtime (runtime_success_sec) or clean exit (exit code 0)
        self.repository.save(process.clone()).await?;

        // 6. Register with supervisor for coordinated monitoring (exit + health)
        if let Some(ref supervisor) = self.supervisor {
            supervisor.register_started_process(&process, spawn_result.exit_handle);
        }

        // 7. Execute post-start hooks (ExecStartPost)
        // Note: We don't fail the start if post-start hooks fail, just log a warning
        if let Err(e) =
            execute_hooks(process.exec_start_post(), "ExecStartPost", process.name()).await
        {
            warn!(
                process = %process.name(),
                error = %e,
                "Failed to execute post-start hooks"
            );
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::Process;
    use crate::infrastructure::{InMemoryProcessRepository, TokioProcessExecutor};

    #[tokio::test]
    async fn test_spawn_and_register_without_supervisor() {
        let repository = Arc::new(InMemoryProcessRepository::new());
        let executor = Arc::new(TokioProcessExecutor::new());
        let service = ProcessLifecycleService::new(repository.clone(), executor);

        // Create a test process
        let process = Process::builder("test".to_string(), "/bin/sleep".to_string())
            .args(vec!["1".to_string()])
            .build()
            .unwrap();
        let process_id = process.id();
        repository.save(process).await.unwrap();

        // Spawn and register (no socket FDs, no timeout)
        service
            .spawn_and_register(&process_id, vec![], 0)
            .await
            .unwrap();

        // Verify process is running with PID
        let process = repository.find_by_id(&process_id).await.unwrap().unwrap();
        assert!(process.is_running());
        assert!(process.pid().is_some());
    }
}
