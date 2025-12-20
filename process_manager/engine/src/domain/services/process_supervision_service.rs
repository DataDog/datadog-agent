//! Process Supervisor Service
//! Event-driven process monitoring and automatic restart handling
//! Uses ProcessWatchingService for immediate exit notifications (no polling)

use crate::domain::ports::{ProcessExecutor, ProcessRepository};
use crate::domain::services::{HealthMonitoringService, ProcessExitEvent};
use crate::domain::{DomainError, ProcessState, RestartPolicy};
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio::time::sleep;
use tokio_util::sync::CancellationToken;
use tracing::{debug, error, info, warn};

/// Process Supervisor Service
/// Single coordinator for all process lifecycle management:
/// - Exit monitoring (via ProcessWatchingService)
/// - Automatic restarts based on policy
/// - Health monitoring coordination
pub struct ProcessSupervisionService {
    repository: Arc<dyn ProcessRepository>,
    executor: Arc<dyn ProcessExecutor>,
    watcher: Option<Arc<crate::domain::services::ProcessWatchingService>>,
    health_monitor: Option<Arc<HealthMonitoringService>>,
}

impl ProcessSupervisionService {
    /// Create a new process supervisor
    pub fn new(repository: Arc<dyn ProcessRepository>, executor: Arc<dyn ProcessExecutor>) -> Self {
        Self {
            repository,
            executor,
            watcher: None,
            health_monitor: None,
        }
    }

    /// Create a new process supervisor with full coordination
    pub fn with_watcher_and_health_monitor(
        repository: Arc<dyn ProcessRepository>,
        executor: Arc<dyn ProcessExecutor>,
        watcher: Arc<crate::domain::services::ProcessWatchingService>,
        health_monitor: Arc<HealthMonitoringService>,
    ) -> Self {
        Self {
            repository,
            executor,
            watcher: Some(watcher),
            health_monitor: Some(health_monitor),
        }
    }

    /// Register a newly started process for monitoring
    /// Call this after successfully spawning a process
    /// Supervisor will handle both exit monitoring and health monitoring
    pub fn register_started_process(
        &self,
        process: &crate::domain::Process,
        exit_handle: Option<crate::domain::ports::ProcessExitHandle>,
    ) {
        // Start exit monitoring if watcher is available
        if let (Some(ref watcher), Some(handle)) = (&self.watcher, exit_handle) {
            tracing::debug!(
                process = %process.name(),
                pid = ?process.pid(),
                "Supervisor: Starting exit monitoring"
            );
            watcher.watch_process(process, handle);
        }

        // Start health monitoring if configured
        if let Some(ref health_monitor) = self.health_monitor {
            if process.health_check().is_some() {
                tracing::info!(
                    process_id = %process.id(),
                    process_name = %process.name(),
                    "Supervisor: Starting health monitoring"
                );
                health_monitor.start_monitoring(process.id());
            }
        }

        // Start runtime success timer to reset failures after stable run
        self.start_runtime_success_timer(process);
    }

    /// Spawn a background task that resets failure counter after runtime_success_sec
    /// This ensures exponential backoff resets once a process has been running stably
    fn start_runtime_success_timer(&self, process: &crate::domain::Process) {
        let runtime_success_sec = process.runtime_success_sec();
        if runtime_success_sec == 0 {
            return; // Disabled
        }

        let process_id = process.id();
        let process_name = process.name().to_string();
        let repository = self.repository.clone();

        tokio::spawn(async move {
            // Wait for the runtime success threshold
            tokio::time::sleep(std::time::Duration::from_secs(runtime_success_sec)).await;

            // Check if process is still running and reset failures
            match repository.find_by_id(&process_id).await {
                Ok(Some(mut proc)) => {
                    if proc.is_running() && proc.consecutive_failures() > 0 {
                        info!(
                            process = %process_name,
                            runtime_success_sec = runtime_success_sec,
                            previous_failures = proc.consecutive_failures(),
                            "Process running stably - resetting failure counter"
                        );
                        proc.reset_failures();
                        if let Err(e) = repository.save(proc).await {
                            error!(
                                process = %process_name,
                                error = %e,
                                "Failed to save process after resetting failures"
                            );
                        }
                    }
                }
                Ok(None) => {
                    debug!(
                        process = %process_name,
                        "Process no longer exists, skipping runtime success check"
                    );
                }
                Err(e) => {
                    error!(
                        process = %process_name,
                        error = %e,
                        "Failed to load process for runtime success check"
                    );
                }
            }
        });
    }

