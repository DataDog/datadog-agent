// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Local-socket client transport shared by the process-manager and executor
//! gRPC clients. The platform connector and named-pipe retry behavior are owned
//! by `dd-procmgr-client`; this module adds executor-specific mTLS.

use std::path::{Path, PathBuf};
use std::time::Duration;

const DUMMY_ENDPOINT: &str = "http://[::]:50051";
const CONNECT_TIMEOUT: Duration = Duration::from_secs(5);

/// Build a lazily-connecting gRPC channel to a local endpoint.
pub fn connect_lazy(path: &Path) -> tonic::transport::Channel {
    dd_procmgr_client::connect_lazy(path)
}

/// Like [`connect_lazy`] but wraps the local stream in mTLS using the agent IPC
/// certificate at `ipc_cert_file` (the control<->executor channel).
///
/// The connector is rebuilt from the file on each connection rather than once
/// at startup. This tolerates the executor creating or rotating the certificate
/// after par-control has started.
pub fn connect_lazy_tls(path: &Path, ipc_cert_file: &Path) -> tonic::transport::Channel {
    let path: PathBuf = path.to_path_buf();
    let ipc_cert_file: PathBuf = ipc_cert_file.to_path_buf();
    tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT)
        .connect_timeout(CONNECT_TIMEOUT)
        .connect_with_connector_lazy(tower::service_fn(move |_| {
            let path = path.clone();
            let cert = ipc_cert_file.clone();
            async move {
                let connector =
                    crate::tls::build_ipc_client_connector(&cert).map_err(std::io::Error::other)?;
                let stream = dd_procmgr_client::connect_stream(&path).await?;
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
