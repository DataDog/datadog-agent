//! Configuration Parser Service
//! Translates YAML ProcessConfig into domain CreateProcessCommand

use crate::domain::{
    CreateProcessCommand, DomainError, HealthCheck, HealthCheckType, RestartPolicy,
};
use crate::infrastructure::ProcessConfig;
use tracing::warn;

/// Service for parsing YAML configuration into domain commands
pub struct ConfigParsingService;

impl ConfigParsingService {
    /// Parse a ProcessConfig (from YAML) into a CreateProcessCommand
    pub fn parse(name: String, config: ProcessConfig) -> Result<CreateProcessCommand, DomainError> {
        // Parse health check if configured
        let health_check =
            if let Some(ref hc_config) = config.health_check {
                Some(Self::parse_health_check(hc_config).map_err(|e| {
                    DomainError::InvalidCommand(format!("Invalid health check: {}", e))
                })?)
            } else {
                None
            };

        // Parse resource limits if configured
        let resource_limits = if let Some(ref limits_config) = config.resource_limits {
            Some(Self::parse_resource_limits(limits_config)?)
        } else {
            None
        };

        // Parse kill mode if configured
        let kill_mode = if let Some(ref kill_mode_str) = config.kill_mode {
            Some(
                kill_mode_str
                    .parse::<crate::domain::KillMode>()
                    .map_err(|e| {
                        DomainError::InvalidCommand(format!("Invalid kill_mode: {}", e))
                    })?,
            )
        } else {
            None
        };

        // Parse conditional starting
        let condition_path_exists = if let Some(ref conditions_str) = config.condition_path_exists {
            conditions_str
                .iter()
                .map(|s| crate::domain::PathCondition::parse(s))
                .collect()
        } else {
            Vec::new()
        };

        // Parse kill signal
        let kill_signal = config
            .kill_signal
            .as_ref()
            .and_then(|s| s.parse::<i32>().ok());

        let restart_policy = Self::parse_restart_policy(&config);

        Ok(CreateProcessCommand {
            name,
            command: config.command,
            args: config.args,
            description: config.description,
            restart: Some(restart_policy),
            restart_sec: config.restart_sec,
            restart_max_delay_sec: config.restart_max_delay_sec,
            start_limit_burst: config.start_limit_burst,
            start_limit_interval_sec: config.start_limit_interval_sec,
            working_dir: config.working_dir,
            env: if config.env.is_empty() {
                None
            } else {
                Some(config.env)
            },
            environment_file: config.environment_file,
            pidfile: config.pidfile,
            start_behavior: crate::domain::StartBehavior::Manual, // Handled separately in LoadConfig
            stdout: config.stdout,
            stderr: config.stderr,
            timeout_start_sec: config.timeout_start_sec,
            timeout_stop_sec: config.timeout_stop_sec,
            kill_signal,
            kill_mode,
            success_exit_status: config.success_exit_status.unwrap_or_default(),
            exec_start_pre: config.exec_start_pre.unwrap_or_default(),
            exec_start_post: config.exec_start_post.unwrap_or_default(),
            exec_stop_post: config.exec_stop_post.unwrap_or_default(),
            user: config.user,
            group: config.group,
            after: config.after.unwrap_or_default(),
            before: config.before.unwrap_or_default(),
            requires: config.requires.unwrap_or_default(),
            wants: config.wants.unwrap_or_default(),
            binds_to: config.binds_to.unwrap_or_default(),
            conflicts: config.conflicts.unwrap_or_default(),
            process_type: None, // TODO: Add to YAML config if needed
            health_check,
            resource_limits,
            condition_path_exists,
            runtime_directory: config.runtime_directory.unwrap_or_default(),
            ambient_capabilities: config.ambient_capabilities.unwrap_or_default(),
        })
    }

    /// Parse restart policy from config string
    fn parse_restart_policy(config: &ProcessConfig) -> RestartPolicy {
        match config.restart.as_deref() {
            Some("always") => RestartPolicy::Always,
            Some("on-failure") => RestartPolicy::OnFailure,
            Some("on-success") => RestartPolicy::OnSuccess,
            Some("never") | None => RestartPolicy::Never,
            Some(other) => {
                warn!(restart_policy = %other, "Unknown restart policy, using 'never'");
                RestartPolicy::Never
            }
        }
    }

    /// Parse health check from config
    fn parse_health_check(
        config: &crate::infrastructure::HealthCheckConfig,
    ) -> Result<HealthCheck, String> {
        let check_type = match config.check_type.to_lowercase().as_str() {
            "http" => HealthCheckType::Http,
            "tcp" => HealthCheckType::Tcp,
            "exec" => HealthCheckType::Exec,
            other => return Err(format!("Unknown health check type: {}", other)),
        };

        let health_check = HealthCheck {
            check_type,
            interval: config.interval,
            timeout: config.timeout,
            retries: config.retries,
            start_period: config.start_period,
            restart_after: config.restart_after,
            http_endpoint: config.endpoint.clone(),
            http_method: config.method.clone(),
            http_expected_status: config.expected_status,
            tcp_host: config.host.clone(),
            tcp_port: config.port,
            exec_command: config.command.clone(),
            exec_args: config.args.clone(),
        };

        // Validate the health check
        health_check.validate()?;

        Ok(health_check)
    }