    /// Start the supervision loop
    /// Receives exit events and handles restarts immediately (no polling!)
    /// Runs until the cancellation token is triggered
    pub async fn run(
        &self,
        mut exit_rx: mpsc::UnboundedReceiver<ProcessExitEvent>,
        cancellation_token: CancellationToken,
    ) {
        info!("Process supervisor started (event-driven)");

        loop {
            tokio::select! {
                _ = cancellation_token.cancelled() => {
                    info!("Process supervisor received shutdown signal");
                    break;
                }
                Some(event) = exit_rx.recv() => {
                    info!(
                        process_id = %event.process_id,
                        pid = event.pid,
                        exit_code = event.exit_code,
                        "Received process exit event"
                    );

                    if let Err(e) = self.handle_exit_event(event).await {
                        error!(error = %e, "Error handling process exit");
                    }
                }
            }
        }

        info!("Process supervisor stopped");
    }

    /// Handle a process exit event
    async fn handle_exit_event(&self, event: ProcessExitEvent) -> Result<(), DomainError> {
        // Find the process
        let mut process = match self.repository.find_by_id(&event.process_id).await? {
            Some(p) => p,
            None => {
                warn!(
                    process_id = %event.process_id,
                    "Process not found for exit event (may have been deleted)"
                );
                return Ok(());
            }
        };

        // Clean up PID file if it exists
        if let Some(pidfile) = process.pidfile() {
            if let Err(e) = std::fs::remove_file(pidfile) {
                // Only log if the error is not "file not found"
                if e.kind() != std::io::ErrorKind::NotFound {
                    warn!(
                        process = %process.name(),
                        pidfile = pidfile,
                        error = %e,
                        "Failed to remove PID file"
                    );
                }
            } else {
                debug!(
                    process = %process.name(),
                    pidfile = pidfile,
                    "Removed PID file"
                );
            }
        }

        // Mark as exited with the exit code
        // If process was in Stopping state, it goes to Stopped (explicit stop)
        // Otherwise, it goes to Exited or Failed based on exit code (spontaneous exit)
        process.mark_exited(event.exit_code)?;

        // Track failures for restart logic (only for spontaneous exits)
        // Check runtime success threshold: if process ran long enough, reset failures
        let runtime_was_successful = if let Some(started_at) = process.started_at() {
            if let Ok(runtime) = started_at.elapsed() {
                runtime.as_secs() >= process.runtime_success_sec()
            } else {
                false
            }
        } else {
            false
        };

        if process.state() == ProcessState::Failed {
            if runtime_was_successful {
                info!(
                    process = %process.name(),
                    exit_code = event.exit_code,
                    runtime_success_sec = process.runtime_success_sec(),
                    "Process failed but ran long enough - resetting failure counter"
                );
                process.reset_failures();
            } else {
                warn!(
                    process = %process.name(),
                    exit_code = event.exit_code,
                    "Process exited with failure"
                );
                process.increment_failures();
            }
        } else if process.state() == ProcessState::Exited {
            info!(
                process = %process.name(),
                exit_code = event.exit_code,
                "Process exited successfully"
            );
            process.reset_failures(); // Successful exit resets failures
        } else if process.state() == ProcessState::Stopped {
            info!(
                process = %process.name(),
                exit_code = event.exit_code,
                "Process stopped explicitly (not evaluating exit code)"
            );
            // Don't reset or increment failures for explicit stops
        }

        self.repository.save(process.clone()).await?;

        // Handle BindsTo cascade: stop all processes bound to this one
        self.handle_binds_to_cascade(&process).await?;

        // Attempt restart based on policy (only for spontaneous exits, not explicit stops)
        if process.state() != ProcessState::Stopped {
            self.attempt_restart(process).await?;
        } else {
            debug!(
                process = %process.name(),
                "Process was explicitly stopped, not attempting restart"
            );
        }

        Ok(())
    }

