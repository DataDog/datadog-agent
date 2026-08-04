// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Local transport for the dd-procmgrd gRPC client.

use std::path::{Path, PathBuf};

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
async fn connect_stream(_path: PathBuf) -> std::io::Result<tokio::net::TcpStream> {
    Err(std::io::Error::new(
        std::io::ErrorKind::Unsupported,
        "Windows named-pipe transport is not yet implemented",
    ))
}
