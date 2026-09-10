// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::sync::Mutex;
use windows_sys::Win32::Foundation::{INVALID_HANDLE_VALUE, TRUE};
use windows_sys::Win32::System::Console::{
    AttachConsole, CTRL_BREAK_EVENT, FreeConsole, GenerateConsoleCtrlEvent, GetStdHandle,
    STD_ERROR_HANDLE, STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, SetConsoleCtrlHandler, SetStdHandle,
};
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW,
};

static CONSOLE_LOCK: Mutex<()> = Mutex::new(());

pub(crate) fn console_lock() -> std::sync::MutexGuard<'static, ()> {
    CONSOLE_LOCK.lock().expect("console lock poisoned")
}

fn std_handle_inheritable(handle: u32) -> bool {
    unsafe {
        let h = GetStdHandle(handle);
        !h.is_null() && h != INVALID_HANDLE_VALUE
    }
}

pub fn stdout_inheritable() -> bool {
    std_handle_inheritable(STD_OUTPUT_HANDLE)
}

pub fn stderr_inheritable() -> bool {
    std_handle_inheritable(STD_ERROR_HANDLE)
}

fn reset_std_handles() {
    unsafe {
        for std_handle in [STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, STD_ERROR_HANDLE] {
            let _ = SetStdHandle(std_handle, std::ptr::null_mut());
        }
    }
}

fn detach_console() {
    unsafe {
        let _ = FreeConsole();
    }
    reset_std_handles();
}

/// Give the child its own process group and console for CTRL_BREAK graceful shutdown.
pub fn setup_process_group(cmd: &mut tokio::process::Command) {
    cmd.creation_flags(CREATE_NEW_PROCESS_GROUP | CREATE_NEW_CONSOLE | CREATE_NO_WINDOW);
}

unsafe extern "system" fn ignore_console_ctrl_events(_: u32) -> i32 {
    TRUE
}

pub fn send_graceful_stop(pid: u32) -> Result<()> {
    let _guard = console_lock();

    unsafe {
        detach_console();
        if AttachConsole(pid) == 0 {
            anyhow::bail!(
                "AttachConsole({pid}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        struct DetachOnDrop;
        impl Drop for DetachOnDrop {
            fn drop(&mut self) {
                detach_console();
            }
        }
        let _detach = DetachOnDrop;

        if SetConsoleCtrlHandler(Some(ignore_console_ctrl_events), 1) == 0 {
            anyhow::bail!("SetConsoleCtrlHandler: {}", std::io::Error::last_os_error());
        }
        let ok = GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid);
        if SetConsoleCtrlHandler(Some(ignore_console_ctrl_events), 0) == 0 {
            log::warn!(
                "SetConsoleCtrlHandler(remove console ctrl ignore handler) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        if ok == 0 {
            anyhow::bail!(
                "GenerateConsoleCtrlEvent(CTRL_BREAK, {pid}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
    }
    Ok(())
}

pub fn send_force_kill(pid: u32) -> Result<()> {
    super::process::terminate_process_by_pid(pid)
}

pub fn last_signal(_status: &std::process::ExitStatus) -> Option<i32> {
    None
}