    /// Attempt to restart a process based on its restart policy
    async fn attempt_restart(
        &self,
        mut process: crate::domain::Process,
    ) -> Result<(), DomainError> {
        let restart_policy = process.restart_policy();
        let should_restart = match restart_policy {
            RestartPolicy::Never => {
                debug!(
                    process = %process.name(),
                    "Restart policy is 'never', not restarting"
                );
                false
            }
            RestartPolicy::Always => {
                debug!(
                    process = %process.name(),
                    "Restart policy is 'always', will restart"
                );
                true
            }
            RestartPolicy::OnFailure => {
                let is_failed = process.state() == ProcessState::Failed;
                debug!(
                    process = %process.name(),
                    is_failed = is_failed,
                    "Restart policy is 'on-failure'"
                );
                is_failed
            }
            RestartPolicy::OnSuccess => {
                let is_success = process.state() == ProcessState::Exited;
                debug!(
                    process = %process.name(),
                    is_success = is_success,
                    "Restart policy is 'on-success'"
                );
                is_success
            }
        };

        if !should_restart {
            return Ok(());
        }

        // Check start limits
        if process.is_start_limit_exceeded() {
            warn!(
                process = %process.name(),
                consecutive_failures = process.consecutive_failures(),
                burst = process.start_limit_burst(),
                interval = process.start_limit_interval_sec(),
                "Start limit exceeded, not restarting"
            );
            return Ok(());
        }

        // Calculate restart delay with exponential backoff
        let restart_delay = process.calculate_restart_delay();

        info!(
            process = %process.name(),
            delay_secs = restart_delay,
            consecutive_failures = process.consecutive_failures(),
            "Scheduling process restart"
        );

        // Wait for the restart delay
        if restart_delay > 0 {
            sleep(Duration::from_secs(restart_delay)).await;
        }

        // Mark as restarting
        process.mark_restarting()?;
        self.repository.save(process.clone()).await?;

        // Attempt to restart the process
        info!(
            process = %process.name(),
            run_count = process.run_count(),
            "Attempting to restart process"
        );

        // Use the restart logic directly (similar to RestartProcess use case)
        // Check if we should actually start (state could have changed)
        let current_process = self
            .repository
            .find_by_id(&process.id())
            .await?
            .ok_or_else(|| DomainError::ProcessNotFound(process.name().to_string()))?;

        if current_process.state() != ProcessState::Restarting {
            debug!(
                process = %process.name(),
                state = ?current_process.state(),
                "Process state changed, aborting restart"
            );
            return Ok(());
        }

        // Record start time and increment run count
        let mut process = current_process;
        process.record_start_time();
        process.increment_run_count();

        // Start the process
        process.mark_starting()?;
        self.repository.save(process.clone()).await?;

        let spawn_config = crate::domain::ports::SpawnConfig::from_process(&process);

        match self.executor.spawn(spawn_config).await {
            Ok(spawn_result) => {
                info!(
                    process = %process.name(),
                    pid = spawn_result.pid,
                    "Process restarted successfully"
                );
                process.mark_running(spawn_result.pid)?;
                // Note: Don't reset failures here - failures should only reset after
                // successful runtime (runtime_success_sec) or clean exit (exit code 0)
                self.repository.save(process.clone()).await?;

                // Register for monitoring (exit + health)
                self.register_started_process(&process, spawn_result.exit_handle);
            }
            Err(e) => {
                error!(
                    process = %process.name(),
                    error = %e,
                    "Failed to restart process"
                );
                // Mark as exited with non-zero exit code to indicate failure
                process.mark_exited(1)?; // This will transition to Failed state
                process.increment_failures();
                self.repository.save(process).await?;
            }
        }

        Ok(())
    }

