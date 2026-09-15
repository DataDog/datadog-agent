// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Test-only child process for graceful-stop integration tests on Windows.
//!
//! Sleeps until it receives `CTRL_BREAK`, then exits via the default console handler.
//! Used instead of `ping.exe`, which ignores console control events.

use std::thread;
use std::time::Duration;

fn main() {
    loop {
        thread::sleep(Duration::from_millis(10));
    }
}
