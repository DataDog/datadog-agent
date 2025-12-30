//! ProcessState value object
//! Represents the lifecycle state of a process

use serde::{Deserialize, Serialize};
use std::fmt;

/// The state of a process in its lifecycle
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize, Default)]
pub enum ProcessState {
    /// Process definition created but never started
    #[default]
    Created,

    /// Process was running but is now stopped
    Stopped,

    /// Process is currently starting up
    Starting,

    /// Process is running normally
    Running,

    /// Process is in the process of stopping
    Stopping,

    /// Process has exited successfully (exit code 0)
    Exited,

    /// Process has failed (non-zero exit code)
    Failed,

    /// Process is being restarted after failure
    Restarting,
}

impl ProcessState {
    /// Check if the process is in a "running" state
    pub fn is_running(&self) -> bool {
        matches!(self, ProcessState::Running | ProcessState::Starting)
    }

    /// Check if the process is in a terminal state
    pub fn is_terminal(&self) -> bool {
        matches!(
            self,
            ProcessState::Exited
                | ProcessState::Failed
                | ProcessState::Stopped
                | ProcessState::Created
        )
    }

    /// Check if the process can be started
    pub fn can_start(&self) -> bool {
        matches!(
            self,
            ProcessState::Created
                | ProcessState::Stopped
                | ProcessState::Exited
                | ProcessState::Failed
        )
    }

    /// Check if the process can be stopped
    pub fn can_stop(&self) -> bool {
        matches!(
            self,
            ProcessState::Starting | ProcessState::Running | ProcessState::Restarting
        )
    }

    /// Validate state transition
    pub fn can_transition_to(&self, new_state: ProcessState) -> bool {
        use ProcessState::*;

        match (self, new_state) {
            // From Created (initial state)
            (Created, Starting) => true,

            // From Stopped
            (Stopped, Starting) => true,

            // From Starting
            (Starting, Running) => true,
            (Starting, Exited) => true, // Oneshot that exits immediately
            (Starting, Failed) => true, // Failed to start
            (Starting, Stopping) => true, // Can cancel startup

            // From Running
            (Running, Stopping) => true,
            (Running, Exited) => true, // Natural exit (e.g., oneshot)
            (Running, Failed) => true, // Crash or error exit

            // From Stopping
            (Stopping, Stopped) => true,
            (Stopping, Failed) => true,

            // From Failed
            (Failed, Starting | Restarting) => true,

            // From Exited
            (Exited, Starting | Restarting) => true,

            // From Restarting
            (Restarting, Starting) => true,

            // Same state is always allowed
            (a, b) if *a == b => true,

            // Everything else is invalid
            _ => false,
        }
    }
}

impl fmt::Display for ProcessState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ProcessState::Created => write!(f, "created"),
            ProcessState::Stopped => write!(f, "stopped"),
            ProcessState::Starting => write!(f, "starting"),
            ProcessState::Running => write!(f, "running"),
            ProcessState::Stopping => write!(f, "stopping"),
            ProcessState::Exited => write!(f, "exited"),
            ProcessState::Failed => write!(f, "failed"),
            ProcessState::Restarting => write!(f, "restarting"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_running() {
        assert!(ProcessState::Running.is_running());
        assert!(ProcessState::Starting.is_running());
        assert!(!ProcessState::Stopped.is_running());
        assert!(!ProcessState::Failed.is_running());
    }

    #[test]
    fn test_is_terminal() {
        assert!(ProcessState::Stopped.is_terminal());
        assert!(ProcessState::Exited.is_terminal());
        assert!(ProcessState::Failed.is_terminal());
        assert!(!ProcessState::Running.is_terminal());
        assert!(!ProcessState::Starting.is_terminal());
    }

    #[test]
    fn test_can_start() {
        assert!(ProcessState::Created.can_start());
        assert!(ProcessState::Stopped.can_start());
        assert!(ProcessState::Exited.can_start());
        assert!(ProcessState::Failed.can_start());
        assert!(!ProcessState::Running.can_start());
        assert!(!ProcessState::Starting.can_start());
    }

    #[test]
    fn test_can_stop() {
        assert!(ProcessState::Running.can_stop());
        assert!(ProcessState::Starting.can_stop());
        assert!(!ProcessState::Stopped.can_stop());
        assert!(!ProcessState::Exited.can_stop());
    }

    #[test]
    fn test_valid_transitions() {
        // Created -> Starting (first start)
        assert!(ProcessState::Created.can_transition_to(ProcessState::Starting));

        // Stopped -> Starting
        assert!(ProcessState::Stopped.can_transition_to(ProcessState::Starting));

        // Starting -> Running
        assert!(ProcessState::Starting.can_transition_to(ProcessState::Running));

        // Running -> Stopping
        assert!(ProcessState::Running.can_transition_to(ProcessState::Stopping));

        // Stopping -> Stopped
        assert!(ProcessState::Stopping.can_transition_to(ProcessState::Stopped));

        // Failed -> Restarting
        assert!(ProcessState::Failed.can_transition_to(ProcessState::Restarting));
    }

    #[test]
    fn test_invalid_transitions() {
        // Can't go from Created directly to Running
        assert!(!ProcessState::Created.can_transition_to(ProcessState::Running));

        // Can't go from Stopped directly to Running
        assert!(!ProcessState::Stopped.can_transition_to(ProcessState::Running));

        // Can't go from Running back to Starting
        assert!(!ProcessState::Running.can_transition_to(ProcessState::Starting));

        // Can't go from Exited to Stopping
        assert!(!ProcessState::Exited.can_transition_to(ProcessState::Stopping));

        // Can't go from Created to Stopped (must start first)
        assert!(!ProcessState::Created.can_transition_to(ProcessState::Stopped));
    }

    #[test]
    fn test_display() {
        assert_eq!(ProcessState::Created.to_string(), "created");
        assert_eq!(ProcessState::Running.to_string(), "running");
        assert_eq!(ProcessState::Stopped.to_string(), "stopped");
        assert_eq!(ProcessState::Failed.to_string(), "failed");
    }

    #[test]
    fn test_default() {
        assert_eq!(ProcessState::default(), ProcessState::Created);
    }
}
