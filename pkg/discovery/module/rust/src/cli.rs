// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::path::PathBuf;

use anyhow::{Result, bail};

#[derive(Debug)]
pub struct Args {
    /// Unix socket path (--socket, required)
    pub socket_path: String,

    /// Log level (--log-level, required)
    pub log_level: String,

    /// PID file path (--pid, optional)
    pub pid_path: Option<PathBuf>,
}

impl Args {
    pub fn parse(args: impl Iterator<Item = String>) -> Result<Self> {
        let all_args: Vec<String> = args.collect();
        let mut socket_path = None;
        let mut log_level = None;
        let mut pid_path = None;
        let mut iter = all_args.iter();

        while let Some(arg) = iter.next() {
            if arg == "--socket" {
                if let Some(next) = iter.next() {
                    socket_path = Some(next.clone());
                }
                continue;
            }
            if arg == "--log-level" {
                if let Some(next) = iter.next() {
                    log_level = Some(next.clone());
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
        let Some(log_level) = log_level else {
            bail!("missing required argument: --log-level");
        };

        Ok(Args {
            socket_path,
            log_level,
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
            args(&[
                "system-probe-lite",
                "--socket",
                "/run/sysprobe.sock",
                "--log-level",
                "info",
            ])
            .into_iter(),
        )
        .unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(a.socket_path, "/run/sysprobe.sock");
        assert_eq!(a.log_level, "info");
        assert!(a.pid_path.is_none());
    }

    #[test]
    fn test_parse_all_args() {
        let a = Args::parse(
            args(&[
                "system-probe-lite",
                "--socket",
                "/run/sysprobe.sock",
                "--log-level",
                "warn",
                "--pid",
                "/var/run/system-probe-lite.pid",
            ])
            .into_iter(),
        )
        .unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(a.socket_path, "/run/sysprobe.sock");
        assert_eq!(a.log_level, "warn");
        assert_eq!(
            a.pid_path,
            Some(PathBuf::from("/var/run/system-probe-lite.pid"))
        );
    }

    #[test]
    fn test_parse_missing_socket() {
        let result = Args::parse(args(&["system-probe-lite", "--log-level", "info"]).into_iter());
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("--socket"),
            "error should mention --socket: {err}"
        );
    }

    #[test]
    fn test_parse_missing_log_level() {
        let result =
            Args::parse(args(&["system-probe-lite", "--socket", "/run/sysprobe.sock"]).into_iter());
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("--log-level"),
            "error should mention --log-level: {err}"
        );
    }

    #[test]
    fn test_parse_missing_both_required() {
        let result = Args::parse(args(&["system-probe-lite"]).into_iter());
        assert!(result.is_err());
    }
}