    /// Handle BindsTo cascade: stop all processes that are bound to the given process
    /// When a process stops/crashes, processes that have `binds_to` to it should also be stopped
    async fn handle_binds_to_cascade(
        &self,
        stopped_process: &crate::domain::Process,
    ) -> Result<(), DomainError> {
        let stopped_process_name = stopped_process.name();

        // Find all processes that bind to the stopped process
        let all_processes = self.repository.find_all().await?;
        let bound_processes: Vec<_> = all_processes
            .into_iter()
            .filter(|p| p.binds_to().contains(&stopped_process_name.to_string()))
            .collect();

        if bound_processes.is_empty() {
            return Ok(());
        }

        info!(
            stopped_process = %stopped_process_name,
            bound_count = bound_processes.len(),
            "Cascading stop to bound processes due to BindsTo dependency"
        );

        // Stop each bound process
        for bound_process in bound_processes {
            // Only stop if it's currently running
            if bound_process.state() != ProcessState::Running {
                continue;
            }

            info!(
                stopped_process = %stopped_process_name,
                bound_process = %bound_process.name(),
                bound_id = %bound_process.id(),
                "Stopping process due to BindsTo dependency"
            );

            // Correct state transition: Running -> Stopping -> Stopped
            let mut process = bound_process;

            // First mark as stopping
            if let Err(e) = process.mark_stopping() {
                error!(
                    process = %process.name(),
                    error = %e,
                    "Failed to mark bound process as stopping"
                );
                continue;
            }

            // Try to kill the process if it has a PID
            if let Some(pid) = process.pid() {
                // Use SIGTERM (15) for cascade stops
                let signal = 15;
                if let Err(e) = self
                    .executor
                    .kill_with_mode(pid, signal, process.kill_mode())
                    .await
                {
                    warn!(
                        process = %process.name(),
                        pid = pid,
                        error = %e,
                        "Failed to kill bound process (may have already exited)"
                    );
                }
            }

            // Then mark as stopped
            if let Err(e) = process.mark_stopped() {
                error!(
                    process = %process.name(),
                    error = %e,
                    "Failed to mark bound process as stopped"
                );
                continue;
            }

            // Save the stopped state
            if let Err(e) = self.repository.save(process).await {
                error!(
                    error = %e,
                    "Failed to save stopped bound process state"
                );
            }
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::{MockRepository, SpawnResult};
    use crate::domain::Process;
    use async_trait::async_trait;

    struct MockExecutor {
        running: std::sync::Arc<tokio::sync::Mutex<std::collections::HashSet<u32>>>,
        exit_codes: std::sync::Arc<tokio::sync::Mutex<std::collections::HashMap<u32, i32>>>,
    }

    impl MockExecutor {
        fn new() -> Self {
            Self {
                running: Arc::new(tokio::sync::Mutex::new(std::collections::HashSet::new())),
                exit_codes: Arc::new(tokio::sync::Mutex::new(std::collections::HashMap::new())),
            }
        }

        #[allow(dead_code)]
        async fn mark_exited(&self, pid: u32) {
            self.running.lock().await.remove(&pid);
        }

        #[allow(dead_code)]
        async fn mark_exited_with_code(&self, pid: u32, exit_code: i32) {
            self.running.lock().await.remove(&pid);
            self.exit_codes.lock().await.insert(pid, exit_code);
        }
    }

    #[async_trait]
    impl ProcessExecutor for MockExecutor {
        async fn spawn(
            &self,
            _config: crate::domain::ports::SpawnConfig,
        ) -> Result<SpawnResult, DomainError> {
            let pid = 12345;
            self.running.lock().await.insert(pid);
            Ok(SpawnResult {
                pid,
                exit_handle: None,
            })
        }

        async fn kill(&self, pid: u32, _signal: i32) -> Result<(), DomainError> {
            self.running.lock().await.remove(&pid);
            Ok(())
        }

        async fn kill_with_mode(
            &self,
            pid: u32,
            _signal: i32,
            _mode: crate::domain::KillMode,
        ) -> Result<(), DomainError> {
            self.running.lock().await.remove(&pid);
            Ok(())
        }

        async fn is_running(&self, pid: u32) -> Result<bool, DomainError> {
            Ok(self.running.lock().await.contains(&pid))
        }

        async fn wait_for_exit(&self, pid: u32) -> Result<i32, DomainError> {
            // Return the exit code if set, otherwise return 0 (success)
            Ok(self.exit_codes.lock().await.get(&pid).copied().unwrap_or(0))
        }
    }

    #[tokio::test]
    async fn test_supervisor_restarts_failed_process() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let supervisor = ProcessSupervisionService::new(repo.clone(), executor.clone());

        // Create a process with restart policy "always"
        let mut process = Process::builder("test-process".to_string(), "/bin/test".to_string())
            .build()
            .unwrap();
        process.set_restart_policy(RestartPolicy::Always);
        process.set_restart_sec(0); // No delay for testing
                                    // Proper state transitions: Created → Starting → Running
        process.mark_starting().unwrap();
        process.mark_running(12345).unwrap();
        process.increment_run_count(); // Simulate that it was started once
        let process_id = process.id();
        repo.save(process.clone()).await.unwrap();

        // Simulate exit event
        let event = ProcessExitEvent {
            process_id,
            pid: 12345,
            exit_code: 1,
        };

        // Handle the exit event
        supervisor.handle_exit_event(event).await.unwrap();

        // Process should be restarted
        let updated = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert_eq!(updated.state(), ProcessState::Running);
        assert_eq!(updated.run_count(), 2); // Initial (1) + restart (1) = 2
    }

    #[tokio::test]
    async fn test_supervisor_respects_never_restart_policy() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let supervisor = ProcessSupervisionService::new(repo.clone(), executor.clone());

        // Create a process with restart policy "never"
        let mut process = Process::builder("test-process".to_string(), "/bin/test".to_string())
            .build()
            .unwrap();
        process.set_restart_policy(RestartPolicy::Never);
        // Proper state transitions: Created → Starting → Running
        process.mark_starting().unwrap();
        process.mark_running(12345).unwrap();
        process.increment_run_count(); // Simulate that it was started once
        let process_id = process.id();
        repo.save(process.clone()).await.unwrap();

        // Simulate exit event
        let event = ProcessExitEvent {
            process_id,
            pid: 12345,
            exit_code: 1,
        };

        // Handle the exit event
        supervisor.handle_exit_event(event).await.unwrap();

        // Process should NOT be restarted
        let updated = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert_eq!(updated.state(), ProcessState::Failed);
        assert_eq!(updated.run_count(), 1); // Only initial (1), no restart
    }

