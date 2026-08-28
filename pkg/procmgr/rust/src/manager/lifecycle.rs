// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::sync::Arc;
use std::sync::atomic::{AtomicU8, Ordering};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Phase {
    Starting,
    Running,
    Stopping,
}

#[derive(Clone)]
pub(crate) struct Lifecycle {
    phase: Arc<AtomicU8>,
}

impl Lifecycle {
    pub(crate) fn new() -> Self {
        Self {
            phase: Arc::new(AtomicU8::new(phase_to_u8(Phase::Starting))),
        }
    }

    fn phase(&self) -> Phase {
        u8_to_phase(self.phase.load(Ordering::Acquire))
    }

    pub(crate) fn begin_running(&self) {
        self.phase
            .store(phase_to_u8(Phase::Running), Ordering::Release);
    }

    pub(in crate::manager) fn begin_stopping(&self) {
        self.phase
            .store(phase_to_u8(Phase::Stopping), Ordering::Release);
    }

    pub(in crate::manager) fn spawns_allowed(&self) -> bool {
        matches!(self.phase(), Phase::Starting | Phase::Running)
    }

    pub(in crate::manager) fn is_stopping(&self) -> bool {
        self.phase() == Phase::Stopping
    }

    pub(crate) fn is_running(&self) -> bool {
        self.phase() == Phase::Running
    }
}

fn phase_to_u8(phase: Phase) -> u8 {
    match phase {
        Phase::Starting => 0,
        Phase::Running => 1,
        Phase::Stopping => 2,
    }
}

fn u8_to_phase(value: u8) -> Phase {
    match value {
        0 => Phase::Starting,
        1 => Phase::Running,
        _ => Phase::Stopping,
    }
}
