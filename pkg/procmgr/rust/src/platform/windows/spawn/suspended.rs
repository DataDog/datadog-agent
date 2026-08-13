// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use log::warn;
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::System::Threading::ResumeThread;

use crate::handle::ProcessHandle;
use crate::process::ManagedProcess;

use super::super::JobObject;
use super::super::win_handle::WinHandle;

pub(super) struct SuspendedChild {
    pid: u32,
    process: WinHandle,
    thread: WinHandle,
}

impl SuspendedChild {
    pub(super) fn new(pid: u32, process: HANDLE, thread: HANDLE) -> Self {
        Self {
            pid,
            process: WinHandle::new(process),
            thread: WinHandle::new(thread),
        }
    }

    pub(super) fn supervise(
        self,
        process: &mut ManagedProcess,
        job: JobObject,
    ) -> Result<ProcessHandle> {
        let process_name = process.name();
        if let Err(e) = job.assign_process(self.pid) {
            warn!("[{process_name}] failed to assign to job object: {e:#}");
            self.abort_before_supervision(process_name, None);
            process.clear_windows_spawn_resources();
            bail!(
                "[{process_name}] failed to assign pid {} to supervision job: {e:#}",
                self.pid
            );
        }

        let previous_count = unsafe { ResumeThread(self.thread.as_handle()) };
        if previous_count == u32::MAX {
            self.abort_before_supervision(process_name, Some(&job));
            process.clear_windows_spawn_resources();
            bail!(
                "ResumeThread({}) failed: {}",
                self.pid,
                std::io::Error::last_os_error()
            );
        }

        process.set_job_object(job);
        Ok(ProcessHandle::from_raw(self.pid, self.process.raw()))
    }

    fn abort_before_supervision(self, process_name: &str, job: Option<&JobObject>) {
        request_termination(process_name, self.pid, self.process.as_handle(), job);
        if let Err(e) = wait_for_process_exit(process_name, self.pid, self.process.as_handle()) {
            warn!(
                "[{process_name}] failed to wait for suspended child (pid={}) termination: {e:#}",
                self.pid
            );
        }
    }
}

fn request_termination(
    process_name: &str,
    pid: u32,
    process_handle: HANDLE,
    job: Option<&JobObject>,
) {
    if let Some(job) = job {
        if let Err(e) = job.terminate() {
            warn!(
                "[{process_name}] failed to terminate suspended child (pid={pid}) via job after spawn failure: {e:#}"
            );
            if let Err(e) = terminate_process_handle(process_name, pid, process_handle) {
                warn!(
                    "[{process_name}] failed to terminate suspended child (pid={pid}) directly after job terminate failure: {e:#}"
                );
            }
        }
        return;
    }

    if let Err(e) = terminate_process_handle(process_name, pid, process_handle) {
        warn!(
            "[{process_name}] failed to terminate unsupervised suspended child (pid={pid}): {e:#}"
        );
    }
}

fn terminate_process_handle(process_name: &str, pid: u32, process_handle: HANDLE) -> Result<()> {
    use windows_sys::Win32::System::Threading::TerminateProcess;

    let ok = unsafe { TerminateProcess(process_handle, 1) };
    if ok == 0 {
        bail!(
            "[{process_name}] TerminateProcess({pid}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(())
}

fn wait_for_process_exit(process_name: &str, pid: u32, process_handle: HANDLE) -> Result<()> {
    use windows_sys::Win32::System::Threading::{
        GetExitCodeProcess, INFINITE, WaitForSingleObject,
    };

    const WAIT_OBJECT_0: u32 = 0;
    const WAIT_FAILED: u32 = 0xFFFF_FFFF;

    let wait_result = unsafe { WaitForSingleObject(process_handle, INFINITE) };
    if wait_result == WAIT_FAILED {
        bail!(
            "[{process_name}] WaitForSingleObject({pid}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    if wait_result != WAIT_OBJECT_0 {
        bail!(
            "[{process_name}] WaitForSingleObject({pid}) returned unexpected status: {wait_result}"
        );
    }

    let mut exit_code: u32 = 0;
    let ok = unsafe { GetExitCodeProcess(process_handle, &mut exit_code) };
    if ok == 0 {
        bail!(
            "[{process_name}] GetExitCodeProcess({pid}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(())
}
