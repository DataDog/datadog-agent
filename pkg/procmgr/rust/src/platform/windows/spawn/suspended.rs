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

/// Child created with `CREATE_SUSPENDED`; resume only after job assignment.
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

    /// Assign the child to `job`, resume the initial thread, then store the job on `process`.
    /// Fails the spawn if job assignment fails so the child is never resumed unsupervised.
    pub(super) fn supervise(
        self,
        process: &mut ManagedProcess,
        job: JobObject,
    ) -> Result<ProcessHandle> {
        let process_name = process.name();
        if let Err(e) = job.assign_process(self.pid) {
            warn!("[{process_name}] failed to assign to job object: {e:#}");
            terminate_unassigned_suspended_child(process_name, self.pid);
            process.clear_windows_spawn_resources();
            bail!(
                "[{process_name}] failed to assign pid {} to supervision job: {e:#}",
                self.pid
            );
        }

        let previous_count = unsafe { ResumeThread(self.thread.as_handle()) };
        if previous_count == u32::MAX {
            terminate_suspended_child(process_name, self.pid, &job);
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
}

fn terminate_unassigned_suspended_child(process_name: &str, pid: u32) {
    if let Err(e) = super::super::send_force_kill(pid) {
        warn!(
            "[{process_name}] failed to terminate unsupervised suspended child (pid={pid}) after job assignment failure: {e:#}"
        );
    }
}

fn terminate_suspended_child(process_name: &str, pid: u32, job: &JobObject) {
    if let Err(e) = job.terminate() {
        warn!(
            "[{process_name}] failed to terminate suspended child (pid={pid}) after ResumeThread failure: {e:#}"
        );
    }
}
