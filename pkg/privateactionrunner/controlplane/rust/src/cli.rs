// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::path::PathBuf;

use anyhow::{Result, bail};

fn parse_log_level(level: &str) -> log::Level {
    match level.to_lowercase().as_str() {
        "trace" => log::Level::Trace,
        "debug" => log::Level::Debug,
        "info" => log::Level::Info,
        "warn" | "warning" => log::Level::Warn,
        "error" | "critical" => log::Level::Error,
        _ => log::Level::Info,
    }
}

/// Parsed CLI arguments for par-control.
///
/// PAR-specific values (private key, URN, API host, timing) are read directly
/// from datadog.yaml via --config.  Only deployment/runtime values that are not
/// in the config file are accepted as flags.
#[derive(Debug)]
pub struct Args {
    /// Path to datadog.yaml (--config, required)
    pub config_path: PathBuf,

    /// Path to par-executor binary (--executor-binary, default: sibling of par-control)
    pub executor_binary: Option<PathBuf>,

    /// Unix socket path for IPC with par-executor (--executor-socket)
    pub executor_socket: String,

    /// Log level (--log-level, default: info)
    pub log_level: log::Level,

    /// Log file path (--log-file, optional)
    pub log_file: Option<PathBuf>,

    /// PID file path (--pid, optional)
    pub pid_path: Option<PathBuf>,

    /// Path to datadog.yaml forwarded to par-executor via --cfgpath.
    /// Defaults to the same path as --config (same file, both processes read it).
    pub executor_cfgpath: Option<PathBuf>,

    /// Extra config files forwarded to par-executor via --extracfgpath (-E).
    /// In K8s this is /etc/datadog-agent/privateactionrunner.yaml — the ConfigMap
    /// the operator mounts with the PAR-specific allowlist and settings.
    pub executor_extracfg: Vec<PathBuf>,

    /// Unknown arguments collected for forward-compatibility.
    pub unknown_args: Vec<String>,
}

impl Args {
    pub fn parse(args: impl Iterator<Item = String>) -> Result<Self> {
        let mut iter = args;
        iter.next(); // skip program name

        match iter.next().as_deref() {
            Some("run") => {}
            Some(other) => bail!("unknown command: {other}. Available commands: run"),
            None => bail!("missing command. Available commands: run"),
        }

        let mut config_path = None;
        let mut executor_binary = None;
        let mut executor_socket = None;
        let mut executor_cfgpath = None;
        let mut executor_extracfg: Vec<PathBuf> = Vec::new();
        let mut log_level = None;
        let mut log_file = None;
        let mut pid_path = None;
        let mut unknown_args = Vec::new();

        while let Some(arg) = iter.next() {
            match arg.as_str() {
                "--config" => {
                    if let Some(v) = iter.next() {
                        config_path = Some(PathBuf::from(v));
                    }
                }
                "--executor-binary" => {
                    if let Some(v) = iter.next() {
                        executor_binary = Some(PathBuf::from(v));
                    }
                }
                "--executor-socket" => {
                    if let Some(v) = iter.next() {
                        executor_socket = Some(v);
                    }
                }
                "--executor-cfgpath" => {
                    if let Some(v) = iter.next() {
                        executor_cfgpath = Some(PathBuf::from(v));
                    }
                }
                "--executor-extracfg" => {
                    if let Some(v) = iter.next() {
                        executor_extracfg.push(PathBuf::from(v));
                    }
                }
                "--log-level" => {
                    if let Some(v) = iter.next() {
                        log_level = Some(v);
                    }
                }
                "--log-file" => {
                    if let Some(v) = iter.next() {
                        log_file = Some(PathBuf::from(v));
                    }
                }
                "--pid" => {
                    if let Some(v) = iter.next() {
                        pid_path = Some(PathBuf::from(v));
                    }
                }
                _ => unknown_args.push(arg),
            }
        }

        let config_path =
            config_path.ok_or_else(|| anyhow::anyhow!("missing required argument: --config"))?;

        Ok(Args {
            config_path,
            executor_binary,
            // Default to /tmp — always a writable emptyDir in K8s and writable on bare metal.
            // /var/run/datadog/ is not guaranteed writable with ReadOnlyRootFilesystem=true.
            executor_socket: executor_socket
                .unwrap_or_else(|| "/tmp/par-executor.sock".to_string()),
            executor_cfgpath,
            executor_extracfg,
            log_level: parse_log_level(log_level.as_deref().unwrap_or("info")),
            log_file,
            pid_path,
            unknown_args,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn args(parts: &[&str]) -> impl Iterator<Item = String> {
        parts.iter().map(|s| s.to_string()).collect::<Vec<_>>().into_iter()
    }

    #[test]
    fn test_parse_required_args() {
        let a = Args::parse(args(&["par-control", "run", "--config", "/etc/datadog/datadog.yaml"]))
            .unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(a.config_path, PathBuf::from("/etc/datadog/datadog.yaml"));
        assert_eq!(a.executor_socket, "/tmp/par-executor.sock");
        assert_eq!(a.log_level, log::Level::Info);
        assert!(a.executor_binary.is_none());
        assert!(a.log_file.is_none());
        assert!(a.pid_path.is_none());
    }

    #[test]
    fn test_parse_all_args() {
        let a = Args::parse(args(&[
            "par-control",
            "run",
            "--config", "/etc/dd/datadog.yaml",
            "--executor-binary", "/opt/datadog-agent/embedded/bin/par-executor",
            "--executor-socket", "/tmp/par-executor.sock",
            "--log-level", "debug",
            "--log-file", "/var/log/datadog/par-control.log",
            "--pid", "/var/run/datadog/par-control.pid",
        ]))
        .unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(a.executor_socket, "/tmp/par-executor.sock");
        assert_eq!(a.log_level, log::Level::Debug);
        assert_eq!(a.log_file, Some(PathBuf::from("/var/log/datadog/par-control.log")));
        assert_eq!(a.pid_path, Some(PathBuf::from("/var/run/datadog/par-control.pid")));
    }

    #[test]
    fn test_parse_missing_config() {
        let result = Args::parse(args(&["par-control", "run", "--log-level", "info"]));
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("--config"));
    }

    #[test]
    fn test_unknown_args_collected() {
        let a = Args::parse(args(&[
            "par-control", "run",
            "--config", "/etc/dd/datadog.yaml",
            "--future-flag", "value",
        ]))
        .unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(a.unknown_args, vec!["--future-flag", "value"]);
    }
}
