//! Configuration loading from YAML files
//!
//! Only directory-based configuration is supported (`/etc/datadog-agent/process-manager/processes.d/*.yaml`).
//! Each YAML file represents ONE process, with the process name derived from the filename.

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::Path;

/// Top-level configuration structure
#[derive(Debug, Serialize, Deserialize)]
pub struct Config {
    #[serde(default)]
    pub processes: HashMap<String, ProcessConfig>,

    #[serde(default)]
    pub sockets: HashMap<String, SocketConfig>,
}

/// Socket configuration from YAML (systemd-compatible)
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct SocketConfig {
    #[serde(default)]
    pub listen_stream: Option<String>, // TCP: "0.0.0.0:8080"

    #[serde(default)]
    pub listen_datagram: Option<String>, // UDP

    #[serde(default)]
    pub listen_unix: Option<String>, // Unix socket path

    pub service: String, // Process name to activate

    #[serde(default)]
    pub accept: bool, // false = single service, true = per-connection

    #[serde(default)]
    pub socket_mode: Option<String>, // Unix socket permissions (octal string, e.g., "660")

    #[serde(default)]
    pub socket_user: Option<String>, // Unix socket owner

    #[serde(default)]
    pub socket_group: Option<String>, // Unix socket group
}

/// Process configuration from YAML
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct ProcessConfig {
    pub command: String,

    #[serde(default)]
    pub description: Option<String>,

    #[serde(default)]
    pub args: Vec<String>,

    #[serde(default)]
    pub auto_start: bool,

    #[serde(default)]
    pub restart: Option<String>,

    #[serde(default)]
    pub restart_sec: Option<u64>,

    #[serde(default)]
    pub restart_max_delay_sec: Option<u64>,

    #[serde(default)]
    pub start_limit_burst: Option<u32>,

    #[serde(default)]
    pub start_limit_interval_sec: Option<u64>,

    #[serde(default)]
    pub working_dir: Option<String>,

    #[serde(default)]
    pub env: HashMap<String, String>,

    #[serde(default)]
    pub environment_file: Option<String>,

    #[serde(default)]
    pub pidfile: Option<String>,

    #[serde(default)]
    pub stdout: Option<String>,

    #[serde(default)]
    pub stderr: Option<String>,

    #[serde(default)]
    pub timeout_start_sec: Option<u64>,

    #[serde(default)]
    pub timeout_stop_sec: Option<u64>,

    #[serde(default)]
    pub kill_signal: Option<String>,

    #[serde(default)]
    pub kill_mode: Option<String>,

    #[serde(default)]
    pub success_exit_status: Option<Vec<i32>>,

    #[serde(default)]
    pub exec_start_pre: Option<Vec<String>>,

    #[serde(default)]
    pub exec_start_post: Option<Vec<String>>,

    #[serde(default)]
    pub exec_stop_post: Option<Vec<String>>,

    #[serde(default)]
    pub user: Option<String>,

    #[serde(default)]
    pub group: Option<String>,

    #[serde(default)]
    pub ambient_capabilities: Option<Vec<String>>,

    // Runtime directories (systemd RuntimeDirectory=)
    #[serde(default)]
    pub runtime_directory: Option<Vec<String>>,

    // Dependencies (systemd-like)
    #[serde(default)]
    pub after: Option<Vec<String>>,

    #[serde(default)]
    pub before: Option<Vec<String>>,

    #[serde(default)]
    pub requires: Option<Vec<String>>,

    #[serde(default)]
    pub wants: Option<Vec<String>>,

    #[serde(default)]
    pub binds_to: Option<Vec<String>>,

    #[serde(default)]
    pub conflicts: Option<Vec<String>>,

    // Process type (systemd-like)
    #[serde(default)]
    pub process_type: Option<String>,

    // Health check configuration
    #[serde(default)]
    pub health_check: Option<HealthCheckConfig>,

    // Resource limits
    #[serde(default)]
    pub resource_limits: Option<ResourceLimitsConfig>,

    // Conditional starting (systemd-like)
    #[serde(default)]
    pub condition_path_exists: Option<Vec<String>>,
}

/// Resource limits configuration from YAML
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct ResourceLimitsConfig {
    /// CPU limit (e.g., "500m", "1", "2.5")
    #[serde(default)]
    pub cpu: Option<String>,

    /// Memory limit (e.g., "256M", "512M", "1G")
    #[serde(default)]
    pub memory: Option<String>,

    /// Maximum number of PIDs (processes/threads)
    #[serde(default)]
    pub pids: Option<u32>,
}

