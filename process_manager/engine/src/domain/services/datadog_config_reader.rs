//! Datadog Configuration Reader
//!
//! Reads Datadog Agent configuration from:
//! 1. Environment variables (highest priority)
//! 2. datadog.yaml config file
//! 3. Default values (lowest priority)
//!
//! This mirrors the behavior of the Go agent's config system,
//! particularly for socket activation settings.

use crate::domain::value_objects::{ConfigSource, SocketConfig};
use crate::domain::DomainError;
use serde::Deserialize;
use std::env;
use std::fs;
use std::path::PathBuf;
use tracing::{debug, info, warn};

/// Resolved socket configuration from Datadog config
#[derive(Debug, Clone)]
pub struct ResolvedSocketConfig {
    /// TCP listen address (e.g., "0.0.0.0:8126")
    pub listen_stream: Option<String>,
    /// Unix socket path
    pub listen_unix: Option<PathBuf>,
    /// Environment variable name for TCP socket FD
    pub tcp_fd_env_var: Option<String>,
    /// Environment variable name for Unix socket FD
    pub unix_fd_env_var: Option<String>,
}

/// Datadog configuration reader
pub struct DatadogConfigReader {
    /// Path to datadog.yaml
    config_path: PathBuf,
    /// Parsed YAML config (lazy loaded)
    yaml_config: Option<DatadogYamlConfig>,
}

/// Partial datadog.yaml structure (only fields we need)
#[derive(Debug, Deserialize, Default)]
struct DatadogYamlConfig {
    #[serde(default)]
    bind_host: Option<String>,

    #[serde(default)]
    apm_config: Option<ApmConfig>,

    #[serde(default)]
    otlp_config: Option<OtlpConfig>,

    #[serde(default)]
    dogstatsd_port: Option<u16>,

    #[serde(default)]
    #[allow(dead_code)] // Reserved for future use
    dogstatsd_socket: Option<String>,
}

#[derive(Debug, Deserialize, Default)]
struct ApmConfig {
    #[serde(default)]
    enabled: Option<bool>,

    #[serde(default)]
    socket_activation: Option<SocketActivationConfig>,

    #[serde(default)]
    receiver_port: Option<u16>,

    #[serde(default)]
    receiver_socket: Option<String>,

    #[serde(default)]
    apm_non_local_traffic: Option<bool>,
}

#[derive(Debug, Deserialize, Default)]
struct SocketActivationConfig {
    #[serde(default)]
    enabled: Option<bool>,
}

#[derive(Debug, Deserialize, Default)]
struct OtlpConfig {
    #[serde(default)]
    traces: Option<OtlpTracesConfig>,
}

#[derive(Debug, Deserialize, Default)]
struct OtlpTracesConfig {
    #[serde(default)]
    enabled: Option<bool>,

    #[serde(default)]
    internal_port: Option<u16>,
}

impl DatadogConfigReader {
    /// Create a new config reader
    ///
    /// Looks for datadog.yaml in:
    /// 1. DD_CONFIG_FILE env var
    /// 2. /etc/datadog-agent/datadog.yaml (Linux)
    /// 3. /opt/datadog-agent/etc/datadog.yaml (macOS)
    /// 4. C:\ProgramData\Datadog\datadog.yaml (Windows)
    pub fn new() -> Self {
        let config_path = Self::find_config_path();
        Self {
            config_path,
            yaml_config: None,
        }
    }

    /// Create with explicit config path
    pub fn with_path(path: PathBuf) -> Self {
        Self {
            config_path: path,
            yaml_config: None,
        }
    }

    /// Find the datadog.yaml config file
    fn find_config_path() -> PathBuf {
        // Check env var first
        if let Ok(path) = env::var("DD_CONFIG_FILE") {
            return PathBuf::from(path);
        }

        // Platform-specific defaults
        #[cfg(target_os = "linux")]
        let default_path = "/etc/datadog-agent/datadog.yaml";

        #[cfg(target_os = "macos")]
        let default_path = "/opt/datadog-agent/etc/datadog.yaml";

        #[cfg(target_os = "windows")]
        let default_path = r"C:\ProgramData\Datadog\datadog.yaml";

        #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
        let default_path = "/etc/datadog-agent/datadog.yaml";

        PathBuf::from(default_path)
    }

