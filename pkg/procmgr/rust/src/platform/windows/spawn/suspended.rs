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
    pub(super) fn supervise(
        self,
        process: &mut ManagedProcess,
        job: JobObject,
    ) -> Result<ProcessHandle> {
        let process_name = process.name();
        let job_assigned = match job.assign_process(self.pid) {
            Ok(()) => true,
            Err(e) => {
                warn!("[{process_name}] failed to assign to job object: {e:#}");
                false
            }
        };

        let previous_count = unsafe { ResumeThread(self.thread.as_handle()) };
        if previous_count == u32::MAX {
            terminate_suspended_child(process_name, self.pid, job_assigned, &job);
            process.clear_windows_spawn_resources();
            bail!(
                "ResumeThread({}) failed: {}",
                self.pid,
                std::io::Error::last_os_error()
            );
        }

        if job_assigned {
            process.set_job_object(job);
        }
        Ok(ProcessHandle::from_raw(self.pid, self.process.raw()))
    }
}

fn terminate_suspended_child(process_name: &str, pid: u32, job_assigned: bool, job: &JobObject) {
    let result = if job_assigned {
        job.terminate()
    } else {
        super::super::send_force_kill(pid)
    };
    if let Err(e) = result {
        warn!(
            "[{process_name}] failed to terminate suspended child (pid={pid}) after ResumeThread failure: {e:#}"
        );
    }
}
