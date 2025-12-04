//! Socket Activation Manager
//!
//! Provides systemd-compatible socket activation:
//! - Pre-creates and listens on sockets (TCP, Unix)
//! - Starts services on-demand when connections arrive
//! - Passes socket FDs via LISTEN_FDS environment variable
//!
//! Platform support:
//! - Linux/macOS: Full support (TCP + Unix sockets)
//! - Windows: TCP sockets only (Unix sockets not available)

use crate::domain::{DomainError, SocketConfig};
use std::collections::HashMap;
use std::net::TcpListener as StdTcpListener;
use std::sync::Arc;
use tokio::sync::{mpsc, Mutex};
use tracing::{debug, error, info};

// warn is used on Windows but not Unix in some configurations
#[cfg(windows)]
use tracing::warn;

// Platform-specific imports
#[cfg(unix)]
use std::os::unix::io::{AsRawFd, RawFd};
#[cfg(unix)]
use std::os::unix::net::UnixListener as StdUnixListener;

// Windows uses a different handle type
#[cfg(windows)]
use std::os::windows::io::{AsRawSocket, RawSocket};

/// Platform-independent file descriptor / socket handle type
#[cfg(unix)]
pub type SocketHandle = RawFd;
#[cfg(windows)]
pub type SocketHandle = RawSocket;
#[cfg(not(any(unix, windows)))]
pub type SocketHandle = i32;

/// Socket activation event - signals that a process should be started
#[derive(Debug, Clone)]
pub struct SocketActivationEvent {
    /// Socket name
    pub socket_name: String,
    /// Service/process name to start
    pub service_name: String,
    /// File descriptor / socket handle to pass to the service
    #[cfg(unix)]
    pub fd: RawFd,
    #[cfg(windows)]
    pub fd: RawSocket,
    #[cfg(not(any(unix, windows)))]
    pub fd: i32,
    /// Accept mode (false = single service, true = per-connection)
    pub accept: bool,
}

/// Socket activation manager
pub struct SocketActivationService {
    sockets: Arc<Mutex<HashMap<String, ManagedSocket>>>,
    event_tx: mpsc::UnboundedSender<SocketActivationEvent>,
}

/// Internal socket state
struct ManagedSocket {
    _name: String,
    #[allow(dead_code)]
    config: SocketConfig,
    #[allow(dead_code)]
    handle: SocketHandle,
}

impl SocketActivationService {
    /// Create a new socket activation manager
    pub fn new() -> (Self, mpsc::UnboundedReceiver<SocketActivationEvent>) {
        let (event_tx, event_rx) = mpsc::unbounded_channel();
        let manager = Self {
            sockets: Arc::new(Mutex::new(HashMap::new())),
            event_tx,
        };
        (manager, event_rx)
    }

    /// Create and start listening on a socket
    pub async fn create_socket(&self, config: SocketConfig) -> Result<String, DomainError> {
        // Validate config
        config.validate().map_err(DomainError::InvalidCommand)?;

        let socket_name = config.name.clone();

        // Create the listener and get its handle
        let handle = self.create_listener(&config)?;

        info!(
            socket = %socket_name,
            service = %config.service,
            socket_type = %config.socket_type(),
            accept = config.accept,
            "Socket created and listening"
        );

        // Store the managed socket
        let mut sockets = self.sockets.lock().await;
        sockets.insert(
            socket_name.clone(),
            ManagedSocket {
                _name: socket_name.clone(),
                config: config.clone(),
                handle,
            },
        );

        // Spawn acceptor task
        self.spawn_acceptor(socket_name.clone(), handle, config);

        Ok(socket_name)
    }

    /// Create the appropriate listener type and return its handle
    fn create_listener(&self, config: &SocketConfig) -> Result<SocketHandle, DomainError> {
        if let Some(ref addr) = config.listen_stream {
            // TCP listener (cross-platform)
            self.create_tcp_listener(addr, config)
        } else if let Some(ref path) = config.listen_unix {
            // Unix listener (Unix-only)
            self.create_unix_listener(path, config)
        } else {
            Err(DomainError::InvalidCommand(
                "Socket config must specify listen_stream or listen_unix".to_string(),
            ))
        }
    }

