//! StopProcess use case
//! Handles stopping a running process

use crate::domain::ports::{ProcessExecutor, ProcessRepository};
use crate::domain::services::{cleanup_runtime_directories, execute_hooks};
#[cfg(test)]
use crate::domain::ProcessId;
use crate::domain::{
    DependencyResolutionService, DomainError, StopProcessCommand, StopProcessResponse,
};
use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::Arc;
use tracing::{debug, info, warn};

/// Parse signal name to signal number
fn parse_signal(signal_name: &str) -> i32 {
    match signal_name.to_uppercase().as_str() {
        "SIGHUP" | "HUP" => 1,
        "SIGINT" | "INT" => 2,
        "SIGQUIT" | "QUIT" => 3,
        "SIGKILL" | "KILL" => 9,
        "SIGTERM" | "TERM" => 15,
        "SIGSTOP" | "STOP" => 19,
        "SIGCONT" | "CONT" => 18,
        _ => {
            // Try to parse as number
            signal_name.parse::<i32>().unwrap_or(15) // Default to SIGTERM
        }
    }
}

/// Convert signal number back to name (for logging)
fn signal_to_name(signal: i32) -> &'static str {
    match signal {
        1 => "SIGHUP",
        2 => "SIGINT",
        3 => "SIGQUIT",
        9 => "SIGKILL",
        15 => "SIGTERM",
        19 => "SIGSTOP",
        18 => "SIGCONT",
        _ => "UNKNOWN",
    }
}

/// Use case for stopping a process
#[async_trait]
pub trait StopProcess: Send + Sync {
    async fn execute(
        &self,
        command: StopProcessCommand,
    ) -> Result<StopProcessResponse, DomainError>;
}

/// Implementation of StopProcess use case
pub struct StopProcessUseCase {
    repository: Arc<dyn ProcessRepository>,
    executor: Arc<dyn ProcessExecutor>,
}

impl StopProcessUseCase {
    pub fn new(repository: Arc<dyn ProcessRepository>, executor: Arc<dyn ProcessExecutor>) -> Self {
        Self {
            repository,
            executor,
        }
    }
}

