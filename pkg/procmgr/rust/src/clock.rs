// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.
//! The time source for the stop path.
//!
//! Production reads tokio's clock. Tests drive a [`ManualClock`] instead, so they
//! do not need a paused tokio runtime, which would require tokio's `test-util`
//! feature in every Rust target built against tokio.

use std::future::Future;
use std::pin::Pin;
use std::time::Duration;
use tokio::time::Instant;

/// A timer that completes once its duration has passed on the clock it came from.
pub(crate) type Sleep = Pin<Box<dyn Future<Output = ()> + Send>>;

#[derive(Clone)]
pub(crate) enum Clock {
    Real,
    #[cfg(test)]
    Manual(std::sync::Arc<std::sync::Mutex<manual::State>>),
}

impl Clock {
    pub(crate) fn now(&self) -> Instant {
        match self {
            Self::Real => Instant::now(),
            #[cfg(test)]
            Self::Manual(state) => state.lock().unwrap().now(),
        }
    }

    pub(crate) fn sleep(&self, duration: Duration) -> Sleep {
        match self {
            Self::Real => Box::pin(tokio::time::sleep(duration)),
            #[cfg(test)]
            Self::Manual(state) => state.lock().unwrap().sleep(duration),
        }
    }
}

#[cfg(test)]
pub(crate) use manual::ManualClock;

#[cfg(test)]
mod manual {
    use super::{Clock, Sleep};
    use std::future::Future;
    use std::sync::{Arc, Mutex};
    use std::time::Duration;
    use tokio::sync::oneshot;
    use tokio::time::Instant;

    /// Upper bound on idle steps in [`ManualClock::drive`]. A future still pending
    /// with no timer left would otherwise spin forever.
    const MAX_IDLE_STEPS: usize = 10_000;

    pub(crate) struct State {
        base: Instant,
        /// Time elapsed since `base`.
        offset: Duration,
        /// Pending timers, each with the offset at which it completes.
        waiters: Vec<(Duration, oneshot::Sender<()>)>,
    }

    impl State {
        pub(crate) fn now(&self) -> Instant {
            self.base + self.offset
        }

        pub(crate) fn sleep(&mut self, duration: Duration) -> Sleep {
            let deadline = self.offset + duration;
            if deadline <= self.offset {
                return Box::pin(async {});
            }
            let (tx, rx) = oneshot::channel();
            self.waiters.push((deadline, tx));
            Box::pin(async move {
                let _ = rx.await;
            })
        }

        /// Drops timers whose `Sleep` was dropped before it fired, so they do not
        /// count as pending work.
        fn prune_cancelled(&mut self) {
            self.waiters.retain(|(_, tx)| !tx.is_closed());
        }

        /// The earliest pending deadline, if any.
        fn next_deadline(&mut self) -> Option<Duration> {
            self.prune_cancelled();
            self.waiters.iter().map(|(deadline, _)| *deadline).min()
        }

        /// Moves to `deadline` and completes every timer due at or before it.
        fn fire(&mut self, deadline: Duration) {
            self.prune_cancelled();
            self.offset = self.offset.max(deadline);
            let (due, pending): (Vec<_>, Vec<_>) = std::mem::take(&mut self.waiters)
                .into_iter()
                .partition(|(at, _)| *at <= self.offset);
            self.waiters = pending;
            for (_, tx) in due {
                let _ = tx.send(());
            }
        }
    }

    /// A clock that only moves when a test moves it.
    pub(crate) struct ManualClock {
        state: Arc<Mutex<State>>,
    }

    impl ManualClock {
        pub(crate) fn new() -> Self {
            Self {
                state: Arc::new(Mutex::new(State {
                    base: Instant::now(),
                    offset: Duration::ZERO,
                    waiters: Vec::new(),
                })),
            }
        }

        pub(crate) fn clock(&self) -> Clock {
            Clock::Manual(Arc::clone(&self.state))
        }

        /// Moves time forward by `duration`, completing each timer that falls due on
        /// the way, in order. The executor runs between timers, so tasks woken by one
        /// timer can start the next.
        #[cfg_attr(not(windows), allow(dead_code, reason = "used by the Windows tests"))]
        pub(crate) async fn advance(&self, duration: Duration) {
            settle().await;
            let target = self.state.lock().unwrap().offset + duration;
            loop {
                let next = {
                    let mut state = self.state.lock().unwrap();
                    state.next_deadline().filter(|at| *at <= target)
                };
                let Some(next) = next else { break };
                self.state.lock().unwrap().fire(next);
                settle().await;
            }
            self.state.lock().unwrap().fire(target);
            settle().await;
        }

        /// Runs `future` to completion. Whenever it is idle, time moves to the next
        /// pending timer, as tokio's paused clock does. The future must not wait on
        /// anything but timers.
        pub(crate) async fn drive<F: Future>(&self, future: F) -> F::Output {
            tokio::pin!(future);
            let mut idle = 0;
            loop {
                tokio::select! {
                    biased;
                    output = &mut future => return output,
                    () = settle() => {}
                }
                let next = self.state.lock().unwrap().next_deadline();
                match next {
                    Some(at) => {
                        self.state.lock().unwrap().fire(at);
                        idle = 0;
                    }
                    None => {
                        idle += 1;
                        assert!(
                            idle < MAX_IDLE_STEPS,
                            "driven future is pending with no timer left"
                        );
                    }
                }
            }
        }
    }

    /// Yields long enough for tasks that are ready to run and register their timers.
    ///
    /// This assumes a task that is woken registers its next timer within 32 polls.
    /// If a task needs more turns, time can move past its timer before it exists,
    /// and timing assertions drift. Keep woken tasks to a few polls per step.
    async fn settle() {
        for _ in 0..32 {
            tokio::task::yield_now().await;
        }
    }
}
