//! Process entity
//! Core domain entity representing a managed process

use crate::domain::{
    constants::*, DomainError, HealthCheck, HealthStatus, KillMode, PathCondition, ProcessId,
    ProcessState, ProcessType, ResourceLimits, RestartPolicy, SocketConfig,
};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::time::SystemTime;

/// Process entity - the core domain aggregate
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Process {
    // Identity
    id: ProcessId,
    name: String,
    description: Option<String>,

    // Configuration
    command: String,
    args: Vec<String>,
    process_type: ProcessType,
    restart_policy: RestartPolicy,

    // Restart timing configuration
    restart_sec: u64,           // Delay before restart (seconds)
    restart_max_delay_sec: u64, // Maximum delay for exponential backoff (seconds)
    consecutive_failures: u32,  // Count of consecutive failed restarts

    // Start limit protection (prevent restart thrashing)
    start_limit_burst: u32,        // Max starts within interval
    start_limit_interval_sec: u64, // Time window for start limiting (seconds)
    start_times: Vec<SystemTime>,  // Timestamps of recent starts

    // Process lifecycle tracking
    run_count: u32, // How many times process has been started

    // Dependencies (systemd-like)
    requires: Vec<String>,  // Hard dependencies - must be running
    binds_to: Vec<String>,  // If dependency stops, this stops too
    conflicts: Vec<String>, // Cannot run if these are running
    after: Vec<String>,     // Start after these (ordering hint)
    before: Vec<String>,    // Start before these (ordering hint)
    wants: Vec<String>,     // Soft dependencies - nice to have

    // Health check configuration
    health_check: Option<HealthCheck>,
    health_status: HealthStatus,
    health_check_failures: u32,
    last_health_check: Option<SystemTime>, // Timestamp of last health check

    // Environment and runtime context
    working_dir: Option<String>,
    env: std::collections::HashMap<String, String>,
    environment_file: Option<String>,
    pidfile: Option<String>,
    stdout: Option<String>, // File path, "inherit", or "null"
    stderr: Option<String>, // File path, "inherit", or "null"
    user: Option<String>,
    group: Option<String>,
    ambient_capabilities: Vec<String>, // Linux capabilities (e.g., CAP_NET_BIND_SERVICE)

    // Lifecycle and exit behavior
    success_exit_status: Vec<i32>, // Exit codes considered successful (default: [0])
    exec_start_pre: Vec<String>,   // Commands to run before starting main process
    exec_start_post: Vec<String>,  // Commands to run after main process starts
    exec_stop_post: Vec<String>,   // Commands to run after main process stops

    // Timeout configuration
    timeout_start_sec: u64, // Timeout for start operation (0 = no timeout)
    timeout_stop_sec: u64,  // Timeout for stop operation (default: 90s)

    // Kill configuration
    kill_signal: String, // Signal to send on stop (default: "SIGTERM")
    kill_mode: KillMode, // How to kill child processes

    // Resource limits
    resource_limits: ResourceLimits, // CPU, memory, and PID limits

    // Conditional starting
    condition_path_exists: Vec<PathCondition>, // Filesystem conditions for starting

    // Socket activation (systemd-compatible)
    socket_activation: Option<SocketConfig>, // Socket-based on-demand activation

    // Runtime directories (systemd RuntimeDirectory=)
    runtime_directory: Vec<String>, // Directories to create under /run/ (e.g., ["datadog"])

    // State
    state: ProcessState,
    pid: Option<u32>,
    exit_code: Option<i32>,
    signal: Option<String>, // Signal that killed the process (e.g., "SIGTERM (15)")

    // Timestamps
    created_at: SystemTime,
    started_at: Option<SystemTime>,
    stopped_at: Option<SystemTime>,
}

impl Process {
    /// Create a builder for constructing a Process with a fluent interface
    ///
    /// # Example
    /// ```
    /// use pm_engine::domain::Process;
    ///
    /// let process = Process::builder("my-service", "/usr/bin/app")
    ///     .args(vec!["--port".to_string(), "8080".to_string()])
    ///     .restart_policy(pm_engine::domain::RestartPolicy::Always)
    ///     .build()?;
    /// # Ok::<(), pm_engine::domain::DomainError>(())
    /// ```
    pub fn builder(name: impl Into<String>, command: impl Into<String>) -> ProcessBuilder {
        ProcessBuilder::new(name, command)
    }

