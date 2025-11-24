//! Core types for the process manager

use serde::ser::SerializeStruct;
use std::collections::HashMap;
use std::fmt;

/// Represents the current state of a process
#[derive(Clone, Debug, PartialEq)]
pub enum ProcessState {
    Unknown,
    Created,
    Starting,
    Running,
    Stopping,
    Stopped,
    Crashed,
    Exited,
    Zombie,
}

impl fmt::Display for ProcessState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let s = match self {
            Self::Unknown => "unknown",
            Self::Created => "created",
            Self::Starting => "starting",
            Self::Running => "running",
            Self::Stopping => "stopping",
            Self::Stopped => "stopped",
            Self::Crashed => "crashed",
            Self::Exited => "exited",
            Self::Zombie => "zombie",
        };
        write!(f, "{}", s)
    }
}

impl ProcessState {
    /// Returns true if the process can be started from this state
    pub fn can_start(&self) -> bool {
        matches!(
            self,
            Self::Created | Self::Stopped | Self::Crashed | Self::Exited
        )
    }

    /// Returns true if the process can be stopped from this state
    pub fn can_stop(&self) -> bool {
        matches!(self, Self::Running | Self::Starting)
    }
}

impl serde::Serialize for ProcessState {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        serializer.serialize_str(&self.to_string())
    }
}

/// Restart policy for processes (systemd-style)
#[derive(Clone, Debug, PartialEq)]
pub enum RestartPolicy {
    Never,
    Always,
    OnFailure,
    OnSuccess,
}

impl std::str::FromStr for RestartPolicy {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "never" => Ok(Self::Never),
            "always" => Ok(Self::Always),
            "on-failure" | "onfailure" => Ok(Self::OnFailure),
            "on-success" | "onsuccess" => Ok(Self::OnSuccess),
            _ => Err(format!("Invalid restart policy: {}", s)),
        }
    }
}

impl RestartPolicy {
    /// Parse a restart policy from a string (deprecated, use FromStr)
    pub fn from_str_opt(s: &str) -> Option<Self> {
        s.parse().ok()
    }

    /// Check if process should restart based on exit status
    pub fn should_restart(&self, success: bool) -> bool {
        match self {
            Self::Never => false,
            Self::Always => true,
            Self::OnFailure => !success,
            Self::OnSuccess => success,
        }
    }
}

/// Kill mode for stopping processes (systemd-style)
#[derive(Clone, Debug, PartialEq, Default)]
pub enum KillMode {
    /// Kill all processes in cgroup (or process-group as fallback) - Default
    #[default]
    ControlGroup,
    /// Kill entire process group
    ProcessGroup,
    /// Kill only the main process
    Process,
    /// Mixed: SIGTERM to main process, SIGKILL to rest of group
    Mixed,
}

impl std::str::FromStr for KillMode {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "control-group" | "controlgroup" | "cgroup" => Ok(Self::ControlGroup),
            "process-group" | "processgroup" => Ok(Self::ProcessGroup),
            "process" => Ok(Self::Process),
            "mixed" => Ok(Self::Mixed),
            _ => Err(format!(
                "Invalid kill mode: {}. Valid options: control-group, process-group, process, mixed",
                s
            )),
        }
    }
}

impl fmt::Display for KillMode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let s = match self {
            Self::ControlGroup => "control-group",
            Self::ProcessGroup => "process-group",
            Self::Process => "process",
            Self::Mixed => "mixed",
        };
        write!(f, "{}", s)
    }
}

/// Path condition type for pre-start validation (systemd-like)
#[derive(Clone, Debug, PartialEq)]
pub enum PathCondition {
    /// Path must exist (AND logic - all must be true)
    Exists(String),
    /// Path must NOT exist (negation)
    NotExists(String),
    /// Path must exist (OR logic - at least one must be true)
    ExistsOr(String),
}

