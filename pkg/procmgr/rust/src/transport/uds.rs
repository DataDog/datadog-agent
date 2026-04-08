// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::warn;
use std::path::{Path, PathBuf};

const DEFAULT_SOCKET_PATH: &str = "/var/run/datadog-procmgrd/dd-procmgrd.sock";
const SOCKET_PERMISSIONS: u32 = 0o660;

/// Placeholder URI for tonic Endpoint when connecting over UDS.
/// The actual address is irrelevant because `connect_with_connector` bypasses it.
pub const DUMMY_ENDPOINT: &str = "http://[::]:50051";

pub fn ipc_path() -> PathBuf {
    std::env::var("DD_PM_SOCKET_PATH")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from(DEFAULT_SOCKET_PATH))
}

pub fn prepare(path: &Path) -> Result<()> {
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

pub fn set_permissions(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    if let Err(e) =
        std::fs::set_permissions(path, std::fs::Permissions::from_mode(SOCKET_PERMISSIONS))
    {
        warn!("failed to set socket permissions: {e}");
    }
}

pub fn cleanup(path: &Path) {
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
    fn test_prepare_creates_parent_dirs() {
        let dir = tempfile::tempdir().unwrap();
        let nested = dir.path().join("a").join("b").join("c").join("test.sock");
        prepare(&nested).unwrap();
        assert!(nested.parent().unwrap().exists());
    }

    #[test]
    fn test_prepare_removes_stale_file() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("stale.sock");
        std::fs::write(&sock, b"leftover").unwrap();
        assert!(sock.exists());
        prepare(&sock).unwrap();
        assert!(!sock.exists());
    }

    #[test]
    fn test_prepare_noop_on_fresh_path() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("fresh.sock");
        assert!(!sock.exists());
        prepare(&sock).unwrap();
        assert!(!sock.exists());
    }

    #[test]
    fn test_set_permissions() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("perm.sock");
        std::fs::write(&file, b"").unwrap();
        set_permissions(&file);
        let mode = std::fs::metadata(&file).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode, SOCKET_PERMISSIONS);
    }

    #[test]
    fn test_cleanup_removes_file() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("cleanup.sock");
        std::fs::write(&sock, b"").unwrap();
        assert!(sock.exists());
        cleanup(&sock);
        assert!(!sock.exists());
    }

    #[test]
    fn test_cleanup_noop_if_missing() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("nonexistent.sock");
        cleanup(&sock);
    }
}
