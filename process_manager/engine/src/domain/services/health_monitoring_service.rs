//! Health monitoring service
//! Continuously monitors process health and stops unhealthy processes
//! ProcessSupervisionService handles restart logic uniformly for all stopped processes

use crate::domain::ports::{HealthCheckExecutor, ProcessExecutor, ProcessRepository};
use crate::domain::{HealthStatus, ProcessId};
use std::sync::Arc;
use std::time::Duration;
use tokio::time::sleep;
use tracing::{debug, error, info, warn};

/// Health monitor service
/// Detects unhealthy processes and stops them
/// ProcessSupervisionService will restart them according to restart policy
pub struct HealthMonitoringService {
    repository: Arc<dyn ProcessRepository>,
    health_executor: Arc<dyn HealthCheckExecutor>,
    process_executor: Arc<dyn ProcessExecutor>,
}

impl HealthMonitoringService {
    pub fn new(
        repository: Arc<dyn ProcessRepository>,
        health_executor: Arc<dyn HealthCheckExecutor>,
        process_executor: Arc<dyn ProcessExecutor>,
    ) -> Self {
        Self {
            repository,
            health_executor,
            process_executor,
        }
    }

    /// Start monitoring a specific process
    /// This spawns a background task that continuously monitors the process
    pub fn start_monitoring(&self, process_id: ProcessId) {
        let repository = self.repository.clone();
        let health_executor = self.health_executor.clone();
        let process_executor = self.process_executor.clone();

        tokio::spawn(async move {
            Self::monitor_process_loop(process_id, repository, health_executor, process_executor)
                .await;
        });
    }

