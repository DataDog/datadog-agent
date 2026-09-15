// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Test-only child process for graceful-stop integration tests on Windows.
//!
//! Allocates a console (managed children use `CREATE_NO_WINDOW`), registers a
//! `CTRL_BREAK` handler, then sleeps until signaled. Used instead of `ping.exe`,
//! which ignores console control events.

use std::sync::atomic::{AtomicBool, Ordering};
use std::thread;
use std::time::Duration;
use windows_sys::Win32::Foundation::TRUE;
use windows_sys::Win32::System::Console::{
    AllocConsole, CTRL_BREAK_EVENT, CTRL_C_EVENT, SetConsoleCtrlHandler,
};

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
        if AllocConsole() == 0 {
            eprintln!("AllocConsole failed: {}", std::io::Error::last_os_error());
            std::process::exit(3);
        }
        if SetConsoleCtrlHandler(Some(on_console_ctrl), 1) == 0 {
            eprintln!(
                "SetConsoleCtrlHandler failed: {}",
                std::io::Error::last_os_error()
            );
            std::process::exit(2);
        }
    }
    while !STOP.load(Ordering::Relaxed) {
        thread::sleep(Duration::from_millis(10));
    }
}
