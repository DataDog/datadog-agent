// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::command::Command;
use crate::grpc::proto;
use crate::grpc::service::ProcessManagerService;
use crate::manager::ProcessManager;
use crate::transport;
use anyhow::{Context, Result};
use tokio::sync::mpsc;
use tonic::transport::Server;

pub fn socket_path() -> std::path::PathBuf {
    transport::ipc_path()
}

pub async fn run(
    mgr: ProcessManager,
    cmd_tx: mpsc::Sender<Command>,
    shutdown: tokio::sync::oneshot::Receiver<()>,
) -> Result<()> {
    let svc = ProcessManagerService::new(mgr, cmd_tx);
    let pm_service = proto::process_manager_server::ProcessManagerServer::new(svc);

    let reflection = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(proto::FILE_DESCRIPTOR_SET)
        .build_v1()
        .context("failed to build gRPC reflection service")?;
    let router = Server::builder()
        .add_service(reflection)
        .add_service(pm_service);

    transport::serve(router, async {
        let _ = shutdown.await;
    })
    .await
}

#[cfg(test)]
mod tests {
    use crate::transport;

    #[test]
    fn test_prepare_creates_parent_dirs() {
        let dir = tempfile::tempdir().unwrap();
        let nested = dir.path().join("a").join("b").join("c").join("test.sock");
        transport::prepare(&nested).unwrap();
        assert!(nested.parent().unwrap().exists());
    }

    #[test]
    fn test_prepare_removes_stale_file() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("stale.sock");
        std::fs::write(&sock, b"leftover").unwrap();
        assert!(sock.exists());
        transport::prepare(&sock).unwrap();
        assert!(!sock.exists());
    }

    #[test]
    fn test_prepare_noop_on_fresh_path() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("fresh.sock");
        assert!(!sock.exists());
        transport::prepare(&sock).unwrap();
        assert!(!sock.exists());
    }

    #[test]
    fn test_set_permissions() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("perm.sock");
        std::fs::write(&file, b"").unwrap();
        transport::set_permissions(&file);
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mode = std::fs::metadata(&file).unwrap().permissions().mode() & 0o777;
            assert_eq!(mode, 0o660);
        }
    }

    #[test]
    fn test_cleanup_removes_file() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("cleanup.sock");
        std::fs::write(&sock, b"").unwrap();
        assert!(sock.exists());
        transport::cleanup(&sock);
        assert!(!sock.exists());
    }

    #[test]
    fn test_cleanup_noop_if_missing() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("nonexistent.sock");
        transport::cleanup(&sock);
    }
}
