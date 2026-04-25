// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows Service Control Manager (SCM) adapter for dd-procmgrd.
//!
//! Implements the SCM protocol using `windows-sys` directly:
//! - `StartServiceCtrlDispatcherW` registers with SCM (blocks the calling thread).
//! - `RegisterServiceCtrlHandlerExW` installs our control-event callback.
//! - `SetServiceStatus` reports lifecycle transitions.
//!
//! When launched interactively (not by SCM), `run_as_service` detects
//! `ERROR_FAILED_SERVICE_CONTROLLER_CONNECT` and falls back to console mode.
//!
//! The control handler bridges SCM stop events into the tokio runtime via
//! [`crate::platform::shutdown_notify()`], so `ProcessManager::run()` shuts
//! down without any API changes.

use std::ffi::c_void;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicPtr, Ordering};
use std::time::{Duration, Instant};

use anyhow::{Context, Result, bail};
use log::{error, info, warn};
use windows_sys::Win32::Foundation::{
    ERROR_FAILED_SERVICE_CONTROLLER_CONNECT, GetLastError, NO_ERROR,
};
use windows_sys::Win32::System::Services::{
    RegisterServiceCtrlHandlerExW, SERVICE_ACCEPT_PRESHUTDOWN, SERVICE_ACCEPT_SHUTDOWN,
    SERVICE_ACCEPT_STOP, SERVICE_CONTROL_INTERROGATE, SERVICE_CONTROL_PRESHUTDOWN,
    SERVICE_CONTROL_SHUTDOWN, SERVICE_CONTROL_STOP, SERVICE_RUNNING, SERVICE_START_PENDING,
    SERVICE_STATUS, SERVICE_STOP_PENDING, SERVICE_STOPPED, SERVICE_TABLE_ENTRYW,
    SERVICE_WIN32_OWN_PROCESS, SetServiceStatus, StartServiceCtrlDispatcherW,
};

use crate::config::YamlConfigLoader;
use crate::manager::ProcessManager;
use crate::platform;
use crate::uuid_gen::V4UuidGenerator;

const SERVICE_NAME: &str = "dd-procmgr-service";
/// SCM wait-hint: advisory value telling SCM how long to wait before
/// considering the stop stalled. Set generously so that
/// `ProcessManager::shutdown` can gracefully stop + force-kill every
/// child without SCM intervening. The actual shutdown budget is driven
/// by each child's `stop_timeout` (default 90s) + `FORCE_KILL_TIMEOUT`
/// (10s).
///
/// NOTE: some Windows tools ignore this hint entirely (see WINA-180).
/// Do not rely on it for correctness — the shutdown logic must be
/// self-contained with its own timeouts.
const SCM_STOP_WAIT_HINT: Duration = Duration::from_secs(180);
const EXIT_GATE: Duration = Duration::from_secs(5);

/// Global status handle set by `service_main` before use in the control handler.
/// On the GNU ABI `SERVICE_STATUS_HANDLE` is `*mut c_void`, not `isize`.
static STATUS_HANDLE: AtomicPtr<c_void> = AtomicPtr::new(std::ptr::null_mut());

/// Encode `SERVICE_NAME` as a null-terminated UTF-16 slice at compile time.
/// `StartServiceCtrlDispatcherW` and `RegisterServiceCtrlHandlerExW` require
/// LPCWSTR pointers.
fn service_name_wide() -> Vec<u16> {
    SERVICE_NAME
        .encode_utf16()
        .chain(std::iter::once(0))
        .collect()
}

fn set_service_status(state: u32, controls: u32, exit_code: u32, wait_hint_ms: u32) {
    let handle = STATUS_HANDLE.load(Ordering::SeqCst);
    if handle.is_null() {
        return;
    }
    let status = SERVICE_STATUS {
        dwServiceType: SERVICE_WIN32_OWN_PROCESS,
        dwCurrentState: state,
        dwControlsAccepted: controls,
        dwWin32ExitCode: exit_code,
        dwServiceSpecificExitCode: 0,
        dwCheckPoint: 0,
        dwWaitHint: wait_hint_ms,
    };
    unsafe {
        SetServiceStatus(handle, &status);
    }
}

/// SCM control-event callback. Runs on an OS thread managed by SCM, not
/// inside the tokio runtime.
unsafe extern "system" fn ctrl_handler(
    control: u32,
    _event_type: u32,
    _event_data: *mut c_void,
    _context: *mut c_void,
) -> u32 {
    match control {
        SERVICE_CONTROL_STOP | SERVICE_CONTROL_SHUTDOWN | SERVICE_CONTROL_PRESHUTDOWN => {
            set_service_status(
                SERVICE_STOP_PENDING,
                0,
                NO_ERROR,
                SCM_STOP_WAIT_HINT.as_millis() as u32,
            );
            platform::shutdown_notify().notify_one();
            NO_ERROR
        }
        SERVICE_CONTROL_INTERROGATE => NO_ERROR,
        _ => NO_ERROR,
    }
}

