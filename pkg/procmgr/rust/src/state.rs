// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::fmt;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessState {
    /// Config loaded, never started.
    Created,
    /// Spawn requested, child not yet confirmed alive.
    Starting,
    /// Child process is alive.
    Running,
    /// Stop requested, waiting for child to exit.
    Stopping,
    /// Exited with code 0.
    Exited,
    /// Exited with non-zero code or signal.
    Failed,
    /// Explicitly stopped during shutdown.
    Stopped,
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
                | (Created, Running)
                | (Starting, Running)
                | (Starting, Failed)
                | (Running, Stopping)
                | (Running, Exited)
                | (Running, Failed)
                | (Running, Stopped)
                | (Stopping, Stopped)
                | (Stopping, Exited)
                | (Stopping, Failed)
                | (Exited, Starting)
                | (Exited, Running)
                | (Failed, Starting)
                | (Failed, Running)
                | (Stopped, Starting)
                | (Stopped, Running)
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
            ProcessState::Failed => write!(f, "failed"),
            ProcessState::Stopped => write!(f, "stopped"),
        }
    }
}