    /// Create a new process definition (internal use only - use builder() instead)
    fn new(name: String, command: String) -> Result<Self, DomainError> {
        // Validate name
        if name.is_empty() {
            return Err(DomainError::InvalidName(
                "Process name cannot be empty".to_string(),
            ));
        }

        if name.contains(char::is_whitespace) {
            return Err(DomainError::InvalidName(format!(
                "Process name '{}' cannot contain whitespace",
                name
            )));
        }

        // Validate command
        if command.is_empty() {
            return Err(DomainError::InvalidCommand(
                "Process command cannot be empty".to_string(),
            ));
        }

        Ok(Self {
            id: ProcessId::generate(),
            name,
            description: None,
            command,
            args: Vec::new(),
            process_type: ProcessType::default(),
            restart_policy: RestartPolicy::default(),
            restart_sec: DEFAULT_RESTART_DELAY_SEC,
            restart_max_delay_sec: DEFAULT_RESTART_MAX_DELAY_SEC,
            consecutive_failures: 0, // No failures initially
            start_limit_burst: DEFAULT_START_LIMIT_BURST,
            start_limit_interval_sec: DEFAULT_START_LIMIT_INTERVAL_SEC,
            start_times: Vec::new(), // No start history
            run_count: 0,            // Not started yet
            requires: Vec::new(),
            binds_to: Vec::new(),
            conflicts: Vec::new(),
            after: Vec::new(),
            before: Vec::new(),
            wants: Vec::new(),
            health_check: None,
            health_status: HealthStatus::default(),
            health_check_failures: 0,
            last_health_check: None,
            working_dir: None,
            env: std::collections::HashMap::new(),
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            user: None,
            group: None,
            ambient_capabilities: Vec::new(),
            runtime_directory: Vec::new(), // No runtime directories by default
            success_exit_status: vec![SUCCESS_EXIT_CODE], // Default: only 0 is success
            exec_start_pre: Vec::new(),
            exec_start_post: Vec::new(),
            exec_stop_post: Vec::new(),
            timeout_start_sec: 0, // No start timeout by default
            timeout_stop_sec: DEFAULT_STOP_TIMEOUT_SEC,
            kill_signal: "SIGTERM".to_string(), // SIGTERM by default
            kill_mode: KillMode::default(),     // control-group by default
            resource_limits: ResourceLimits::default(),
            condition_path_exists: Vec::new(),
            socket_activation: None,
            state: ProcessState::default(), // Created
            pid: None,
            exit_code: None,
            signal: None, // No signal initially
            created_at: SystemTime::now(),
            started_at: None,
            stopped_at: None,
        })
    }

    // ===== Getters =====

    pub fn id(&self) -> ProcessId {
        self.id
    }

    pub fn name(&self) -> &str {
        &self.name
    }

    pub fn description(&self) -> Option<&str> {
        self.description.as_deref()
    }

    pub fn command(&self) -> &str {
        &self.command
    }

    pub fn args(&self) -> &[String] {
        &self.args
    }

    pub fn state(&self) -> ProcessState {
        self.state
    }

    pub fn pid(&self) -> Option<u32> {
        self.pid
    }

    pub fn exit_code(&self) -> Option<i32> {
        self.exit_code
    }

    pub fn signal(&self) -> Option<&str> {
        self.signal.as_deref()
    }

    pub fn set_signal(&mut self, signal: Option<String>) {
        self.signal = signal;
    }

    pub fn process_type(&self) -> ProcessType {
        self.process_type
    }

    pub fn restart_policy(&self) -> RestartPolicy {
        self.restart_policy
    }

    pub fn restart_sec(&self) -> u64 {
        self.restart_sec
    }

    pub fn restart_max_delay_sec(&self) -> u64 {
        self.restart_max_delay_sec
    }

    pub fn consecutive_failures(&self) -> u32 {
        self.consecutive_failures
    }

    pub fn start_limit_burst(&self) -> u32 {
        self.start_limit_burst
    }

    pub fn start_limit_interval_sec(&self) -> u64 {
        self.start_limit_interval_sec
    }

    pub fn start_times(&self) -> &[SystemTime] {
        &self.start_times
    }

    pub fn run_count(&self) -> u32 {
        self.run_count
    }

    pub fn requires(&self) -> &[String] {
        &self.requires
    }

    pub fn binds_to(&self) -> &[String] {
        &self.binds_to
    }

    pub fn conflicts(&self) -> &[String] {
        &self.conflicts
    }

    pub fn after(&self) -> &[String] {
        &self.after
    }

    pub fn before(&self) -> &[String] {
        &self.before
    }

    pub fn wants(&self) -> &[String] {
        &self.wants
    }

    pub fn health_check(&self) -> Option<&HealthCheck> {
        self.health_check.as_ref()
    }

    pub fn health_status(&self) -> HealthStatus {
        self.health_status
    }

    pub fn last_health_check(&self) -> Option<SystemTime> {
        self.last_health_check
    }

    pub fn health_check_failures(&self) -> u32 {
        self.health_check_failures
    }

    pub fn working_dir(&self) -> Option<&str> {
        self.working_dir.as_deref()
    }

    pub fn env(&self) -> &std::collections::HashMap<String, String> {
        &self.env
    }

    pub fn environment_file(&self) -> Option<&str> {
        self.environment_file.as_deref()
    }

    pub fn pidfile(&self) -> Option<&str> {
        self.pidfile.as_deref()
    }

    pub fn stdout(&self) -> Option<&str> {
        self.stdout.as_deref()
    }

    pub fn stderr(&self) -> Option<&str> {
        self.stderr.as_deref()
    }

    pub fn user(&self) -> Option<&str> {
        self.user.as_deref()
    }

