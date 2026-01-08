//! Proto-to-Domain conversions using TryFrom pattern
//!
//! This module provides idiomatic Rust conversions from protobuf types to domain types
//! using the TryFrom trait. It includes helper traits for handling proto's "zero/empty means None"
//! semantics in a type-safe way.

use crate::domain::{CreateProcessCommand, UpdateProcessCommand};
use crate::proto::process_manager::{CreateRequest, UpdateRequest};
use tonic::Status;

/// Extension trait for converting proto primitives to Option
///
/// Proto uses zero values (0, empty string) to represent "not set",
/// while Rust domain models use Option<T>. This trait provides
/// a type-safe conversion.
pub trait ProtoOptional: Sized {
    fn into_option(self) -> Option<Self>;
}

impl ProtoOptional for u64 {
    fn into_option(self) -> Option<Self> {
        if self > 0 {
            Some(self)
        } else {
            None
        }
    }
}

impl ProtoOptional for u32 {
    fn into_option(self) -> Option<Self> {
        if self > 0 {
            Some(self)
        } else {
            None
        }
    }
}

impl ProtoOptional for i32 {
    fn into_option(self) -> Option<Self> {
        if self != 0 {
            Some(self)
        } else {
            None
        }
    }
}

impl ProtoOptional for String {
    fn into_option(self) -> Option<Self> {
        if self.is_empty() {
            None
        } else {
            Some(self)
        }
    }
}

/// Extension trait for converting Vec to Option<Vec> (empty vec becomes None)
pub trait ProtoVecOptional<T> {
    fn into_option(self) -> Option<Vec<T>>;
}

impl<T> ProtoVecOptional<T> for Vec<T> {
    fn into_option(self) -> Option<Vec<T>> {
        if self.is_empty() {
            None
        } else {
            Some(self)
        }
    }
}

/// Convert protobuf CreateRequest to domain CreateProcessCommand
impl TryFrom<CreateRequest> for CreateProcessCommand {
    type Error = Status;

    fn try_from(req: CreateRequest) -> Result<Self, Self::Error> {
        // Validate required fields
        if req.name.is_empty() {
            return Err(Status::invalid_argument("Process name is required"));
        }
        if req.command.is_empty() {
            return Err(Status::invalid_argument("Process command is required"));
        }

        Ok(Self {
            // Required fields
            name: req.name,
            command: req.command,
            args: req.args,

            description: req.description.into_option(),

            // Restart configuration
            restart: super::mappers::proto_to_restart_policy(req.restart),
            restart_sec: req.restart_sec.into_option(),
            restart_max_delay_sec: req.restart_max_delay.into_option(),
            start_limit_burst: req.start_limit_burst.into_option(),
            start_limit_interval_sec: req.start_limit_interval.into_option(),

            // Execution context
            working_dir: req.working_dir.into_option(),
            env: if req.env.is_empty() {
                None
            } else {
                Some(req.env)
            },
            environment_file: req.environment_file.into_option(),
            pidfile: req.pidfile.into_option(),
            start_behavior: if req.auto_start {
                crate::domain::StartBehavior::Automatic
            } else {
                crate::domain::StartBehavior::Manual
            },

            // Output redirection
            stdout: req.stdout.into_option(),
            stderr: req.stderr.into_option(),

            // Timeouts
            timeout_start_sec: req.timeout_start_sec.into_option(),
            timeout_stop_sec: req.timeout_stop_sec.into_option(),

            // Kill configuration
            kill_signal: req.kill_signal.into_option(),
            kill_mode: super::mappers::proto_to_kill_mode(req.kill_mode),

            // Exit status
            success_exit_status: req.success_exit_status,

            // Hooks
            exec_start_pre: req.exec_start_pre,
            exec_start_post: req.exec_start_post,
            exec_stop_post: req.exec_stop_post,

            // User/Group
            user: req.user.into_option(),
            group: req.group.into_option(),

            // Dependencies
            after: req.after,
            before: req.before,
            requires: req.requires,
            wants: req.wants,
            binds_to: req.binds_to,
            conflicts: req.conflicts,

            // Process type
            process_type: super::mappers::proto_to_process_type(req.process_type),

            // Health check
            health_check: req
                .health_check
                .and_then(|hc| super::mappers::proto_to_health_check(&hc)),

            // Resource limits
            resource_limits: req
                .resource_limits
                .map(|rl| super::mappers::proto_to_resource_limits(&rl)),

            // Conditional execution
            condition_path_exists: super::mappers::parse_path_conditions(
                &req.condition_path_exists,
            ),

            // Runtime directories
            runtime_directory: req.runtime_directory,

            // Ambient capabilities
            ambient_capabilities: req.ambient_capabilities,
        })
    }
}

