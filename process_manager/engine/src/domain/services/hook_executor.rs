//! Hook Executor Service
//! Handles execution of pre/post hooks for process lifecycle

use crate::domain::DomainError;
use std::process::Stdio;
use tokio::process::Command;
use tracing::{debug, warn};

/// Execute a list of hook commands sequentially
/// Returns error if any hook fails
pub async fn execute_hooks(
    hooks: &[String],
    hook_type: &str,
    process_name: &str,
) -> Result<(), DomainError> {
    if hooks.is_empty() {
        return Ok(());
    }

    debug!(
        process = %process_name,
        hook_type = %hook_type,
        count = hooks.len(),
        "Executing hooks"
    );

    for (index, hook_cmd) in hooks.iter().enumerate() {
        debug!(
            process = %process_name,
            hook_type = %hook_type,
            index = index + 1,
            command = %hook_cmd,
            "Executing hook"
        );

        // Parse command (simple split on whitespace for now)
        let parts: Vec<&str> = hook_cmd.split_whitespace().collect();
        if parts.is_empty() {
            warn!(
                process = %process_name,
                hook_type = %hook_type,
                "Empty hook command, skipping"
            );
            continue;
        }

        // Execute hook command
        let status = Command::new(parts[0])
            .args(&parts[1..])
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .await
            .map_err(|e| {
                DomainError::InvalidCommand(format!(
                    "Failed to execute {} hook '{}': {}",
                    hook_type, hook_cmd, e
                ))
            })?;

        if !status.success() {
            let exit_code = status.code().unwrap_or(-1);
            warn!(
                process = %process_name,
                hook_type = %hook_type,
                command = %hook_cmd,
                exit_code = exit_code,
                "Hook failed"
            );
            return Err(DomainError::InvalidCommand(format!(
                "{} hook failed with exit code {}: {}",
                hook_type, exit_code, hook_cmd
            )));
        }

        debug!(
            process = %process_name,
            hook_type = %hook_type,
            command = %hook_cmd,
            "Hook executed successfully"
        );
    }

    debug!(
        process = %process_name,
        hook_type = %hook_type,
        "All hooks executed successfully"
    );

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_execute_empty_hooks() {
        let result = execute_hooks(&[], "test", "my-process").await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    #[cfg(target_os = "linux")]
    async fn test_execute_successful_hook() {
        let hooks = vec!["/bin/true".to_string()];
        let result = execute_hooks(&hooks, "test", "my-process").await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    #[cfg(target_os = "linux")]
    async fn test_execute_failing_hook() {
        let hooks = vec!["/bin/false".to_string()];
        let result = execute_hooks(&hooks, "test", "my-process").await;
        assert!(result.is_err());
    }

    #[tokio::test]
    #[cfg(target_os = "linux")]
    async fn test_execute_multiple_hooks() {
        let hooks = vec!["/bin/true".to_string(), "/bin/echo test".to_string()];
        let result = execute_hooks(&hooks, "test", "my-process").await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    #[cfg(target_os = "linux")]
    async fn test_execute_hooks_stops_on_first_failure() {
        let hooks = vec![
            "/bin/true".to_string(),
            "/bin/false".to_string(),
            "/bin/true".to_string(), // Should not execute
        ];
        let result = execute_hooks(&hooks, "test", "my-process").await;
        assert!(result.is_err());
    }
}