/// Health check configuration from YAML
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct HealthCheckConfig {
    #[serde(rename = "type")]
    pub check_type: String, // http, tcp, exec

    #[serde(default = "default_interval")]
    pub interval: u64,

    #[serde(default = "default_timeout")]
    pub timeout: u64,

    #[serde(default = "default_retries")]
    pub retries: u32,

    #[serde(default)]
    pub start_period: u64,

    #[serde(default)]
    pub restart_after: u32,

    // HTTP-specific
    #[serde(default)]
    pub endpoint: Option<String>,

    #[serde(default)]
    pub method: Option<String>,

    #[serde(default)]
    pub expected_status: Option<u16>,

    // TCP-specific
    #[serde(default)]
    pub host: Option<String>,

    #[serde(default)]
    pub port: Option<u16>,

    // Exec-specific
    #[serde(default)]
    pub command: Option<String>,

    #[serde(default)]
    pub args: Option<Vec<String>>,
}

fn default_interval() -> u64 {
    30
}

fn default_timeout() -> u64 {
    5
}

fn default_retries() -> u32 {
    3
}

impl Config {
    /// Load configuration from a YAML file
    pub fn load(path: &str) -> Result<Self, String> {
        let contents = std::fs::read_to_string(path)
            .map_err(|e| format!("Failed to read config file '{}': {}", path, e))?;

        // Validate no duplicate process names (serde_yaml silently overwrites duplicates)
        Self::validate_no_duplicate_process_names(&contents)?;

        // Now parse into the Config struct
        serde_yaml::from_str(&contents)
            .map_err(|e| format!("Failed to parse YAML from '{}': {}", path, e))
    }

    /// Validate that the YAML doesn't contain duplicate process names
    /// This catches duplicates that serde_yaml would silently overwrite
    fn validate_no_duplicate_process_names(yaml_content: &str) -> Result<(), String> {
        use std::collections::HashSet;

        let mut seen_names = HashSet::new();
        let mut in_processes_section = false;
        let mut base_indent: Option<usize> = None;

        for line in yaml_content.lines() {
            let trimmed = line.trim();

            // Skip empty lines and comments
            if trimmed.is_empty() || trimmed.starts_with('#') {
                continue;
            }

            // Check if we're entering the processes section
            if trimmed == "processes:" {
                in_processes_section = true;
                base_indent = Some(line.len() - line.trim_start().len());
                continue;
            }

            if in_processes_section {
                let line_indent = line.len() - line.trim_start().len();

                // If we're at or before the base indent level, we've left processes section
                if let Some(base) = base_indent {
                    if line_indent <= base && !trimmed.starts_with('#') {
                        break;
                    }

                    // Process names are one level deeper than "processes:"
                    // and must have a colon (they define keys)
                    if line_indent == base + 2 && trimmed.contains(':') && !trimmed.starts_with('-')
                    {
                        // Extract process name (everything before the colon)
                        if let Some(name) = trimmed.split(':').next() {
                            let process_name = name.trim();
                            if !process_name.is_empty()
                                && !seen_names.insert(process_name.to_string())
                            {
                                return Err(format!(
                                    "Duplicate process name '{}' in configuration. Each process must have a unique name.",
                                    process_name
                                ));
                            }
                        }
                    }
                }
            }
        }

        Ok(())
    }
}

/// Load processes from a configuration directory
///
/// Only directory-based configuration is supported. Each YAML file in the directory
/// represents one process, with the process name derived from the filename.
///
/// # Example
/// ```text
/// /etc/datadog-agent/process-manager/processes.d/
/// ├── datadog-agent.yaml      # Process name: "datadog-agent"
/// ├── trace-agent.yaml        # Process name: "trace-agent"
/// └── process-agent.yaml      # Process name: "process-agent"
/// ```
pub fn load_config_from_path(config_path: &str) -> Result<Vec<(String, ProcessConfig)>, String> {
    let path = Path::new(config_path);

    if path.is_dir() {
        load_from_directory(config_path)
    } else if path.is_file() {
        Err(format!(
            "Single-file configuration is not supported: '{}'\n\n\
            Please use directory-based configuration instead.\n\
            Create a directory (e.g., /etc/datadog-agent/process-manager/processes.d/) with one YAML file per process:\n\n\
            Example:\n\
              /etc/datadog-agent/process-manager/processes.d/\n\
              ├── my-service.yaml     # Process name derived from filename\n\
              └── another-service.yaml\n\n\
            Each file should contain the process configuration directly (no 'processes:' wrapper).",
            config_path
        ))
    } else {
        Err(format!(
            "Configuration directory does not exist: {}",
            config_path
        ))
    }
}