impl PathCondition {
    /// Parse a path condition from a string with optional prefix
    /// - No prefix: Exists (AND logic)
    /// - "!" prefix: NotExists
    /// - "|" prefix: ExistsOr (OR logic)
    pub fn parse(s: &str) -> Self {
        if let Some(path) = s.strip_prefix('!') {
            Self::NotExists(path.to_string())
        } else if let Some(path) = s.strip_prefix('|') {
            Self::ExistsOr(path.to_string())
        } else {
            Self::Exists(s.to_string())
        }
    }

    /// Get the path for this condition
    pub fn path(&self) -> &str {
        match self {
            Self::Exists(p) | Self::NotExists(p) | Self::ExistsOr(p) => p,
        }
    }

    /// Check if this condition is satisfied
    pub fn check(&self) -> bool {
        let exists = std::path::Path::new(self.path()).exists();
        match self {
            Self::Exists(_) => exists,
            Self::NotExists(_) => !exists,
            Self::ExistsOr(_) => exists,
        }
    }
}

impl fmt::Display for PathCondition {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Exists(p) => write!(f, "{}", p),
            Self::NotExists(p) => write!(f, "!{}", p),
            Self::ExistsOr(p) => write!(f, "|{}", p),
        }
    }
}

impl RestartPolicy {
    pub fn to_string(&self) -> &'static str {
        match self {
            Self::Never => "never",
            Self::Always => "always",
            Self::OnFailure => "on-failure",
            Self::OnSuccess => "on-success",
        }
    }
}

/// Summary information about a process (used for list operations)
#[derive(Clone)]
pub struct ProcessInfo {
    pub id: String,
    pub name: String,
    pub pid: Option<u32>,
    pub command: String,
    pub state: ProcessState,
    pub run_count: u32,
    pub exit_code: Option<i32>,
    pub signal: Option<String>,
    pub created_at: Option<i64>,
    pub started_at: Option<i64>,
    pub ended_at: Option<i64>,
}

impl serde::Serialize for ProcessInfo {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let mut state = serializer.serialize_struct("ProcessInfo", 11)?;
        state.serialize_field("id", &self.id)?;
        state.serialize_field("name", &self.name)?;
        state.serialize_field("pid", &self.pid)?;
        state.serialize_field("command", &self.command)?;
        state.serialize_field("state", &self.state)?;
        state.serialize_field("run_count", &self.run_count)?;
        state.serialize_field("exit_code", &self.exit_code)?;
        state.serialize_field("signal", &self.signal)?;
        state.serialize_field("created_at", &self.created_at)?;
        state.serialize_field("started_at", &self.started_at)?;
        state.serialize_field("ended_at", &self.ended_at)?;
        state.end()
    }
}

/// Detailed information about a process (used for describe operations)
pub struct ProcessDetail {
    pub id: String,
    pub name: String,
    pub pid: Option<u32>,
    pub command: String,
    pub args: Vec<String>,
    pub state: ProcessState,
    pub restart_policy: RestartPolicy,
    pub run_count: u32,
    pub exit_code: Option<i32>,
    pub signal: Option<String>,
    pub created_at: Option<i64>,
    pub started_at: Option<i64>,
    pub ended_at: Option<i64>,
    pub working_dir: Option<String>,
    pub env: HashMap<String, String>,
    // Dependencies (systemd-like)
    pub after: Vec<String>,
    pub before: Vec<String>,
    pub requires: Vec<String>,
    pub wants: Vec<String>,
    pub binds_to: Vec<String>,
    pub conflicts: Vec<String>,
    // Process type (systemd-like)
    pub process_type: String,
    // Health check
    pub health_status: Option<HealthStatus>,
    // Conditional execution
    pub condition_path_exists: Vec<PathCondition>,
    // Runtime directories
    pub runtime_directory: Vec<String>,
    // Ambient capabilities (Linux-only)
    pub ambient_capabilities: Vec<String>,
}

/// Health check type (Docker/Kubernetes-style)
#[derive(Clone, Debug, PartialEq)]
pub enum HealthCheckType {
    Http,
    Tcp,
    Exec,
}

impl fmt::Display for HealthCheckType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let s = match self {
            Self::Http => "http",
            Self::Tcp => "tcp",
            Self::Exec => "exec",
        };
        write!(f, "{}", s)
    }
}