    /// Load and parse the YAML config file
    fn load_yaml_config(&mut self) -> Option<&DatadogYamlConfig> {
        if self.yaml_config.is_some() {
            return self.yaml_config.as_ref();
        }

        if !self.config_path.exists() {
            debug!(path = %self.config_path.display(), "Datadog config file not found");
            return None;
        }

        match fs::read_to_string(&self.config_path) {
            Ok(contents) => match serde_yaml::from_str::<DatadogYamlConfig>(&contents) {
                Ok(config) => {
                    info!(path = %self.config_path.display(), "Loaded Datadog config");
                    self.yaml_config = Some(config);
                    self.yaml_config.as_ref()
                }
                Err(e) => {
                    warn!(path = %self.config_path.display(), error = %e, "Failed to parse Datadog config");
                    None
                }
            },
            Err(e) => {
                warn!(path = %self.config_path.display(), error = %e, "Failed to read Datadog config");
                None
            }
        }
    }

    /// Get a string value with env var priority
    /// Checks env vars first (in order), then yaml, then returns default
    fn get_string(&mut self, yaml_path: &str, env_vars: &[&str], default: &str) -> String {
        // Check env vars first (in order of priority)
        for env_var in env_vars {
            if let Ok(value) = env::var(env_var) {
                debug!(env = env_var, value = %value, "Using env var");
                return value;
            }
        }

        // Check YAML config - load first, then query
        self.load_yaml_config();
        if let Some(ref yaml) = self.yaml_config {
            if let Some(value) = Self::get_yaml_value_static(yaml, yaml_path) {
                debug!(path = yaml_path, value = %value, "Using YAML config");
                return value;
            }
        }

        // Return default
        debug!(path = yaml_path, default = default, "Using default value");
        default.to_string()
    }

    /// Get an integer value with env var priority
    fn get_int(&mut self, yaml_path: &str, env_vars: &[&str], default: i32) -> i32 {
        // Check env vars first
        for env_var in env_vars {
            if let Ok(value) = env::var(env_var) {
                if let Ok(parsed) = value.parse::<i32>() {
                    debug!(env = env_var, value = parsed, "Using env var");
                    return parsed;
                }
            }
        }

        // Check YAML config - load first, then query
        self.load_yaml_config();
        if let Some(ref yaml) = self.yaml_config {
            if let Some(value) = Self::get_yaml_int_static(yaml, yaml_path) {
                debug!(path = yaml_path, value = value, "Using YAML config");
                return value;
            }
        }

        // Return default
        debug!(path = yaml_path, default = default, "Using default value");
        default
    }

    /// Get a boolean value with env var priority
    fn get_bool(&mut self, yaml_path: &str, env_vars: &[&str], default: bool) -> bool {
        // Check env vars first
        for env_var in env_vars {
            if let Ok(value) = env::var(env_var) {
                let parsed = matches!(value.to_lowercase().as_str(), "true" | "1" | "yes");
                debug!(env = env_var, value = parsed, "Using env var");
                return parsed;
            }
        }

        // Check YAML config - load first, then query
        self.load_yaml_config();
        if let Some(ref yaml) = self.yaml_config {
            if let Some(value) = Self::get_yaml_bool_static(yaml, yaml_path) {
                debug!(path = yaml_path, value = value, "Using YAML config");
                return value;
            }
        }

        // Return default
        debug!(path = yaml_path, default = default, "Using default value");
        default
    }

    /// Get a string value from YAML by dotted path (static version)
    fn get_yaml_value_static(yaml: &DatadogYamlConfig, path: &str) -> Option<String> {
        match path {
            "bind_host" => yaml.bind_host.clone(),
            "apm_config.receiver_socket" => {
                yaml.apm_config.as_ref()?.receiver_socket.clone()
            }
            _ => None,
        }
    }

    /// Get an int value from YAML by dotted path (static version)
    fn get_yaml_int_static(yaml: &DatadogYamlConfig, path: &str) -> Option<i32> {
        match path {
            "apm_config.receiver_port" => {
                yaml.apm_config.as_ref()?.receiver_port.map(|p| p as i32)
            }
            "dogstatsd_port" => yaml.dogstatsd_port.map(|p| p as i32),
            "otlp_config.traces.internal_port" => {
                yaml.otlp_config.as_ref()?.traces.as_ref()?.internal_port.map(|p| p as i32)
            }
            _ => None,
        }
    }

