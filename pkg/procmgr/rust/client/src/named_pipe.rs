// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ffi::OsStr;
use std::io;
use std::path::Path;
use std::time::Duration;
use tokio::net::windows::named_pipe::ClientOptions;
use windows_sys::Win32::Foundation::ERROR_PIPE_BUSY;

pub type IpcStream = tokio::net::windows::named_pipe::NamedPipeClient;

const PIPE_BUSY_RETRIES: u32 = 5;
const PIPE_BUSY_BACKOFF_MS: u64 = 50;

pub async fn connect(path: &Path) -> io::Result<IpcStream> {
    open_with_retry(path.as_os_str()).await
}

/// Open a named-pipe client, retrying when every server instance is busy.
async fn open_with_retry(name: &OsStr) -> io::Result<IpcStream> {
    let mut backoff = PIPE_BUSY_BACKOFF_MS;
    for attempt in 0..PIPE_BUSY_RETRIES {
        match ClientOptions::new().open(name) {
            Ok(client) => return Ok(client),
            Err(error)
                if error.raw_os_error() == Some(ERROR_PIPE_BUSY as i32)
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
