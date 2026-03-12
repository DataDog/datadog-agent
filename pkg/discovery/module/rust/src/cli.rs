// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::path::PathBuf;

use anyhow::{Result, bail};

fn parse_log_level(level: &str) -> log::Level {
    match level.to_lowercase().as_str() {
        "trace" => log::Level::Trace,
        "debug" => log::Level::Debug,
        "info" => log::Level::Info,
        "warn" | "warning" => log::Level::Warn,
        "error" | "critical" => log::Level::Error,
        "off" => log::Level::Error, // Rust log crate doesn't have "off", use Error as minimal logging
        _ => log::Level::Info,
    }
}

#[derive(Debug)]
pub struct Args {
    /// Unix socket path (--socket, required)
    pub socket_path: String,

    /// Log level (--log-level, default: info)
    pub log_level: log::Level,

    /// Log file path (--log-file, optional)
    pub log_file: Option<PathBuf>,

    /// PID file path (--pid, optional)
    pub pid_path: Option<PathBuf>,
}

impl Args {
    pub fn parse(args: impl Iterator<Item = String>) -> Result<Self> {
        let mut iter = args;

        // Skip program name.
        iter.next();

        // Expect subcommand as first real argument.
        match iter.next().as_deref() {
            Some("run") => {}
            Some(other) => bail!("unknown command: {other}. Available commands: run"),
            None => bail!("missing command. Available commands: run"),
        }

        let mut socket_path = None;
        let mut log_level = None;
        let mut log_file = None;
        let mut pid_path = None;

        while let Some(arg) = iter.next() {
            if arg == "--socket" {
                if let Some(next) = iter.next() {
                    socket_path = Some(next);
                }
                continue;
            }
            if arg == "--log-level" {
                if let Some(next) = iter.next() {
                    log_level = Some(next);
                }
                continue;
            }
            if arg == "--log-file" {
                if let Some(next) = iter.next() {
                    log_file = Some(PathBuf::from(next));
                }
                continue;
            }
            if arg == "--pid" {
                if let Some(next) = iter.next() {
                    pid_path = Some(PathBuf::from(next));
                }
                continue;
            }
        }

        let Some(socket_path) = socket_path else {
            bail!("missing required argument: --socket");
        };
        let log_level = parse_log_level(log_level.as_deref().unwrap_or("info"));

        Ok(Args {
            socket_path,
            log_level,
            log_file,
            pid_path,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn args(parts: &[&str]) -> Vec<String> {
        parts.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn test_parse_required_args() {
        let a = Args::parse(
            args(&["system-probe-lite", "run", "--socket", "/run/sysprobe.sock"]).into_iter(),
        )
        .unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(a.socket_path, "/run/sysprobe.sock");
        assert_eq!(a.log_level, log::Level::Info);
        assert!(a.log_file.is_none());
        assert!(a.pid_path.is_none());
    }

    #[test]
    fn test_parse_with_log_file() {
        let a = Args::parse(
            args(&[
                "system-probe-lite",
                "run",
                "--socket",
                "/run/sysprobe.sock",
                "--log-level",
                "info",
                "--log-file",
                "/var/log/datadog/system-probe.log",
            ])
            .into_iter(),
        )
        .unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(a.socket_path, "/run/sysprobe.sock");
        assert_eq!(a.log_level, log::Level::Info);
        assert_eq!(
            a.log_file,
            Some(PathBuf::from("/var/log/datadog/system-probe.log"))
        );
        assert!(a.pid_path.is_none());
    }

    #[test]
    fn test_parse_all_args() {
        let a = Args::parse(
            args(&[
                "system-probe-lite",
                "run",
                "--socket",
                "/run/sysprobe.sock",
                "--log-level",
                "warn",
                "--log-file",
                "/var/log/datadog/system-probe.log",
                "--pid",
                "/var/run/system-probe-lite.pid",
            ])
            .into_iter(),
        )
        .unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(a.socket_path, "/run/sysprobe.sock");
        assert_eq!(a.log_level, log::Level::Warn);
        assert_eq!(
            a.log_file,
            Some(PathBuf::from("/var/log/datadog/system-probe.log"))
        );
        assert_eq!(
            a.pid_path,
            Some(PathBuf::from("/var/run/system-probe-lite.pid"))
        );
    }

    #[test]
    fn test_parse_missing_socket() {
        let result =
            Args::parse(args(&["system-probe-lite", "run", "--log-level", "info"]).into_iter());
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("--socket"),
            "error should mention --socket: {err}"
        );
    }

    #[test]
    fn test_parse_log_level_defaults_to_info() {
        let a = Args::parse(
            args(&["system-probe-lite", "run", "--socket", "/run/sysprobe.sock"]).into_iter(),
        )
        .unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(a.log_level, log::Level::Info);
    }

    #[test]
    fn test_parse_missing_subcommand() {
        let result = Args::parse(args(&["system-probe-lite"]).into_iter());
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("missing command"),
            "error should mention missing command: {err}"
        );
        assert!(
            err.contains("Available commands: run"),
            "error should list available commands: {err}"
        );
    }

    #[test]
    fn test_parse_unknown_subcommand() {
        let result = Args::parse(args(&["system-probe-lite", "bogus"]).into_iter());
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("unknown command: bogus"),
            "error should mention unknown command: {err}"
        );
        assert!(
            err.contains("Available commands: run"),
            "error should list available commands: {err}"
        );
    }

    #[test]
    fn test_parse_log_level() {
        assert_eq!(parse_log_level("trace"), log::Level::Trace);
        assert_eq!(parse_log_level("debug"), log::Level::Debug);
        assert_eq!(parse_log_level("info"), log::Level::Info);
        assert_eq!(parse_log_level("warn"), log::Level::Warn);
        assert_eq!(parse_log_level("warning"), log::Level::Warn);
        assert_eq!(parse_log_level("error"), log::Level::Error);
        assert_eq!(parse_log_level("critical"), log::Level::Error);
        assert_eq!(parse_log_level("off"), log::Level::Error);
        assert_eq!(parse_log_level("INFO"), log::Level::Info);
        assert_eq!(parse_log_level("unknown"), log::Level::Info);
    }
}