    /// Get a bool value from YAML by dotted path (static version)
    fn get_yaml_bool_static(yaml: &DatadogYamlConfig, path: &str) -> Option<bool> {
        match path {
            "apm_config.enabled" => yaml.apm_config.as_ref()?.enabled,
            "apm_config.socket_activation.enabled" => {
                yaml.apm_config.as_ref()?.socket_activation.as_ref()?.enabled
            }
            "apm_config.apm_non_local_traffic" => {
                yaml.apm_config.as_ref()?.apm_non_local_traffic
            }
            "otlp_config.traces.enabled" => {
                yaml.otlp_config.as_ref()?.traces.as_ref()?.enabled
            }
            _ => None,
        }
    }

    // =========================================================================
    // Public API - APM Configuration
    // =========================================================================

    /// Check if APM socket activation is enabled
    pub fn is_apm_socket_activation_enabled(&mut self) -> bool {
        self.get_bool(
            "apm_config.socket_activation.enabled",
            &["DD_APM_SOCKET_ACTIVATION_ENABLED"],
            false,
        )
    }

    /// Check if APM is enabled
    pub fn is_apm_enabled(&mut self) -> bool {
        self.get_bool("apm_config.enabled", &["DD_APM_ENABLED"], true)
    }

    /// Get the APM receiver port
    pub fn get_apm_receiver_port(&mut self) -> i32 {
        self.get_int(
            "apm_config.receiver_port",
            &["DD_APM_RECEIVER_PORT", "DD_RECEIVER_PORT"],
            8126,
        )
    }

    /// Get the APM receiver socket path
    pub fn get_apm_receiver_socket(&mut self) -> String {
        #[cfg(target_os = "linux")]
        let default = "/var/run/datadog/apm.socket";

        #[cfg(not(target_os = "linux"))]
        let default = "";

        self.get_string(
            "apm_config.receiver_socket",
            &["DD_APM_RECEIVER_SOCKET"],
            default,
        )
    }

    /// Get the bind host for APM receiver
    pub fn get_apm_bind_host(&mut self) -> String {
        // Check non-local traffic first (overrides to 0.0.0.0)
        let non_local = self.get_bool(
            "apm_config.apm_non_local_traffic",
            &["DD_APM_NON_LOCAL_TRAFFIC"],
            false,
        );

        if non_local {
            return "0.0.0.0".to_string();
        }

        // Check bind_host
        // Default to 127.0.0.1 instead of "localhost" to avoid dual-stack binding
        // on Windows where "localhost" resolves to both IPv4 and IPv6 addresses
        self.get_string("bind_host", &["DD_BIND_HOST"], "127.0.0.1")
    }

    // =========================================================================
    // Public API - OTLP Configuration
    // =========================================================================

    /// Check if OTLP traces are enabled
    pub fn is_otlp_traces_enabled(&mut self) -> bool {
        self.get_bool("otlp_config.traces.enabled", &[], true)
    }

    /// Get the OTLP internal port
    pub fn get_otlp_internal_port(&mut self) -> i32 {
        self.get_int("otlp_config.traces.internal_port", &[], 5003)
    }

    // =========================================================================
    // Socket Config Resolution
    // =========================================================================

    /// Resolve a SocketConfig from its config_source
    ///
    /// Reads env vars and datadog.yaml to fill in listen_stream, listen_unix, etc.
    pub fn resolve_socket_config(
        &mut self,
        config: &SocketConfig,
    ) -> Result<Vec<ResolvedSocketConfig>, DomainError> {
        match config.config_source {
            ConfigSource::Explicit => {
                // No resolution needed - use explicit values
                Ok(vec![ResolvedSocketConfig {
                    listen_stream: config.listen_stream.clone(),
                    listen_unix: config.listen_unix.clone(),
                    tcp_fd_env_var: config.fd_env_var.clone(),
                    unix_fd_env_var: None,
                }])
            }

            ConfigSource::DatadogApm => self.resolve_apm_sockets(),

            ConfigSource::DatadogOtlp => self.resolve_otlp_sockets(),

            ConfigSource::DatadogDogstatsd => self.resolve_dogstatsd_sockets(),
        }
    }

