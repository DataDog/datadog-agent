// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::{debug, warn};
use serde::Deserialize;
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::time::Duration;

const DEFAULT_CONFIG_DIR: &str = "/etc/datadog-agent/processes.d";

fn default_true() -> bool {
    true
}

fn default_inherit() -> String {
    "inherit".to_string()
}

fn default_restart() -> RestartPolicy {
    RestartPolicy::Never
}

#[derive(Debug, Clone, PartialEq, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum RestartPolicy {
    Always,
    OnFailure,
    OnSuccess,
    Never,
}

#[derive(Debug, Deserialize)]
pub struct ProcessConfig {
    #[serde(default)]
    #[allow(dead_code)]
    pub description: Option<String>,
    pub command: String,
    #[serde(default)]
    pub args: Vec<String>,
    #[serde(default)]
    pub env: HashMap<String, String>,
    pub environment_file: Option<String>,
    pub working_dir: Option<String>,
    #[allow(dead_code)]
    pub pidfile: Option<String>,
    #[serde(default = "default_inherit")]
    pub stdout: String,
    #[serde(default = "default_inherit")]
    pub stderr: String,
    #[serde(default = "default_true")]
    pub auto_start: bool,
    pub condition_path_exists: Option<String>,
    pub stop_timeout: Option<u64>,
    #[serde(default = "default_restart")]
    pub restart: RestartPolicy,
    pub restart_sec: Option<f64>,
    pub restart_max_delay_sec: Option<f64>,
    pub start_limit_burst: Option<u32>,
    pub start_limit_interval_sec: Option<u64>,
    pub runtime_success_sec: Option<u64>,
}

const DEFAULT_STOP_TIMEOUT_SECS: u64 = 90;
const DEFAULT_RESTART_SEC: f64 = 1.0;
const DEFAULT_RESTART_MAX_DELAY_SEC: f64 = 60.0;
const DEFAULT_START_LIMIT_BURST: u32 = 5;
const DEFAULT_START_LIMIT_INTERVAL_SEC: u64 = 10;
const DEFAULT_RUNTIME_SUCCESS_SEC: u64 = 1;

impl ProcessConfig {
    pub fn stop_timeout(&self) -> Duration {
        Duration::from_secs(self.stop_timeout.unwrap_or(DEFAULT_STOP_TIMEOUT_SECS))
    }

    pub fn restart_delay(&self) -> f64 {
        self.restart_sec.unwrap_or(DEFAULT_RESTART_SEC)
    }

    pub fn max_restart_delay(&self) -> f64 {
        self.restart_max_delay_sec
            .unwrap_or(DEFAULT_RESTART_MAX_DELAY_SEC)
    }

    pub fn burst_limit(&self) -> u32 {
        self.start_limit_burst.unwrap_or(DEFAULT_START_LIMIT_BURST)
    }

    pub fn burst_interval(&self) -> Duration {
        Duration::from_secs(
            self.start_limit_interval_sec
                .unwrap_or(DEFAULT_START_LIMIT_INTERVAL_SEC),
        )
    }

    pub fn runtime_success(&self) -> Duration {
        Duration::from_secs(
            self.runtime_success_sec
                .unwrap_or(DEFAULT_RUNTIME_SUCCESS_SEC),
        )
    }
}

