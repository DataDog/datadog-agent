//! UpdateProcess use case
//! Handles updating process configuration with hot-update support

use crate::domain::ports::ProcessRepository;
use crate::domain::{DomainError, UpdateProcessCommand, UpdateProcessResponse};
use async_trait::async_trait;
use std::sync::Arc;

/// Use case for updating a process
#[async_trait]
pub trait UpdateProcess: Send + Sync {
    async fn execute(
        &self,
        command: UpdateProcessCommand,
    ) -> Result<UpdateProcessResponse, DomainError>;
}

/// Implementation of UpdateProcess use case
pub struct UpdateProcessUseCase {
    repository: Arc<dyn ProcessRepository>,
    stop_process: Arc<dyn super::StopProcess>,
    start_process: Arc<dyn super::StartProcess>,
}

impl UpdateProcessUseCase {
    pub fn new(
        repository: Arc<dyn ProcessRepository>,
        stop_process: Arc<dyn super::StopProcess>,
        start_process: Arc<dyn super::StartProcess>,
    ) -> Self {
        Self {
            repository,
            stop_process,
            start_process,
        }
    }
}

#[async_trait]
impl UpdateProcess for UpdateProcessUseCase {
    async fn execute(
        &self,
        command: UpdateProcessCommand,
    ) -> Result<UpdateProcessResponse, DomainError> {
        // 1. Load the process from repository (by ID or name)
        let mut process = self
            .repository
            .find_by_id_or_name(command.process_id.as_ref(), command.process_name.as_deref())
            .await?;

        let process_id = process.id();
        let was_running = process.is_running();
        let mut updated_fields = Vec::new();
        let mut restart_required_fields = Vec::new();

        // 2. Apply hot-update fields (no restart required)
        if let Some(restart_policy) = command.restart_policy {
            process.set_restart_policy(restart_policy);
            updated_fields.push("restart_policy".to_string());
        }

        if let Some(timeout) = command.timeout_stop_sec {
            process.set_timeout_stop_sec(timeout);
            updated_fields.push("timeout_stop_sec".to_string());
        }

        if let Some(restart_sec) = command.restart_sec {
            process.set_restart_sec(restart_sec);
            updated_fields.push("restart_sec".to_string());
        }

        if let Some(restart_max) = command.restart_max_delay {
            process.set_restart_max_delay_sec(restart_max);
            updated_fields.push("restart_max_delay".to_string());
        }

        if let Some(limits) = command.resource_limits {
            process.set_resource_limits(limits);
            updated_fields.push("resources".to_string());
        }

        if let Some(health_check) = command.health_check {
            process.set_health_check(health_check);
            updated_fields.push("health_check".to_string());
        }

        if let Some(exit_codes) = command.success_exit_status {
            if !exit_codes.is_empty() {
                process.set_success_exit_status(exit_codes);
                updated_fields.push("success_exit_status".to_string());
            }
        }

        // 3. Apply restart-required fields
        if let Some(env) = command.env {
            if !env.is_empty() {
                process.set_env(env);
                updated_fields.push("env".to_string());
                restart_required_fields.push("env".to_string());
            }
        }

        if let Some(env_file) = command.environment_file {
            process.set_environment_file(Some(env_file));
            updated_fields.push("environment_file".to_string());
            restart_required_fields.push("environment_file".to_string());
        }

        if let Some(working_dir) = command.working_dir {
            process.set_working_dir(Some(working_dir));
            updated_fields.push("working_dir".to_string());
            restart_required_fields.push("working_dir".to_string());
        }

        if let Some(user) = command.user {
            process.set_user(Some(user));
            updated_fields.push("user".to_string());
            restart_required_fields.push("user".to_string());
        }

        if let Some(group) = command.group {
            process.set_group(Some(group));
            updated_fields.push("group".to_string());
            restart_required_fields.push("group".to_string());
        }

        if let Some(runtime_dirs) = command.runtime_directory {
            if !runtime_dirs.is_empty() {
                process.set_runtime_directory(runtime_dirs).map_err(|e| {
                    DomainError::InvalidCommand(format!("Invalid runtime directory: {}", e))
                })?;
                updated_fields.push("runtime_directory".to_string());
                restart_required_fields.push("runtime_directory".to_string());
            }
        }

        if let Some(caps) = command.ambient_capabilities {
            if !caps.is_empty() {
                process.set_ambient_capabilities(caps);
                updated_fields.push("ambient_capabilities".to_string());
                restart_required_fields.push("ambient_capabilities".to_string());
            }
        }

        if let Some(kill_mode) = command.kill_mode {
            process.set_kill_mode(kill_mode);
            updated_fields.push("kill_mode".to_string());
            restart_required_fields.push("kill_mode".to_string());
        }

        if let Some(signal) = command.kill_signal {
            process.set_kill_signal(signal.to_string());
            updated_fields.push("kill_signal".to_string());
            restart_required_fields.push("kill_signal".to_string());
        }

        if let Some(pidfile) = command.pidfile {
            process.set_pidfile(Some(pidfile));
            updated_fields.push("pidfile".to_string());
            restart_required_fields.push("pidfile".to_string());
        }

        // 4. If dry-run, return without saving
        if command.dry_run {
            return Ok(UpdateProcessResponse {
                process_id,
                updated_fields,
                restart_required_fields,
                process_restarted: false,
            });
        }

        // 5. Save updated process
        self.repository.save(process).await?;

        // 6. Handle restart if requested and process was running
        let mut process_restarted = false;
        if command.restart_process && was_running && !restart_required_fields.is_empty() {
            // Stop the process
            let stop_cmd = crate::domain::StopProcessCommand::from_id(process_id);
            self.stop_process.execute(stop_cmd).await?;

            // Start the process
            let start_cmd = crate::domain::StartProcessCommand::from_id(process_id);
            self.start_process.execute(start_cmd).await?;

            process_restarted = true;
        }

        // 7. Return response
        Ok(UpdateProcessResponse {
            process_id,
            updated_fields,
            restart_required_fields,
            process_restarted,
        })
    }
}

// Tests are covered by comprehensive E2E tests in tests/e2e_update.rs