    pub fn group(&self) -> Option<&str> {
        self.group.as_deref()
    }

    pub fn ambient_capabilities(&self) -> &[String] {
        &self.ambient_capabilities
    }

    pub fn runtime_directory(&self) -> &[String] {
        &self.runtime_directory
    }

    pub fn set_runtime_directory(&mut self, directories: Vec<String>) -> Result<(), DomainError> {
        // Validate that all paths are relative (systemd-compatible behavior)
        for dir in &directories {
            if dir.starts_with('/') {
                return Err(DomainError::InvalidCommand(format!(
                    "RuntimeDirectory must be relative paths, not absolute: {}",
                    dir
                )));
            }
        }
        self.runtime_directory = directories;
        Ok(())
    }

    pub fn success_exit_status(&self) -> &[i32] {
        &self.success_exit_status
    }

    pub fn exec_start_pre(&self) -> &[String] {
        &self.exec_start_pre
    }

    pub fn exec_start_post(&self) -> &[String] {
        &self.exec_start_post
    }

    pub fn exec_stop_post(&self) -> &[String] {
        &self.exec_stop_post
    }

    pub fn timeout_start_sec(&self) -> u64 {
        self.timeout_start_sec
    }

    pub fn timeout_stop_sec(&self) -> u64 {
        self.timeout_stop_sec
    }

    pub fn kill_signal(&self) -> &str {
        &self.kill_signal
    }

    pub fn kill_mode(&self) -> KillMode {
        self.kill_mode
    }

    pub fn resource_limits(&self) -> &ResourceLimits {
        &self.resource_limits
    }

    pub fn condition_path_exists(&self) -> &[PathCondition] {
        &self.condition_path_exists
    }

    pub fn socket_activation(&self) -> Option<&SocketConfig> {
        self.socket_activation.as_ref()
    }

    pub fn created_at(&self) -> SystemTime {
        self.created_at
    }

    pub fn started_at(&self) -> Option<SystemTime> {
        self.started_at
    }

    pub fn stopped_at(&self) -> Option<SystemTime> {
        self.stopped_at
    }

    // ===== Business Logic: State Transitions =====

    /// Mark the process as starting
    pub fn mark_starting(&mut self) -> Result<(), DomainError> {
        if !self.state.can_transition_to(ProcessState::Starting) {
            return Err(DomainError::InvalidStateTransition {
                from: self.state.to_string(),
                to: "starting".to_string(),
            });
        }

        self.state = ProcessState::Starting;
        self.started_at = Some(SystemTime::now());
        Ok(())
    }

    /// Mark the process as running with a PID
    pub fn mark_running(&mut self, pid: u32) -> Result<(), DomainError> {
        if !self.state.can_transition_to(ProcessState::Running) {
            return Err(DomainError::InvalidStateTransition {
                from: self.state.to_string(),
                to: "running".to_string(),
            });
        }

        self.state = ProcessState::Running;
        self.pid = Some(pid);
        Ok(())
    }

    /// Mark the process as stopping
    pub fn mark_stopping(&mut self) -> Result<(), DomainError> {
        if !self.state.can_transition_to(ProcessState::Stopping) {
            return Err(DomainError::InvalidStateTransition {
                from: self.state.to_string(),
                to: "stopping".to_string(),
            });
        }

        self.state = ProcessState::Stopping;
        Ok(())
    }

    /// Mark the process as stopped
    pub fn mark_stopped(&mut self) -> Result<(), DomainError> {
        if !self.state.can_transition_to(ProcessState::Stopped) {
            return Err(DomainError::InvalidStateTransition {
                from: self.state.to_string(),
                to: "stopped".to_string(),
            });
        }

        self.state = ProcessState::Stopped;
        self.pid = None;
        self.stopped_at = Some(SystemTime::now());
        Ok(())
    }

    /// Mark the process as exited with an exit code
    /// Uses success_exit_status to determine if exit was successful
    pub fn mark_exited(&mut self, exit_code: i32) -> Result<(), DomainError> {
        // Systemd-like behavior: if the process was explicitly stopped (Stopping or already Stopped),
        // don't change the state or evaluate exit code.
        // Only evaluate exit codes for spontaneous exits (from Running/Starting states).

        // If already stopped, just record the exit code and return
        if self.state == ProcessState::Stopped {
            self.exit_code = Some(exit_code);
            return Ok(());
        }

        let new_state = if self.state == ProcessState::Stopping {
            // Explicit stop in progress - always go to Stopped, never Failed
            ProcessState::Stopped
        } else {
            // Spontaneous exit - evaluate exit code
            let is_success = self.is_exit_code_success(exit_code);
            if is_success {
                ProcessState::Exited
            } else {
                ProcessState::Failed
            }
        };

        if !self.state.can_transition_to(new_state) {
            return Err(DomainError::InvalidStateTransition {
                from: self.state.to_string(),
                to: new_state.to_string(),
            });
        }

        self.state = new_state;
        self.exit_code = Some(exit_code);
        self.pid = None;
        self.stopped_at = Some(SystemTime::now());
        Ok(())
    }

