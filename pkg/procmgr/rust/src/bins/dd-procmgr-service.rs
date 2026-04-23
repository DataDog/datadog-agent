// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows service entry point for dd-procmgr-service.
//!
//! When launched by the Windows Service Control Manager (SCM), this binary
//! registers with SCM via `StartServiceCtrlDispatcherW` and runs the process
//! manager inside a tokio runtime on a worker thread. When launched
//! interactively (not by SCM), it falls back to console mode.
//!
//! On non-Windows platforms this binary exits immediately — use `dd-procmgrd`
//! instead.

fn main() {
    #[cfg(windows)]
    {
        if let Err(e) = dd_procmgrd::service::run_as_service() {
            eprintln!("dd-procmgr-service failed: {e:#}");
            std::process::exit(1);
        }
    }
    #[cfg(not(windows))]
    {
        eprintln!("dd-procmgr-service is Windows-only; use dd-procmgrd on Unix");
        std::process::exit(1);
    }
}