#[async_trait]
impl StopProcess for StopProcessUseCase {
    async fn execute(
        &self,
        command: StopProcessCommand,
    ) -> Result<StopProcessResponse, DomainError> {
        // 1. Load the process from repository (by ID or name)
        let mut process = self
            .repository
            .find_by_id_or_name(command.process_id.as_ref(), command.process_name.as_deref())
            .await?;

        // 2. Check if process can be stopped
        if !process.can_stop() {
            return Err(DomainError::NotRunning);
        }

        // 3. Mark as stopping
        process.mark_stopping()?;
        self.repository.save(process.clone()).await?;

        // 4. Kill the system process (with timeout and kill mode)
        let pid = process.pid().ok_or(DomainError::NotRunning)?;
        let signal = command
            .signal
            .unwrap_or_else(|| parse_signal(process.kill_signal()));
        let kill_mode = process.kill_mode();

        debug!(
            process = %process.name(),
            pid = pid,
            signal = signal,
            kill_mode = %kill_mode,
            "Stopping process with configured kill mode"
        );

        // Use configured timeout or default to 90s (systemd-like behavior)
        let timeout_duration = if process.timeout_stop_sec() > 0 {
            std::time::Duration::from_secs(process.timeout_stop_sec())
        } else {
            std::time::Duration::from_secs(crate::domain::constants::DEFAULT_STOP_TIMEOUT_SEC)
        };

        match tokio::time::timeout(
            timeout_duration,
            self.executor.kill_with_mode(pid, signal, kill_mode),
        )
        .await
        {
            Ok(result) => {
                result?;
                // Record the signal that was sent
                let signal_name = signal_to_name(signal);
                process.set_signal(Some(format!(
                    "{} ({}) - stopped via {}",
                    signal_name, signal, kill_mode
                )));
            }
            Err(_) => {
                warn!(
                    process = %process.name(),
                    pid = pid,
                    timeout_secs = timeout_duration.as_secs(),
                    "Process stop timed out, sending SIGKILL to process group"
                );
                // Force kill entire process group with SIGKILL (9)
                self.executor
                    .kill_with_mode(pid, 9, crate::domain::KillMode::ProcessGroup)
                    .await?;
                // Record the timeout and forced kill
                process.set_signal(Some("SIGKILL (9) - force kill after timeout".to_string()));
            }
        }

        // 5. Mark as stopped
        process.mark_stopped()?;
        let process_id = process.id();
        let process_name = process.name().to_string();

        // 5a. Clean up PID file if configured
        if let Some(pidfile) = process.pidfile() {
            if let Err(e) = std::fs::remove_file(pidfile) {
                warn!(
                    pidfile = %pidfile,
                    error = %e,
                    "Failed to remove PID file (may not exist)"
                );
            } else {
                debug!(pidfile = %pidfile, "Removed PID file");
            }
        }

        // 5b. Clean up runtime directories if configured
        let runtime_dirs = process.runtime_directory().to_vec();
        if !runtime_dirs.is_empty() {
            cleanup_runtime_directories(&runtime_dirs);
        }

        // 5c. Execute post-stop hooks
        let exec_stop_post = process.exec_stop_post().to_vec();
        self.repository.save(process).await?;

        execute_hooks(&exec_stop_post, "ExecStopPost", &process_name).await?;

        // 6. Stop bound processes (processes that bind_to this one)
        let all_processes_vec = self.repository.find_all().await?;
        let all_processes: HashMap<String, _> = all_processes_vec
            .into_iter()
            .map(|p| (p.name().to_string(), p))
            .collect();

        let bound_process_names =
            DependencyResolutionService::get_bound_processes(&process_name, &all_processes);
        let mut stopped_bound_processes = Vec::new();

        for bound_name in &bound_process_names {
            info!(
                stopped_process = %process_name,
                bound_process = %bound_name,
                "Stopping bound process"
            );

            if let Ok(Some(mut bound_process)) = self.repository.find_by_name(bound_name).await {
                if bound_process.is_running() {
                    // Mark as stopping
                    if let Err(e) = bound_process.mark_stopping() {
                        warn!(
                            process = %bound_name,
                            error = %e,
                            "Failed to mark bound process as stopping"
                        );
                        continue;
                    }
                    let _ = self.repository.save(bound_process.clone()).await;

                    // Kill the process
                    if let Some(bound_pid) = bound_process.pid() {
                        if let Err(e) = self.executor.kill(bound_pid, signal).await {
                            warn!(
                                process = %bound_name,
                                pid = bound_pid,
                                error = %e,
                                "Failed to kill bound process"
                            );
                        }
                    }

                    // Mark as stopped
                    if let Err(e) = bound_process.mark_stopped() {
                        warn!(
                            process = %bound_name,
                            error = %e,
                            "Failed to mark bound process as stopped"
                        );
                        continue;
                    }

                    // Clean up PID file for bound process
                    if let Some(pidfile) = bound_process.pidfile() {
                        if let Err(e) = std::fs::remove_file(pidfile) {
                            warn!(
                                process = %bound_name,
                                pidfile = %pidfile,
                                error = %e,
                                "Failed to remove PID file for bound process"
                            );
                        } else {
                            debug!(process = %bound_name, pidfile = %pidfile, "Removed PID file");
                        }
                    }

                    let _ = self.repository.save(bound_process).await;

                    stopped_bound_processes.push(bound_name.to_string());
                    info!(process = %bound_name, "Bound process stopped successfully");
                }
            }
        }

        // 7. Return response
        Ok(StopProcessResponse {
            process_id,
            bound_processes_stopped: stopped_bound_processes,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::{
        MockRepository, ProcessExecutor as ExecutorTrait, SpawnConfig, SpawnResult,
    };
    use crate::domain::Process;
    use async_trait::async_trait;

    // Mock executor for testing
    struct MockExecutor;

    #[async_trait]
    impl ExecutorTrait for MockExecutor {
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
            Ok(false)
        }

        async fn wait_for_exit(&self, _pid: u32) -> Result<i32, DomainError> {
            Ok(0)
        }
    }

    #[tokio::test]
    async fn test_stop_process_success() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let use_case = StopProcessUseCase::new(repo.clone(), executor);

        // Create and start a process
        let mut process = Process::builder("test-process".to_string(), "/bin/sleep".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_running(1234).unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Stop the process by ID
        let command = StopProcessCommand::from_id(process_id);
        let result = use_case.execute(command).await.unwrap();

        assert_eq!(result.process_id, process_id);
        assert_eq!(result.bound_processes_stopped.len(), 0);

        // Verify process is stopped in repository
        let updated = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert!(!updated.is_running());
        assert_eq!(updated.pid(), None);
    }

    #[tokio::test]
    async fn test_stop_nonexistent_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let use_case = StopProcessUseCase::new(repo, executor);

        let command = StopProcessCommand::from_id(ProcessId::generate());
        let result = use_case.execute(command).await;

        assert!(matches!(result, Err(DomainError::ProcessNotFound(_))));
    }

    #[tokio::test]
    async fn test_stop_not_running_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let use_case = StopProcessUseCase::new(repo.clone(), executor);

        // Create process but don't start it
        let process = Process::builder("test".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Try to stop
        let command = StopProcessCommand::from_id(process_id);
        let result = use_case.execute(command).await;

        assert!(matches!(result, Err(DomainError::NotRunning)));
    }

    #[tokio::test]
    async fn test_stop_process_by_name() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor);
        let use_case = StopProcessUseCase::new(repo.clone(), executor);

        // Create and start a process
        let mut process = Process::builder("nginx".to_string(), "/usr/sbin/nginx".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_running(5678).unwrap();
        let process_id = process.id();
        repo.save(process).await.unwrap();

        // Stop the process by name
        let command = StopProcessCommand::from_name("nginx".to_string());
        let result = use_case.execute(command).await.unwrap();

        assert_eq!(result.process_id, process_id);
        assert_eq!(result.bound_processes_stopped.len(), 0);

        // Verify process is stopped in repository
        let updated = repo.find_by_name("nginx").await.unwrap().unwrap();
        assert!(!updated.is_running());
    }
}