    #[tokio::test]
    async fn test_supervisor_respects_start_limits() {
        let repo = Arc::new(MockRepository::new());
        let executor = Arc::new(MockExecutor::new());
        let supervisor = ProcessSupervisionService::new(repo.clone(), executor.clone());

        // Create a process with restart policy "always" but low start limit
        let mut process = Process::builder("test-process".to_string(), "/bin/test".to_string())
            .restart_policy(RestartPolicy::Always)
            .restart_delay_sec(0) // No delay
            .start_limit_burst(1) // Only allow 1 start in interval
            .start_limit_interval_sec(60)
            .build()
            .unwrap();

        // Simulate multiple starts to exceed limit
        process.record_start_time();
        process.record_start_time();

        // Proper state transitions: Created → Starting → Running
        process.mark_starting().unwrap();
        process.mark_running(12345).unwrap();
        process.increment_run_count(); // Simulate that it was started once
        let process_id = process.id();
        repo.save(process.clone()).await.unwrap();

        // Simulate exit event
        let event = ProcessExitEvent {
            process_id,
            pid: 12345,
            exit_code: 1,
        };

        // Handle the exit event
        supervisor.handle_exit_event(event).await.unwrap();

        // Process should NOT be restarted due to start limit
        let updated = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert_eq!(updated.state(), ProcessState::Failed);
    }
}