    pub fn mark_restarting(&mut self) -> Result<(), DomainError> {
        if !self.state.can_transition_to(ProcessState::Restarting) {
            return Err(DomainError::InvalidStateTransition {
                from: self.state.to_string(),
                to: "restarting".to_string(),
            });
        }

        self.state = ProcessState::Restarting;
        Ok(())
    }

    // ===== Business Logic: Queries =====

    /// Check if the process is currently running
    pub fn is_running(&self) -> bool {
        self.state.is_running()
    }

    /// Check if the process can be started
    pub fn can_start(&self) -> bool {
        self.state.can_start()
    }

    /// Check if the process can be stopped
    pub fn can_stop(&self) -> bool {
        self.state.can_stop()
    }

    /// Check if the process should be restarted based on exit code
    pub fn should_restart(&self) -> bool {
        if let Some(code) = self.exit_code {
            self.restart_policy.should_restart(code)
        } else {
            false
        }
    }

    // ===== Configuration Methods =====

    pub fn set_restart_policy(&mut self, restart_policy: RestartPolicy) {
        self.restart_policy = restart_policy;
    }

    pub fn set_restart_sec(&mut self, restart_sec: u64) {
        self.restart_sec = restart_sec;
    }

    pub fn set_restart_max_delay_sec(&mut self, restart_max_delay_sec: u64) {
        self.restart_max_delay_sec = restart_max_delay_sec;
    }

    /// Increment run count (called when process starts)
    pub fn increment_run_count(&mut self) {
        self.run_count += 1;
    }

    /// Increment consecutive failures
    pub fn increment_failures(&mut self) {
        self.consecutive_failures += 1;
    }

    /// Reset consecutive failures (called on successful start)
    pub fn reset_failures(&mut self) {
        self.consecutive_failures = 0;
    }

    /// Record a start time for start limit checking
    pub fn record_start_time(&mut self) {
        self.start_times.push(SystemTime::now());
    }

    /// Check if start limit is exceeded
    /// Returns true if process has been started too many times within the interval
    pub fn is_start_limit_exceeded(&mut self) -> bool {
        let now = SystemTime::now();
        let cutoff = now - std::time::Duration::from_secs(self.start_limit_interval_sec);

        // Remove old start times outside the interval
        self.start_times.retain(|&time| time >= cutoff);

        // Check if we've exceeded the burst limit
        self.start_times.len() >= self.start_limit_burst as usize
    }

    /// Calculate restart delay with exponential backoff
    /// Returns delay in seconds
    pub fn calculate_restart_delay(&self) -> u64 {
        if self.consecutive_failures == 0 {
            return self.restart_sec;
        }

        // Exponential backoff: base_delay * 2^failures
        let exponential_delay =
            self.restart_sec * (RESTART_BACKOFF_BASE as u64).pow(self.consecutive_failures - 1);

        // Cap at restart_max_delay_sec
        exponential_delay.min(self.restart_max_delay_sec)
    }

    pub fn update_command(&mut self, command: String) -> Result<(), DomainError> {
        if command.is_empty() {
            return Err(DomainError::InvalidCommand(
                "Process command cannot be empty".to_string(),
            ));
        }
        self.command = command;
        Ok(())
    }

    // ===== Dependency Configuration =====

    // ===== Health Check Configuration =====

    pub fn set_health_check(&mut self, health_check: HealthCheck) {
        self.health_check = Some(health_check);
    }

    pub fn clear_health_check(&mut self) {
        self.health_check = None;
        self.health_status = HealthStatus::Unknown;
        self.health_check_failures = 0;
        self.last_health_check = None;
    }

    pub fn update_health_status(&mut self, status: HealthStatus) {
        self.health_status = status;
        self.last_health_check = Some(SystemTime::now()); // Record timestamp of health check

        // Reset failure count on success
        if status == HealthStatus::Healthy {
            self.health_check_failures = 0;
        }
    }

    pub fn increment_health_check_failures(&mut self) {
        self.health_check_failures += 1;
    }

    pub fn reset_health_check_failures(&mut self) {
        self.health_check_failures = 0;
    }

    // ===== Environment and Runtime Configuration =====

    pub fn set_working_dir(&mut self, working_dir: Option<String>) {
        self.working_dir = working_dir;
    }

    pub fn set_env(&mut self, env: std::collections::HashMap<String, String>) {
        self.env = env;
    }

    pub fn add_env_var(&mut self, key: String, value: String) {
        self.env.insert(key, value);
    }

    pub fn remove_env_var(&mut self, key: &str) {
        self.env.remove(key);
    }

    pub fn set_environment_file(&mut self, environment_file: Option<String>) {
        self.environment_file = environment_file;
    }

    pub fn set_pidfile(&mut self, pidfile: Option<String>) {
        self.pidfile = pidfile;
    }

    pub fn set_user(&mut self, user: Option<String>) {
        self.user = user;
    }

    pub fn set_group(&mut self, group: Option<String>) {
        self.group = group;
    }

