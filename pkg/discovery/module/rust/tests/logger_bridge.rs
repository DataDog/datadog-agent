// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Integration test for `dd_discovery::ffi::dd_discovery_init_logger`.
//!
//! Cargo runs each `tests/*.rs` as its own binary, so the process-global
//! `log::logger` and the bridge's `LOGGER_CALLBACK` are isolated from other
//! tests. This makes it safe to install the bridge and call the real Rust
//! `log!` macros without disturbing anything else.

#![allow(clippy::unwrap_used)]
#![allow(clippy::expect_used)]

use std::os::raw::c_char;
use std::sync::Mutex;

use dd_discovery::ffi::dd_discovery_init_logger;

/// Records captured by `capture_callback`. A `Mutex<Vec<...>>` is enough
/// because `log!` macros invoke the logger synchronously on the calling
/// thread.
static CAPTURED: Mutex<Vec<(u32, String)>> = Mutex::new(Vec::new());

/// Test callback that mimics the Go-side cgo entry point: it copies the
/// message bytes into a Rust `String` and stores `(level, message)`.
///
/// # Safety
/// Caller must guarantee `msg` is valid for `msg_len` bytes.
unsafe extern "C" fn capture_callback(level: u32, msg: *const c_char, msg_len: usize) {
    if msg.is_null() || msg_len == 0 {
        CAPTURED.lock().unwrap().push((level, String::new()));
        return;
    }
    // SAFETY: contract of dd_log_fn — pointer is valid for the call duration
    // and refers to UTF-8 bytes of length msg_len.
    let bytes = unsafe { std::slice::from_raw_parts(msg.cast::<u8>(), msg_len) };
    let s = std::str::from_utf8(bytes)
        .expect("dd_log_fn callback received non-UTF-8 bytes")
        .to_string();
    CAPTURED.lock().unwrap().push((level, s));
}

/// Single test per binary: validates that
///   1. records emitted via `log!` reach the registered callback,
///   2. each of the five Rust log levels maps to the expected `dd_log_fn`
///      level encoding (1..=5),
///   3. formatted args are rendered (proves `record.args().to_string()` runs),
///   4. a second `dd_discovery_init_logger` call is a no-op (idempotency
///      via `OnceLock`) — the callback is not re-registered, so we keep
///      receiving exactly one record per `log!` invocation.
#[test]
fn forwards_records_with_correct_levels_and_init_is_idempotent() {
    // SAFETY: capture_callback has the dd_log_fn signature and lives for the
    // process lifetime (it's a static fn).
    unsafe { dd_discovery_init_logger(capture_callback) };
    // Second call: must be ignored by the OnceLock guard.
    // SAFETY: same as above.
    unsafe { dd_discovery_init_logger(capture_callback) };

    log::error!("err {}", 1);
    log::warn!("warn {}", 2);
    log::info!("info {}", 3);
    log::debug!("debug {}", 4);
    log::trace!("trace {}", 5);

    let cap = CAPTURED.lock().unwrap();
    let by_level: std::collections::HashMap<u32, Vec<&str>> =
        cap.iter()
            .fold(std::collections::HashMap::new(), |mut acc, (l, m)| {
                acc.entry(*l).or_default().push(m.as_str());
                acc
            });

    // Level mapping: each `log::*!` macro must produce exactly one record at
    // the expected `dd_log_fn` level encoding.
    for (rust_level, dd_level, expected) in [
        ("error", 1, "err 1"),
        ("warn", 2, "warn 2"),
        ("info", 3, "info 3"),
        ("debug", 4, "debug 4"),
        ("trace", 5, "trace 5"),
    ] {
        let entries = by_level
            .get(&dd_level)
            .unwrap_or_else(|| panic!("no record at dd_log_fn level {dd_level} ({rust_level})"));
        assert_eq!(
            entries.len(),
            1,
            "expected exactly one {rust_level} record (idempotent init); got {entries:?}"
        );
        assert_eq!(
            entries[0], expected,
            "formatted args must be rendered before the FFI hop"
        );
    }
}
