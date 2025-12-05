//! Domain-level errors
//! These represent business rule violations, not infrastructure failures

use thiserror::Error;

#[derive(Debug, Error, Clone)]
pub enum DomainError {
    // Process lifecycle errors
    #[error("Process '{0}' not found")]
    ProcessNotFound(String),

    #[error("Process '{0}' already exists")]
    DuplicateProcess(String),

    #[error("Process is already running (PID: {0})")]
    AlreadyRunning(u32),

    #[error("Process is not running")]
    NotRunning,

    #[error("Invalid state transition from {from:?} to {to:?}")]
    InvalidStateTransition { from: String, to: String },

    // Dependency errors
    #[error("Dependency '{0}' not found")]
    DependencyNotFound(String),

    #[error("Dependency cycle detected")]
    DependencyCycle,

    #[error("Circular dependency detected")]
    CircularDependency,

    #[error("Process '{process}' conflicts with '{conflicting_with}'")]
    ConflictingProcess {
        process: String,
        conflicting_with: String,
    },

    #[error("Process '{process}' requires '{dependency}' to be running")]
    DependencyNotRunning { process: String, dependency: String },

    #[error("Required dependency '{0}' failed to start")]
    RequiredDependencyFailed(String),

    // Validation errors
    #[error("Invalid process name: {0}")]
    InvalidName(String),

    #[error("Invalid command: {0}")]
    InvalidCommand(String),

    #[error("Condition not met: {0}")]
    ConditionNotMet(String),

    // Configuration errors
    #[error("Invalid configuration: {0}")]
    InvalidConfiguration(String),

    // Resource errors
    #[error("Resource limit exceeded: {0}")]
    ResourceLimitExceeded(String),

    // Health check errors
    #[error("Health check failed: {0}")]
    HealthCheckFailed(String),
}

pub type Result<T> = std::result::Result<T, DomainError>;
