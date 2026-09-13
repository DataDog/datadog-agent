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
use super::stdio::{map_stdio_handle_nul, map_stdio_setting};
use super::win32::{build_windows_command_line, env_block_from_baseline_plus_overrides};

/// Inheritable stdio handles passed to `CreateProcess*`.
pub(crate) struct SpawnStdio {
    pub stdin: HANDLE,
    pub stdout: HANDLE,
    pub stderr: HANDLE,
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
        let stdout_handle = map_stdio_setting(
            process_name,
            request.stdout_setting(),
            STD_OUTPUT_HANDLE,
            credential,
        )?;
        let stderr_handle = map_stdio_setting(
            process_name,
            request.stderr_setting(),
            STD_ERROR_HANDLE,
            credential,
        )?;
        let stdin_handle = map_stdio_handle_nul()?;

        let command_line = build_windows_command_line(request.command(), request.args());
        let command_line = NullTerminatedWide::from_os_str(std::ffi::OsStr::new(&command_line));

        let current_dir = request
            .working_dir()
            .map(|d| NullTerminatedWide::from_os_str(d.as_os_str()));

        let env_block =
            env_block_from_baseline_plus_overrides(process_name, env_token, request.env())?;

        Ok(Self {
            command_line,
            current_dir,
            env_block,
            stdio: SpawnStdio {
                stdin: stdin_handle.raw(),
                stdout: stdout_handle.raw(),
                stderr: stderr_handle.raw(),
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