    pub fn set_ambient_capabilities(&mut self, capabilities: Vec<String>) {
        self.ambient_capabilities = capabilities;
    }

    pub fn set_success_exit_status(&mut self, success_exit_status: Vec<i32>) {
        self.success_exit_status = success_exit_status;
    }

    pub fn set_timeout_stop_sec(&mut self, timeout_stop_sec: u64) {
        self.timeout_stop_sec = timeout_stop_sec;
    }

    pub fn set_kill_signal(&mut self, kill_signal: String) {
        self.kill_signal = kill_signal;
    }

    pub fn set_kill_mode(&mut self, kill_mode: KillMode) {
        self.kill_mode = kill_mode;
    }

    pub fn set_resource_limits(&mut self, resource_limits: ResourceLimits) {
        self.resource_limits = resource_limits;
    }

    /// Check if a given exit code should be considered successful for this process
    pub fn is_exit_code_success(&self, exit_code: i32) -> bool {
        self.success_exit_status.contains(&exit_code)
    }

    /// Clone this process with a new name and ID
    ///
    /// Creates a new process instance with the same configuration but fresh state.
    /// This is used for socket activation with Accept=yes (spawn instance per connection)
    /// and for creating multiple instances from a template.
    ///
    /// # Business Rules
    /// - Cannot clone a running process (must be stopped first)
    /// - New process gets a fresh ID and state (Stopped, no PID, no run history)
    /// - Configuration is copied (command, restart policy, limits, etc.)
    /// - Dependencies are NOT copied (they represent relationships, not config)
    /// - PID file, stdout/stderr are NOT copied (each instance needs its own)
    ///
    /// # Arguments
    /// * `name` - Unique name for the new instance
    ///
    /// # Returns
    /// * `Ok(Process)` - New process instance with copied configuration
    /// * `Err(DomainError)` - If process is running or name is invalid
    pub fn clone_with_name(&self, name: String) -> Result<Process, DomainError> {
        // Business rule: can't clone a running process
        if self.state == ProcessState::Running || self.state == ProcessState::Starting {
            return Err(DomainError::InvalidStateTransition {
                from: self.state.to_string(),
                to: "cloned".to_string(),
            });
        }

        // Create new process with fresh identity
        let mut cloned = Process::builder(name, self.command.clone()).build()?;

        // Copy command and arguments
        cloned.args = self.args.clone();

        // Copy process type and restart configuration
        cloned.process_type = self.process_type;
        cloned.restart_policy = self.restart_policy;
        cloned.restart_sec = self.restart_sec;
        cloned.restart_max_delay_sec = self.restart_max_delay_sec;
        cloned.start_limit_burst = self.start_limit_burst;
        cloned.start_limit_interval_sec = self.start_limit_interval_sec;

        // Copy execution environment
        cloned.working_dir = self.working_dir.clone();
        cloned.env = self.env.clone();
        cloned.environment_file = self.environment_file.clone();
        cloned.user = self.user.clone();
        cloned.group = self.group.clone();

        // Copy lifecycle hooks
        cloned.exec_start_pre = self.exec_start_pre.clone();
        cloned.exec_start_post = self.exec_start_post.clone();
        cloned.exec_stop_post = self.exec_stop_post.clone();

        // Copy timeouts and kill configuration
        cloned.timeout_start_sec = self.timeout_start_sec;
        cloned.timeout_stop_sec = self.timeout_stop_sec;
        cloned.kill_mode = self.kill_mode;
        cloned.kill_signal = self.kill_signal.clone();

        // Copy exit status configuration
        cloned.success_exit_status = self.success_exit_status.clone();

        // Copy health check configuration
        cloned.health_check = self.health_check.clone();

        // Copy resource limits
        cloned.resource_limits = self.resource_limits.clone();

        // Copy conditional execution
        cloned.condition_path_exists = self.condition_path_exists.clone();

        // Copy runtime directories
        cloned.runtime_directory = self.runtime_directory.clone();

        // Copy ambient capabilities
        cloned.ambient_capabilities = self.ambient_capabilities.clone();

        // DON'T copy:
        // - Dependencies (requires, binds_to, conflicts, after, before, wants)
        //   These represent relationships between processes, not configuration
        // - pid_file: Each instance should have its own PID file
        // - stdout/stderr: Each instance should have its own log files
        // - socket: Socket activation is per-template, not per-instance
        // - State fields: pid, exit_code, signal, run_count, consecutive_failures,
        //   start_times, created_at, started_at, ended_at
        // - Health status: health_status, health_check_failures, last_health_check

        Ok(cloned)
    }
}

// ============================================================================
// ProcessBuilder - Fluent interface for creating Process entities
// ============================================================================

/// Builder for creating Process entities with a fluent interface
///
/// # Example
/// ```
/// use pm_engine::domain::Process;
///
/// # fn main() -> Result<(), pm_engine::domain::DomainError> {
/// let process = Process::builder("my-service", "/usr/bin/app")
///     .args(vec!["--port".to_string(), "8080".to_string()])
///     .restart_policy(pm_engine::domain::RestartPolicy::Always)
///     .restart_delay_sec(5)
///     .build()?;
/// # Ok(())
/// # }
/// ```
pub struct ProcessBuilder {
    // Mandatory fields (set in constructor)
    name: String,
    command: String,