impl std::str::FromStr for HealthCheckType {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "http" => Ok(Self::Http),
            "tcp" => Ok(Self::Tcp),
            "exec" => Ok(Self::Exec),
            _ => Err(format!("Invalid health check type: {}", s)),
        }
    }
}

/// Health status of a process
#[derive(Clone, Debug, PartialEq)]
pub enum HealthStatus {
    Healthy,
    Unhealthy,
    Starting, // Within start_period, not yet checking
    Unknown,  // Not yet checked or no health check configured
}

impl fmt::Display for HealthStatus {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let s = match self {
            Self::Healthy => "healthy",
            Self::Unhealthy => "unhealthy",
            Self::Starting => "starting",
            Self::Unknown => "unknown",
        };
        write!(f, "{}", s)
    }
}

impl serde::Serialize for HealthStatus {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        serializer.serialize_str(&self.to_string())
    }
}

/// Health check configuration
#[derive(Clone, Debug)]
pub struct HealthCheckConfig {
    pub check_type: HealthCheckType,
    pub interval: u64,      // seconds between checks
    pub timeout: u64,       // seconds before timeout
    pub retries: u32,       // consecutive failures before unhealthy
    pub start_period: u64,  // grace period after start (seconds)
    pub restart_after: u32, // kill and restart after N failures (0 = never, K8s liveness probe)

    // HTTP-specific fields
    pub http_endpoint: Option<String>,
    pub http_method: Option<String>,
    pub http_expected_status: Option<u16>,

    // TCP-specific fields
    pub tcp_host: Option<String>,
    pub tcp_port: Option<u16>,

    // Exec-specific fields
    pub exec_command: Option<String>,
    pub exec_args: Option<Vec<String>>,
}

impl Default for HealthCheckConfig {
    fn default() -> Self {
        Self {
            check_type: HealthCheckType::Http,
            interval: crate::constants::health_check::DEFAULT_INTERVAL,
            timeout: crate::constants::health_check::DEFAULT_TIMEOUT,
            retries: crate::constants::health_check::DEFAULT_RETRIES,
            start_period: 0,
            restart_after: 0, // Default: never restart (informational only)
            http_endpoint: None,
            http_method: Some(crate::constants::health_check::DEFAULT_HTTP_METHOD.to_string()),
            http_expected_status: Some(crate::constants::health_check::DEFAULT_HTTP_STATUS),
            tcp_host: None,
            tcp_port: None,
            exec_command: None,
            exec_args: None,
        }
    }
}

/// Resource requests (guaranteed resources)
#[derive(Clone, Debug, Default, PartialEq)]
pub struct ResourceRequests {
    /// CPU request in millicores (e.g., 500 = 0.5 cores)
    pub cpu: Option<u64>,
    /// Memory request in bytes
    pub memory: Option<u64>,
}

/// Resource limits (maximum resources)
#[derive(Clone, Debug, Default, PartialEq)]
pub struct ResourceLimits {
    /// CPU limit in millicores (e.g., 1000 = 1 core)
    pub cpu: Option<u64>,
    /// Memory limit in bytes
    pub memory: Option<u64>,
    /// Maximum number of PIDs (processes/threads)
    pub pids: Option<u32>,
}

/// Complete resource configuration (K8s-style)
#[derive(Clone, Debug, Default, PartialEq)]
pub struct ResourceConfig {
    /// Guaranteed resources (soft limits, priority)
    pub requests: ResourceRequests,
    /// Maximum resources (hard limits)
    pub limits: ResourceLimits,
    /// OOM score adjustment (-1000 to 1000, lower = less likely to be killed)
    pub oom_score_adj: Option<i32>,
}

impl ResourceConfig {
    /// Check if any resource limits are configured
    pub fn has_limits(&self) -> bool {
        self.requests.cpu.is_some()
            || self.requests.memory.is_some()
            || self.limits.cpu.is_some()
            || self.limits.memory.is_some()
            || self.limits.pids.is_some()
            || self.oom_score_adj.is_some()
    }
}
