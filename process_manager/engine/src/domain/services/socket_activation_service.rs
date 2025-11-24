//! Socket Activation Manager
//!
//! Provides systemd-compatible socket activation:
//! - Pre-creates and listens on sockets (TCP, Unix)
//! - Starts services on-demand when connections arrive
//! - Passes socket FDs via LISTEN_FDS environment variable

use crate::domain::{DomainError, SocketConfig};
use std::collections::HashMap;
use std::net::TcpListener as StdTcpListener;
use std::os::unix::io::{AsRawFd, RawFd};
use std::os::unix::net::UnixListener as StdUnixListener;
use std::sync::Arc;
use tokio::sync::{mpsc, Mutex};
use tracing::{debug, error, info};

/// Socket activation event - signals that a process should be started
#[derive(Debug, Clone)]
pub struct SocketActivationEvent {
    /// Socket name
    pub socket_name: String,
    /// Service/process name to start
    pub service_name: String,
    /// File descriptor to pass to the service
    pub fd: RawFd,
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
    #[allow(dead_code)] // May be used for future features like socket inspection
    config: SocketConfig,
    #[allow(dead_code)] // May be used for future features like FD management
    fd: RawFd,
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

        // Create the listener and get its FD
        let fd = self.create_listener(&config)?;

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
                fd,
            },
        );

        // Spawn acceptor task
        self.spawn_acceptor(socket_name.clone(), fd, config);

        Ok(socket_name)
    }

    /// Create the appropriate listener type and return its FD
    fn create_listener(&self, config: &SocketConfig) -> Result<RawFd, DomainError> {
        if let Some(ref addr) = config.listen_stream {
            // TCP listener
            let listener = StdTcpListener::bind(addr).map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to bind TCP socket {}: {}", addr, e))
            })?;
            // Keep socket in BLOCKING mode (systemd compatibility, child processes expect blocking)
            listener.set_nonblocking(false).map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to set blocking: {}", e))
            })?;
            let fd = listener.as_raw_fd();
            // Leak the listener to keep the FD open
            std::mem::forget(listener);
            Ok(fd)
        } else if let Some(ref path) = config.listen_unix {
            // Unix listener
            // Remove existing socket file if it exists
            let _ = std::fs::remove_file(path);

            let listener = StdUnixListener::bind(path).map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to bind Unix socket {:?}: {}", path, e))
            })?;
            // Keep socket in BLOCKING mode (systemd compatibility, child processes expect blocking)
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

            let fd = listener.as_raw_fd();
            // Leak the listener to keep the FD open
            std::mem::forget(listener);
            Ok(fd)
        } else {
            Err(DomainError::InvalidCommand(
                "Socket config must specify listen_stream or listen_unix".to_string(),
            ))
        }
    }

    /// Spawn a thread to accept connections and trigger activation
    /// Uses blocking I/O (systemd compatibility)
    fn spawn_acceptor(&self, socket_name: String, fd: RawFd, config: SocketConfig) {
        let event_tx = self.event_tx.clone();
        let service_name = config.service.clone();
        let accept_mode = config.accept;

        if accept_mode {
            // Accept=yes: per-connection spawning (inetd-style)
            Self::accept_loop_multi(socket_name, fd, service_name, event_tx, config);
        } else {
            // Accept=no: single service instance (default)
            Self::accept_once_single(socket_name, fd, service_name, event_tx, config);
        }
    }

    /// Accept=no: Wait for first connection, then trigger service start once
    /// Uses select() to detect connection without accepting (systemd compatibility)
    /// The child process will call accept() itself
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

        // Spawn a blocking thread to continuously monitor for connections
        // We DON'T call accept() - the child process will do that
        std::thread::spawn(move || {
            use std::mem::MaybeUninit;

            // Continuously wait for connections (reactivation support)
            loop {
                // Use select() to wait for the socket to become readable (connection waiting)
                unsafe {
                    let mut readfds: libc::fd_set = MaybeUninit::zeroed().assume_init();
                    libc::FD_ZERO(&mut readfds);
                    libc::FD_SET(fd, &mut readfds);

                    // Wait indefinitely for a connection
                    let result = libc::select(
                        fd + 1,
                        &mut readfds,
                        std::ptr::null_mut(),
                        std::ptr::null_mut(),
                        std::ptr::null_mut(), // No timeout
                    );

                    if result > 0 && libc::FD_ISSET(fd, &readfds) {
                        info!(
                            socket = %socket_name,
                            service = %service_name,
                            "Connection detected, triggering service activation"
                        );

                        // Send activation event with the listening socket FD
                        // The child process will call accept() on this FD
                        if let Err(e) = event_tx.send(SocketActivationEvent {
                            socket_name: socket_name.clone(),
                            service_name: service_name.clone(),
                            fd,
                            accept: false,
                        }) {
                            error!(
                                socket = %socket_name,
                                error = %e,
                                "Failed to send activation event"
                            );
                            break; // Channel closed, exit loop
                        }

                        // Sleep briefly to avoid tight loop while child accepts connection
                        // The child process needs time to start and call accept()
                        std::thread::sleep(std::time::Duration::from_millis(100));
                    } else if result == -1 {
                        let err = std::io::Error::last_os_error();
                        error!(
                            socket = %socket_name,
                            error = %err,
                            "select() failed"
                        );
                        break; // Error, exit loop
                    }
                }
            }
        });
    }

    /// Accept=yes: Accept each connection and spawn a new service instance
    /// Uses blocking I/O in a separate thread (systemd compatibility)
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

        // Spawn a blocking thread for accept loop
        std::thread::spawn(move || {
            // Convert the raw FD back into a listener
            let listener = if config.listen_stream.is_some() {
                unsafe {
                    use std::os::unix::io::FromRawFd;
                    StdTcpListener::from_raw_fd(fd)
                }
            } else {
                error!(socket = %socket_name, "Unix sockets not yet supported for Accept=yes");
                return;
            };

            // Accept connections in a loop (blocking)
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

                        // Send activation event for this connection
                        if let Err(e) = event_tx.send(SocketActivationEvent {
                            socket_name: socket_name.clone(),
                            service_name: service_name.clone(),
                            fd: accepted_fd,
                            accept: true,
                        }) {
                            error!(
                                socket = %socket_name,
                                error = %e,
                                "Failed to send activation event"
                            );
                        }

                        // Leak the stream to keep the FD alive for the child process
                        std::mem::forget(stream);
                    }
                    Err(e) => {
                        error!(
                            socket = %socket_name,
                            error = %e,
                            "Failed to accept connection"
                        );
                        // Sleep briefly to avoid tight loop on persistent errors
                        std::thread::sleep(std::time::Duration::from_millis(100));
                    }
                }
            }
        });
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
    async fn test_create_unix_socket() {
        let (manager, _rx) = SocketActivationService::new();

        let temp_path = std::env::temp_dir().join("test_socket.sock");
        let _ = std::fs::remove_file(&temp_path); // Clean up if exists

        let config = SocketConfig::new("test".to_string(), "service".to_string())
            .with_unix(temp_path.clone());

        let result = manager.create_socket(config).await;
        assert!(result.is_ok());

        let sockets = manager.list_sockets().await;
        assert_eq!(sockets.len(), 1);
        assert_eq!(sockets[0], "test");

        // Cleanup
        let _ = std::fs::remove_file(&temp_path);
    }

    #[tokio::test]
    async fn test_stop_socket() {
        let (manager, _rx) = SocketActivationService::new();

        let temp_path = std::env::temp_dir().join("test_socket2.sock");
        let _ = std::fs::remove_file(&temp_path);

        let config = SocketConfig::new("test2".to_string(), "service".to_string())
            .with_unix(temp_path.clone());

        manager.create_socket(config).await.unwrap();

        let result = manager.stop_socket("test2").await;
        assert!(result.is_ok());

        let sockets = manager.list_sockets().await;
        assert!(sockets.is_empty());

        // Cleanup
        let _ = std::fs::remove_file(&temp_path);
    }
}
