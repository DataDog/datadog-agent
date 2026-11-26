// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::System::Console::{STD_ERROR_HANDLE, STD_OUTPUT_HANDLE};

use crate::spawn::SpawnRequest;

use super::super::wide::{NullTerminatedWide, WideEnvBlock};
use super::credential::SpawnCredential;
use super::stdio::{
    MappedStdioHandle, discard_off_thread, map_stdio_handle_nul, map_stdio_setting,
};
use super::win32::{build_windows_command_line, env_block_from_baseline_plus_overrides};

/// Hands a value to `discard` when dropped, unless it was released first. `prepare` holds each
/// opened log handle this way: if a later step fails, the handle may be the last reference to a
/// remote file, and closing that can block the spawn thread.
struct DiscardOnDrop<T> {
    value: Option<T>,
    discard: fn(T),
}

impl<T> DiscardOnDrop<T> {
    fn new(value: T, discard: fn(T)) -> Self {
        Self {
            value: Some(value),
            discard,
        }
    }

    fn release(mut self) -> T {
        self.value.take().expect("held until released")
    }
}

impl<T> Drop for DiscardOnDrop<T> {
    fn drop(&mut self) {
        if let Some(value) = self.value.take() {
            (self.discard)(value);
        }
    }
}

/// Inheritable stdio handles passed to `CreateProcess*`.
///
/// Owns [`MappedStdioHandle`] wrappers so handles stay open until after `CreateProcess*`
/// returns (the wrappers close them on drop).
pub(crate) struct SpawnStdio {
    stdin: MappedStdioHandle,
    stdout: MappedStdioHandle,
    stderr: MappedStdioHandle,
}

impl SpawnStdio {
    pub(crate) fn stdin(&self) -> HANDLE {
        self.stdin.raw()
    }

    pub(crate) fn stdout(&self) -> HANDLE {
        self.stdout.raw()
    }

    pub(crate) fn stderr(&self) -> HANDLE {
        self.stderr.raw()
    }
}

/// Win32-owned inputs for `CreateProcessW` / `CreateProcessAsUserW`.
///
/// Built from a [`SpawnRequest`] immediately before process creation. Pointers returned by
/// [`Self::command_line_mut_ptr`], [`Self::env_ptr`], and [`Self::cwd_ptr`] must remain valid
/// until the corresponding `CreateProcess*` call returns.
pub(crate) struct SpawnInputs {
    command_line: NullTerminatedWide,
    current_dir: Option<NullTerminatedWide>,
    env_block: WideEnvBlock,
    pub stdio: SpawnStdio,
}

impl SpawnInputs {
    pub(crate) fn prepare(
        process_name: &str,
        request: &SpawnRequest,
        credential: &SpawnCredential,
        env_token: HANDLE,
    ) -> Result<Self> {
        // On a failure below the guards hand stderr off before stdout. Each handle closes on its
        // own thread, so the order the closes finish in is not promised.
        let stdout_handle = DiscardOnDrop::new(
            map_stdio_setting(
                process_name,
                request.stdout_setting(),
                STD_OUTPUT_HANDLE,
                credential,
            )?,
            discard_off_thread,
        );
        let stderr_handle = DiscardOnDrop::new(
            map_stdio_setting(
                process_name,
                request.stderr_setting(),
                STD_ERROR_HANDLE,
                credential,
            )?,
            discard_off_thread,
        );
        let stdin_handle = map_stdio_handle_nul()?;

        let command_line = build_windows_command_line(request.command(), request.args());
        let command_line = NullTerminatedWide::from_os_str(std::ffi::OsStr::new(&command_line));

        let current_dir = request
            .working_dir()
            .map(|d| NullTerminatedWide::from_os_str(d.as_os_str()));

        let env_block = env_block_from_baseline_plus_overrides(process_name, env_token, request)?;

        Ok(Self {
            command_line,
            current_dir,
            env_block,
            stdio: SpawnStdio {
                stdin: stdin_handle,
                stdout: stdout_handle.release(),
                stderr: stderr_handle.release(),
            },
        })
    }

    pub(crate) fn command_line_mut_ptr(&mut self) -> *mut u16 {
        self.command_line.as_mut_ptr()
    }

    pub(crate) fn env_ptr(&self) -> *const std::ffi::c_void {
        self.env_block.as_ptr()
    }

    pub(crate) fn cwd_ptr(&self) -> *const u16 {
        self.current_dir
            .as_ref()
            .map(NullTerminatedWide::as_ptr)
            .unwrap_or(std::ptr::null())
    }
}

#[cfg(test)]
mod tests {
    use super::DiscardOnDrop;
    use std::sync::{Arc, Mutex};

    struct Probe(Arc<Mutex<Vec<&'static str>>>, &'static str);

    fn record(probe: Probe) {
        probe.0.lock().unwrap().push(probe.1);
    }

    #[test]
    fn a_failure_hands_off_stderr_before_stdout() {
        let seen = Arc::new(Mutex::new(Vec::new()));
        {
            let _stdout = DiscardOnDrop::new(Probe(Arc::clone(&seen), "stdout"), record);
            let _stderr = DiscardOnDrop::new(Probe(Arc::clone(&seen), "stderr"), record);
        }
        assert_eq!(*seen.lock().unwrap(), ["stderr", "stdout"]);
    }

    #[test]
    fn a_released_value_is_not_discarded() {
        let seen = Arc::new(Mutex::new(Vec::new()));
        let held = DiscardOnDrop::new(Probe(Arc::clone(&seen), "stdout"), record);
        let probe = held.release();
        assert!(seen.lock().unwrap().is_empty());
        assert_eq!(probe.1, "stdout");
    }
}
