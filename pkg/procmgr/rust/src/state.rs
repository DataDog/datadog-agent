// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::fmt;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessState {
    Created,
    Starting,
    Running,
    Stopping,
    Exited,
    /// Died without returning a value: a signal on Unix, a fatal exception code
    /// on Windows. Terminal and restartable, same shape as `Failed`.
    Crashed,
    Failed,
    Stopped,
    /// Start pass declined to spawn, and no child has ever existed.
    Skipped,
}

impl ProcessState {
    pub fn is_alive(self) -> bool {
        matches!(
            self,
            ProcessState::Running | ProcessState::Starting | ProcessState::Stopping
        )
    }

    pub(crate) fn can_transition_to(self, next: ProcessState) -> bool {
        use ProcessState::*;
        matches!(
            (self, next),
            (Created, Starting)
                | (Created, Skipped)
                | (Skipped, Starting)
                | (Starting, Running)
                // No (Starting, Crashed): a spawn that never produced a process
                // image cannot have died without returning a value.
                | (Starting, Failed)
                | (Running, Stopping)
                | (Running, Exited)
                | (Running, Crashed)
                | (Running, Failed)
                | (Running, Stopped)
                | (Stopping, Stopped)
                | (Exited, Starting)
                | (Crashed, Starting)
                | (Failed, Starting)
                | (Stopped, Starting)
        )
    }
}

impl fmt::Display for ProcessState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ProcessState::Created => write!(f, "created"),
            ProcessState::Starting => write!(f, "starting"),
            ProcessState::Running => write!(f, "running"),
            ProcessState::Stopping => write!(f, "stopping"),
            ProcessState::Exited => write!(f, "exited"),
            ProcessState::Crashed => write!(f, "crashed"),
            ProcessState::Failed => write!(f, "failed"),
            ProcessState::Stopped => write!(f, "stopped"),
            ProcessState::Skipped => write!(f, "skipped"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::ProcessState::*;

    #[test]
    fn crashed_is_reachable_from_running_and_restartable() {
        assert!(Running.can_transition_to(Crashed));
        assert!(Crashed.can_transition_to(Starting));
    }

    /// A spawn failure stays `Failed`: nothing ran, so nothing crashed. And a
    /// crashed process reaches `Running` only by going through `Starting`,
    /// which is what makes the restart spend a burst slot.
    #[test]
    fn crashed_rejects_spawn_failure_and_direct_resume() {
        assert!(!Starting.can_transition_to(Crashed));
        assert!(!Crashed.can_transition_to(Running));
    }

    #[test]
    fn crashed_is_not_alive() {
        assert!(!Crashed.is_alive());
    }

    #[test]
    fn crashed_displays_lowercase() {
        assert_eq!(Crashed.to_string(), "crashed");
    }

    #[test]
    fn skipped_is_reachable_from_created_and_startable() {
        assert!(Created.can_transition_to(Skipped));
        assert!(Skipped.can_transition_to(Starting));
        assert!(!Skipped.is_alive());
        assert_eq!(Skipped.to_string(), "skipped");
    }

    #[test]
    fn skipped_rejects_post_child_states() {
        assert!(!Running.can_transition_to(Skipped));
        assert!(!Exited.can_transition_to(Skipped));
        assert!(!Failed.can_transition_to(Skipped));
        assert!(!Stopped.can_transition_to(Skipped));
        assert!(!Crashed.can_transition_to(Skipped));
        assert!(!Skipped.can_transition_to(Failed));
    }
}