    // Optional fields
    description: Option<String>,
    args: Vec<String>,
    restart: Option<RestartPolicy>,
    restart_sec: Option<u64>,
    restart_max_delay_sec: Option<u64>,
    start_limit_burst: Option<u32>,
    start_limit_interval_sec: Option<u64>,
    working_dir: Option<String>,
    env: HashMap<String, String>,
    environment_file: Option<String>,
    pidfile: Option<String>,
    stdout: Option<String>,
    stderr: Option<String>,
    timeout_start_sec: Option<u64>,
    timeout_stop_sec: Option<u64>,
    kill_signal: Option<i32>,
    kill_mode: Option<KillMode>,
    success_exit_status: Vec<i32>,
    exec_start_pre: Vec<String>,
    exec_start_post: Vec<String>,
    exec_stop_post: Vec<String>,
    user: Option<String>,
    group: Option<String>,
    after: Vec<String>,
    before: Vec<String>,
    requires: Vec<String>,
    wants: Vec<String>,
    binds_to: Vec<String>,
    conflicts: Vec<String>,
    process_type: Option<ProcessType>,
    health_check: Option<HealthCheck>,
    resource_limits: Option<ResourceLimits>,
    condition_path_exists: Vec<PathCondition>,
    runtime_directory: Vec<String>,
    ambient_capabilities: Vec<String>,
    socket: Option<SocketConfig>,
}

