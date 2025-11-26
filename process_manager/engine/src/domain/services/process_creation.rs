//! Process Creation Service
//!
//! Domain service that handles process entity creation and configuration.
//! This service encapsulates the logic for creating and configuring Process entities
//! from a CreateProcessCommand, used by both CreateProcess and LoadConfig use cases.

use crate::domain::ports::ProcessRepository;
use crate::domain::services::EnvironmentFileParsingService;
use crate::domain::{CreateProcessCommand, DomainError, Process, ProcessId};
use std::sync::Arc;
use tracing::debug;

/// Process Creation Service
/// Handles creating and configuring process entities from commands
pub struct ProcessCreationService {
    repository: Arc<dyn ProcessRepository>,
}

impl ProcessCreationService {
    /// Create a new process creation service
    pub fn new(repository: Arc<dyn ProcessRepository>) -> Self {
        Self { repository }
    }

    /// Create and configure a process entity from a command
    ///
    /// This validates uniqueness, creates the entity using the builder pattern,
    /// and saves it to the repository.
    ///
    /// Returns the created process ID
    pub async fn create_from_command(
        &self,
        command: CreateProcessCommand,
    ) -> Result<(ProcessId, String), DomainError> {
        // 1. Check if process with same name already exists
        if self.repository.exists_by_name(&command.name).await? {
            return Err(DomainError::DuplicateProcess(command.name.clone()));
        }

        debug!(
            name = %command.name,
            "Creating process entity"
        );

        // 2. Build process entity using builder pattern
        let process = self.build_process_from_command(&command)?;

        let id = process.id();
        let name = process.name().to_string();

        // 3. Save to repository
        self.repository.save(process).await?;

        Ok((id, name))
    }

    /// Build a process entity from a command using the builder pattern
    fn build_process_from_command(
        &self,
        command: &CreateProcessCommand,
    ) -> Result<Process, DomainError> {
        // Start with mandatory fields
        let mut builder = Process::builder(&command.name, &command.command);

        // Apply basic configuration
        if let Some(ref description) = command.description {
            builder = builder.description(description);
        }
        if !command.args.is_empty() {
            builder = builder.args(command.args.clone());
        }
        if let Some(process_type) = command.process_type {
            builder = builder.process_type(process_type);
        }
        if !command.success_exit_status.is_empty() {
            builder = builder.success_exit_status(command.success_exit_status.clone());
        }

        // Apply restart configuration
        if let Some(restart) = command.restart {
            builder = builder.restart_policy(restart);
        }
        if let Some(restart_sec) = command.restart_sec {
            builder = builder.restart_delay_sec(restart_sec);
        }
        if let Some(restart_max_delay_sec) = command.restart_max_delay_sec {
            builder = builder.restart_max_delay_sec(restart_max_delay_sec);
        }
        if let Some(start_limit_burst) = command.start_limit_burst {
            builder = builder.start_limit_burst(start_limit_burst);
        }
        if let Some(start_limit_interval_sec) = command.start_limit_interval_sec {
            builder = builder.start_limit_interval_sec(start_limit_interval_sec);
        }

        // Apply execution context
        if let Some(ref working_dir) = command.working_dir {
            builder = builder.working_dir(working_dir);
        }

        // Environment variables (merge file + explicit vars)
        let merged_env = self.merge_environment_variables(command)?;
        if !merged_env.is_empty() {
            builder = builder.env(merged_env);
        }

        if let Some(ref environment_file) = command.environment_file {
            builder = builder.environment_file(environment_file);
        }
        if let Some(ref pidfile) = command.pidfile {
            builder = builder.pidfile(pidfile);
        }
        if let Some(ref stdout) = command.stdout {
            builder = builder.stdout(stdout);
        }
        if let Some(ref stderr) = command.stderr {
            builder = builder.stderr(stderr);
        }

        // Apply timeouts
        if let Some(timeout_start_sec) = command.timeout_start_sec {
            builder = builder.timeout_start_sec(timeout_start_sec);
        }
        if let Some(timeout_stop_sec) = command.timeout_stop_sec {
            builder = builder.timeout_stop_sec(timeout_stop_sec);
        }

        // Apply kill configuration
        if let Some(kill_signal) = command.kill_signal {
            builder = builder.kill_signal(kill_signal);
        }
        if let Some(kill_mode) = command.kill_mode {
            builder = builder.kill_mode(kill_mode);
        }

        // Apply lifecycle hooks
        if !command.exec_start_pre.is_empty() {
            builder = builder.exec_start_pre(command.exec_start_pre.clone());
        }
        if !command.exec_start_post.is_empty() {
            builder = builder.exec_start_post(command.exec_start_post.clone());
        }
        if !command.exec_stop_post.is_empty() {
            builder = builder.exec_stop_post(command.exec_stop_post.clone());
        }

        // Apply security configuration
        if let Some(ref user) = command.user {
            builder = builder.user(user);
        }
        if let Some(ref group) = command.group {
            builder = builder.group(group);
        }
        if !command.runtime_directory.is_empty() {
            // Validate runtime directories before building
            Self::validate_runtime_directories(&command.runtime_directory)?;
            builder = builder.runtime_directory(command.runtime_directory.clone());
        }
        if !command.ambient_capabilities.is_empty() {
            builder = builder.ambient_capabilities(command.ambient_capabilities.clone());
        }

        // Apply dependencies
        if !command.after.is_empty() {
            builder = builder.after(command.after.clone());
        }
        if !command.before.is_empty() {
            builder = builder.before(command.before.clone());
        }
        if !command.requires.is_empty() {
            builder = builder.requires(command.requires.clone());
        }
        if !command.wants.is_empty() {
            builder = builder.wants(command.wants.clone());
        }
        if !command.binds_to.is_empty() {
            builder = builder.binds_to(command.binds_to.clone());
        }
        if !command.conflicts.is_empty() {
            builder = builder.conflicts(command.conflicts.clone());
        }

        // Apply health check
        if let Some(ref health_check) = command.health_check {
            builder = builder.health_check(health_check.clone());
        }

        // Apply resource limits (with validation)
        if let Some(ref resource_limits) = command.resource_limits {
            Self::validate_resource_limits(resource_limits)?;
            builder = builder.resource_limits(resource_limits.clone());
        }

        // Apply conditions
        if !command.condition_path_exists.is_empty() {
            builder = builder.condition_path_exists(command.condition_path_exists.clone());
        }

        // Apply socket activation
        if let Some(ref socket) = command.socket {
            builder = builder.socket(socket.clone());
        }

        // Build the process
        builder.build()
    }