/// Entry point called by SCM on a new thread via `StartServiceCtrlDispatcherW`.
unsafe extern "system" fn service_main(_argc: u32, _argv: *mut *mut u16) {
    let name = service_name_wide();

    let handle = unsafe {
        RegisterServiceCtrlHandlerExW(name.as_ptr(), Some(ctrl_handler), std::ptr::null_mut())
    };
    if handle.is_null() {
        error!("RegisterServiceCtrlHandlerExW failed: {}", unsafe {
            GetLastError()
        });
        return;
    }
    STATUS_HANDLE.store(handle, Ordering::SeqCst);

    set_service_status(SERVICE_START_PENDING, 0, NO_ERROR, 10_000);

    if let Err(e) = run_service_inner() {
        error!("service failed: {e:#}");
        set_service_status(SERVICE_STOPPED, 0, 1, 0);
        return;
    }

    set_service_status(SERVICE_STOPPED, 0, NO_ERROR, 0);
}

/// Default log file path, following the agent convention
/// (`C:\ProgramData\Datadog\logs\<service>.log`).
fn default_log_file() -> PathBuf {
    let base = std::env::var("ProgramData").unwrap_or_else(|_| r"C:\ProgramData".to_string());
    PathBuf::from(base).join(r"Datadog\logs\dd-procmgr.log")
}

/// Core service logic: creates the tokio runtime, runs ProcessManager, then
/// waits for the exit gate before reporting stopped.
fn run_service_inner() -> Result<()> {
    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PROCMGR",
        level: log::Level::Info,
        log_file: Some(default_log_file()),
    })
    .context("failed to initialize logging")?;

    info!("dd-procmgrd starting (SCM mode)");

    let runtime = tokio::runtime::Runtime::new().context("failed to create tokio runtime")?;

    set_service_status(
        SERVICE_RUNNING,
        SERVICE_ACCEPT_STOP | SERVICE_ACCEPT_SHUTDOWN | SERVICE_ACCEPT_PRESHUTDOWN,
        NO_ERROR,
        0,
    );

    let running_since = Instant::now();

    let result = runtime.block_on(async {
        let loader = Arc::new(YamlConfigLoader::from_env());
        let mgr = ProcessManager::new(loader, Arc::new(V4UuidGenerator));
        mgr.run().await
    });

    if let Err(ref e) = result {
        warn!("ProcessManager exited with error: {e:#}");
    }

    // Keep the service alive long enough for SCM to treat the start as
    // successful (mirrors Go's runTimeExitGate). If the process has
    // already been running longer than EXIT_GATE this is a no-op.
    if let Some(remaining) = EXIT_GATE.checked_sub(running_since.elapsed()) {
        std::thread::sleep(remaining);
    }

    result
}

/// Run as a Windows service. If launched interactively (not by SCM), falls
/// back to console mode so the binary remains debuggable.
pub fn run_as_service() -> Result<()> {
    let name = service_name_wide();

    let table: [SERVICE_TABLE_ENTRYW; 2] = [
        SERVICE_TABLE_ENTRYW {
            lpServiceName: name.as_ptr() as *mut u16,
            lpServiceProc: Some(service_main),
        },
        // Null-terminated sentinel entry.
        SERVICE_TABLE_ENTRYW {
            lpServiceName: std::ptr::null_mut(),
            lpServiceProc: None,
        },
    ];

    let ok = unsafe { StartServiceCtrlDispatcherW(table.as_ptr()) };
    if ok != 0 {
        return Ok(());
    }

    let err = unsafe { GetLastError() };
    if err == ERROR_FAILED_SERVICE_CONTROLLER_CONNECT {
        info!("not launched by SCM, falling back to console mode");
        return run_console_fallback();
    }

    bail!("StartServiceCtrlDispatcherW failed: error {err}");
}

/// Fallback console mode: runs the process manager directly without SCM,
/// useful for interactive debugging.
fn run_console_fallback() -> Result<()> {
    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PROCMGR",
        level: log::Level::Info,
        log_file: None,
    })
    .context("failed to initialize logging")?;

    info!("dd-procmgrd starting (console mode)");

    let runtime = tokio::runtime::Runtime::new().context("failed to create tokio runtime")?;
    runtime.block_on(async {
        let loader = Arc::new(YamlConfigLoader::from_env());
        let mgr = ProcessManager::new(loader, Arc::new(V4UuidGenerator));
        mgr.run().await
    })
}
