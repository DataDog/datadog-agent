// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Local-socket client transport shared by the process-manager and executor
//! gRPC clients. par-control is a pure client, so this only needs to dial an
//! existing Unix domain socket (Windows named-pipe support is a follow-up, per
//! the PRD's out-of-scope note on Windows transport details).

use std::path::{Path, PathBuf};
use std::sync::Arc;

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
            async move {
                tokio::net::UnixStream::connect(p)
                    .await
                    .map(hyper_util::rt::TokioIo::new)
            }
        }),
    )
}

/// Like [`connect_lazy`] but wraps the Unix-socket stream in mTLS using the given
/// connector (the control<->executor channel, secured with the agent IPC cert).
pub fn connect_lazy_tls(
    path: &Path,
    connector: tokio_native_tls::TlsConnector,
) -> tonic::transport::Channel {
    let path: PathBuf = path.to_path_buf();
    let connector = Arc::new(connector);
    tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT).connect_with_connector_lazy(
        tower::service_fn(move |_| {
            let p = path.clone();
            let connector = Arc::clone(&connector);
            async move {
                let stream = tokio::net::UnixStream::connect(p).await?;
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