    /// Create a TCP listener (cross-platform)
    fn create_tcp_listener(
        &self,
        addr: &str,
        _config: &SocketConfig,
    ) -> Result<SocketHandle, DomainError> {
        let listener = StdTcpListener::bind(addr).map_err(|e| {
            DomainError::InvalidCommand(format!("Failed to bind TCP socket {}: {}", addr, e))
        })?;

        // Keep socket in BLOCKING mode (systemd compatibility, child processes expect blocking)
        listener.set_nonblocking(false).map_err(|e| {
            DomainError::InvalidCommand(format!("Failed to set blocking: {}", e))
        })?;

        #[cfg(unix)]
        let handle = listener.as_raw_fd();
        #[cfg(windows)]
        let handle = listener.as_raw_socket();

        // Leak the listener to keep the handle open
        std::mem::forget(listener);

        Ok(handle)
    }

    /// Create a Unix socket listener (Unix-only)
    #[cfg(unix)]
    fn create_unix_listener(
        &self,
        path: &std::path::PathBuf,
        config: &SocketConfig,
    ) -> Result<SocketHandle, DomainError> {
        // Remove existing socket file if it exists
        let _ = std::fs::remove_file(path);

        let listener = StdUnixListener::bind(path).map_err(|e| {
            DomainError::InvalidCommand(format!("Failed to bind Unix socket {:?}: {}", path, e))
        })?;

        // Keep socket in BLOCKING mode
        listener.set_nonblocking(false).map_err(|e| {
            DomainError::InvalidCommand(format!("Failed to set blocking: {}", e))
        })?;

        // Set permissions if specified
        if let Some(mode) = config.socket_mode {
            use std::os::unix::fs::PermissionsExt;
            let permissions = std::fs::Permissions::from_mode(mode);
            std::fs::set_permissions(path, permissions).map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to set socket permissions: {}", e))
            })?;
        }

        let handle = listener.as_raw_fd();
        std::mem::forget(listener);

        Ok(handle)
    }

    /// Create a Unix socket listener (Windows - not supported)
    #[cfg(windows)]
    fn create_unix_listener(
        &self,
        path: &std::path::PathBuf,
        _config: &SocketConfig,
    ) -> Result<SocketHandle, DomainError> {
        warn!(
            path = ?path,
            "Unix sockets are not supported on Windows. Use TCP sockets instead."
        );
        Err(DomainError::InvalidCommand(
            "Unix sockets are not supported on Windows. Use listen_stream for TCP instead."
                .to_string(),
        ))
    }

    /// Create a Unix socket listener (fallback for other platforms)
    #[cfg(not(any(unix, windows)))]
    fn create_unix_listener(
        &self,
        path: &std::path::PathBuf,
        _config: &SocketConfig,
    ) -> Result<SocketHandle, DomainError> {
        Err(DomainError::InvalidCommand(format!(
            "Unix sockets not supported on this platform: {:?}",
            path
        )))
    }

    /// Spawn a thread to accept connections and trigger activation
    fn spawn_acceptor(&self, socket_name: String, handle: SocketHandle, config: SocketConfig) {
        let event_tx = self.event_tx.clone();
        let service_name = config.service.clone();
        let accept_mode = config.accept;

        if accept_mode {
            // Accept=yes: per-connection spawning (inetd-style)
            Self::accept_loop_multi(socket_name, handle, service_name, event_tx, config);
        } else {
            // Accept=no: single service instance (default)
            Self::accept_once_single(socket_name, handle, service_name, event_tx, config);
        }
    }

    /// Accept=no: Wait for first connection, then trigger service start once
    #[cfg(unix)]
    fn accept_once_single(
        socket_name: String,
        fd: RawFd,
        service_name: String,
        event_tx: mpsc::UnboundedSender<SocketActivationEvent>,
        _config: SocketConfig,
    ) {
        debug!(
            socket = %socket_name,
            service = %service_name,
            "Waiting for connection to trigger service activation (Accept=no)"
        );

        std::thread::spawn(move || {
            use std::mem::MaybeUninit;

            loop {
                unsafe {
                    let mut readfds: libc::fd_set = MaybeUninit::zeroed().assume_init();
                    libc::FD_ZERO(&mut readfds);
                    libc::FD_SET(fd, &mut readfds);

                    let result = libc::select(
                        fd + 1,
                        &mut readfds,
                        std::ptr::null_mut(),
                        std::ptr::null_mut(),
                        std::ptr::null_mut(),
                    );

                    if result > 0 && libc::FD_ISSET(fd, &readfds) {
                        info!(
                            socket = %socket_name,
                            service = %service_name,
                            "Connection detected, triggering service activation"
                        );

                        if let Err(e) = event_tx.send(SocketActivationEvent {
                            socket_name: socket_name.clone(),
                            service_name: service_name.clone(),
                            fd,
                            accept: false,
                        }) {
                            error!(socket = %socket_name, error = %e, "Failed to send activation event");
                            break;
                        }

                        std::thread::sleep(std::time::Duration::from_millis(100));
                    } else if result == -1 {
                        let err = std::io::Error::last_os_error();
                        error!(socket = %socket_name, error = %err, "select() failed");
                        break;
                    }
                }
            }
        });
    }

    /// Accept=no: Windows implementation using select-like polling
    #[cfg(windows)]
    fn accept_once_single(
        socket_name: String,
        handle: RawSocket,
        service_name: String,
        event_tx: mpsc::UnboundedSender<SocketActivationEvent>,
        _config: SocketConfig,
    ) {
        debug!(
            socket = %socket_name,
            service = %service_name,
            "Waiting for connection to trigger service activation (Accept=no) - Windows"
        );

        std::thread::spawn(move || {
            use std::net::TcpListener;
            use std::os::windows::io::FromRawSocket;

            // Reconstruct the TcpListener from the raw socket
            let listener = unsafe { TcpListener::from_raw_socket(handle) };

            // Set to non-blocking for polling
            if let Err(e) = listener.set_nonblocking(true) {
                error!(socket = %socket_name, error = %e, "Failed to set non-blocking");
                return;
            }

            loop {
                match listener.accept() {
                    Ok((_stream, _addr)) => {
                        info!(
                            socket = %socket_name,
                            service = %service_name,
                            "Connection detected, triggering service activation"
                        );

                        // Reset to blocking for child process
                        let _ = listener.set_nonblocking(false);

                        if let Err(e) = event_tx.send(SocketActivationEvent {
                            socket_name: socket_name.clone(),
                            service_name: service_name.clone(),
                            fd: handle,
                            accept: false,
                        }) {
                            error!(socket = %socket_name, error = %e, "Failed to send activation event");
                            break;
                        }

                        // Sleep to let child process start
                        std::thread::sleep(std::time::Duration::from_millis(100));

                        // Continue listening for reactivation
                        let _ = listener.set_nonblocking(true);
                    }
                    Err(ref e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                        // No connection yet, sleep and retry
                        std::thread::sleep(std::time::Duration::from_millis(50));
                    }
                    Err(e) => {
                        error!(socket = %socket_name, error = %e, "accept() failed");
                        break;
                    }
                }
            }
        });
    }

    /// Accept=no: Fallback for unsupported platforms
    #[cfg(not(any(unix, windows)))]
    fn accept_once_single(
        socket_name: String,
        _handle: SocketHandle,
        service_name: String,
        _event_tx: mpsc::UnboundedSender<SocketActivationEvent>,
        _config: SocketConfig,
    ) {
        warn!(
            socket = %socket_name,
            service = %service_name,
            "Socket activation not supported on this platform"
        );
    }

    /// Accept=yes: Accept each connection and spawn a new service instance
    #[cfg(unix)]
    fn accept_loop_multi(
        socket_name: String,
        fd: RawFd,
        service_name: String,
        event_tx: mpsc::UnboundedSender<SocketActivationEvent>,
        config: SocketConfig,
    ) {
        debug!(
            socket = %socket_name,
            service = %service_name,
            "Accepting connections, will spawn service per connection (Accept=yes)"
        );

        std::thread::spawn(move || {
            // Only TCP is supported for Accept=yes mode currently
            if config.listen_stream.is_none() {
                error!(socket = %socket_name, "Unix sockets not yet supported for Accept=yes");
                return;
            }

            use std::os::unix::io::FromRawFd;
            let listener = unsafe { StdTcpListener::from_raw_fd(fd) };

            loop {
                match listener.accept() {
                    Ok((stream, addr)) => {
                        let accepted_fd = stream.as_raw_fd();

                        info!(
                            socket = %socket_name,
                            service = %service_name,
                            client = ?addr,
                            fd = accepted_fd,
                            "Connection accepted, spawning new service instance"
                        );

                        if let Err(e) = event_tx.send(SocketActivationEvent {
                            socket_name: socket_name.clone(),
                            service_name: service_name.clone(),
                            fd: accepted_fd,
                            accept: true,
                        }) {
                            error!(socket = %socket_name, error = %e, "Failed to send activation event");
                        }

                        // Leak the stream to keep the FD alive for the child process
                        std::mem::forget(stream);
                    }
                    Err(e) => {
                        error!(socket = %socket_name, error = %e, "Failed to accept connection");
                        std::thread::sleep(std::time::Duration::from_millis(100));
                    }
                }
            }
        });
    }

    /// Accept=yes: Windows implementation
    #[cfg(windows)]
    fn accept_loop_multi(
        socket_name: String,
        handle: RawSocket,
        service_name: String,
        event_tx: mpsc::UnboundedSender<SocketActivationEvent>,
        config: SocketConfig,
    ) {
        debug!(
            socket = %socket_name,
            service = %service_name,
            "Accepting connections, will spawn service per connection (Accept=yes) - Windows"
        );

        std::thread::spawn(move || {
            use std::net::TcpListener;
            use std::os::windows::io::FromRawSocket;

            if config.listen_stream.is_none() {
                error!(socket = %socket_name, "Only TCP sockets are supported on Windows");
                return;
            }

            let listener = unsafe { TcpListener::from_raw_socket(handle) };

            loop {
                match listener.accept() {
                    Ok((stream, addr)) => {
                        let accepted_handle = stream.as_raw_socket();

                        info!(
                            socket = %socket_name,
                            service = %service_name,
                            client = ?addr,
                            "Connection accepted, spawning new service instance"
                        );

                        if let Err(e) = event_tx.send(SocketActivationEvent {
                            socket_name: socket_name.clone(),
                            service_name: service_name.clone(),
                            fd: accepted_handle,
                            accept: true,
                        }) {
                            error!(socket = %socket_name, error = %e, "Failed to send activation event");
                        }

                        std::mem::forget(stream);
                    }
                    Err(e) => {
                        error!(socket = %socket_name, error = %e, "Failed to accept connection");
                        std::thread::sleep(std::time::Duration::from_millis(100));
                    }
                }
            }
        });
    }

    /// Accept=yes: Fallback for unsupported platforms
    #[cfg(not(any(unix, windows)))]
    fn accept_loop_multi(
        socket_name: String,
        _handle: SocketHandle,
        service_name: String,
        _event_tx: mpsc::UnboundedSender<SocketActivationEvent>,
        _config: SocketConfig,
    ) {
        warn!(
            socket = %socket_name,
            service = %service_name,
            "Socket activation not supported on this platform"
        );
    }

    /// Stop a socket
    pub async fn stop_socket(&self, socket_name: &str) -> Result<(), DomainError> {
        let mut sockets = self.sockets.lock().await;
        if sockets.remove(socket_name).is_some() {
            info!(socket = %socket_name, "Socket stopped");
            Ok(())
        } else {
            Err(DomainError::InvalidCommand(format!(
                "Socket '{}' not found",
                socket_name
            )))
        }
    }

    /// List all managed sockets
    pub async fn list_sockets(&self) -> Vec<String> {
        let sockets = self.sockets.lock().await;
        sockets.keys().cloned().collect()
    }
}

