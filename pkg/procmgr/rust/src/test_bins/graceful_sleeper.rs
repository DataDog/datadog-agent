// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Test-only child process for graceful-stop integration tests on Windows.
//!
//! Sleeps until it receives `CTRL_BREAK`, then exits promptly. Used instead of
//! `ping.exe`, which ignores console control events.

use windows_sys::Win32::Foundation::TRUE;
use windows_sys::Win32::System::Console::{
    CTRL_BREAK_EVENT, CTRL_C_EVENT, SetConsoleCtrlHandler,
};

unsafe extern "system" fn on_console_ctrl(ctrl: u32) -> i32 {
    if ctrl == CTRL_BREAK_EVENT || ctrl == CTRL_C_EVENT {
        // Exit from a worker thread: calling exit() inside the handler is unsafe.
        std::thread::spawn(|| std::process::exit(0));
        return TRUE;
    }
    0
}

fn main() {
    unsafe {
        SetConsoleCtrlHandler(Some(on_console_ctrl), 1);
    }
    std::thread::park();
}
