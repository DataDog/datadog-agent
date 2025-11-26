//! Daemon configuration from environment variables
//!
//! All configuration is read from environment variables with sensible defaults.
//! This eliminates the need for command-line argument parsing (clap dependency).

use std::env;

// Default configuration values
const DEFAULT_GRPC_PORT: u16 = 50051;
const DEFAULT_REST_PORT: u16 = 3000;
const DEFAULT_GRPC_SOCKET: &str = "/var/run/datadog/process-manager.sock";
const DEFAULT_REST_SOCKET: &str = "/var/run/datadog/process-manager-api.sock";
const DEFAULT_ENABLE_REST: bool = false;
const DEFAULT_LOG_LEVEL: &str = "info";

/// Daemon configuration loaded from environment variables
#[derive(Debug, Clone)]
pub struct DaemonConfig {
    /// Transport mode: "unix" or "tcp"
    pub transport_mode: TransportMode,

    /// gRPC port (TCP mode only)
    pub grpc_port: u16,

    /// REST port (TCP mode only)
    pub rest_port: u16,

    /// gRPC Unix socket path (Unix socket mode only)
    pub grpc_socket: String,

    /// REST Unix socket path (Unix socket mode only)
    pub rest_socket: String,

    /// Enable REST API
    pub enable_rest: bool,

    /// Config file path
    pub config_file: Option<String>,

    /// Config directory path
    pub config_dir: Option<String>,

    /// Log level
    pub log_level: String,
}

#[derive(Debug, Clone, PartialEq, Default)]
pub enum TransportMode {
    #[default]
    Unix,
    Tcp,
}

impl DaemonConfig {
    /// Load configuration from environment variables
    pub fn from_env() -> Self {
        Self {
            transport_mode: Self::parse_transport_mode(),
            grpc_port: Self::parse_u16("DD_PM_GRPC_PORT", DEFAULT_GRPC_PORT)
                .or_else(|| Self::parse_u16("DD_PM_DAEMON_PORT", DEFAULT_GRPC_PORT)) // Backward compat
                .unwrap_or(DEFAULT_GRPC_PORT),
            rest_port: Self::parse_u16("DD_PM_REST_PORT", DEFAULT_REST_PORT)
                .unwrap_or(DEFAULT_REST_PORT),
            grpc_socket: env::var("DD_PM_GRPC_SOCKET")
                .unwrap_or_else(|_| DEFAULT_GRPC_SOCKET.to_string()),
            rest_socket: env::var("DD_PM_REST_SOCKET")
                .unwrap_or_else(|_| DEFAULT_REST_SOCKET.to_string()),
            enable_rest: Self::parse_bool("DD_PM_ENABLE_REST", DEFAULT_ENABLE_REST),
            config_file: env::var("DD_PM_CONFIG_FILE").ok(),
            config_dir: env::var("DD_PM_CONFIG_DIR").ok(),
            log_level: Self::parse_log_level(),
        }
    }

    fn parse_transport_mode() -> TransportMode {
        env::var("DD_PM_TRANSPORT_MODE")
            .ok()
            .and_then(|s| match s.to_lowercase().as_str() {
                "tcp" => Some(TransportMode::Tcp),
                "unix" => Some(TransportMode::Unix),
                _ => None,
            })
            .unwrap_or_default()
    }

    fn parse_u16(var_name: &str, _default: u16) -> Option<u16> {
        env::var(var_name).ok().and_then(|s| s.parse().ok())
    }

    fn parse_bool(var_name: &str, default: bool) -> bool {
        env::var(var_name)
            .ok()
            .and_then(|s| match s.to_lowercase().as_str() {
                "true" | "1" | "yes" | "on" => Some(true),
                "false" | "0" | "no" | "off" => Some(false),
                _ => None,
            })
            .unwrap_or(default)
    }

    fn parse_log_level() -> String {
        // Priority: DD_PM_LOG_LEVEL > RUST_LOG > default
        env::var("DD_PM_LOG_LEVEL")
            .or_else(|_| env::var("RUST_LOG"))
            .unwrap_or_else(|_| DEFAULT_LOG_LEVEL.to_string())
    }

