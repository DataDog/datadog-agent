// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Shared `CreateProcess*` pipeline: job-list at create time, post-assign fallback, resume.

use std::mem;

use anyhow::{Result, bail};
use windows_sys::Win32::Foundation::{CloseHandle, ERROR_INVALID_PARAMETER};
use windows_sys::Win32::System::Threading::{
    CreateProcessAsUserW, CreateProcessW, PROCESS_INFORMATION,
};

use crate::handle::ProcessHandle;

use super::super::JobObject;
use super::super::job_object::current_process_in_job;
use super::super::process::terminate_process;
use super::inputs::SpawnInputs;
use super::startup_info_ex::StartupInfoEx;
use super::win32::{ManagedProcessCreationFlags, resume_child_primary_thread};

/// Which Win32 create API to invoke for a managed child spawn.
pub(super) enum CreateProcessInvoker {
    /// `CreateProcessW` with the supervisor's token inherited by the child.
    InheritSupervisor,
    /// `CreateProcessAsUserW` with a primary token from `LogonUser`.
    AsUser(windows_sys::Win32::Foundation::HANDLE),
}

impl CreateProcessInvoker {
    unsafe fn invoke(
        &self,
        inputs: &mut SpawnInputs,
        startup_info: &mut windows_sys::Win32::System::Threading::STARTUPINFOW,
        creation_flags: u32,
        process_info: &mut PROCESS_INFORMATION,
    ) -> i32 {
        unsafe {
            match self {
                Self::InheritSupervisor => CreateProcessW(
                    std::ptr::null(),
                    inputs.command_line_mut_ptr(),
                    std::ptr::null(),
                    std::ptr::null(),
                    1,
                    creation_flags,
                    inputs.env_ptr(),
                    inputs.cwd_ptr(),
                    startup_info,
                    process_info,
                ),
                Self::AsUser(token) => CreateProcessAsUserW(
                    *token,
                    std::ptr::null(),
                    inputs.command_line_mut_ptr(),
                    std::ptr::null(),
                    std::ptr::null(),
                    1,
                    creation_flags,
                    inputs.env_ptr(),
                    inputs.cwd_ptr(),
                    startup_info,
                    process_info,
                ),
            }
        }
    }
}

/// Spawn a managed child: try job-list at create time, then post-assign fallback.
pub(super) fn spawn_managed_child(
    process_name: &str,
    inputs: &mut SpawnInputs,
    job: &JobObject,
    invoker: CreateProcessInvoker,
) -> Result<ProcessHandle> {
    if !current_process_in_job() {
        let mut startup_info = StartupInfoEx::with_stdio_and_job(
            inputs.stdio.stdin(),
            inputs.stdio.stdout(),
            inputs.stdio.stderr(),
            job.raw_handle(),
        )?;
        let mut process_info: PROCESS_INFORMATION = unsafe { mem::zeroed() };
        let ok = unsafe {
            invoker.invoke(
                inputs,
                startup_info.startup_info(),
                ManagedProcessCreationFlags::JobListAtCreate.bits(),
                &mut process_info,
            )
        };
        if ok != 0 {
            return finish_managed_spawn(process_name, process_info);
        }
        let err = std::io::Error::last_os_error();
        if err.raw_os_error() != Some(ERROR_INVALID_PARAMETER as i32) {
            match invoker {
                CreateProcessInvoker::InheritSupervisor => {
                    bail!("[{process_name}] CreateProcessW failed: {err}");
                }
                CreateProcessInvoker::AsUser(_) => {
                    bail!("[{process_name}] CreateProcessAsUserW failed: {err}");
                }
            }
        }
    }

    spawn_post_assign(process_name, inputs, job, invoker)
}

fn spawn_post_assign(
    process_name: &str,
    inputs: &mut SpawnInputs,
    job: &JobObject,
    invoker: CreateProcessInvoker,
) -> Result<ProcessHandle> {
    let mut startup_info = StartupInfoEx::with_stdio_handles(
        inputs.stdio.stdin(),
        inputs.stdio.stdout(),
        inputs.stdio.stderr(),
    )?;
    let mut process_info: PROCESS_INFORMATION = unsafe { mem::zeroed() };
    let ok = unsafe {
        invoker.invoke(
            inputs,
            startup_info.startup_info(),
            ManagedProcessCreationFlags::PostAssign.bits(),
            &mut process_info,
        )
    };
    if ok == 0 {
        let err = std::io::Error::last_os_error();
        match invoker {
            CreateProcessInvoker::InheritSupervisor => {
                bail!("[{process_name}] CreateProcessW failed: {err}");
            }
            CreateProcessInvoker::AsUser(_) => {
                bail!("[{process_name}] CreateProcessAsUserW failed: {err}");
            }
        }
    }

    if let Err(e) = job.assign_process(process_info.dwProcessId) {
        abort_suspended_spawn(&process_info);
        return Err(anyhow::anyhow!(
            "[{process_name}] post-create job assignment failed: {e}"
        ));
    }

    if let Err(e) =
        resume_child_primary_thread(process_name, process_info.dwProcessId, process_info.hThread)
    {
        abort_suspended_spawn(&process_info);
        return Err(e);
    }

    finish_managed_spawn(process_name, process_info)
}

fn finish_managed_spawn(
    _process_name: &str,
    process_info: PROCESS_INFORMATION,
) -> Result<ProcessHandle> {
    let pid = process_info.dwProcessId;
    let handle = match ProcessHandle::from_borrowed(pid, process_info.hProcess) {
        Ok(handle) => handle,
        Err(e) => {
            let _ = terminate_process(process_info.hProcess);
            unsafe {
                CloseHandle(process_info.hProcess);
                CloseHandle(process_info.hThread);
            }
            return Err(e);
        }
    };

    unsafe {
        CloseHandle(process_info.hProcess);
        CloseHandle(process_info.hThread);
    }

    Ok(handle)
}

fn abort_suspended_spawn(process_info: &PROCESS_INFORMATION) {
    let _ = terminate_process(process_info.hProcess);
    unsafe {
        CloseHandle(process_info.hProcess);
        CloseHandle(process_info.hThread);
    }
}
