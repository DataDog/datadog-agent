// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! How long a listener waits after a failed accept, and when the failures stop looking
//! transient.
//!
//! Free of platform types on purpose. The transport this serves is Windows only, and
//! standing up a real pipe there needs the installed agent's SID, so the policy is the
//! only part that can be tested on a developer's machine rather than on CI alone.

use std::time::Duration;

/// Wait after a single failure. Long enough that a failure returning immediately cannot
/// spin the runtime, short enough to be invisible to a client that is already waiting.
const INITIAL_DELAY: Duration = Duration::from_millis(100);

/// Ceiling for the wait. A broken listener retries at this cadence forever: giving up
/// would leave the supervisor with no control plane at all.
const MAX_DELAY: Duration = Duration::from_secs(5);

/// Consecutive failures after which this is reported as a broken transport rather than a
/// hiccup.
const PERSISTENT_AFTER: u32 = 5;

/// Tracks consecutive accept failures.
#[derive(Default)]
pub(crate) struct AcceptBackoff {
    consecutive: u32,
}

/// What one failure earns.
#[derive(Debug, PartialEq, Eq)]
pub(crate) struct Retry {
    /// How long to wait before trying again. Never zero.
    pub(crate) delay: Duration,
    /// Failures since the last success, worth putting in the log line so a streak is
    /// visible without counting lines.
    pub(crate) consecutive: u32,
    /// Whether the caller should report this loudly.
    pub(crate) persistent: bool,
}

impl AcceptBackoff {
    pub(crate) fn new() -> Self {
        Self::default()
    }

    pub(crate) fn record_failure(&mut self) -> Retry {
        self.consecutive = self.consecutive.saturating_add(1);
        Retry {
            delay: delay_after(self.consecutive),
            consecutive: self.consecutive,
            persistent: self.consecutive >= PERSISTENT_AFTER,
        }
    }

    pub(crate) fn record_success(&mut self) {
        self.consecutive = 0;
    }
}

/// Doubles per consecutive failure up to the ceiling. `failures` counts from 1.
fn delay_after(failures: u32) -> Duration {
    let doublings = failures.saturating_sub(1).min(u32::BITS - 1);
    INITIAL_DELAY
        .checked_mul(1u32 << doublings)
        .unwrap_or(MAX_DELAY)
        .min(MAX_DELAY)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_single_failure_waits_before_retrying() {
        let mut backoff = AcceptBackoff::new();

        let retry = backoff.record_failure();

        assert_eq!(retry.delay, INITIAL_DELAY);
        assert_eq!(retry.consecutive, 1);
        assert!(
            !retry.persistent,
            "one failure is a hiccup, not a broken transport"
        );
    }

    #[test]
    fn the_delay_doubles_per_consecutive_failure() {
        let mut backoff = AcceptBackoff::new();

        let delays: Vec<Duration> = (0..4).map(|_| backoff.record_failure().delay).collect();

        assert_eq!(
            delays,
            vec![
                Duration::from_millis(100),
                Duration::from_millis(200),
                Duration::from_millis(400),
                Duration::from_millis(800),
            ]
        );
    }

    #[test]
    fn a_failing_listener_settles_at_the_ceiling_and_stays_there() {
        let mut backoff = AcceptBackoff::new();

        // Far past the point where doubling would overflow a Duration.
        let retries: Vec<Retry> = (0..200).map(|_| backoff.record_failure()).collect();

        for retry in &retries {
            assert!(
                retry.delay <= MAX_DELAY,
                "waited {:?}, over the {MAX_DELAY:?} ceiling",
                retry.delay
            );
            assert!(!retry.delay.is_zero(), "a zero wait spins the runtime");
        }
        assert_eq!(retries.last().expect("retries").delay, MAX_DELAY);
    }

    #[test]
    fn failures_turn_persistent_only_once_the_streak_is_long_enough() {
        let mut backoff = AcceptBackoff::new();

        for failure in 1..PERSISTENT_AFTER {
            assert!(
                !backoff.record_failure().persistent,
                "failure {failure} of {PERSISTENT_AFTER} should still read as transient"
            );
        }

        assert!(backoff.record_failure().persistent);
    }

    #[test]
    fn a_successful_accept_clears_the_streak() {
        let mut backoff = AcceptBackoff::new();
        for _ in 0..PERSISTENT_AFTER + 3 {
            backoff.record_failure();
        }

        backoff.record_success();
        let retry = backoff.record_failure();

        assert_eq!(retry.delay, INITIAL_DELAY, "the wait should start over");
        assert_eq!(retry.consecutive, 1);
        assert!(!retry.persistent);
    }
}
