// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Local-socket client transport shared by the process-manager and executor
//! gRPC clients: a Unix domain socket on Linux, a named pipe on Windows.
//! par-control is a pure client, so this only ever dials an existing endpoint.

use std::path::{Path, PathBuf};
use std::time::Duration;

/// Placeholder URI for the tonic Endpoint when connecting over a local socket.
/// The actual address is irrelevant because `connect_with_connector` bypasses it.
const DUMMY_ENDPOINT: &str = "http://[::]:50051";

/// Bounds the dial itself. A local socket either answers promptly or is broken,
/// and on Windows a named pipe whose every instance is busy would otherwise
/// block indefinitely.
const CONNECT_TIMEOUT: Duration = Duration::from_secs(5);

/// Build a lazily-connecting gRPC channel to a server on the given Unix domain
/// socket. Lazy connection matters for the executor, whose socket does not exist
/// until the control plane has started it; the connection is established on the
/// first RPC and re-dialed after the executor is restarted.
pub fn connect_lazy(path: &Path) -> tonic::transport::Channel {
    let path: PathBuf = path.to_path_buf();
    tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT)
        .connect_timeout(CONNECT_TIMEOUT)
        .connect_with_connector_lazy(tower::service_fn(move |_| {
            let p = path.clone();
            async move { connect_stream(p).await.map(hyper_util::rt::TokioIo::new) }
        }))
}

/// Like [`connect_lazy`] but wraps the Unix-socket stream in mTLS using the agent
/// IPC cert at `ipc_cert_file` (the control<->executor channel).
///
/// The connector is rebuilt from the file on each connection rather than once at
/// startup. par-control is `auto_start: true` under the process manager, so it can
/// come up before anything has generated the IPC cert — possibly before the very
/// executor it is about to launch, which creates the cert when missing. A
/// connector built once at startup would turn that ordering into a permanent
/// failure instead of a transient one, and would also miss a rotated cert.
/// Connections happen at most per dispatch and parsing a small PEM is cheap, so
/// caching buys nothing.
pub fn connect_lazy_tls(path: &Path, ipc_cert_file: &Path) -> tonic::transport::Channel {
    let path: PathBuf = path.to_path_buf();
    let ipc_cert_file: PathBuf = ipc_cert_file.to_path_buf();
    tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT)
        .connect_timeout(CONNECT_TIMEOUT)
        .connect_with_connector_lazy(tower::service_fn(move |_| {
            let p = path.clone();
            let cert = ipc_cert_file.clone();
            async move {
                let connector =
                    crate::tls::build_ipc_client_connector(&cert).map_err(std::io::Error::other)?;
                let stream = connect_stream(p).await?;
                // Domain is ignored: hostname verification is disabled for the
                // local socket (see tls::build_ipc_client_connector).
                let tls = connector
                    .connect("localhost", stream)
                    .await
                    .map_err(std::io::Error::other)?;
                Ok::<_, std::io::Error>(hyper_util::rt::TokioIo::new(tls))
            }
        }))
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
const PIPE_BUSY_RETRIES: u32 = 4;
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
    // One attempt per retry, then a final attempt whose error is propagated.
    for _ in 0..PIPE_BUSY_RETRIES {
        match ClientOptions::new().open(name) {
            Ok(client) => return Ok(client),
            Err(error) if error.raw_os_error() == Some(ERROR_PIPE_BUSY as i32) => {
                tokio::time::sleep(Duration::from_millis(backoff)).await;
                backoff *= 2;
            }
            Err(error) => return Err(error),
        }
    }
    ClientOptions::new().open(name)
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
