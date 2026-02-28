// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::fmt;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessState {
    /// Config loaded, never started.
    Created,
    /// Child process is alive.
    Running,
    /// Exited with code 0.
    Exited,
    /// Exited with non-zero code or signal.
    Failed,
    /// Explicitly stopped during shutdown.
    Stopped,
}

impl ProcessState {
    pub fn is_alive(self) -> bool {
        self == ProcessState::Running
    }

    pub(crate) fn can_transition_to(self, next: ProcessState) -> bool {
        use ProcessState::*;
        matches!(
            (self, next),
            (Created, Running)
                | (Running, Exited)
                | (Running, Failed)
                | (Running, Stopped)
                | (Exited, Running)
                | (Failed, Running)
                | (Stopped, Running)
        )
    }
}

impl fmt::Display for ProcessState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ProcessState::Created => write!(f, "created"),
            ProcessState::Running => write!(f, "running"),
            ProcessState::Exited => write!(f, "exited"),
            ProcessState::Failed => write!(f, "failed"),
            ProcessState::Stopped => write!(f, "stopped"),
        }
    }
}
