// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::sync::Mutex;
use std::time::Duration;
use windows_sys::Win32::Foundation::{INVALID_HANDLE_VALUE, TRUE};
use windows_sys::Win32::System::Console::{
    AttachConsole, CTRL_BREAK_EVENT, FreeConsole, GenerateConsoleCtrlEvent, GetStdHandle,
    STD_ERROR_HANDLE, STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, SetConsoleCtrlHandler, SetStdHandle,
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

unsafe extern "system" fn ignore_console_ctrl_events(ctrl: u32) -> i32 {
    if ctrl == CTRL_BREAK_EVENT { TRUE } else { 0 }
}

struct IgnoreCtrlGuard;

impl IgnoreCtrlGuard {
    fn install() -> Result<Self> {
        unsafe {
            if SetConsoleCtrlHandler(Some(ignore_console_ctrl_events), 1) == 0 {
                anyhow::bail!("SetConsoleCtrlHandler: {}", std::io::Error::last_os_error());
            }
        }
        Ok(Self)
    }
}

impl Drop for IgnoreCtrlGuard {
    fn drop(&mut self) {
        unsafe {
            if SetConsoleCtrlHandler(Some(ignore_console_ctrl_events), 0) == 0 {
                log::warn!(
                    "SetConsoleCtrlHandler(remove console ctrl ignore handler) failed: {}",
                    std::io::Error::last_os_error()
                );
            }
        }
    }
}

pub fn send_graceful_stop(pid: u32) -> Result<()> {
    let _guard = console_lock();

    unsafe {
        // Ignore CTRL_BREAK on the caller before attaching so we do not exit when the
        // signal is delivered to our process group.
        let _ignore_ctrl = IgnoreCtrlGuard::install()?;
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
        // Managed children are spawned with CREATE_NEW_PROCESS_GROUP, so pid == pgid.
        let ok = GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid);
        if ok == 0 {
            anyhow::bail!(
                "GenerateConsoleCtrlEvent(CTRL_BREAK, {pid}) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        std::thread::sleep(Duration::from_millis(200));
    }
    Ok(())
}

pub fn send_force_kill(pid: u32) -> Result<()> {
    super::process::terminate_process_by_pid(pid)
}

pub fn last_signal(_status: &std::process::ExitStatus) -> Option<i32> {
    None
}