    /// Resolve APM socket configuration
    fn resolve_apm_sockets(&mut self) -> Result<Vec<ResolvedSocketConfig>, DomainError> {
        // Check if socket activation is enabled
        if !self.is_apm_socket_activation_enabled() {
            return Err(DomainError::InvalidConfiguration(
                "APM socket activation is not enabled (DD_APM_SOCKET_ACTIVATION_ENABLED=false)"
                    .to_string(),
            ));
        }

        // Check if APM is enabled
        if !self.is_apm_enabled() {
            return Err(DomainError::InvalidConfiguration(
                "APM is not enabled (DD_APM_ENABLED=false)".to_string(),
            ));
        }

        let mut result = ResolvedSocketConfig {
            listen_stream: None,
            listen_unix: None,
            tcp_fd_env_var: None,
            unix_fd_env_var: None,
        };

        // TCP receiver
        let port = self.get_apm_receiver_port();
        if port > 0 {
            let host = self.get_apm_bind_host();
            result.listen_stream = Some(format!("{}:{}", host, port));
            result.tcp_fd_env_var = Some("DD_APM_NET_RECEIVER_FD".to_string());
            info!(
                host = %host,
                port = port,
                "APM TCP receiver configured"
            );
        }

        // Unix socket receiver (Linux only)
        let socket_path = self.get_apm_receiver_socket();
        if !socket_path.is_empty() {
            result.listen_unix = Some(PathBuf::from(&socket_path));
            result.unix_fd_env_var = Some("DD_APM_UNIX_RECEIVER_FD".to_string());
            info!(path = %socket_path, "APM Unix socket configured");
        }

        if result.listen_stream.is_none() && result.listen_unix.is_none() {
            return Err(DomainError::InvalidConfiguration(
                "No APM receivers configured (both TCP and Unix disabled)".to_string(),
            ));
        }

        Ok(vec![result])
    }

    /// Resolve OTLP socket configuration
    fn resolve_otlp_sockets(&mut self) -> Result<Vec<ResolvedSocketConfig>, DomainError> {
        if !self.is_otlp_traces_enabled() {
            return Err(DomainError::InvalidConfiguration(
                "OTLP traces are not enabled".to_string(),
            ));
        }

        let port = self.get_otlp_internal_port();
        let host = self.get_apm_bind_host(); // Uses same bind_host as APM

        Ok(vec![ResolvedSocketConfig {
            listen_stream: Some(format!("{}:{}", host, port)),
            listen_unix: None,
            tcp_fd_env_var: Some("DD_OTLP_CONFIG_GRPC_FD".to_string()),
            unix_fd_env_var: None,
        }])
    }

    /// Resolve DogStatsD socket configuration
    fn resolve_dogstatsd_sockets(&mut self) -> Result<Vec<ResolvedSocketConfig>, DomainError> {
        let port = self.get_int("dogstatsd_port", &["DD_DOGSTATSD_PORT"], 8125);
        let socket_path = self.get_string(
            "dogstatsd_socket",
            &["DD_DOGSTATSD_SOCKET"],
            "/var/run/datadog/dsd.socket",
        );

        let mut result = ResolvedSocketConfig {
            listen_stream: None,
            listen_unix: None,
            tcp_fd_env_var: None,
            unix_fd_env_var: None,
        };

        if port > 0 {
            result.listen_stream = Some(format!("0.0.0.0:{}", port));
            // DogStatsD uses UDP, but we'll set env var for consistency
            result.tcp_fd_env_var = Some("DD_DOGSTATSD_FD".to_string());
        }

        if !socket_path.is_empty() {
            result.listen_unix = Some(PathBuf::from(socket_path));
            result.unix_fd_env_var = Some("DD_DOGSTATSD_SOCKET_FD".to_string());
        }

        Ok(vec![result])
    }
}

impl Default for DatadogConfigReader {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::env;
    use std::sync::Mutex;

    // Global mutex to ensure tests run serially (env vars are shared across threads)
    static ENV_MUTEX: Mutex<()> = Mutex::new(());

    /// Clean all DD env vars to ensure test isolation
    fn clean_env_vars() {
        env::remove_var("DD_APM_RECEIVER_PORT");
        env::remove_var("DD_RECEIVER_PORT");
        env::remove_var("DD_APM_SOCKET_ACTIVATION_ENABLED");
        env::remove_var("DD_APM_ENABLED");
        env::remove_var("DD_APM_RECEIVER_SOCKET");
        env::remove_var("DD_APM_NON_LOCAL_TRAFFIC");
        env::remove_var("DD_BIND_HOST");
    }