    /// Merge environment variables from file and explicit command
    fn merge_environment_variables(
        &self,
        command: &CreateProcessCommand,
    ) -> Result<std::collections::HashMap<String, String>, DomainError> {
        let mut merged_env = std::collections::HashMap::new();

        // First, load from environment file if specified
        if let Some(ref environment_file) = command.environment_file {
            match EnvironmentFileParsingService::parse_file(environment_file) {
                Ok(file_vars) => {
                    merged_env.extend(file_vars);
                }
                Err(e) => {
                    // If file starts with '-', it's optional (ignore if missing)
                    if !environment_file.starts_with('-') {
                        return Err(e);
                    }
                    // Otherwise, try without the '-' prefix
                    let path_without_prefix = &environment_file[1..];
                    if let Ok(file_vars) =
                        EnvironmentFileParsingService::parse_file(path_without_prefix)
                    {
                        merged_env.extend(file_vars);
                    }
                    // If still fails, ignore (optional file)
                }
            }
        }

        // Then, overlay explicit env vars (they override file vars)
        if let Some(ref env) = command.env {
            merged_env.extend(env.clone());
        }

        Ok(merged_env)
    }

    /// Validate runtime directories
    ///
    /// Runtime directories must be:
    /// - Relative paths (not absolute)
    /// - Not contain parent directory references (..)
    /// - Not empty
    fn validate_runtime_directories(dirs: &[String]) -> Result<(), DomainError> {
        for dir in dirs {
            if dir.is_empty() {
                return Err(DomainError::InvalidCommand(
                    "Runtime directory cannot be empty".to_string(),
                ));
            }

            if dir.starts_with('/') {
                return Err(DomainError::InvalidCommand(format!(
                    "Runtime directory '{}' must be relative (not absolute)",
                    dir
                )));
            }

            if dir.contains("..") {
                return Err(DomainError::InvalidCommand(format!(
                    "Runtime directory '{}' cannot contain '..' (parent directory reference)",
                    dir
                )));
            }

            // Check for invalid characters
            if dir.contains('\0') {
                return Err(DomainError::InvalidCommand(format!(
                    "Runtime directory '{}' contains invalid null character",
                    dir
                )));
            }
        }

        Ok(())
    }

