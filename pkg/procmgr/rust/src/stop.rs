// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! The stop request, recorded where code that runs before the shutdown future is polled can
//! see it.

use std::sync::atomic::{AtomicBool, Ordering};

/// Set by the early SIGTERM and SIGINT handler, which tokio's handler chains to. Never cleared,
/// so it outlives any one `ProcessManager` run.
static STOP: AtomicBool = AtomicBool::new(false);

/// The stop signal number that set `STOP`, or 0. Stored before `STOP` is set.
#[cfg(unix)]
static STOP_SIGNAL: std::sync::atomic::AtomicI32 = std::sync::atomic::AtomicI32::new(0);

/// Records a stop request from `signal`. Only lock-free atomic operations, so it is safe inside a
/// signal handler.
#[cfg(unix)]
pub(crate) fn request_stop(signal: i32) {
    // Keep the first signal, so the name matches the one that set the flag.
    let _ = STOP_SIGNAL.compare_exchange(0, signal, Ordering::SeqCst, Ordering::SeqCst);
    STOP.store(true, Ordering::SeqCst);
}

/// Name of the recorded stop signal.
#[cfg(unix)]
pub(crate) fn stop_signal_name() -> &'static str {
    use nix::libc::{SIGINT, SIGTERM};
    match STOP_SIGNAL.load(Ordering::SeqCst) {
        SIGTERM => "SIGTERM",
        SIGINT => "SIGINT",
        _ => "stop signal",
    }
}

/// Name of the stop signal on platforms without signal numbers.
#[cfg(not(unix))]
pub(crate) fn stop_signal_name() -> &'static str {
    "stop signal"
}

static LOGGED: std::sync::Once = std::sync::Once::new();

/// Logs `received <signal>` once per process. Both the main thread and the watcher may see the
/// stop. A second caller waits until the first has finished logging.
pub(crate) fn log_received(signal: &str) {
    LOGGED.call_once(|| log::info!("received {signal}"));
}

pub(crate) fn stop_requested_flag() -> bool {
    STOP.load(Ordering::SeqCst)
}