/// Convert protobuf UpdateRequest to domain UpdateProcessCommand
impl TryFrom<UpdateRequest> for UpdateProcessCommand {
    type Error = Status;

    fn try_from(req: UpdateRequest) -> Result<Self, Self::Error> {
        use std::collections::HashMap;

        // Validate required field
        if req.id.is_empty() {
            return Err(Status::invalid_argument("Process ID is required"));
        }

        // Parse process ID or name
        let (process_id, process_name) = if let Ok(pid) = super::mappers::parse_process_id(&req.id)
        {
            (Some(pid), None)
        } else {
            (None, Some(req.id))
        };

        // Validate restart policy if provided
        let restart_policy = if let Some(ref policy_str) = req.restart_policy {
            match super::mappers::parse_restart_policy_string(policy_str) {
                Some(policy) => Some(policy),
                None => {
                    return Err(Status::invalid_argument(format!(
                        "Invalid restart policy: '{}'. Must be one of: never, always, on-failure, on-success",
                        policy_str
                    )));
                }
            }
        } else {
            None
        };

        // Parse environment variables from KEY=VALUE format
        let env = if req.env.is_empty() {
            None
        } else {
            let mut env_map = HashMap::new();
            for entry in &req.env {
                if let Some((key, value)) = entry.split_once('=') {
                    env_map.insert(key.to_string(), value.to_string());
                }
            }
            if env_map.is_empty() {
                None
            } else {
                Some(env_map)
            }
        };

        // Parse kill signal (name or number)
        let kill_signal = req.kill_signal.as_ref().and_then(|s| {
            match s.to_uppercase().as_str() {
                "SIGTERM" => Some(15),
                "SIGKILL" => Some(9),
                "SIGINT" => Some(2),
                "SIGHUP" => Some(1),
                "SIGQUIT" => Some(3),
                "SIGUSR1" => Some(10),
                "SIGUSR2" => Some(12),
                _ => s.parse().ok(), // Try parsing as number
            }
        });

        Ok(Self {
            process_id,
            process_name,

            // Hot-update fields
            restart_policy,
            timeout_stop_sec: req.timeout_stop_sec,
            restart_sec: req.restart_sec,
            restart_max_delay: req.restart_max,
            resource_limits: req
                .resource_limits
                .as_ref()
                .map(super::mappers::proto_to_resource_limits),
            health_check: req
                .health_check
                .as_ref()
                .and_then(super::mappers::proto_to_health_check),
            success_exit_status: if req.success_exit_status.is_empty() {
                None
            } else {
                Some(req.success_exit_status.iter().map(|&x| x as i32).collect())
            },

            // Restart-required fields
            env,
            environment_file: req.env_file,
            working_dir: req.working_dir,
            user: req.user,
            group: req.group,
            runtime_directory: if req.runtime_directory.is_empty() {
                None
            } else {
                Some(req.runtime_directory)
            },
            ambient_capabilities: if req.ambient_capabilities.is_empty() {
                None
            } else {
                Some(req.ambient_capabilities)
            },
            kill_mode: req
                .kill_mode
                .as_ref()
                .and_then(|s| super::mappers::parse_kill_mode_string(s)),
            kill_signal,
            pidfile: req.pidfile,

            // Flags
            restart_process: req.restart_process,
            dry_run: req.dry_run,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_proto_optional_u64() {
        assert_eq!(0u64.into_option(), None);
        assert_eq!(1u64.into_option(), Some(1));
        assert_eq!(100u64.into_option(), Some(100));
    }

    #[test]
    fn test_proto_optional_u32() {
        assert_eq!(0u32.into_option(), None);
        assert_eq!(1u32.into_option(), Some(1));
        assert_eq!(100u32.into_option(), Some(100));
    }

    #[test]
    fn test_proto_optional_i32() {
        assert_eq!(0i32.into_option(), None);
        assert_eq!(1i32.into_option(), Some(1));
        assert_eq!((-1i32).into_option(), Some(-1));
    }

    #[test]
    fn test_proto_optional_string() {
        assert_eq!("".to_string().into_option(), None);
        assert_eq!("hello".to_string().into_option(), Some("hello".to_string()));
    }

    #[test]
    fn test_proto_vec_optional() {
        let empty: Vec<String> = vec![];
        assert_eq!(empty.into_option(), None);

        let non_empty = vec!["a".to_string(), "b".to_string()];
        assert_eq!(
            non_empty.clone().into_option(),
            Some(vec!["a".to_string(), "b".to_string()])
        );
    }

    #[test]
    fn test_create_request_conversion_minimal() {
        let req = CreateRequest {
            name: "test".to_string(),
            command: "/bin/test".to_string(),
            ..Default::default()
        };

        let result = CreateProcessCommand::try_from(req);
        assert!(result.is_ok());

        let cmd = result.unwrap();
        assert_eq!(cmd.name, "test");
        assert_eq!(cmd.command, "/bin/test");
        assert_eq!(cmd.restart_sec, None);
        assert_eq!(cmd.working_dir, None);
    }

    #[test]
    fn test_create_request_conversion_with_optionals() {
        let req = CreateRequest {
            name: "test".to_string(),
            command: "/bin/test".to_string(),
            restart_sec: 5,
            working_dir: "/tmp".to_string(),
            auto_start: true,
            ..Default::default()
        };

        let result = CreateProcessCommand::try_from(req);
        assert!(result.is_ok());

        let cmd = result.unwrap();
        assert_eq!(cmd.restart_sec, Some(5));
        assert_eq!(cmd.working_dir, Some("/tmp".to_string()));
        assert_eq!(cmd.start_behavior, crate::domain::StartBehavior::Automatic);
    }

    #[test]
    fn test_create_request_validation_empty_name() {
        let req = CreateRequest {
            name: "".to_string(),
            command: "/bin/test".to_string(),
            ..Default::default()
        };

        let result = CreateProcessCommand::try_from(req);
        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .message()
            .contains("Process name is required"));
    }

    #[test]
    fn test_create_request_validation_empty_command() {
        let req = CreateRequest {
            name: "test".to_string(),
            command: "".to_string(),
            ..Default::default()
        };

        let result = CreateProcessCommand::try_from(req);
        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .message()
            .contains("Process command is required"));
    }

    #[test]
    fn test_update_request_conversion() {
        let req = UpdateRequest {
            id: "test-id".to_string(),
            restart_sec: Some(10),
            timeout_stop_sec: Some(30),
            restart_process: true,
            ..Default::default()
        };

        let result = UpdateProcessCommand::try_from(req);
        assert!(result.is_ok());

        let cmd = result.unwrap();
        assert_eq!(cmd.process_name, Some("test-id".to_string()));
        assert_eq!(cmd.restart_sec, Some(10));
        assert_eq!(cmd.timeout_stop_sec, Some(30));
        assert!(cmd.restart_process);
    }

    #[test]
    fn test_update_request_validation_empty_id() {
        let req = UpdateRequest {
            id: "".to_string(),
            ..Default::default()
        };

        let result = UpdateProcessCommand::try_from(req);
        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .message()
            .contains("Process ID is required"));
    }

    #[test]
    fn test_update_request_restart_policy_validation() {
        let req = UpdateRequest {
            id: "test-id".to_string(),
            restart_policy: Some("invalid-policy".to_string()),
            ..Default::default()
        };

        let result = UpdateProcessCommand::try_from(req);
        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .message()
            .contains("Invalid restart policy"));
    }

    #[test]
    fn test_update_request_env_parsing() {
        let req = UpdateRequest {
            id: "test-id".to_string(),
            env: vec!["KEY1=value1".to_string(), "KEY2=value2".to_string()],
            ..Default::default()
        };

        let result = UpdateProcessCommand::try_from(req);
        assert!(result.is_ok());

        let cmd = result.unwrap();
        assert!(cmd.env.is_some());
        let env = cmd.env.unwrap();
        assert_eq!(env.get("KEY1"), Some(&"value1".to_string()));
        assert_eq!(env.get("KEY2"), Some(&"value2".to_string()));
    }
}
