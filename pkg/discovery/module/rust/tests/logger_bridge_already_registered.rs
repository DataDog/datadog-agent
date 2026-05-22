// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Integration test for the "another logger already registered" fallback in
//! `dd_discovery::ffi::dd_discovery_init_logger`.
//!
//! In a real deployment the agent owns the process and registers no other
//! logger, but in test harnesses (or downstream consumers that compose
//! multiple loggers) the global `log::logger` may already be set. The bridge
//! must surface this to the Go side via the callback rather than silently
//! dropping records.
//!
//! Cargo runs each `tests/*.rs` as its own binary, so installing a custom
//! logger here does not affect any other test binary.

#![allow(clippy::unwrap_used)]
#![allow(clippy::expect_used)]

use std::os::raw::c_char;
use std::sync::Mutex;

use dd_discovery::ffi::dd_discovery_init_logger;

static CAPTURED: Mutex<Vec<(u32, String)>> = Mutex::new(Vec::new());

/// # Safety
/// Caller must guarantee `msg` is valid for `msg_len` bytes.
unsafe extern "C" fn capture_callback(level: u32, msg: *const c_char, msg_len: usize) {
    if msg.is_null() || msg_len == 0 {
        CAPTURED.lock().unwrap().push((level, String::new()));
        return;
    }
    // SAFETY: contract of dd_log_fn.
    let bytes = unsafe { std::slice::from_raw_parts(msg.cast::<u8>(), msg_len) };
    let s = std::str::from_utf8(bytes)
        .expect("dd_log_fn callback received non-UTF-8 bytes")
        .to_string();
    CAPTURED.lock().unwrap().push((level, s));
}

/// Sink logger that drops every record. Stands in for whatever logger the
/// host process may have registered before calling `dd_discovery_init_logger`.
struct Sink;

impl log::Log for Sink {
    fn enabled(&self, _: &log::Metadata) -> bool {
        false
    }
    fn log(&self, _: &log::Record) {}
    fn flush(&self) {}
}

static SINK: Sink = Sink;

#[test]
fn surfaces_conflict_via_callback_when_logger_already_registered() {
    // Pre-register a different logger so `log::set_logger` inside
    // `dd_discovery_init_logger` returns Err.
    log::set_logger(&SINK).expect("first set_logger must succeed in a fresh process");
    log::set_max_level(log::LevelFilter::Trace);

    // SAFETY: capture_callback has the dd_log_fn signature and lives for the
    // process lifetime.
    unsafe { dd_discovery_init_logger(capture_callback) };

    let cap = CAPTURED.lock().unwrap();
    assert_eq!(
        cap.len(),
        1,
        "fallback must invoke the callback exactly once: got {cap:?}"
    );
    assert_eq!(cap[0].0, 1, "fallback must report at Error level");
    assert!(
        cap[0].1.contains("a Rust logger was already registered"),
        "fallback message must explain the conflict; got {:?}",
        cap[0].1
    );

    // Subsequent log records go to SINK (the pre-registered logger), NOT to
    // the bridge callback. Verify the bridge does not start swallowing
    // records after a failed install.
    drop(cap);
    log::error!("must not reach the bridge");
    log::info!("nor this");
    assert_eq!(
        CAPTURED.lock().unwrap().len(),
        1,
        "GoLogger was not installed, so the bridge callback must not see further records"
    );
}