    /// Validate resource limits
    ///
    /// Resource limits must be positive values
    fn validate_resource_limits(limits: &crate::domain::ResourceLimits) -> Result<(), DomainError> {
        // Validate CPU limit
        if let Some(cpu) = limits.cpu_millis {
            if cpu == 0 {
                return Err(DomainError::InvalidCommand(
                    "CPU limit must be positive (greater than 0)".to_string(),
                ));
            }
        }

        // Validate memory limit
        if let Some(memory) = limits.memory_bytes {
            if memory == 0 {
                return Err(DomainError::InvalidCommand(
                    "Memory limit must be positive (greater than 0)".to_string(),
                ));
            }
        }

        // Validate PIDs limit
        if let Some(pids) = limits.max_pids {
            if pids == 0 {
                return Err(DomainError::InvalidCommand(
                    "PIDs limit must be positive (greater than 0)".to_string(),
                ));
            }
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::InMemoryProcessRepository;

    #[tokio::test]
    async fn test_create_from_command() {
        let repository = Arc::new(InMemoryProcessRepository::new());
        let service = ProcessCreationService::new(repository.clone());

        let command = CreateProcessCommand {
            name: "test".to_string(),
            command: "/bin/echo".to_string(),
            args: vec!["hello".to_string()],
            ..Default::default()
        };

        let result = service.create_from_command(command).await;
        assert!(result.is_ok());

        let (id, name) = result.unwrap();
        assert_eq!(name, "test");

        // Verify process was saved
        let process = repository.find_by_id(&id).await.unwrap().unwrap();
        assert_eq!(process.name(), "test");
        assert_eq!(process.command(), "/bin/echo");
    }

    #[tokio::test]
    async fn test_duplicate_name_fails() {
        let repository = Arc::new(InMemoryProcessRepository::new());
        let service = ProcessCreationService::new(repository.clone());

        let command = CreateProcessCommand {
            name: "test".to_string(),
            command: "/bin/echo".to_string(),
            ..Default::default()
        };

        // First creation should succeed
        service.create_from_command(command.clone()).await.unwrap();

        // Second creation with same name should fail
        let result = service.create_from_command(command).await;
        assert!(matches!(result, Err(DomainError::DuplicateProcess(_))));
    }

    #[tokio::test]
    async fn test_create_with_description() {
        let repository = Arc::new(InMemoryProcessRepository::new());
        let service = ProcessCreationService::new(repository.clone());

        let command = CreateProcessCommand {
            name: "my-service".to_string(),
            command: "/bin/app".to_string(),
            description: Some("My Application Service".to_string()),
            ..Default::default()
        };

        let result = service.create_from_command(command).await;
        assert!(result.is_ok());

        let (id, _) = result.unwrap();
        let process = repository.find_by_id(&id).await.unwrap().unwrap();
        assert_eq!(process.description(), Some("My Application Service"));
    }

    #[test]
    fn test_validate_runtime_directories_valid() {
        let dirs = vec!["run".to_string(), "tmp/cache".to_string()];
        assert!(ProcessCreationService::validate_runtime_directories(&dirs).is_ok());
    }

    #[test]
    fn test_validate_runtime_directories_absolute_path() {
        let dirs = vec!["/tmp".to_string()];
        let result = ProcessCreationService::validate_runtime_directories(&dirs);
        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
        assert!(result.unwrap_err().to_string().contains("must be relative"));
    }

    #[test]
    fn test_validate_runtime_directories_parent_reference() {
        let dirs = vec!["../etc".to_string()];
        let result = ProcessCreationService::validate_runtime_directories(&dirs);
        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
        assert!(result.unwrap_err().to_string().contains(".."));
    }

    #[test]
    fn test_validate_runtime_directories_empty() {
        let dirs = vec!["".to_string()];
        let result = ProcessCreationService::validate_runtime_directories(&dirs);
        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
        assert!(result.unwrap_err().to_string().contains("cannot be empty"));
    }

    #[test]
    fn test_validate_resource_limits_valid() {
        let limits = crate::domain::ResourceLimits {
            cpu_millis: Some(1000),
            memory_bytes: Some(1024 * 1024),
            max_pids: Some(100),
        };
        assert!(ProcessCreationService::validate_resource_limits(&limits).is_ok());
    }

    #[test]
    fn test_validate_resource_limits_zero_cpu() {
        let limits = crate::domain::ResourceLimits {
            cpu_millis: Some(0),
            memory_bytes: None,
            max_pids: None,
        };
        let result = ProcessCreationService::validate_resource_limits(&limits);
        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
        assert!(result.unwrap_err().to_string().contains("CPU limit"));
    }

    #[test]
    fn test_validate_resource_limits_zero_memory() {
        let limits = crate::domain::ResourceLimits {
            cpu_millis: None,
            memory_bytes: Some(0),
            max_pids: None,
        };
        let result = ProcessCreationService::validate_resource_limits(&limits);
        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
        assert!(result.unwrap_err().to_string().contains("Memory limit"));
    }

    #[test]
    fn test_validate_resource_limits_zero_pids() {
        let limits = crate::domain::ResourceLimits {
            cpu_millis: None,
            memory_bytes: None,
            max_pids: Some(0),
        };
        let result = ProcessCreationService::validate_resource_limits(&limits);
        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
        assert!(result.unwrap_err().to_string().contains("PIDs limit"));
    }
}
