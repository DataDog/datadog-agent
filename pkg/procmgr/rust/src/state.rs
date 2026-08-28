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
    Failed,
    Stopped,
}

impl ProcessState {
    pub fn is_alive(self) -> bool {
        matches!(
            self,
            ProcessState::Running | ProcessState::Starting | ProcessState::Stopping
        )
    }

    pub(crate) fn awaiting_stop(self) -> bool {
        matches!(self, ProcessState::Running | ProcessState::Stopping)
    }

    pub(crate) fn can_transition_to(self, next: ProcessState) -> bool {
        use ProcessState::*;
        matches!(
            (self, next),
            (Created, Starting)
                | (Starting, Running)
                | (Starting, Failed)
                | (Starting, Stopped)
                | (Running, Stopping)
                | (Running, Exited)
                | (Running, Failed)
                | (Running, Stopped)
                | (Stopping, Stopped)
                | (Exited, Starting)
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
            ProcessState::Failed => write!(f, "failed"),
            ProcessState::Stopped => write!(f, "stopped"),
        }
    }
}