impl ProcessBuilder {
    /// Create a new builder with mandatory fields
    ///
    /// # Arguments
    /// * `name` - Process name (must be non-empty, no whitespace)
    /// * `command` - Command to execute (must be non-empty)
    pub fn new(name: impl Into<String>, command: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            command: command.into(),
            description: None,
            args: Vec::new(),
            restart: None,
            restart_sec: None,
            restart_max_delay_sec: None,
            start_limit_burst: None,
            start_limit_interval_sec: None,
            working_dir: None,
            env: HashMap::new(),
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            timeout_start_sec: None,
            timeout_stop_sec: None,
            kill_signal: None,
            kill_mode: None,
            success_exit_status: Vec::new(),
            exec_start_pre: Vec::new(),
            exec_start_post: Vec::new(),
            exec_stop_post: Vec::new(),
            user: None,
            group: None,
            after: Vec::new(),
            before: Vec::new(),
            requires: Vec::new(),
            wants: Vec::new(),
            binds_to: Vec::new(),
            conflicts: Vec::new(),
            process_type: None,
            health_check: None,
            resource_limits: None,
            condition_path_exists: Vec::new(),
            runtime_directory: Vec::new(),
            ambient_capabilities: Vec::new(),
            socket: None,
        }
    }

    /// Set description
    pub fn description(mut self, description: impl Into<String>) -> Self {
        self.description = Some(description.into());
        self
    }

    /// Set command arguments
    pub fn args(mut self, args: Vec<String>) -> Self {
        self.args = args;
        self
    }

    /// Add a single argument
    pub fn arg(mut self, arg: impl Into<String>) -> Self {
        self.args.push(arg.into());
        self
    }

    /// Set restart policy
    pub fn restart_policy(mut self, policy: RestartPolicy) -> Self {
        self.restart = Some(policy);
        self
    }

    /// Set restart delay in seconds
    pub fn restart_delay_sec(mut self, seconds: u64) -> Self {
        self.restart_sec = Some(seconds);
        self
    }

    /// Set maximum restart delay in seconds (for exponential backoff)
    pub fn restart_max_delay_sec(mut self, seconds: u64) -> Self {
        self.restart_max_delay_sec = Some(seconds);
        self
    }

    /// Set start limit burst (number of restarts allowed in interval)
    pub fn start_limit_burst(mut self, burst: u32) -> Self {
        self.start_limit_burst = Some(burst);
        self
    }

    /// Set start limit interval in seconds
    pub fn start_limit_interval_sec(mut self, seconds: u64) -> Self {
        self.start_limit_interval_sec = Some(seconds);
        self
    }

    /// Set working directory
    pub fn working_dir(mut self, dir: impl Into<String>) -> Self {
        self.working_dir = Some(dir.into());
        self
    }

    /// Set environment variables
    pub fn env(mut self, env: HashMap<String, String>) -> Self {
        self.env = env;
        self
    }

    /// Add a single environment variable
    pub fn env_var(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.env.insert(key.into(), value.into());
        self
    }

    /// Set environment file path
    pub fn environment_file(mut self, path: impl Into<String>) -> Self {
        self.environment_file = Some(path.into());
        self
    }

    /// Set PID file path
    pub fn pidfile(mut self, path: impl Into<String>) -> Self {
        self.pidfile = Some(path.into());
        self
    }

    /// Set stdout redirection path
    pub fn stdout(mut self, path: impl Into<String>) -> Self {
        self.stdout = Some(path.into());
        self
    }

    /// Set stderr redirection path
    pub fn stderr(mut self, path: impl Into<String>) -> Self {
        self.stderr = Some(path.into());
        self
    }

    /// Set start timeout in seconds
    pub fn timeout_start_sec(mut self, seconds: u64) -> Self {
        self.timeout_start_sec = Some(seconds);
        self
    }

    /// Set stop timeout in seconds
    pub fn timeout_stop_sec(mut self, seconds: u64) -> Self {
        self.timeout_stop_sec = Some(seconds);
        self
    }

    /// Set kill signal
    pub fn kill_signal(mut self, signal: i32) -> Self {
        self.kill_signal = Some(signal);
        self
    }

    /// Set kill mode
    pub fn kill_mode(mut self, mode: KillMode) -> Self {
        self.kill_mode = Some(mode);
        self
    }

    /// Set success exit status codes
    pub fn success_exit_status(mut self, codes: Vec<i32>) -> Self {
        self.success_exit_status = codes;
        self
    }

    /// Set pre-start hooks
    pub fn exec_start_pre(mut self, hooks: Vec<String>) -> Self {
        self.exec_start_pre = hooks;
        self
    }

    /// Set post-start hooks
    pub fn exec_start_post(mut self, hooks: Vec<String>) -> Self {
        self.exec_start_post = hooks;
        self
    }

    /// Set post-stop hooks
    pub fn exec_stop_post(mut self, hooks: Vec<String>) -> Self {
        self.exec_stop_post = hooks;
        self
    }

    /// Set user
    pub fn user(mut self, user: impl Into<String>) -> Self {
        self.user = Some(user.into());
        self
    }

    /// Set group
    pub fn group(mut self, group: impl Into<String>) -> Self {
        self.group = Some(group.into());
        self
    }

    /// Set "after" dependencies
    pub fn after(mut self, deps: Vec<String>) -> Self {
        self.after = deps;
        self
    }

    /// Set "before" dependencies
    pub fn before(mut self, deps: Vec<String>) -> Self {
        self.before = deps;
        self
    }

    /// Set "requires" dependencies
    pub fn requires(mut self, deps: Vec<String>) -> Self {
        self.requires = deps;
        self
    }

    /// Set "wants" dependencies
    pub fn wants(mut self, deps: Vec<String>) -> Self {
        self.wants = deps;
        self
    }

    /// Set "binds_to" dependencies
    pub fn binds_to(mut self, deps: Vec<String>) -> Self {
        self.binds_to = deps;
        self
    }

    /// Set "conflicts" dependencies
    pub fn conflicts(mut self, deps: Vec<String>) -> Self {
        self.conflicts = deps;
        self
    }

    /// Set process type
    pub fn process_type(mut self, ptype: ProcessType) -> Self {
        self.process_type = Some(ptype);
        self
    }

    /// Set health check
    pub fn health_check(mut self, check: HealthCheck) -> Self {
        self.health_check = Some(check);
        self
    }

    /// Set resource limits
    pub fn resource_limits(mut self, limits: ResourceLimits) -> Self {
        self.resource_limits = Some(limits);
        self
    }

    /// Set path existence conditions
    pub fn condition_path_exists(mut self, conditions: Vec<PathCondition>) -> Self {
        self.condition_path_exists = conditions;
        self
    }

    /// Set runtime directories
    pub fn runtime_directory(mut self, dirs: Vec<String>) -> Self {
        self.runtime_directory = dirs;
        self
    }

    /// Set ambient capabilities
    pub fn ambient_capabilities(mut self, caps: Vec<String>) -> Self {
        self.ambient_capabilities = caps;
        self
    }

    /// Set socket configuration
    pub fn socket(mut self, config: SocketConfig) -> Self {
        self.socket = Some(config);
        self
    }

    /// Build the Process entity with validation
    ///
    /// This validates the mandatory fields and constructs the Process entity.
    /// All optional fields are applied using the entity's setter methods.
    pub fn build(self) -> Result<Process, DomainError> {
        // Create process with validated mandatory fields
        let mut process = Process::new(self.name, self.command)?;

        // Apply optional fields
        if let Some(description) = self.description {
            process.description = Some(description);
        }
        if !self.args.is_empty() {
            process.args = self.args;
        }
        if let Some(restart) = self.restart {
            process.restart_policy = restart;
        }
        if let Some(restart_sec) = self.restart_sec {
            process.restart_sec = restart_sec;
        }
        if let Some(restart_max_delay_sec) = self.restart_max_delay_sec {
            process.restart_max_delay_sec = restart_max_delay_sec;
        }
        if let Some(start_limit_burst) = self.start_limit_burst {
            process.start_limit_burst = start_limit_burst;
        }
        if let Some(start_limit_interval_sec) = self.start_limit_interval_sec {
            process.start_limit_interval_sec = start_limit_interval_sec;
        }
        if let Some(working_dir) = self.working_dir {
            process.working_dir = Some(working_dir);
        }
        if !self.env.is_empty() {
            process.env = self.env;
        }
        if let Some(environment_file) = self.environment_file {
            process.environment_file = Some(environment_file);
        }
        if let Some(pidfile) = self.pidfile {
            process.pidfile = Some(pidfile);
        }
        if let Some(stdout) = self.stdout {
            process.stdout = Some(stdout);
        }
        if let Some(stderr) = self.stderr {
            process.stderr = Some(stderr);
        }
        if let Some(timeout_start_sec) = self.timeout_start_sec {
            process.timeout_start_sec = timeout_start_sec;
        }
        if let Some(timeout_stop_sec) = self.timeout_stop_sec {
            process.timeout_stop_sec = timeout_stop_sec;
        }
        if let Some(kill_signal) = self.kill_signal {
            process.kill_signal = kill_signal.to_string();
        }
        if let Some(kill_mode) = self.kill_mode {
            process.kill_mode = kill_mode;
        }
        if !self.success_exit_status.is_empty() {
            process.success_exit_status = self.success_exit_status;
        }
        if !self.exec_start_pre.is_empty() {
            process.exec_start_pre = self.exec_start_pre;
        }
        if !self.exec_start_post.is_empty() {
            process.exec_start_post = self.exec_start_post;
        }
        if !self.exec_stop_post.is_empty() {
            process.exec_stop_post = self.exec_stop_post;
        }
        if let Some(user) = self.user {
            process.user = Some(user);
        }
        if let Some(group) = self.group {
            process.group = Some(group);
        }
        if !self.after.is_empty() {
            process.after = self.after;
        }
        if !self.before.is_empty() {
            process.before = self.before;
        }
        if !self.requires.is_empty() {
            process.requires = self.requires;
        }
        if !self.wants.is_empty() {
            process.wants = self.wants;
        }
        if !self.binds_to.is_empty() {
            process.binds_to = self.binds_to;
        }
        if !self.conflicts.is_empty() {
            process.conflicts = self.conflicts;
        }
        if let Some(process_type) = self.process_type {
            process.process_type = process_type;
        }
        if let Some(health_check) = self.health_check {
            process.health_check = Some(health_check);
        }
        if let Some(resource_limits) = self.resource_limits {
            process.resource_limits = resource_limits;
        }
        if !self.condition_path_exists.is_empty() {
            process.condition_path_exists = self.condition_path_exists;
        }
        if !self.runtime_directory.is_empty() {
            process.set_runtime_directory(self.runtime_directory)?;
        }
        if !self.ambient_capabilities.is_empty() {
            process.ambient_capabilities = self.ambient_capabilities;
        }
        if let Some(socket) = self.socket {
            process.socket_activation = Some(socket);
        }

        Ok(process)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_builder_minimal() {
        let process = ProcessBuilder::new("test", "/bin/echo").build().unwrap();
        assert_eq!(process.name(), "test");
        assert_eq!(process.command(), "/bin/echo");
    }

    #[test]
    fn test_builder_with_args() {
        let process = ProcessBuilder::new("test", "/bin/echo")
            .args(vec!["hello".to_string(), "world".to_string()])
            .build()
            .unwrap();
        assert_eq!(process.args(), &["hello", "world"]);
    }

    #[test]
    fn test_builder_with_restart_policy() {
        let process = ProcessBuilder::new("test", "/bin/sleep")
            .restart_policy(RestartPolicy::Always)
            .restart_delay_sec(5)
            .restart_max_delay_sec(60)
            .build()
            .unwrap();
        assert_eq!(process.restart_policy(), RestartPolicy::Always);
        assert_eq!(process.restart_sec(), 5);
        assert_eq!(process.restart_max_delay_sec(), 60);
    }

    #[test]
    fn test_builder_fluent_interface() {
        let process = ProcessBuilder::new("web-server", "/usr/bin/nginx")
            .arg("-g")
            .arg("daemon off;")
            .working_dir("/var/www")
            .env_var("PORT", "8080")
            .env_var("ENV", "production")
            .user("www-data")
            .group("www-data")
            .restart_policy(RestartPolicy::Always)
            .build()
            .unwrap();

        assert_eq!(process.name(), "web-server");
        assert_eq!(process.args(), &["-g", "daemon off;"]);
        assert_eq!(process.working_dir(), Some("/var/www"));
        assert_eq!(process.user(), Some("www-data"));
    }

    #[test]
    fn test_builder_with_description() {
        let process = ProcessBuilder::new("my-app", "/bin/app")
            .description("My Application Service")
            .build()
            .unwrap();

        assert_eq!(process.name(), "my-app");
        assert_eq!(process.description(), Some("My Application Service"));
    }

    #[test]
    fn test_builder_validates_name() {
        let result = ProcessBuilder::new("", "/bin/echo").build();
        assert!(matches!(result, Err(DomainError::InvalidName(_))));

        let result = ProcessBuilder::new("my service", "/bin/echo").build();
        assert!(matches!(result, Err(DomainError::InvalidName(_))));
    }

    #[test]
    fn test_builder_validates_command() {
        let result = ProcessBuilder::new("test", "").build();
        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
    }
}
