// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Rust client plumbing for the local dd-procmgrd gRPC API.
//!
//! This crate owns the process-manager protobuf bindings, endpoint resolution,
//! and platform connector. Daemon server transport and process supervision stay
//! in `dd-procmgrd`.

use anyhow::{Context as _, Result};
use std::path::{Path, PathBuf};
use std::time::Duration;

#[cfg(windows)]
mod named_pipe;
#[cfg(unix)]
mod uds;

#[cfg(not(bazel))]
pub mod proto {
    tonic::include_proto!("datadog.procmgr");

    pub const FILE_DESCRIPTOR_SET: &[u8] =
        tonic::include_file_descriptor_set!("process_manager_descriptor");
}

#[cfg(bazel)]
pub mod proto {
    pub use procmgr_proto::datadog::procmgr::*;
    pub const FILE_DESCRIPTOR_SET: &[u8] = procmgr_proto::datadog::procmgr::FILE_DESCRIPTOR_SET;
}

#[cfg(windows)]
pub use named_pipe::IpcStream;
#[cfg(unix)]
pub use uds::IpcStream;

#[cfg(unix)]
const DEFAULT_IPC_PATH: &str = "/var/run/datadog-procmgrd/dd-procmgrd.sock";
#[cfg(windows)]
const DEFAULT_IPC_PATH: &str = r"\\.\pipe\datadog-procmgrd";

const DUMMY_ENDPOINT: &str = "http://[::]:50051";
const CONNECT_TIMEOUT: Duration = Duration::from_secs(5);

/// Return the platform's default dd-procmgrd endpoint.
pub fn default_ipc_path() -> PathBuf {
    PathBuf::from(DEFAULT_IPC_PATH)
}

/// Resolve the daemon endpoint from `DD_PM_SOCKET_PATH`, falling back to the
/// platform default.
pub fn ipc_path() -> PathBuf {
    std::env::var("DD_PM_SOCKET_PATH")
        .map(PathBuf::from)
        .unwrap_or_else(|_| default_ipc_path())
}

/// Open the platform-local stream used by dd-procmgrd.
pub async fn connect_stream(path: &Path) -> std::io::Result<IpcStream> {
    connect_platform(path).await
}

/// Connect eagerly to dd-procmgrd at `path`.
pub async fn connect(path: &Path) -> Result<tonic::transport::Channel> {
    let endpoint = path.to_path_buf();
    let display_path = endpoint.clone();
    tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT)
        .connect_with_connector(tower::service_fn(move |_| {
            let endpoint = endpoint.clone();
            async move {
                connect_platform(&endpoint)
                    .await
                    .map(hyper_util::rt::TokioIo::new)
            }
        }))
        .await
        .with_context(|| {
            format!(
                "failed to connect to local endpoint {}",
                display_path.display()
            )
        })
}

/// Build a channel that connects on the first RPC and reconnects after the
/// daemon restarts.
pub fn connect_lazy(path: &Path) -> tonic::transport::Channel {
    let path = path.to_path_buf();
    tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT)
        .connect_timeout(CONNECT_TIMEOUT)
        .connect_with_connector_lazy(tower::service_fn(move |_| {
            let path = path.clone();
            async move {
                connect_platform(&path)
                    .await
                    .map(hyper_util::rt::TokioIo::new)
            }
        }))
}

#[cfg(unix)]
async fn connect_platform(path: &Path) -> std::io::Result<IpcStream> {
    uds::connect(path).await
}

#[cfg(windows)]
async fn connect_platform(path: &Path) -> std::io::Result<IpcStream> {
    named_pipe::connect(path).await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[cfg(unix)]
    #[tokio::test]
    async fn connects_to_unix_listener() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("procmgr.sock");
        let listener = tokio::net::UnixListener::bind(&path).unwrap();

        let accept = tokio::spawn(async move { listener.accept().await.unwrap() });
        connect_stream(&path).await.unwrap();
        accept.await.unwrap();
    }

    #[tokio::test]
    async fn missing_endpoint_reports_an_os_error() {
        let error = connect_stream(Path::new(MISSING_ENDPOINT))
            .await
            .expect_err("connecting to a nonexistent endpoint should fail");
        assert_ne!(error.kind(), std::io::ErrorKind::Unsupported);
        assert!(error.raw_os_error().is_some(), "{error:?}");
    }

    #[cfg(unix)]
    const MISSING_ENDPOINT: &str = "/tmp/dd-procmgr-client-does-not-exist.sock";
    #[cfg(windows)]
    const MISSING_ENDPOINT: &str = r"\\.\pipe\dd-procmgr-client-does-not-exist";
}
