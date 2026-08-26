// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ffi::OsStr;
use std::io;
use std::os::windows::ffi::OsStrExt;
use std::os::windows::io::RawHandle;
use std::path::Path;
use std::time::Duration;
use tokio::net::windows::named_pipe::NamedPipeClient;
use windows_sys::Win32::Foundation::INVALID_HANDLE_VALUE;
use windows_sys::Win32::Storage::FileSystem::{
    CreateFileW, FILE_ATTRIBUTE_NORMAL, FILE_FLAG_OVERLAPPED, FILE_GENERIC_READ, FILE_WRITE_DATA,
    OPEN_EXISTING, SECURITY_IDENTIFICATION, SECURITY_SQOS_PRESENT,
};

pub type IpcStream = NamedPipeClient;

const PIPE_BUSY_RETRIES: u32 = 5;
const PIPE_BUSY_BACKOFF_MS: u64 = 50;

// Client-side pair of `AGENT_PIPE_CLIENT_ACCESS_MASK` in
// `dd-procmgrd/src/platform/windows/pipe_security.rs` (server ACE). Match
// FILE_GENERIC_READ | FILE_WRITE_DATA. Do not use FILE_GENERIC_WRITE (includes
// FILE_CREATE_PIPE_INSTANCE on named pipes).
const PIPE_CLIENT_DESIRED_ACCESS: u32 = FILE_GENERIC_READ | FILE_WRITE_DATA;
const PIPE_CLIENT_FLAGS: u32 =
    FILE_FLAG_OVERLAPPED | SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION | FILE_ATTRIBUTE_NORMAL;

pub async fn connect(path: &Path) -> io::Result<IpcStream> {
    open_with_retry(path.as_os_str()).await
}

async fn open_with_retry(name: &OsStr) -> io::Result<IpcStream> {
    let mut backoff = PIPE_BUSY_BACKOFF_MS;
    for attempt in 0..PIPE_BUSY_RETRIES {
        match open_pipe_client(name) {
            Ok(client) => return Ok(client),
            Err(error)
                if error.raw_os_error()
                    == Some(windows_sys::Win32::Foundation::ERROR_PIPE_BUSY as i32)
                    && attempt + 1 < PIPE_BUSY_RETRIES =>
            {
                tokio::time::sleep(Duration::from_millis(backoff)).await;
                backoff *= 2;
            }
            Err(error) => return Err(error),
        }
    }
    unreachable!()
}

fn open_pipe_client(name: &OsStr) -> io::Result<IpcStream> {
    let name_wide = null_terminated_wide(name);
    let handle = unsafe {
        CreateFileW(
            name_wide.as_ptr(),
            PIPE_CLIENT_DESIRED_ACCESS,
            0,
            std::ptr::null(),
            OPEN_EXISTING,
            PIPE_CLIENT_FLAGS,
            std::ptr::null_mut(),
        )
    };
    if handle == INVALID_HANDLE_VALUE {
        return Err(io::Error::last_os_error());
    }

    unsafe { NamedPipeClient::from_raw_handle(handle as RawHandle) }
}

fn null_terminated_wide(value: &OsStr) -> Vec<u16> {
    value.encode_wide().chain(std::iter::once(0)).collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use windows_sys::Win32::Storage::FileSystem::{FILE_CREATE_PIPE_INSTANCE, FILE_GENERIC_WRITE};

    #[test]
    fn pipe_client_desired_access_excludes_create_pipe_instance() {
        assert_eq!(
            PIPE_CLIENT_DESIRED_ACCESS & FILE_CREATE_PIPE_INSTANCE,
            0,
            "client open must not require create-instance rights"
        );
        assert_ne!(
            FILE_GENERIC_WRITE & FILE_CREATE_PIPE_INSTANCE,
            0,
            "sanity: generic write is what we are avoiding"
        );
    }
}