/// Load processes from a directory of YAML files
/// Each file represents ONE process, filename (without extension) is the process name
fn load_from_directory(config_dir: &str) -> Result<Vec<(String, ProcessConfig)>, String> {
    use std::ffi::OsStr;

    let mut entries: Vec<_> = std::fs::read_dir(config_dir)
        .map_err(|e| format!("Failed to read config directory '{}': {}", config_dir, e))?
        .filter_map(|entry| entry.ok())
        .filter(|entry| {
            let path = entry.path();
            let file_name = path.file_name().and_then(|n| n.to_str()).unwrap_or("");

            // Include .yaml/.yml files, but exclude .socket.yaml/.socket.yml files
            let has_yaml_ext = path.extension() == Some(OsStr::new("yaml"))
                || path.extension() == Some(OsStr::new("yml"));
            let is_socket_file =
                file_name.ends_with(".socket.yaml") || file_name.ends_with(".socket.yml");

            has_yaml_ext && !is_socket_file
        })
        .collect();

    if entries.is_empty() {
        return Ok(Vec::new());
    }

    // Sort by filename for deterministic load order
    entries.sort_by_key(|e| e.file_name());

    let mut all_processes = Vec::new();

    // Load each file (each file = one process)
    for entry in entries {
        let path = entry.path();

        // Extract process name from filename (without extension) as default
        let filename_name = path
            .file_stem()
            .and_then(|s| s.to_str())
            .ok_or_else(|| format!("Invalid filename: {:?}", path))?
            .to_string();

        match path.to_str() {
            Some(path_str) => {
                match load_single_process_file_with_name(path_str) {
                    Ok((explicit_name, config)) => {
                        // Use explicit name from YAML if provided (processes: wrapper format)
                        // Otherwise use filename
                        let process_name = explicit_name.unwrap_or(filename_name);
                        all_processes.push((process_name, config));
                    }
                    Err(e) => {
                        // Log error but continue with other files
                        eprintln!(
                            "Warning: Failed to load config file {:?}: {}",
                            path.file_name(),
                            e
                        );
                    }
                }
            }
            None => {
                eprintln!("Warning: Invalid UTF-8 in file path, skipping: {:?}", path);
            }
        }
    }

    Ok(all_processes)
}

/// Load a single process config from a file with its name
/// Returns (Option<explicit_name>, ProcessConfig)
/// Supports two formats:
/// 1. Direct ProcessConfig (no wrapper) - returns (None, config), name from filename
/// 2. Config with 'processes:' wrapper - returns (Some(name), config), name from YAML
fn load_single_process_file_with_name(
    path: &str,
) -> Result<(Option<String>, ProcessConfig), String> {
    let contents = std::fs::read_to_string(path)
        .map_err(|e| format!("Failed to read file '{}': {}", path, e))?;

    // Try to parse as direct ProcessConfig first (no wrapper)
    if let Ok(config) = serde_yaml::from_str::<ProcessConfig>(&contents) {
        // No explicit name, will use filename
        return Ok((None, config));
    }

    // Fall back to Config format with 'processes:' wrapper
    // This is for backward compatibility with old-style directory configs
    if let Ok(full_config) = serde_yaml::from_str::<Config>(&contents) {
        // If there's exactly one process, return it with its name
        if full_config.processes.len() == 1 {
            let (name, config) = full_config.processes.into_iter().next().unwrap();
            return Ok((Some(name), config));
        } else if !full_config.processes.is_empty() {
            return Err(format!(
                "Config file '{}' contains multiple processes ({}). Directory mode expects one process per file.",
                path,
                full_config.processes.len()
            ));
        }
    }

    Err(format!(
        "Failed to parse YAML from '{}': neither ProcessConfig nor Config format",
        path
    ))
}

/// Load sockets from a configuration directory
///
/// Loads .socket.yaml files from the directory (systemd-style naming convention).
///
/// # Example
/// ```text
/// /etc/datadog-agent/process-manager/processes.d/
/// ├── web.socket.yaml         # Socket name: "web"
/// └── api.socket.yaml         # Socket name: "api"
/// ```
pub fn load_sockets_from_path(config_path: &str) -> Result<Vec<(String, SocketConfig)>, String> {
    let path = Path::new(config_path);

    if path.is_dir() {
        load_sockets_from_directory(config_path)
    } else if path.is_file() {
        Err(format!(
            "Single-file socket configuration is not supported: '{}'\n\n\
            Please use directory-based configuration with .socket.yaml files.\n\
            Example: /etc/datadog-agent/process-manager/processes.d/my-service.socket.yaml",
            config_path
        ))
    } else {
        Err(format!(
            "Configuration directory does not exist: {}",
            config_path
        ))
    }
}