    #[test]
    fn test_env_var_priority() {
        let _lock = ENV_MUTEX.lock().unwrap();
        clean_env_vars();

        // Set env var
        env::set_var("DD_APM_RECEIVER_PORT", "9999");

        let mut reader = DatadogConfigReader::with_path(PathBuf::from("/nonexistent"));
        let port = reader.get_apm_receiver_port();

        assert_eq!(port, 9999);

        clean_env_vars();
    }

    #[test]
    fn test_default_values() {
        let _lock = ENV_MUTEX.lock().unwrap();
        clean_env_vars();

        let mut reader = DatadogConfigReader::with_path(PathBuf::from("/nonexistent"));
        let port = reader.get_apm_receiver_port();

        assert_eq!(port, 8126); // Default

        clean_env_vars();
    }

    #[test]
    fn test_env_var_alias() {
        let _lock = ENV_MUTEX.lock().unwrap();
        clean_env_vars();

        // Test that DD_RECEIVER_PORT works as alias
        env::set_var("DD_RECEIVER_PORT", "7777");

        let mut reader = DatadogConfigReader::with_path(PathBuf::from("/nonexistent"));
        let port = reader.get_apm_receiver_port();

        assert_eq!(port, 7777);

        clean_env_vars();
    }

    #[test]
    fn test_primary_env_var_takes_precedence() {
        let _lock = ENV_MUTEX.lock().unwrap();
        clean_env_vars();

        // Both env vars set - primary should win
        env::set_var("DD_APM_RECEIVER_PORT", "1111");
        env::set_var("DD_RECEIVER_PORT", "2222");

        let mut reader = DatadogConfigReader::with_path(PathBuf::from("/nonexistent"));
        let port = reader.get_apm_receiver_port();

        assert_eq!(port, 1111); // Primary wins

        clean_env_vars();
    }

    #[test]
    fn test_socket_activation_disabled_by_default() {
        let _lock = ENV_MUTEX.lock().unwrap();
        clean_env_vars();

        let mut reader = DatadogConfigReader::with_path(PathBuf::from("/nonexistent"));
        assert!(!reader.is_apm_socket_activation_enabled());

        clean_env_vars();
    }

    #[test]
    fn test_socket_activation_enabled_via_env() {
        let _lock = ENV_MUTEX.lock().unwrap();
        clean_env_vars();

        env::set_var("DD_APM_SOCKET_ACTIVATION_ENABLED", "true");

        let mut reader = DatadogConfigReader::with_path(PathBuf::from("/nonexistent"));
        assert!(reader.is_apm_socket_activation_enabled());

        clean_env_vars();
    }

    #[test]
    fn test_non_local_traffic_overrides_bind_host() {
        let _lock = ENV_MUTEX.lock().unwrap();
        clean_env_vars();

        env::set_var("DD_APM_NON_LOCAL_TRAFFIC", "true");

        let mut reader = DatadogConfigReader::with_path(PathBuf::from("/nonexistent"));
        let host = reader.get_apm_bind_host();

        assert_eq!(host, "0.0.0.0");

        clean_env_vars();
    }

    #[test]
    fn test_resolve_apm_requires_socket_activation_enabled() {
        let _lock = ENV_MUTEX.lock().unwrap();
        clean_env_vars();

        let config = SocketConfig::from_datadog_apm("apm".to_string(), "trace-agent".to_string());
        let mut reader = DatadogConfigReader::with_path(PathBuf::from("/nonexistent"));

        let result = reader.resolve_socket_config(&config);
        assert!(result.is_err());

        clean_env_vars();
    }

    #[test]
    fn test_resolve_apm_sockets() {
        let _lock = ENV_MUTEX.lock().unwrap();
        clean_env_vars();

        env::set_var("DD_APM_SOCKET_ACTIVATION_ENABLED", "true");
        env::set_var("DD_APM_ENABLED", "true");
        env::set_var("DD_APM_RECEIVER_PORT", "8126");

        let config = SocketConfig::from_datadog_apm("apm".to_string(), "trace-agent".to_string());
        let mut reader = DatadogConfigReader::with_path(PathBuf::from("/nonexistent"));

        let result = reader.resolve_socket_config(&config).unwrap();
        assert_eq!(result.len(), 1);
        assert!(result[0].listen_stream.is_some());
        assert_eq!(
            result[0].tcp_fd_env_var,
            Some("DD_APM_NET_RECEIVER_FD".to_string())
        );

        clean_env_vars();
    }
}