impl Default for SocketActivationService {
    fn default() -> Self {
        Self::new().0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_socket_activation_manager_creation() {
        let (manager, _rx) = SocketActivationService::new();
        let sockets = manager.list_sockets().await;
        assert!(sockets.is_empty());
    }

    #[tokio::test]
    #[cfg(unix)]
    async fn test_create_unix_socket() {
        let (manager, _rx) = SocketActivationService::new();

        let temp_path = std::env::temp_dir().join("test_socket.sock");
        let _ = std::fs::remove_file(&temp_path);

        let config = SocketConfig::new("test".to_string(), "service".to_string())
            .with_unix(temp_path.clone());

        let result = manager.create_socket(config).await;
        assert!(result.is_ok());

        let sockets = manager.list_sockets().await;
        assert_eq!(sockets.len(), 1);
        assert_eq!(sockets[0], "test");

        let _ = std::fs::remove_file(&temp_path);
    }

    #[tokio::test]
    async fn test_create_tcp_socket() {
        let (manager, _rx) = SocketActivationService::new();

        let config = SocketConfig::new("tcp_test".to_string(), "service".to_string())
            .with_tcp("127.0.0.1:0".to_string());

        let result = manager.create_socket(config).await;
        assert!(result.is_ok());

        let sockets = manager.list_sockets().await;
        assert_eq!(sockets.len(), 1);
        assert_eq!(sockets[0], "tcp_test");
    }

    #[tokio::test]
    async fn test_stop_socket() {
        let (manager, _rx) = SocketActivationService::new();

        let config = SocketConfig::new("stop_test".to_string(), "service".to_string())
            .with_tcp("127.0.0.1:0".to_string());

        manager.create_socket(config).await.unwrap();

        let result = manager.stop_socket("stop_test").await;
        assert!(result.is_ok());

        let sockets = manager.list_sockets().await;
        assert!(sockets.is_empty());
    }
}
