// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Local-socket client transport shared by the process-manager and executor
//! gRPC clients. par-control is a pure client, so this only needs to dial an
//! existing Unix domain socket (Windows named-pipe support is a follow-up, per
//! the PRD's out-of-scope note on Windows transport details).

use std::path::{Path, PathBuf};

/// Placeholder URI for the tonic Endpoint when connecting over UDS. The actual
/// address is irrelevant because `connect_with_connector` bypasses it.
const DUMMY_ENDPOINT: &str = "http://[::]:50051";

/// Build a lazily-connecting gRPC channel to a server on the given Unix domain
/// socket. Lazy connection matters for the executor, whose socket does not exist
/// until the control plane has started it; the connection is established on the
/// first RPC and re-dialed after the executor is restarted.
pub fn connect_lazy(path: &Path) -> tonic::transport::Channel {
    let path: PathBuf = path.to_path_buf();
    tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT).connect_with_connector_lazy(
        tower::service_fn(move |_| {
            let p = path.clone();
            async move { connect_stream(p).await.map(hyper_util::rt::TokioIo::new) }
        }),
    )
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
    tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT).connect_with_connector_lazy(
        tower::service_fn(move |_| {
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
