// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Shared `CreateProcess*` pipeline with create-time `PROC_THREAD_ATTRIBUTE_JOB_LIST`.

use std::mem;

use anyhow::{Result, bail};
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW, CREATE_UNICODE_ENVIRONMENT,
    CreateProcessAsUserW, CreateProcessW, EXTENDED_STARTUPINFO_PRESENT, PROCESS_INFORMATION,
};

use crate::handle::ProcessHandle;

use super::super::JobObject;
use super::super::process::terminate_process;
use super::inputs::SpawnInputs;
use super::startup_info_ex::StartupInfoEx;

/// Spawn a managed child in the supervisor's security context (`CreateProcessW`).
pub(super) fn spawn_managed_child(
    process_name: &str,
    inputs: &mut SpawnInputs,
    job: &JobObject,
) -> Result<ProcessHandle> {
    create_managed_child(process_name, inputs, job, None)
}

/// Spawn a managed child with a primary token (`CreateProcessAsUserW`).
pub(super) fn spawn_managed_child_as_user(
    process_name: &str,
    inputs: &mut SpawnInputs,
    job: &JobObject,
    primary_token: HANDLE,
) -> Result<ProcessHandle> {
    create_managed_child(process_name, inputs, job, Some(primary_token))
}

fn create_managed_child(
    process_name: &str,
    inputs: &mut SpawnInputs,
    job: &JobObject,
    primary_token: Option<HANDLE>,
) -> Result<ProcessHandle> {
    let mut startup_info = StartupInfoEx::with_stdio_and_job(
        inputs.stdio.stdin(),
        inputs.stdio.stdout(),
        inputs.stdio.stderr(),
        job.raw_handle(),
    )?;
    let mut process_info: PROCESS_INFORMATION = unsafe { mem::zeroed() };
    // Allocate a private console so `AttachConsole` + CTRL_BREAK graceful shutdown work.
    // When both `CREATE_NEW_CONSOLE` and `CREATE_NO_WINDOW` are set, Windows keeps the
    // console allocated but suppresses the visible window.
    let creation_flags = CREATE_NEW_PROCESS_GROUP
        | CREATE_NEW_CONSOLE
        | CREATE_NO_WINDOW
        | CREATE_UNICODE_ENVIRONMENT
        | EXTENDED_STARTUPINFO_PRESENT;
    let ok = unsafe {
        match primary_token {
            None => CreateProcessW(
                std::ptr::null(),
                inputs.command_line_mut_ptr(),
                std::ptr::null(),
                std::ptr::null(),
                1,
                creation_flags,
                inputs.env_ptr(),
                inputs.cwd_ptr(),
                startup_info.startup_info(),
                &mut process_info,
            ),
            Some(token) => CreateProcessAsUserW(
                token,
                std::ptr::null(),
                inputs.command_line_mut_ptr(),
                std::ptr::null(),
                std::ptr::null(),
                1,
                creation_flags,
                inputs.env_ptr(),
                inputs.cwd_ptr(),
                startup_info.startup_info(),
                &mut process_info,
            ),
        }
    };
    if ok == 0 {
        let api_name = if primary_token.is_some() {
            "CreateProcessAsUserW"
        } else {
            "CreateProcessW"
        };
        bail!(
            "[{process_name}] {api_name} failed: {}",
            std::io::Error::last_os_error()
        );
    }

    let pid = process_info.dwProcessId;
    let handle = ProcessHandle::from_borrowed(pid, process_info.hProcess).inspect_err(|_| {
        let _ = terminate_process(process_info.hProcess);
    });
    unsafe {
        CloseHandle(process_info.hProcess);
        CloseHandle(process_info.hThread);
    }
    handle
}
