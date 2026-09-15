// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Test-only child process for graceful-stop integration tests on Windows.
//!
//! Sleeps until it receives `CTRL_BREAK`, then exits promptly. Used instead of
//! `ping.exe`, which ignores console control events.

use std::sync::atomic::{AtomicBool, Ordering};
use std::thread;
use std::time::Duration;
use windows_sys::Win32::Foundation::TRUE;
use windows_sys::Win32::System::Console::{CTRL_BREAK_EVENT, CTRL_C_EVENT, SetConsoleCtrlHandler};

static STOP: AtomicBool = AtomicBool::new(false);

unsafe extern "system" fn on_console_ctrl(ctrl: u32) -> i32 {
    if ctrl == CTRL_BREAK_EVENT || ctrl == CTRL_C_EVENT {
        STOP.store(true, Ordering::Relaxed);
        return TRUE;
    }
    0
}

fn main() {
    unsafe {
        if SetConsoleCtrlHandler(Some(on_console_ctrl), 1) == 0 {
            std::process::exit(2);
        }
    }
    while !STOP.load(Ordering::Relaxed) {
        thread::sleep(Duration::from_millis(10));
    }
}