/// Load sockets from a directory of .socket.yaml files (systemd-style)
///
/// Naming convention:
/// - `web.socket.yaml` or `web.socket.yml` -> socket name is "web"
/// - Each file contains ONE socket definition
///
/// Supports two formats:
/// 1. Direct SocketConfig (no wrapper) - socket name from filename
/// 2. Config with 'sockets:' wrapper - socket name from YAML key
fn load_sockets_from_directory(config_dir: &str) -> Result<Vec<(String, SocketConfig)>, String> {
    let mut entries: Vec<_> = std::fs::read_dir(config_dir)
        .map_err(|e| format!("Failed to read config directory '{}': {}", config_dir, e))?
        .filter_map(|entry| entry.ok())
        .filter(|entry| {
            let path = entry.path();
            let file_name = path.file_name().and_then(|n| n.to_str()).unwrap_or("");

            // Match *.socket.yaml or *.socket.yml
            file_name.ends_with(".socket.yaml") || file_name.ends_with(".socket.yml")
        })
        .collect();

    if entries.is_empty() {
        return Ok(Vec::new());
    }

    // Sort by filename for deterministic load order
    entries.sort_by_key(|e| e.file_name());

    let mut all_sockets = Vec::new();

    // Load each socket file
    for entry in entries {
        let path = entry.path();

        // Extract socket name from filename: "web.socket.yaml" -> "web"
        let filename_name = path
            .file_name()
            .and_then(|n| n.to_str())
            .and_then(|s| {
                // Remove .socket.yaml or .socket.yml suffix
                s.strip_suffix(".socket.yaml")
                    .or_else(|| s.strip_suffix(".socket.yml"))
            })
            .ok_or_else(|| format!("Invalid socket filename: {:?}", path))?
            .to_string();

        match path.to_str() {
            Some(path_str) => {
                match load_single_socket_file_with_name(path_str) {
                    Ok((explicit_name, config)) => {
                        // Use explicit name from YAML if provided (sockets: wrapper format)
                        // Otherwise use filename
                        let socket_name = explicit_name.unwrap_or(filename_name);
                        all_sockets.push((socket_name, config));
                    }
                    Err(e) => {
                        // Log error but continue with other files
                        eprintln!(
                            "Warning: Failed to load socket file {:?}: {}",
                            path.file_name(),
                            e
                        );
                    }
                }
            }
            None => {
                eprintln!("Warning: Invalid UTF-8 in file path, skipping: {:?}", path);
            }
        }
    }

    Ok(all_sockets)
}

/// Load a single socket config from a file with its name
/// Returns (Option<explicit_name>, SocketConfig)
/// Supports two formats:
/// 1. Direct SocketConfig (no wrapper) - returns (None, config), name from filename
/// 2. Config with 'sockets:' wrapper - returns (Some(name), config), name from YAML
fn load_single_socket_file_with_name(path: &str) -> Result<(Option<String>, SocketConfig), String> {
    let contents = std::fs::read_to_string(path)
        .map_err(|e| format!("Failed to read file '{}': {}", path, e))?;

    // Try to parse as direct SocketConfig first (no wrapper)
    if let Ok(config) = serde_yaml::from_str::<SocketConfig>(&contents) {
        // No explicit name, will use filename
        return Ok((None, config));
    }

    // Fall back to Config format with 'sockets:' wrapper
    if let Ok(full_config) = serde_yaml::from_str::<Config>(&contents) {
        // If there's exactly one socket, return it with its name
        if full_config.sockets.len() == 1 {
            let (name, config) = full_config.sockets.into_iter().next().unwrap();
            return Ok((Some(name), config));
        } else if !full_config.sockets.is_empty() {
            return Err(format!(
                "Socket file '{}' contains multiple sockets ({}). Directory mode expects one socket per file.",
                path,
                full_config.sockets.len()
            ));
        }
    }

    Err(format!(
        "Failed to parse YAML from '{}': neither SocketConfig nor Config format",
        path
    ))
}

/// Determine configuration directory path using precedence rules
///
/// Precedence (first match wins):
/// 1. DD_PM_CONFIG_DIR environment variable
/// 2. /etc/datadog-agent/process-manager/processes.d/ (if directory exists)
/// 3. None (start empty)
///
/// Note: Single-file configuration is not supported.
pub fn get_default_config_path() -> Option<String> {
    // 1. Check DD_PM_CONFIG_DIR environment variable
    if let Ok(path) = std::env::var("DD_PM_CONFIG_DIR") {
        return Some(path);
    }

    // 2. Default directory
    if Path::new("/etc/datadog-agent/process-manager/processes.d").is_dir() {
        return Some("/etc/datadog-agent/process-manager/processes.d".to_string());
    }

    // 3. No config found
    None
}