    /// Main monitoring loop for a single process
    async fn monitor_process_loop(
        process_id: ProcessId,
        repository: Arc<dyn ProcessRepository>,
        health_executor: Arc<dyn HealthCheckExecutor>,
        process_executor: Arc<dyn ProcessExecutor>,
    ) {
        loop {
            // Load process
            let process = match repository.find_by_id(&process_id).await {
                Ok(Some(p)) => p,
                Ok(None) => {
                    debug!(process_id = %process_id, "Process no longer exists, stopping health monitor");
                    break;
                }
                Err(e) => {
                    error!(process_id = %process_id, error = %e, "Failed to load process for health check");
                    sleep(Duration::from_secs(30)).await;
                    continue;
                }
            };

            // Check if health check is configured
            let health_check = match process.health_check() {
                Some(hc) => hc.clone(),
                None => {
                    debug!(process_id = %process_id, "No health check configured, stopping monitor");
                    break;
                }
            };

            // Wait for interval
            sleep(Duration::from_secs(health_check.interval)).await;

            // Skip if process is not running
            if !process.is_running() {
                debug!(
                    process_id = %process_id,
                    name = %process.name(),
                    "Process not running, skipping health check"
                );
                continue;
            }

            // Check if we're in start period (grace period)
            if let Some(started_at) = process.started_at() {
                if let Ok(elapsed) = started_at.elapsed() {
                    if elapsed.as_secs() < health_check.start_period {
                        debug!(
                            process_id = %process_id,
                            name = %process.name(),
                            elapsed = elapsed.as_secs(),
                            start_period = health_check.start_period,
                            "In start period, skipping health check"
                        );

                        // Set health status to Starting during start_period
                        if process.health_status() != HealthStatus::Starting {
                            let mut updated_process = process.clone();
                            updated_process.update_health_status(HealthStatus::Starting);
                            if let Err(e) = repository.save(updated_process).await {
                                error!(
                                    process_id = %process_id,
                                    error = %e,
                                    "Failed to save Starting health status"
                                );
                            }
                        }
                        continue;
                    }
                }
            }

            // Perform health check
            debug!(
                process_id = %process_id,
                name = %process.name(),
                "Performing health check"
            );

            let status = match health_executor.check(&health_check).await {
                Ok(s) => s,
                Err(e) => {
                    error!(
                        process_id = %process_id,
                        name = %process.name(),
                        error = %e,
                        "Health check failed with error"
                    );
                    HealthStatus::Unhealthy
                }
            };

            // Update health status
            let mut updated_process = process.clone();
            updated_process.update_health_status(status);

            // Handle unhealthy status
            if status == HealthStatus::Unhealthy {
                updated_process.increment_health_check_failures();
                let failures = updated_process.health_check_failures();

                warn!(
                    process_id = %process_id,
                    name = %process.name(),
                    failures = failures,
                    retries = health_check.retries,
                    "Health check failed"
                );

                // Check if we should stop the process (ProcessSupervisionService will restart it)
                if health_check.restart_after > 0 && failures >= health_check.restart_after {
                    warn!(
                        process_id = %process_id,
                        name = %process.name(),
                        failures = failures,
                        "Health check failure threshold reached, stopping unhealthy process"
                    );

                    // Get PID before stopping
                    if let Some(pid) = updated_process.pid() {
                        // Reset failure count before stopping
                        updated_process.reset_health_check_failures();
                        let _ = repository.save(updated_process.clone()).await;

                        // Kill the process - ProcessSupervisionService will handle restart according to policy
                        match process_executor.kill(pid, 15).await {
                            Ok(_) => {
                                info!(
                                    process_id = %process_id,
                                    name = %updated_process.name(),
                                    pid = pid,
                                    "Stopped unhealthy process (ProcessSupervisionService will restart if policy allows)"
                                );
                                // Stop monitoring this process - it will be restarted with a new PID
                                break;
                            }
                            Err(e) => {
                                error!(
                                    process_id = %process_id,
                                    name = %updated_process.name(),
                                    error = %e,
                                    "Failed to stop unhealthy process"
                                );
                            }
                        }
                    }
                }
            } else {
                debug!(
                    process_id = %process_id,
                    name = %process.name(),
                    "Health check passed"
                );

                // If runtime_success_sec is 0 (disabled), use health check success to reset failures
                // This allows health checks to serve as the "successful run" indicator
                if updated_process.runtime_success_sec() == 0
                    && updated_process.consecutive_failures() > 0
                {
                    info!(
                        process_id = %process_id,
                        name = %process.name(),
                        previous_failures = updated_process.consecutive_failures(),
                        "Health check passed with runtime_success_sec=0 - resetting failure counter"
                    );
                    updated_process.reset_failures();
                }
            }

            // Save updated process
            if let Err(e) = repository.save(updated_process).await {
                error!(
                    process_id = %process_id,
                    error = %e,
                    "Failed to save process after health check"
                );
            }
        }

        info!(process_id = %process_id, "Health monitor stopped");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ports::MockRepository;
    use crate::domain::{DomainError, HealthCheck, Process};
    use async_trait::async_trait;

    // Mock health executor that always returns healthy
    struct MockHealthyExecutor;

    #[async_trait]
    impl HealthCheckExecutor for MockHealthyExecutor {
        async fn check(&self, _config: &HealthCheck) -> Result<HealthStatus, DomainError> {
            Ok(HealthStatus::Healthy)
        }
    }

    // Mock health executor that always returns unhealthy
    struct MockUnhealthyExecutor;

    #[async_trait]
    impl HealthCheckExecutor for MockUnhealthyExecutor {
        async fn check(&self, _config: &HealthCheck) -> Result<HealthStatus, DomainError> {
            Ok(HealthStatus::Unhealthy)
        }
    }

    // Mock process executor
    use crate::domain::ports::{ProcessExecutor, SpawnConfig, SpawnResult};

    struct MockProcessExecutor {
        kill_count: Arc<tokio::sync::Mutex<u32>>,
    }

    impl MockProcessExecutor {
        fn new() -> Self {
            Self {
                kill_count: Arc::new(tokio::sync::Mutex::new(0)),
            }
        }

        async fn get_kill_count(&self) -> u32 {
            *self.kill_count.lock().await
        }
    }

    #[async_trait]
    impl ProcessExecutor for MockProcessExecutor {
        async fn spawn(&self, _config: SpawnConfig) -> Result<SpawnResult, DomainError> {
            Ok(SpawnResult {
                pid: 1234,
                exit_handle: None,
            })
        }

        async fn kill(&self, _pid: u32, _signal: i32) -> Result<(), DomainError> {
            *self.kill_count.lock().await += 1;
            Ok(())
        }

        async fn kill_with_mode(
            &self,
            _pid: u32,
            _signal: i32,
            _mode: crate::domain::KillMode,
        ) -> Result<(), DomainError> {
            *self.kill_count.lock().await += 1;
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
    async fn test_health_monitor_healthy_process() {
        let repo = Arc::new(MockRepository::new());
        let health_exec = Arc::new(MockHealthyExecutor);
        let process_exec = Arc::new(MockProcessExecutor::new());

        // Create process with health check
        let mut process = Process::builder("test".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_running(1234).unwrap();
        process.set_health_check(HealthCheck::tcp("localhost".to_string(), 8080).with_interval(1));
        let process_id = process.id();
        repo.save(process).await.unwrap();

        let monitor = HealthMonitoringService::new(repo.clone(), health_exec, process_exec.clone());
        monitor.start_monitoring(process_id);

        // Wait for a few health checks
        sleep(Duration::from_millis(2500)).await;

        // Process should still be running, no kills
        let updated = repo.find_by_id(&process_id).await.unwrap().unwrap();
        assert!(updated.is_running());
        assert_eq!(updated.health_status(), HealthStatus::Healthy);
        assert_eq!(process_exec.get_kill_count().await, 0);
    }

    #[tokio::test]
    async fn test_health_monitor_unhealthy_process_stops() {
        let repo = Arc::new(MockRepository::new());
        let health_exec = Arc::new(MockUnhealthyExecutor);
        let process_exec = Arc::new(MockProcessExecutor::new());

        // Create process with health check that stops after 2 failures
        let mut process = Process::builder("test".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        process.mark_starting().unwrap();
        process.mark_running(1234).unwrap();
        process.set_health_check(
            HealthCheck::tcp("localhost".to_string(), 8080)
                .with_interval(1)
                .with_restart_after(2),
        );
        let process_id = process.id();
        repo.save(process).await.unwrap();

        let monitor = HealthMonitoringService::new(repo.clone(), health_exec, process_exec.clone());
        monitor.start_monitoring(process_id);

        // Wait for health checks to run and trigger stop
        sleep(Duration::from_millis(2500)).await;

        // Should have triggered at least one kill
        assert!(process_exec.get_kill_count().await >= 1);
    }
}
