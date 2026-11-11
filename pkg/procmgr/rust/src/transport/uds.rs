// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, anyhow};
use log::{info, warn};
use nix::libc;
use std::fs::{File, OpenOptions, Permissions};
use std::future::Future;
use std::io;
use std::os::fd::{AsFd, AsRawFd, OwnedFd};
use std::os::unix::fs::{MetadataExt, OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};
use std::time::{Duration, Instant};
use tokio::net::UnixListener;
use tokio_stream::wrappers::UnixListenerStream;

const SOCKET_PERMISSIONS: u32 = 0o660;
const LOCK_TIMEOUT: Duration = Duration::from_secs(5);
const LOCK_RETRY_INTERVAL: Duration = Duration::from_millis(50);

pub fn ipc_path() -> PathBuf {
    dd_procmgr_client::ipc_path()
}

/// Exclusive flock on `<socket path>.lock`, which serializes one daemon's prepare and
/// bind against another daemon's ownership check and unlink. Empty when the lock could
/// not be taken and the caller proceeds unlocked.
pub struct SocketLock(Option<File>);

impl SocketLock {
    pub fn is_locked(&self) -> bool {
        self.0.is_some()
    }
}

/// The socket this daemon bound. `cleanup` unlinks the path only while it still has
/// this `(dev, ino)`.
pub struct BoundSocket {
    identity: Option<(u64, u64)>,
    // A bound socket pins its path inode, so holding a duplicate of the listener
    // keeps that inode number from being reused by a newer bind before the check.
    _listener: Option<OwnedFd>,
}

impl BoundSocket {
    fn new(path: &Path, listener: &UnixListener) -> Self {
        let identity = match std::fs::symlink_metadata(path) {
            Ok(meta) => Some((meta.dev(), meta.ino())),
            Err(e) => {
                warn!("failed to stat bound socket {}: {e}", path.display());
                None
            }
        };
        let listener = match listener.as_fd().try_clone_to_owned() {
            Ok(fd) => Some(fd),
            Err(e) => {
                warn!(
                    "failed to duplicate the listener for {}: {e}",
                    path.display()
                );
                None
            }
        };
        Self {
            identity,
            _listener: listener,
        }
    }
}

fn lock_path(path: &Path) -> PathBuf {
    let mut lock = path.as_os_str().to_owned();
    lock.push(".lock");
    PathBuf::from(lock)
}

/// Opens the lock file without following a symlink. std always adds O_CLOEXEC.
fn open_lock_file(lock: &Path) -> io::Result<File> {
    match OpenOptions::new()
        .read(true)
        .write(true)
        .create_new(true)
        .mode(SOCKET_PERMISSIONS)
        .custom_flags(libc::O_NOFOLLOW)
        .open(lock)
    {
        Ok(file) => {
            // The create mode is filtered by the umask.
            if let Err(e) = file.set_permissions(Permissions::from_mode(SOCKET_PERMISSIONS)) {
                warn!(
                    "failed to set lock file permissions {}: {e}",
                    lock.display()
                );
            }
            return Ok(file);
        }
        Err(e) if e.kind() == io::ErrorKind::AlreadyExists => {}
        Err(e) => return Err(e),
    }
    // NFS maps an exclusive flock to an fcntl write lock, which needs a writable fd.
    match OpenOptions::new()
        .read(true)
        .write(true)
        .custom_flags(libc::O_NOFOLLOW)
        .open(lock)
    {
        Err(e) if e.raw_os_error() == Some(libc::EACCES) => OpenOptions::new()
            .read(true)
            .custom_flags(libc::O_NOFOLLOW)
            .open(lock),
        result => result,
    }
}

