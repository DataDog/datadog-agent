// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::grpc::proto;
use crate::grpc::service::ProcessManagerService;
use crate::process::ManagedProcess;
use anyhow::{Context, Result};
use log::{info, warn};
use std::path::{Path, PathBuf};
use std::sync::Arc;
use tokio::net::UnixListener;
use tokio::sync::RwLock;
use tokio_stream::wrappers::UnixListenerStream;
use tonic::transport::Server;

const DEFAULT_SOCKET_PATH: &str = "/var/run/datadog/dd-procmgrd.sock";
const SOCKET_PERMISSIONS: u32 = 0o660;

pub fn socket_path() -> PathBuf {
    std::env::var("DD_PM_SOCKET_PATH")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from(DEFAULT_SOCKET_PATH))
}

pub async fn run(
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
    config_path: String,
    shutdown: tokio::sync::oneshot::Receiver<()>,
) -> Result<()> {
    let path = socket_path();
    prepare_socket(&path)?;

    let uds = UnixListener::bind(&path)
        .with_context(|| format!("failed to bind Unix socket: {}", path.display()))?;
    set_socket_permissions(&path);
    info!("gRPC server listening on {}", path.display());

    let uds_stream = UnixListenerStream::new(uds);

    let svc = ProcessManagerService::new(processes, config_path);

    let reflection = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(proto::FILE_DESCRIPTOR_SET)
        .build_v1()
        .context("failed to build gRPC reflection service")?;

    Server::builder()
        .add_service(reflection)
        .add_service(proto::process_manager_server::ProcessManagerServer::new(
            svc,
        ))
        .serve_with_incoming_shutdown(uds_stream, async {
            let _ = shutdown.await;
        })
        .await
        .context("gRPC server error")?;

    info!("gRPC server stopped");
    cleanup_socket(&path);
    Ok(())
}

fn prepare_socket(path: &Path) -> Result<()> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)
            .with_context(|| format!("failed to create socket directory: {}", parent.display()))?;
    }
    if path.exists() {
        std::fs::remove_file(path)
            .with_context(|| format!("failed to remove stale socket: {}", path.display()))?;
    }
    Ok(())
}

fn set_socket_permissions(path: &Path) {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        if let Err(e) =
            std::fs::set_permissions(path, std::fs::Permissions::from_mode(SOCKET_PERMISSIONS))
        {
            warn!("failed to set socket permissions: {e}");
        }
    }
}

fn cleanup_socket(path: &Path) {
    if path.exists()
        && let Err(e) = std::fs::remove_file(path)
    {
        warn!("failed to clean up socket {}: {e}", path.display());
    }
}
