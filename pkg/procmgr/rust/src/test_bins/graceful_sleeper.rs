// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Test child for Windows graceful-stop integration tests.
//!
//! Managed children use `CREATE_NO_WINDOW` and `stdout: null`, so this process starts
//! without a console unless we allocate one. Without a console, `CTRL_BREAK` cannot
//! be delivered.

use std::sync::atomic::{AtomicBool, Ordering};
use std::thread;
use std::time::Duration;
use windows_sys::Win32::Foundation::TRUE;
use windows_sys::Win32::System::Console::{
    AllocConsole, CTRL_BREAK_EVENT, GetConsoleCP, GetConsoleWindow, SetConsoleCtrlHandler,
};

static STOP: AtomicBool = AtomicBool::new(false);

unsafe extern "system" fn on_console_ctrl(ctrl: u32) -> i32 {
    if ctrl == CTRL_BREAK_EVENT {
        STOP.store(true, Ordering::Release);
        return TRUE;
    }
    0
}

fn ensure_console() {
    unsafe {
        if !GetConsoleWindow().is_null() {
            return;
        }
        if AllocConsole() == 0 && GetConsoleCP() == 0 {
            std::process::exit(3);
        }
    }
}

fn main() {
    ensure_console();
    unsafe {
        if SetConsoleCtrlHandler(Some(on_console_ctrl), 1) == 0 {
            std::process::exit(2);
        }
    }
    while !STOP.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(10));
    }
}