fn acquire_lock(lock: &Path, timeout: Duration) -> Result<File> {
    let file = open_lock_file(lock)
        .with_context(|| format!("failed to open socket lock {}", lock.display()))?;
    let deadline = Instant::now() + timeout;
    loop {
        // SAFETY: the fd belongs to `file`, which outlives the call.
        if unsafe { libc::flock(file.as_raw_fd(), libc::LOCK_EX | libc::LOCK_NB) } == 0 {
            return Ok(file);
        }
        let err = io::Error::last_os_error();
        match err.raw_os_error() {
            Some(libc::EINTR) => {}
            Some(libc::EWOULDBLOCK) => {
                let now = Instant::now();
                if now >= deadline {
                    return Err(anyhow!(
                        "timed out after {timeout:?} waiting for socket lock {}",
                        lock.display()
                    ));
                }
                std::thread::sleep(LOCK_RETRY_INTERVAL.min(deadline - now));
            }
            _ => {
                return Err(err)
                    .with_context(|| format!("failed to lock socket lock {}", lock.display()));
            }
        }
    }
}

/// Never fails: an unlocked daemon only reopens the narrow replace-then-unlink race.
fn lock_or_warn(path: &Path, timeout: Duration) -> SocketLock {
    match acquire_lock(&lock_path(path), timeout) {
        Ok(file) => SocketLock(Some(file)),
        Err(e) => {
            warn!("continuing without the socket lock: {e:#}");
            SocketLock(None)
        }
    }
}

/// Creates the socket directory and removes a stale socket, so the newest daemon wins.
/// Hold the returned lock until the socket is bound.
pub fn prepare(path: &Path) -> Result<SocketLock> {
    prepare_with_timeout(path, LOCK_TIMEOUT)
}

fn prepare_with_timeout(path: &Path, timeout: Duration) -> Result<SocketLock> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)
            .with_context(|| format!("failed to create socket directory: {}", parent.display()))?;
    }
    let lock = lock_or_warn(path, timeout);
    match std::fs::remove_file(path) {
        Ok(()) => {}
        Err(e) if e.kind() == io::ErrorKind::NotFound => {}
        Err(e) => {
            return Err(e)
                .with_context(|| format!("failed to remove stale socket: {}", path.display()));
        }
    }
    Ok(lock)
}

/// Binds the socket while holding the lock taken by `prepare`.
pub fn bind(path: &Path) -> Result<(UnixListener, BoundSocket)> {
    let _lock = prepare(path)?;
    let listener = UnixListener::bind(path)
        .with_context(|| format!("failed to bind Unix socket: {}", path.display()))?;
    set_permissions(path);
    let bound = BoundSocket::new(path, &listener);
    Ok((listener, bound))
}

pub fn set_permissions(path: &Path) {
    if let Err(e) = std::fs::set_permissions(path, Permissions::from_mode(SOCKET_PERMISSIONS)) {
        warn!("failed to set socket permissions: {e}");
    }
}

pub async fn serve<F>(router: tonic::transport::server::Router, shutdown: F) -> Result<()>
where
    F: Future<Output = ()>,
{
    let path = ipc_path();
    let (uds, bound) = bind(&path)?;
    info!("gRPC server listening on {}", path.display());

    let uds_stream = UnixListenerStream::new(uds);

    router
        .serve_with_incoming_shutdown(uds_stream, shutdown)
        .await
        .context("gRPC server error")?;

    info!("gRPC server stopped");
    cleanup(&path, bound);
    Ok(())
}

