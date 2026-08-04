// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Local transport for the dd-procmgrd gRPC client: a Unix domain socket on
//! Linux, a named pipe on Windows.

use std::path::{Path, PathBuf};

/// Ignored by the custom connector, but tonic requires a well-formed URI.
const DUMMY_ENDPOINT: &str = "http://[::]:50051";

/// Build a channel that connects lazily to dd-procmgrd's local socket.
pub fn connect_lazy(path: &Path) -> tonic::transport::Channel {
    let path = path.to_path_buf();
    tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT).connect_with_connector_lazy(
        tower::service_fn(move |_| {
            let path = path.clone();
            async move { connect_stream(path).await.map(hyper_util::rt::TokioIo::new) }
        }),
    )
}

#[cfg(unix)]
async fn connect_stream(path: PathBuf) -> std::io::Result<tokio::net::UnixStream> {
    tokio::net::UnixStream::connect(path).await
}

#[cfg(windows)]
async fn connect_stream(
    path: PathBuf,
) -> std::io::Result<tokio::net::windows::named_pipe::NamedPipeClient> {
    open_pipe_with_retry(path.as_os_str()).await
}

#[cfg(windows)]
const PIPE_BUSY_RETRIES: u32 = 5;
#[cfg(windows)]
const PIPE_BUSY_BACKOFF_MS: u64 = 50;

/// Open a named-pipe client, retrying on `ERROR_PIPE_BUSY`.
///
/// Every server instance may already be occupied when the client calls `open()`;
/// Windows named-pipe clients are expected to wait and retry in that case.
///
/// Deliberately a copy of `open_pipe_with_retry` in
/// `pkg/procmgr/rust/src/transport/named_pipe.rs`. Sharing it would mean
/// extracting a crate out of the shipped dd-procmgrd daemon; keep the two in
/// sync until that refactor happens.
#[cfg(windows)]
async fn open_pipe_with_retry(
    name: &std::ffi::OsStr,
) -> std::io::Result<tokio::net::windows::named_pipe::NamedPipeClient> {
    use tokio::net::windows::named_pipe::ClientOptions;
    use windows_sys::Win32::Foundation::ERROR_PIPE_BUSY;

    let mut backoff = PIPE_BUSY_BACKOFF_MS;
    for attempt in 0..PIPE_BUSY_RETRIES {
        match ClientOptions::new().open(name) {
            Ok(client) => return Ok(client),
            Err(error)
                if error.raw_os_error() == Some(ERROR_PIPE_BUSY as i32)
                    && attempt + 1 < PIPE_BUSY_RETRIES =>
            {
                tokio::time::sleep(std::time::Duration::from_millis(backoff)).await;
                backoff *= 2;
            }
            Err(error) => return Err(error),
        }
    }
    unreachable!("the last attempt either returns a client or propagates its error")
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A missing endpoint must surface the OS error so the caller can retry, and
    /// must never report `Unsupported` — which is how an unimplemented platform
    /// transport shows up.
    #[tokio::test]
    async fn missing_endpoint_reports_an_os_error() {
        let error = connect_stream(PathBuf::from(MISSING_ENDPOINT))
            .await
            .expect_err("connecting to a nonexistent endpoint should fail");
        assert_ne!(error.kind(), std::io::ErrorKind::Unsupported);
        assert!(error.raw_os_error().is_some(), "{error:?}");
    }

    #[cfg(unix)]
    const MISSING_ENDPOINT: &str = "/tmp/dd-procmgrd-does-not-exist.sock";
    #[cfg(windows)]
    const MISSING_ENDPOINT: &str = r"\\.\pipe\dd-procmgrd-does-not-exist";
}
