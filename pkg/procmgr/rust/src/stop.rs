// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! The stop request, recorded where code that runs before the shutdown future is polled can
//! see it.

use std::sync::atomic::{AtomicBool, Ordering};

/// Set by the SIGTERM and SIGINT handler that `dd-procmgrd` installs first in `main`. Before the
/// runtime exists, the early handler is the only one, so a signal delivered earlier is kept here.
/// tokio's handler chains to it, so the flag also rises for later signals. The flag is
/// process-wide and never cleared, so it outlives any one `ProcessManager` run.
static STOP: AtomicBool = AtomicBool::new(false);

/// The number of the stop signal that set `STOP`, or 0 if none. The handler stores it before
/// setting `STOP`, so a reader that sees the flag also sees the signal.
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

/// Name of the recorded stop signal. Call it after `stop_requested_flag` returns true.
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

/// Set once the stop request has been logged.
static LOGGED: std::sync::Once = std::sync::Once::new();

/// Logs `received <signal>` once per process. The main thread and the watcher can both see the
/// stop, so only the first caller logs it. A second caller waits until the first has finished
/// logging, so the line is written before that caller goes on.
pub(crate) fn log_received(signal: &str) {
    LOGGED.call_once(|| log::info!("received {signal}"));
}

/// Whether a stop request has arrived.
pub(crate) fn stop_requested_flag() -> bool {
    STOP.load(Ordering::SeqCst)
}