/// Unlinks the socket unless a newer daemon has replaced it since `bind`.
pub fn cleanup(path: &Path, bound: BoundSocket) {
    let _lock = lock_or_warn(path, LOCK_TIMEOUT);
    let current = match std::fs::symlink_metadata(path) {
        Ok(meta) => (meta.dev(), meta.ino()),
        Err(e) if e.kind() == io::ErrorKind::NotFound => return,
        Err(e) => {
            warn!("failed to stat socket {}: {e}", path.display());
            return;
        }
    };
    if bound.identity != Some(current) {
        info!(
            "leaving socket {}: it is no longer the one this daemon bound",
            path.display()
        );
        return;
    }
    match std::fs::remove_file(path) {
        Ok(()) => {}
        Err(e) if e.kind() == io::ErrorKind::NotFound => {}
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

    fn identity(path: &Path) -> (u64, u64) {
        let meta = std::fs::symlink_metadata(path).unwrap();
        (meta.dev(), meta.ino())
    }

    #[tokio::test]
    async fn test_cleanup_removes_its_own_socket() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("cleanup.sock");
        let (_listener, bound) = bind(&sock).unwrap();
        assert!(sock.exists());
        cleanup(&sock, bound);
        assert!(!sock.exists());
        assert!(lock_path(&sock).exists(), "the lock file is never deleted");
    }

    #[tokio::test]
    async fn test_cleanup_noop_if_missing() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("nonexistent.sock");
        let (_listener, bound) = bind(&sock).unwrap();
        std::fs::remove_file(&sock).unwrap();
        cleanup(&sock, bound);
        assert!(!sock.exists());
    }

    #[tokio::test]
    async fn test_cleanup_keeps_a_replacement_socket() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("replaced.sock");
        let (_old_listener, old) = bind(&sock).unwrap();

        std::fs::remove_file(&sock).unwrap();
        let _replacement = std::os::unix::net::UnixListener::bind(&sock).unwrap();
        let replacement_identity = identity(&sock);

        cleanup(&sock, old);
        assert!(sock.exists(), "the newer daemon's socket must survive");
        assert_eq!(identity(&sock), replacement_identity);
    }

    #[test]
    fn test_lock_file_is_created_0660() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("mode.sock");
        let lock = prepare(&sock).unwrap();
        assert!(lock.is_locked());
        let mode = std::fs::metadata(lock_path(&sock))
            .unwrap()
            .permissions()
            .mode()
            & 0o777;
        assert_eq!(mode, SOCKET_PERMISSIONS);
    }

    #[test]
    fn test_prepare_proceeds_unlocked_after_lock_timeout() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("contended.sock");
        std::fs::write(&sock, b"stale").unwrap();
        let _held = acquire_lock(&lock_path(&sock), LOCK_TIMEOUT).unwrap();
        let timeout = Duration::from_millis(200);

        let err = acquire_lock(&lock_path(&sock), timeout).unwrap_err();
        assert!(
            err.to_string().contains("timed out"),
            "the warning names the timeout: {err:#}"
        );

        let started = Instant::now();
        let lock = prepare_with_timeout(&sock, timeout).unwrap();
        assert!(
            started.elapsed() >= timeout,
            "prepare waits for the retry window"
        );
        assert!(!lock.is_locked());
        assert!(!sock.exists(), "prepare still removes the stale socket");
    }

    #[tokio::test]
    async fn test_read_only_lock_file_still_locks() {
        if nix::unistd::Uid::effective().is_root() {
            eprintln!("skipping: root bypasses file modes");
            return;
        }
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("readonly.sock");
        let lock = lock_path(&sock);
        std::fs::write(&lock, b"").unwrap();
        std::fs::set_permissions(&lock, Permissions::from_mode(0o444)).unwrap();

        assert!(prepare(&sock).unwrap().is_locked());
        let (_listener, bound) = bind(&sock).unwrap();
        cleanup(&sock, bound);
        assert!(!sock.exists());
        let mode = std::fs::metadata(&lock).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode, 0o444, "an existing lock file keeps its mode");
    }

    #[tokio::test]
    async fn test_symlinked_lock_path_is_refused() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("symlink.sock");
        let target = dir.path().join("target");
        std::fs::write(&target, b"untouched").unwrap();
        std::os::unix::fs::symlink(&target, lock_path(&sock)).unwrap();

        assert!(acquire_lock(&lock_path(&sock), LOCK_TIMEOUT).is_err());
        assert!(!prepare(&sock).unwrap().is_locked());
        let (_listener, bound) = bind(&sock).unwrap();
        cleanup(&sock, bound);
        assert!(!sock.exists());
        assert!(
            std::fs::symlink_metadata(lock_path(&sock))
                .unwrap()
                .file_type()
                .is_symlink()
        );
        assert_eq!(std::fs::read(&target).unwrap(), b"untouched");
    }
}
