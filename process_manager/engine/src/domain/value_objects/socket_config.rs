//! Socket configuration for socket activation
//! Systemd-compatible socket activation support

use serde::{Deserialize, Serialize};
use std::path::PathBuf;

/// Configuration source for socket settings
///
/// Determines where socket configuration values (port, path, etc.) are read from.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
#[serde(rename_all = "kebab-case")]
pub enum ConfigSource {
    /// Use explicit values from this socket config file (default)
    #[default]
    Explicit,

    /// Read from Datadog APM config (datadog.yaml + DD_APM_* env vars)
    ///
    /// Reads:
    /// - DD_APM_SOCKET_ACTIVATION_ENABLED (must be true)
    /// - DD_APM_RECEIVER_PORT / DD_RECEIVER_PORT (default: 8126)
    /// - DD_APM_RECEIVER_SOCKET (default: /var/run/datadog/apm.socket on Linux)
    /// - DD_BIND_HOST / bind_host (default: localhost)
    /// - DD_APM_NON_LOCAL_TRAFFIC (overrides to 0.0.0.0)
    ///
    /// Sets env vars for child process:
    /// - DD_APM_NET_RECEIVER_FD for TCP socket
    /// - DD_APM_UNIX_RECEIVER_FD for Unix socket
    DatadogApm,

    /// Read from Datadog OTLP config
    ///
    /// Reads:
    /// - otlp_config.traces.internal_port (default: 5003)
    ///
    /// Sets env var: DD_OTLP_CONFIG_GRPC_FD
    DatadogOtlp,

    /// Read from Datadog DogStatsD config
    ///
    /// Reads:
    /// - DD_DOGSTATSD_PORT (default: 8125)
    /// - DD_DOGSTATSD_SOCKET (default: /var/run/datadog/dsd.socket)
    DatadogDogstatsd,
}

/// Socket configuration for process activation
///
/// Systemd-compatible socket activation allows processes to be started
/// on-demand when a connection arrives on a socket.
///
/// ## Config Source
///
/// By default, socket addresses are read from explicit fields in this config.
/// Set `config_source` to read from Datadog configuration instead:
///
/// ```yaml
/// # Use Datadog APM config (datadog.yaml + DD_APM_* env vars)
/// config_source: datadog-apm
/// service: trace-agent
/// ```
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

    /// Configuration source - where to read socket settings from
    /// Default: explicit (use values from this config file)
    #[serde(default)]
    pub config_source: ConfigSource,

    /// TCP stream socket address (e.g., "0.0.0.0:8080", "[::]:8080")
    /// Ignored if config_source is set to a Datadog source
    pub listen_stream: Option<String>,

    /// UDP datagram socket address
    pub listen_datagram: Option<String>,

    /// Unix domain socket path
    /// Ignored if config_source is set to a Datadog source
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

    /// Custom environment variable name for passing the socket FD to the child process
    /// Default: LISTEN_FDS (systemd-compatible)
    ///
    /// For Datadog components, this is auto-set based on config_source:
    /// - datadog-apm TCP: DD_APM_NET_RECEIVER_FD
    /// - datadog-apm Unix: DD_APM_UNIX_RECEIVER_FD
    /// - datadog-otlp: DD_OTLP_CONFIG_GRPC_FD
    pub fd_env_var: Option<String>,
}

impl SocketConfig {
    /// Create a new socket configuration with explicit values
    pub fn new(name: String, service: String) -> Self {
        Self {
            name,
            config_source: ConfigSource::Explicit,
            listen_stream: None,
            listen_datagram: None,
            listen_unix: None,
            service,
            accept: false, // Accept=no by default (systemd default)
            socket_mode: None,
            socket_user: None,
            socket_group: None,
            fd_env_var: None,
        }
    }

    /// Create a socket configuration that reads from Datadog APM config
    pub fn from_datadog_apm(name: String, service: String) -> Self {
        Self {
            name,
            config_source: ConfigSource::DatadogApm,
            listen_stream: None,
            listen_datagram: None,
            listen_unix: None,
            service,
            accept: false,
            socket_mode: None,
            socket_user: None,
            socket_group: None,
            fd_env_var: None, // Will be set during resolution
        }
    }

    /// Create a socket configuration that reads from Datadog OTLP config
    pub fn from_datadog_otlp(name: String, service: String) -> Self {
        Self {
            name,
            config_source: ConfigSource::DatadogOtlp,
            listen_stream: None,
            listen_datagram: None,
            listen_unix: None,
            service,
            accept: false,
            socket_mode: None,
            socket_user: None,
            socket_group: None,
            fd_env_var: None,
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

    /// Set custom FD environment variable name
    pub fn with_fd_env_var(mut self, var_name: String) -> Self {
        self.fd_env_var = Some(var_name);
        self
    }

    /// Check if this config uses a Datadog config source
    pub fn uses_datadog_config(&self) -> bool {
        !matches!(self.config_source, ConfigSource::Explicit)
    }

    /// Validate that at least one socket type is configured
    /// For Datadog config sources, validation is deferred to resolution time
    pub fn validate(&self) -> Result<(), String> {
        // Datadog config sources are validated during resolution
        if self.uses_datadog_config() {
            return Ok(());
        }

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