pub fn config_dir() -> PathBuf {
    std::env::var("DD_PM_CONFIG_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from(DEFAULT_CONFIG_DIR))
}

/// Scan a directory for `*.yaml` files and parse each into a ProcessConfig.
/// The process name is derived from the filename (without extension).
/// Files that fail to parse are logged and skipped.
pub fn load_configs(dir: &Path) -> Result<Vec<(String, ProcessConfig)>> {
    let entries = std::fs::read_dir(dir)
        .with_context(|| format!("failed to read config directory: {}", dir.display()))?;

    let mut yaml_files: Vec<_> = entries
        .filter_map(|e| match e {
            Ok(entry) => Some(entry),
            Err(e) => {
                warn!("skipping unreadable entry in {}: {e}", dir.display());
                None
            }
        })
        .filter(|e| {
            let is_yaml = e
                .path()
                .extension()
                .is_some_and(|ext| ext == "yaml" || ext == "yml");
            if !is_yaml {
                debug!("skipping non-YAML file: {}", e.path().display());
            }
            is_yaml
        })
        .collect();

    yaml_files.sort_by_key(|e| e.file_name());

    let mut configs = Vec::new();
    for entry in yaml_files {
        let path = entry.path();
        let name = path
            .file_stem()
            .and_then(|s| s.to_str())
            .unwrap_or("unknown")
            .to_string();

        match parse_config(&path) {
            Ok(config) => configs.push((name, config)),
            Err(e) => warn!("skipping {}: {e:#}", path.display()),
        }
    }

    Ok(configs)
}

fn parse_config(path: &Path) -> Result<ProcessConfig> {
    let contents =
        std::fs::read_to_string(path).with_context(|| format!("reading {}", path.display()))?;
    let config: ProcessConfig =
        serde_yaml::from_str(&contents).with_context(|| format!("parsing {}", path.display()))?;
    Ok(config)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn test_parse_full_config() {
        let dir = tempfile::tempdir().unwrap();
        let yaml = r#"
description: Test Process
command: /usr/bin/sleep
args:
  - "9999"
env:
  FOO: bar
  BAZ: qux
working_dir: /tmp
pidfile: /tmp/test.pid
stdout: inherit
stderr: inherit
auto_start: true
condition_path_exists: /usr/bin/sleep
"#;
        fs::write(dir.path().join("test-proc.yaml"), yaml).unwrap();

        let configs = load_configs(dir.path()).unwrap();
        assert_eq!(configs.len(), 1);

        let (name, cfg) = &configs[0];
        assert_eq!(name, "test-proc");
        assert_eq!(cfg.command, "/usr/bin/sleep");
        assert_eq!(cfg.args, vec!["9999"]);
        assert_eq!(cfg.env.get("FOO").unwrap(), "bar");
        assert_eq!(cfg.working_dir.as_deref(), Some("/tmp"));
        assert_eq!(cfg.pidfile.as_deref(), Some("/tmp/test.pid"));
        assert!(cfg.auto_start);
        assert_eq!(cfg.condition_path_exists.as_deref(), Some("/usr/bin/sleep"));
    }

    #[test]
    fn test_parse_minimal_config() {
        let dir = tempfile::tempdir().unwrap();
        let yaml = "command: /usr/bin/true\n";
        fs::write(dir.path().join("minimal.yaml"), yaml).unwrap();

        let configs = load_configs(dir.path()).unwrap();
        assert_eq!(configs.len(), 1);

        let (name, cfg) = &configs[0];
        assert_eq!(name, "minimal");
        assert_eq!(cfg.command, "/usr/bin/true");
        assert!(cfg.args.is_empty());
        assert!(cfg.env.is_empty());
        assert!(cfg.auto_start);
        assert_eq!(cfg.stdout, "inherit");
        assert_eq!(cfg.stderr, "inherit");
        assert!(cfg.condition_path_exists.is_none());
    }

    #[test]
    fn test_skips_invalid_files() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("good.yaml"), "command: /usr/bin/true\n").unwrap();
        fs::write(dir.path().join("bad.yaml"), "not: valid: yaml: [").unwrap();

        let configs = load_configs(dir.path()).unwrap();
        assert_eq!(configs.len(), 1);
        assert_eq!(configs[0].0, "good");
    }

    #[test]
    fn test_sorted_alphabetically() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("charlie.yaml"), "command: /c\n").unwrap();
        fs::write(dir.path().join("alpha.yaml"), "command: /a\n").unwrap();
        fs::write(dir.path().join("bravo.yaml"), "command: /b\n").unwrap();

        let configs = load_configs(dir.path()).unwrap();
        let names: Vec<&str> = configs.iter().map(|(n, _)| n.as_str()).collect();
        assert_eq!(names, vec!["alpha", "bravo", "charlie"]);
    }

    #[test]
    fn test_ignores_non_yaml() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("proc.yaml"), "command: /a\n").unwrap();
        fs::write(dir.path().join("readme.txt"), "not a config").unwrap();
        fs::write(dir.path().join("notes.md"), "also not").unwrap();

        let configs = load_configs(dir.path()).unwrap();
        assert_eq!(configs.len(), 1);
    }

    #[test]
    fn test_empty_directory() {
        let dir = tempfile::tempdir().unwrap();
        let configs = load_configs(dir.path()).unwrap();
        assert!(configs.is_empty());
    }

    #[test]
    fn test_auto_start_defaults_true() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("p.yaml"), "command: /a\n").unwrap();
        let configs = load_configs(dir.path()).unwrap();
        assert!(configs[0].1.auto_start);
    }

    #[test]
    fn test_auto_start_false() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(
            dir.path().join("p.yaml"),
            "command: /a\nauto_start: false\n",
        )
        .unwrap();
        let configs = load_configs(dir.path()).unwrap();
        assert!(!configs[0].1.auto_start);
    }

    #[test]
    fn test_load_configs_nonexistent_directory() {
        let result = load_configs(Path::new("/nonexistent/processes.d"));
        assert!(result.is_err());
    }

    #[cfg(unix)]
    #[test]
    fn test_load_configs_unreadable_directory() {
        use nix::unistd::Uid;
        use std::os::unix::fs::PermissionsExt;

        if Uid::effective().is_root() {
            eprintln!("skipping test_load_configs_unreadable_directory: running as root");
            return;
        }

        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("proc.yaml"), "command: /a\n").unwrap();
        fs::set_permissions(dir.path(), fs::Permissions::from_mode(0o000)).unwrap();

        let result = load_configs(dir.path());
        fs::set_permissions(dir.path(), fs::Permissions::from_mode(0o755)).unwrap();
        assert!(result.is_err());
    }

    #[test]
    fn test_ddot_example_config() {
        let dir = tempfile::tempdir().unwrap();
        let yaml = r#"
description: Datadog Distribution of OpenTelemetry Collector
command: /opt/datadog-agent/ext/ddot/embedded/bin/otel-agent
args:
  - run
  - --config
  - /etc/datadog-agent/otel-config.yaml
  - --core-config
  - /etc/datadog-agent/datadog.yaml
  - --pidfile
  - /opt/datadog-agent/run/otel-agent.pid
auto_start: true
condition_path_exists: /opt/datadog-agent/ext/ddot/embedded/bin/otel-agent
env:
  DD_FLEET_POLICIES_DIR: /etc/datadog-agent/managed/datadog-agent/stable
stdout: inherit
stderr: inherit
"#;
        fs::write(dir.path().join("datadog-agent-ddot.yaml"), yaml).unwrap();

        let configs = load_configs(dir.path()).unwrap();
        assert_eq!(configs.len(), 1);
        let (name, cfg) = &configs[0];
        assert_eq!(name, "datadog-agent-ddot");
        assert_eq!(
            cfg.command,
            "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent"
        );
        assert_eq!(cfg.args.len(), 7);
        assert!(cfg.auto_start);
        assert_eq!(
            cfg.condition_path_exists.as_deref(),
            Some("/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent")
        );
    }
}
