//! ProcessExecutor port
//! Interface for spawning and managing system processes

use crate::domain::{DomainError, KillMode, Process, ResourceLimits};
use async_trait::async_trait;
use std::future::Future;
use std::pin::Pin;

/// Configuration for spawning a process
#[derive(Debug, Clone)]
pub struct SpawnConfig {
    pub command: String,
    pub args: Vec<String>,
    pub working_dir: Option<String>,
    pub env_vars: Vec<(String, String)>,
    pub environment_file: Option<String>,
    pub pidfile: Option<String>,
    pub stdout: Option<String>, // File path, "inherit", or "null"
    pub stderr: Option<String>, // File path, "inherit", or "null"
    pub user: Option<String>,
    pub group: Option<String>,
    pub ambient_capabilities: Vec<String>,
    pub resource_limits: ResourceLimits,

    // Socket activation (systemd-compatible)
    /// File descriptors to pass to child process
    /// For systemd socket activation: FDs will be passed starting at FD 3
    /// LISTEN_FDS environment variable will be set automatically
    pub listen_fds: Vec<i32>,

    /// Custom environment variable names for each FD
    /// If empty, uses LISTEN_FDS (systemd-compatible)
    /// Example: vec!["DD_APM_NET_RECEIVER_FD", "DD_APM_UNIX_RECEIVER_FD"]
    /// Each entry corresponds to the FD at the same index in listen_fds
    pub fd_env_var_names: Vec<String>,
}

/// Handle for monitoring process exit
/// This allows event-driven monitoring without polling
pub type ProcessExitHandle = Pin<Box<dyn Future<Output = Result<i32, DomainError>> + Send>>;

/// Result of spawning a process
pub struct SpawnResult {
    pub pid: u32,
    /// Optional handle to wait for process exit
    /// None means the process cannot be monitored (e.g., forking/daemon processes)
    pub exit_handle: Option<ProcessExitHandle>,
}

impl std::fmt::Debug for SpawnResult {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("SpawnResult")
            .field("pid", &self.pid)
            .field("exit_handle", &self.exit_handle.is_some())
            .finish()
    }
}

/// Port for executing system processes
#[async_trait]
pub trait ProcessExecutor: Send + Sync {
    /// Spawn a new process
    async fn spawn(&self, config: SpawnConfig) -> Result<SpawnResult, DomainError>;

    /// Kill a running process
    async fn kill(&self, pid: u32, signal: i32) -> Result<(), DomainError>;

    /// Kill a process using a specific kill mode
    /// - Process: Kill only the main PID
    /// - ProcessGroup: Kill the entire process group
    /// - ControlGroup: Kill all processes in the cgroup (fallback to process group if cgroups unavailable)
    /// - Mixed: Send SIGTERM to main process, SIGKILL to process group
    async fn kill_with_mode(
        &self,
        pid: u32,
        signal: i32,
        mode: KillMode,
    ) -> Result<(), DomainError>;

    /// Check if a process is still running
    async fn is_running(&self, pid: u32) -> Result<bool, DomainError>;

    /// Wait for a process to exit and get its exit code
    async fn wait_for_exit(&self, pid: u32) -> Result<i32, DomainError>;
}

impl SpawnConfig {
    pub fn from_process(process: &Process) -> Self {
        // Use command and args directly from Process entity
        let command = process.command().to_string();
        let args = process.args().to_vec();

        // Convert env HashMap to vec of tuples
        let env_vars: Vec<(String, String)> = process
            .env()
            .iter()
            .map(|(k, v)| (k.clone(), v.clone()))
            .collect();

        Self {
            command,
            args,
            working_dir: process.working_dir().map(|s| s.to_string()),
            env_vars,
            environment_file: process.environment_file().map(|s| s.to_string()),
            pidfile: process.pidfile().map(|s| s.to_string()),
            stdout: process.stdout().map(|s| s.to_string()),
            stderr: process.stderr().map(|s| s.to_string()),
            user: process.user().map(|s| s.to_string()),
            group: process.group().map(|s| s.to_string()),
            ambient_capabilities: process.ambient_capabilities().to_vec(),
            resource_limits: process.resource_limits().clone(),
            listen_fds: Vec::new(), // No socket activation by default
            fd_env_var_names: Vec::new(), // Use LISTEN_FDS by default
        }
    }

    /// Add a socket FD with a custom environment variable name
    pub fn add_socket_fd(&mut self, fd: i32, env_var_name: Option<String>) {
        self.listen_fds.push(fd);
        self.fd_env_var_names
            .push(env_var_name.unwrap_or_default());
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_spawn_config_from_process() {
        let process = Process::builder("test".to_string(), "/bin/echo".to_string())
            .args(vec!["hello".to_string(), "world".to_string()])
            .build()
            .unwrap();
        let config = SpawnConfig::from_process(&process);

        assert_eq!(config.command, "/bin/echo");
        assert_eq!(config.args, vec!["hello".to_string(), "world".to_string()]);
        assert_eq!(config.working_dir, None);
        assert!(config.env_vars.is_empty());
        assert_eq!(config.pidfile, None);
        assert_eq!(config.stdout, None);
        assert_eq!(config.stderr, None);
        assert_eq!(config.user, None);
        assert_eq!(config.group, None);
    }

    #[test]
    fn test_spawn_config_single_command() {
        let process = Process::builder("test".to_string(), "/bin/sleep".to_string())
            .build()
            .unwrap();
        let config = SpawnConfig::from_process(&process);

        assert_eq!(config.command, "/bin/sleep");
        assert!(config.args.is_empty());
    }

    #[test]
    fn test_spawn_config_with_env_and_working_dir() {
        let mut process = Process::builder("test".to_string(), "/bin/app".to_string())
            .build()
            .unwrap();
        process.set_working_dir(Some("/tmp".to_string()));
        process.add_env_var("FOO".to_string(), "bar".to_string());
        process.add_env_var("BAZ".to_string(), "qux".to_string());

        let config = SpawnConfig::from_process(&process);

        assert_eq!(config.command, "/bin/app");
        assert_eq!(config.working_dir, Some("/tmp".to_string()));
        assert_eq!(config.env_vars.len(), 2);
        assert!(config
            .env_vars
            .contains(&("FOO".to_string(), "bar".to_string())));
        assert!(config
            .env_vars
            .contains(&("BAZ".to_string(), "qux".to_string())));
    }
}
