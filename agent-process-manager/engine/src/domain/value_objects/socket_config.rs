//! Socket configuration for socket activation
//! Systemd-compatible socket activation support

use serde::{Deserialize, Serialize};
use std::path::PathBuf;

/// Socket configuration for process activation
///
/// Systemd-compatible socket activation allows processes to be started
/// on-demand when a connection arrives on a socket.
///
/// ## Accept Modes
///
/// **Accept=false** (default): Single service instance handles all connections
/// - Socket manager detects first connection
/// - Starts ONE service instance
/// - Service receives the listening socket (FD 3)
/// - Service accepts and handles all future connections
/// - Best for long-running services (web servers, databases)
///
/// **Accept=true**: New service instance per connection (inetd-style)
/// - Socket manager accepts each connection
/// - Starts a NEW service instance for each connection
/// - Service receives the accepted connection socket (FD 3)
/// - Service handles one connection then exits
/// - Best for stateless request handlers
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct SocketConfig {
    /// Unique name for this socket
    pub name: String,

    /// TCP stream socket address (e.g., "0.0.0.0:8080", "[::]:8080")
    pub listen_stream: Option<String>,

    /// UDP datagram socket address
    pub listen_datagram: Option<String>,

    /// Unix domain socket path
    pub listen_unix: Option<PathBuf>,

    /// Process name to activate when connection arrives
    pub service: String,

    /// Accept mode:
    /// - false (Accept=no): Single service instance (default)
    /// - true (Accept=yes): Per-connection service instances
    pub accept: bool,

    /// Unix socket file permissions (octal, e.g., 0o660)
    pub socket_mode: Option<u32>,

    /// Unix socket file owner
    pub socket_user: Option<String>,

    /// Unix socket file group
    pub socket_group: Option<String>,
}

impl SocketConfig {
    /// Create a new socket configuration
    pub fn new(name: String, service: String) -> Self {
        Self {
            name,
            listen_stream: None,
            listen_datagram: None,
            listen_unix: None,
            service,
            accept: false, // Accept=no by default (systemd default)
            socket_mode: None,
            socket_user: None,
            socket_group: None,
        }
    }

    /// Set TCP listen address
    pub fn with_tcp(mut self, addr: String) -> Self {
        self.listen_stream = Some(addr);
        self
    }

    /// Set Unix socket path
    pub fn with_unix(mut self, path: PathBuf) -> Self {
        self.listen_unix = Some(path);
        self
    }

    /// Set UDP socket address
    pub fn with_udp(mut self, addr: String) -> Self {
        self.listen_datagram = Some(addr);
        self
    }

    /// Set accept mode
    pub fn with_accept(mut self, accept: bool) -> Self {
        self.accept = accept;
        self
    }

    /// Set Unix socket permissions
    pub fn with_socket_mode(mut self, mode: u32) -> Self {
        self.socket_mode = Some(mode);
        self
    }

    /// Set Unix socket owner
    pub fn with_socket_user(mut self, user: String) -> Self {
        self.socket_user = Some(user);
        self
    }

    /// Set Unix socket group
    pub fn with_socket_group(mut self, group: String) -> Self {
        self.socket_group = Some(group);
        self
    }

    /// Validate that at least one socket type is configured
    pub fn validate(&self) -> Result<(), String> {
        if self.listen_stream.is_none()
            && self.listen_datagram.is_none()
            && self.listen_unix.is_none()
        {
            return Err(
                "Socket config must specify one of: listen_stream, listen_unix, listen_datagram"
                    .to_string(),
            );
        }
        Ok(())
    }

    /// Get the socket type description
    pub fn socket_type(&self) -> &str {
        if self.listen_stream.is_some() {
            "TCP"
        } else if self.listen_unix.is_some() {
            "Unix"
        } else if self.listen_datagram.is_some() {
            "UDP"
        } else {
            "None"
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_socket_config_tcp() {
        let config = SocketConfig::new("web".to_string(), "nginx".to_string())
            .with_tcp("0.0.0.0:8080".to_string());

        assert_eq!(config.name, "web");
        assert_eq!(config.service, "nginx");
        assert_eq!(config.listen_stream, Some("0.0.0.0:8080".to_string()));
        assert!(!config.accept);
        assert_eq!(config.socket_type(), "TCP");
        assert!(config.validate().is_ok());
    }

    #[test]
    fn test_socket_config_unix() {
        let config = SocketConfig::new("api".to_string(), "api-server".to_string())
            .with_unix(PathBuf::from("/var/run/api.sock"))
            .with_socket_mode(0o660);

        assert_eq!(config.listen_unix, Some(PathBuf::from("/var/run/api.sock")));
        assert_eq!(config.socket_mode, Some(0o660));
        assert_eq!(config.socket_type(), "Unix");
        assert!(config.validate().is_ok());
    }

    #[test]
    fn test_socket_config_accept_mode() {
        let config = SocketConfig::new("echo".to_string(), "echo-service".to_string())
            .with_tcp("127.0.0.1:7777".to_string())
            .with_accept(true);

        assert!(config.accept);
        assert!(config.validate().is_ok());
    }

    #[test]
    fn test_socket_config_validation_fails() {
        let config = SocketConfig::new("invalid".to_string(), "service".to_string());

        assert!(config.validate().is_err());
    }

    #[test]
    fn test_socket_config_udp() {
        let config = SocketConfig::new("dns".to_string(), "dns-service".to_string())
            .with_udp("0.0.0.0:53".to_string());

        assert_eq!(config.listen_datagram, Some("0.0.0.0:53".to_string()));
        assert_eq!(config.socket_type(), "UDP");
        assert!(config.validate().is_ok());
    }
}