    /// Parse resource limits from config
    fn parse_resource_limits(
        limits_config: &crate::infrastructure::ResourceLimitsConfig,
    ) -> Result<crate::domain::ResourceLimits, DomainError> {
        let mut limits = crate::domain::ResourceLimits::new();

        if let Some(cpu_str) = &limits_config.cpu {
            let cpu_millis = crate::domain::ResourceLimits::parse_cpu(cpu_str)
                .map_err(|e| DomainError::InvalidCommand(format!("Invalid CPU limit: {}", e)))?;
            limits = limits.with_cpu_millis(cpu_millis);
        }

        if let Some(mem_str) = &limits_config.memory {
            let mem_bytes = crate::domain::ResourceLimits::parse_memory(mem_str)
                .map_err(|e| DomainError::InvalidCommand(format!("Invalid memory limit: {}", e)))?;
            limits = limits.with_memory_bytes(mem_bytes);
        }

        if let Some(pids) = limits_config.pids {
            limits = limits.with_max_pids(pids);
        }

        Ok(limits)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_restart_policy() {
        let mut config = ProcessConfig {
            command: "test".to_string(),
            description: None,
            args: vec![],
            auto_start: false,
            restart: Some("always".to_string()),
            restart_sec: None,
            restart_max_delay_sec: None,
            start_limit_burst: None,
            start_limit_interval_sec: None,
            working_dir: None,
            env: Default::default(),
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            timeout_start_sec: None,
            timeout_stop_sec: None,
            kill_signal: None,
            kill_mode: None,
            success_exit_status: None,
            exec_start_pre: None,
            exec_start_post: None,
            exec_stop_post: None,
            user: None,
            group: None,
            ambient_capabilities: None,
            runtime_directory: None,
            after: None,
            before: None,
            requires: None,
            wants: None,
            binds_to: None,
            conflicts: None,
            process_type: None,
            health_check: None,
            resource_limits: None,
            condition_path_exists: None,
        };

        assert!(matches!(
            ConfigParsingService::parse_restart_policy(&config),
            RestartPolicy::Always
        ));

        config.restart = Some("on-failure".to_string());
        assert!(matches!(
            ConfigParsingService::parse_restart_policy(&config),
            RestartPolicy::OnFailure
        ));

        config.restart = Some("never".to_string());
        assert!(matches!(
            ConfigParsingService::parse_restart_policy(&config),
            RestartPolicy::Never
        ));

        config.restart = None;
        assert!(matches!(
            ConfigParsingService::parse_restart_policy(&config),
            RestartPolicy::Never
        ));
    }

    #[test]
    fn test_parse_basic_config() {
        let config = ProcessConfig {
            command: "/bin/echo".to_string(),
            description: None,
            args: vec!["hello".to_string()],
            auto_start: true,
            restart: Some("always".to_string()),
            restart_sec: Some(5),
            restart_max_delay_sec: None,
            start_limit_burst: None,
            start_limit_interval_sec: None,
            working_dir: Some("/tmp".to_string()),
            env: Default::default(),
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            timeout_start_sec: None,
            timeout_stop_sec: None,
            kill_signal: None,
            kill_mode: None,
            success_exit_status: None,
            exec_start_pre: None,
            exec_start_post: None,
            exec_stop_post: None,
            user: None,
            group: None,
            ambient_capabilities: None,
            runtime_directory: None,
            after: None,
            before: None,
            requires: None,
            wants: None,
            binds_to: None,
            conflicts: None,
            process_type: None,
            health_check: None,
            resource_limits: None,
            condition_path_exists: None,
        };

        let result = ConfigParsingService::parse("test-process".to_string(), config);
        assert!(result.is_ok());

        let cmd = result.unwrap();
        assert_eq!(cmd.name, "test-process");
        assert_eq!(cmd.command, "/bin/echo");
        assert_eq!(cmd.args, vec!["hello".to_string()]);
        assert_eq!(cmd.restart, Some(RestartPolicy::Always));
        assert_eq!(cmd.restart_sec, Some(5));
        assert_eq!(cmd.working_dir, Some("/tmp".to_string()));
    }

    #[test]
    fn test_parse_config_with_description() {
        let config = ProcessConfig {
            command: "/bin/app".to_string(),
            description: Some("My Application Service".to_string()),
            args: vec![],
            auto_start: false,
            restart: None,
            restart_sec: None,
            restart_max_delay_sec: None,
            start_limit_burst: None,
            start_limit_interval_sec: None,
            working_dir: None,
            env: Default::default(),
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            timeout_start_sec: None,
            timeout_stop_sec: None,
            kill_signal: None,
            kill_mode: None,
            success_exit_status: None,
            exec_start_pre: None,
            exec_start_post: None,
            exec_stop_post: None,
            user: None,
            group: None,
            ambient_capabilities: None,
            runtime_directory: None,
            after: None,
            before: None,
            requires: None,
            wants: None,
            binds_to: None,
            conflicts: None,
            process_type: None,
            health_check: None,
            resource_limits: None,
            condition_path_exists: None,
        };

        let result = ConfigParsingService::parse("my-app".to_string(), config);
        assert!(result.is_ok());

        let cmd = result.unwrap();
        assert_eq!(cmd.name, "my-app");
        assert_eq!(cmd.description, Some("My Application Service".to_string()));
    }
}