    /// Validate configuration
    pub fn validate(&self) -> Result<(), String> {
        if self.config_file.is_some() && self.config_dir.is_some() {
            return Err("Cannot specify both DD_PM_CONFIG_FILE and DD_PM_CONFIG_DIR".to_string());
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    // Mutex to serialize tests that modify environment variables
    // This prevents race conditions when tests run in parallel
    static ENV_MUTEX: Mutex<()> = Mutex::new(());

    #[test]
    fn test_default_config() {
        let _lock = ENV_MUTEX.lock().unwrap();
        // Clear env vars
        env::remove_var("DD_PM_TRANSPORT_MODE");
        env::remove_var("DD_PM_GRPC_PORT");
        env::remove_var("DD_PM_GRPC_SOCKET");
        env::remove_var("DD_PM_REST_SOCKET");
        env::remove_var("DD_PM_LOG_LEVEL");
        env::remove_var("RUST_LOG");

        let config = DaemonConfig::from_env();
        assert_eq!(config.transport_mode, TransportMode::Unix);
        assert_eq!(config.grpc_port, DEFAULT_GRPC_PORT);
        assert_eq!(config.rest_port, DEFAULT_REST_PORT);
        assert_eq!(config.grpc_socket, "/var/run/datadog/process-manager.sock");
        assert_eq!(
            config.rest_socket,
            "/var/run/datadog/process-manager-api.sock"
        );
        assert_eq!(config.enable_rest, DEFAULT_ENABLE_REST);
        assert_eq!(config.log_level, DEFAULT_LOG_LEVEL);
    }

    #[test]
    fn test_tcp_mode() {
        let _lock = ENV_MUTEX.lock().unwrap();
        env::set_var("DD_PM_TRANSPORT_MODE", "tcp");
        let config = DaemonConfig::from_env();
        assert_eq!(config.transport_mode, TransportMode::Tcp);
        env::remove_var("DD_PM_TRANSPORT_MODE");
    }

    #[test]
    fn test_custom_ports() {
        let _lock = ENV_MUTEX.lock().unwrap();
        env::set_var("DD_PM_GRPC_PORT", "9999");
        env::set_var("DD_PM_REST_PORT", "8888");
        let config = DaemonConfig::from_env();
        assert_eq!(config.grpc_port, 9999);
        assert_eq!(config.rest_port, 8888);
        env::remove_var("DD_PM_GRPC_PORT");
        env::remove_var("DD_PM_REST_PORT");
    }

    #[test]
    fn test_backward_compat_daemon_port() {
        let _lock = ENV_MUTEX.lock().unwrap();
        env::set_var("DD_PM_DAEMON_PORT", "7777");
        let config = DaemonConfig::from_env();
        assert_eq!(config.grpc_port, 7777);
        env::remove_var("DD_PM_DAEMON_PORT");
    }

    #[test]
    fn test_bool_parsing() {
        let _lock = ENV_MUTEX.lock().unwrap();
        for val in &["true", "1", "yes", "on", "TRUE", "Yes"] {
            env::set_var("DD_PM_ENABLE_REST", val);
            let config = DaemonConfig::from_env();
            assert!(config.enable_rest, "Failed for value: {}", val);
        }

        for val in &["false", "0", "no", "off", "FALSE", "No"] {
            env::set_var("DD_PM_ENABLE_REST", val);
            let config = DaemonConfig::from_env();
            assert!(!config.enable_rest, "Failed for value: {}", val);
        }

        env::remove_var("DD_PM_ENABLE_REST");
    }

    #[test]
    fn test_log_level_priority() {
        let _lock = ENV_MUTEX.lock().unwrap();
        // DD_PM_LOG_LEVEL takes priority
        env::set_var("DD_PM_LOG_LEVEL", "debug");
        env::set_var("RUST_LOG", "trace");
        let config = DaemonConfig::from_env();
        assert_eq!(config.log_level, "debug");

        // RUST_LOG is fallback
        env::remove_var("DD_PM_LOG_LEVEL");
        let config = DaemonConfig::from_env();
        assert_eq!(config.log_level, "trace");

        // Default to "info"
        env::remove_var("RUST_LOG");
        let config = DaemonConfig::from_env();
        assert_eq!(config.log_level, "info");
    }

    #[test]
    fn test_validation() {
        let _lock = ENV_MUTEX.lock().unwrap();
        let mut config = DaemonConfig::from_env();
        assert!(config.validate().is_ok());

        config.config_file = Some("/etc/pm/config.yaml".to_string());
        config.config_dir = Some("/etc/pm/conf.d".to_string());
        assert!(config.validate().is_err());
    }
}
