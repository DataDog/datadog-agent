// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! The stop request, recorded where code that runs before the shutdown future is polled can
//! see it.

use std::sync::atomic::{AtomicBool, Ordering};

/// Set by the SIGTERM and SIGINT handler that `dd-procmgrd` installs first in `main`. Before the
/// runtime exists, the early handler is the only one, so a signal delivered earlier is kept here.
/// tokio's handler chains to it, so the flag also rises for later signals.
static STOP: AtomicBool = AtomicBool::new(false);

/// Records a stop request. Only an atomic store, so it is safe inside a signal handler.
#[cfg(unix)]
pub(crate) fn request_stop() {
    STOP.store(true, Ordering::SeqCst);
}

/// Whether a stop request has arrived.
pub(crate) fn stop_requested_flag() -> bool {
    STOP.load(Ordering::SeqCst)
}
