// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::command::Command;
use crate::grpc::proto;
use crate::grpc::service::ProcessManagerService;
use crate::manager::ProcessManager;
use anyhow::{Context, Result};
use log::{info, warn};
use std::path::{Path, PathBuf};
use tokio::net::UnixListener;
use tokio::sync::mpsc;
use tokio_stream::wrappers::UnixListenerStream;
use tonic::transport::Server;

const DEFAULT_SOCKET_PATH: &str = "/var/run/datadog-procmgrd/dd-procmgrd.sock";
const SOCKET_PERMISSIONS: u32 = 0o660;

pub fn socket_path() -> PathBuf {
    std::env::var("DD_PM_SOCKET_PATH")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from(DEFAULT_SOCKET_PATH))
}

pub async fn run(
    mgr: ProcessManager,
    cmd_tx: mpsc::Sender<Command>,
    shutdown: tokio::sync::oneshot::Receiver<()>,
) -> Result<()> {
    let path = socket_path();
    prepare_socket(&path)?;

    let uds = UnixListener::bind(&path)
        .with_context(|| format!("failed to bind Unix socket: {}", path.display()))?;
    set_socket_permissions(&path);
    info!("gRPC server listening on {}", path.display());

    let uds_stream = UnixListenerStream::new(uds);

    let svc = ProcessManagerService::new(mgr, cmd_tx);
    let pm_service = proto::process_manager_server::ProcessManagerServer::new(svc);

    let reflection = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(proto::FILE_DESCRIPTOR_SET)
        .build_v1()
        .context("failed to build gRPC reflection service")?;
    let router = Server::builder()
        .add_service(reflection)
        .add_service(pm_service);

    router
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
    match std::fs::remove_file(path) {
        Ok(()) => {}
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(e) => {
            return Err(e)
                .with_context(|| format!("failed to remove stale socket: {}", path.display()));
        }
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
    match std::fs::remove_file(path) {
        Ok(()) => {}
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(e) => warn!("failed to clean up socket {}: {e}", path.display()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::os::unix::fs::PermissionsExt;

    #[test]
    fn test_prepare_socket_creates_parent_dirs() {
        let dir = tempfile::tempdir().unwrap();
        let nested = dir.path().join("a").join("b").join("c").join("test.sock");
        prepare_socket(&nested).unwrap();
        assert!(nested.parent().unwrap().exists());
    }

    #[test]
    fn test_prepare_socket_removes_stale_file() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("stale.sock");
        std::fs::write(&sock, b"leftover").unwrap();
        assert!(sock.exists());
        prepare_socket(&sock).unwrap();
        assert!(!sock.exists());
    }

    #[test]
    fn test_prepare_socket_noop_on_fresh_path() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("fresh.sock");
        assert!(!sock.exists());
        prepare_socket(&sock).unwrap();
        assert!(!sock.exists());
    }

    #[test]
    fn test_set_socket_permissions() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("perm.sock");
        std::fs::write(&file, b"").unwrap();
        set_socket_permissions(&file);
        let mode = std::fs::metadata(&file).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode, SOCKET_PERMISSIONS);
    }

    #[test]
    fn test_cleanup_socket_removes_file() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("cleanup.sock");
        std::fs::write(&sock, b"").unwrap();
        assert!(sock.exists());
        cleanup_socket(&sock);
        assert!(!sock.exists());
    }

    #[test]
    fn test_cleanup_socket_noop_if_missing() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("nonexistent.sock");
        cleanup_socket(&sock);
    }
}
