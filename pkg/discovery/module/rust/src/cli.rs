// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::path::PathBuf;

#[derive(Debug, Default)]
pub struct Args {
    /// Path to system-probe binary (extracted from first arg after --)
    pub fallback_binary: Option<PathBuf>,

    /// All system-probe arguments (everything after --)
    pub system_probe_args: Vec<String>,

    /// Config path (extracted from --config in system-probe args)
    pub config_path: Option<PathBuf>,

    /// PID file path (extracted from --pid in system-probe args)
    pub pid_path: Option<PathBuf>,
}

impl Args {
    pub fn parse(mut args: impl Iterator<Item = String>) -> Self {
        // Find the -- separator
        let Some(_) = args.find(|arg| arg == "--") else {
            // No -- separator, standalone mode
            return Args::default();
        };

        // First arg after -- is the binary path
        let Some(next_arg) = args.next() else {
            // -- was provided but no args after it
            return Args::default();
        };
        let binary = PathBuf::from(next_arg);

        let rest: Vec<String> = args.collect();
        let (config, pid) = extract_paths(&rest);

        Args {
            fallback_binary: Some(binary),
            system_probe_args: rest,
            config_path: config,
            pid_path: pid,
        }
    }
}

/// Extract --config and --pid paths from arguments in a single pass
fn extract_paths(args: &[String]) -> (Option<PathBuf>, Option<PathBuf>) {
    let mut config = None;
    let mut pid = None;
    let mut args = args.iter();

    while let Some(arg) = args.next() {
        // Check for --config or -c
        if matches!(arg.as_str(), "--config" | "-c")
            && let Some(next_arg) = args.next()
        {
            config = Some(PathBuf::from(next_arg));
            continue;
        }
        if let Some(path) = arg.strip_prefix("--config=") {
            config = Some(PathBuf::from(path));
            continue;
        }

        // Check for --pid
        if arg == "--pid"
            && let Some(next_arg) = args.next()
        {
            pid = Some(PathBuf::from(next_arg));
            continue;
        }
        if let Some(path) = arg.strip_prefix("--pid=") {
            pid = Some(PathBuf::from(path));
        }
    }

    (config, pid)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_paths_config_space_separated() {
        let args = vec![
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "--config".to_string(),
            "/etc/config.yaml".to_string(),
        ];
        let (config, pid) = extract_paths(&args);
        assert_eq!(config, Some(PathBuf::from("/etc/config.yaml")));
        assert_eq!(pid, None);
    }

    #[test]
    fn test_extract_paths_config_equals() {
        let args = vec![
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "--config=/etc/config.yaml".to_string(),
        ];
        let (config, pid) = extract_paths(&args);
        assert_eq!(config, Some(PathBuf::from("/etc/config.yaml")));
        assert_eq!(pid, None);
    }

    #[test]
    fn test_extract_paths_config_not_present() {
        let args = vec![
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "--pid=/var/run/sp.pid".to_string(),
        ];
        let (config, pid) = extract_paths(&args);
        assert_eq!(config, None);
        assert_eq!(pid, Some(PathBuf::from("/var/run/sp.pid")));
    }

    #[test]
    fn test_parse_no_separator() {
        let args = vec!["sd-agent".to_string(), "--some-flag".to_string()];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(parsed.fallback_binary, None);
        assert!(parsed.system_probe_args.is_empty());
        assert_eq!(parsed.config_path, None);
    }

    #[test]
    fn test_parse_separator_with_no_args() {
        let args = vec!["sd-agent".to_string(), "--".to_string()];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(parsed.fallback_binary, None);
        assert!(parsed.system_probe_args.is_empty());
        assert_eq!(parsed.config_path, None);
    }

    #[test]
    fn test_parse_separator_with_binary_only() {
        let args = vec![
            "sd-agent".to_string(),
            "--".to_string(),
            "/usr/bin/system-probe".to_string(),
        ];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(
            parsed.fallback_binary,
            Some(PathBuf::from("/usr/bin/system-probe"))
        );
        assert!(parsed.system_probe_args.is_empty());
        assert_eq!(parsed.config_path, None);
    }

    #[test]
    fn test_parse_separator_with_binary_and_args() {
        let args = vec![
            "sd-agent".to_string(),
            "--".to_string(),
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "--pid=/var/run/sp.pid".to_string(),
        ];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(
            parsed.fallback_binary,
            Some(PathBuf::from("/usr/bin/system-probe"))
        );
        assert_eq!(
            parsed.system_probe_args,
            vec!["run", "--pid=/var/run/sp.pid"]
        );
        assert_eq!(parsed.config_path, None);
    }

    #[test]
    fn test_parse_with_config_space_separated() {
        let args = vec![
            "sd-agent".to_string(),
            "--".to_string(),
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "--config".to_string(),
            "/etc/config.yaml".to_string(),
        ];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(
            parsed.fallback_binary,
            Some(PathBuf::from("/usr/bin/system-probe"))
        );
        assert_eq!(
            parsed.system_probe_args,
            vec!["run", "--config", "/etc/config.yaml"]
        );
        assert_eq!(parsed.config_path, Some(PathBuf::from("/etc/config.yaml")));
    }

    #[test]
    fn test_parse_with_config_equals() {
        let args = vec![
            "sd-agent".to_string(),
            "--".to_string(),
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "--config=/etc/config.yaml".to_string(),
        ];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(
            parsed.fallback_binary,
            Some(PathBuf::from("/usr/bin/system-probe"))
        );
        assert_eq!(
            parsed.system_probe_args,
            vec!["run", "--config=/etc/config.yaml"]
        );
        assert_eq!(parsed.config_path, Some(PathBuf::from("/etc/config.yaml")));
    }

    #[test]
    fn test_parse_with_config_short_flag() {
        let args = vec![
            "sd-agent".to_string(),
            "--".to_string(),
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "-c".to_string(),
            "/etc/config.yaml".to_string(),
        ];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(
            parsed.fallback_binary,
            Some(PathBuf::from("/usr/bin/system-probe"))
        );
        assert_eq!(
            parsed.system_probe_args,
            vec!["run", "-c", "/etc/config.yaml"]
        );
        assert_eq!(parsed.config_path, Some(PathBuf::from("/etc/config.yaml")));
    }

    #[test]
    fn test_parse_ignores_args_before_separator() {
        let args = vec![
            "sd-agent".to_string(),
            "--some-flag".to_string(),
            "--another-flag=value".to_string(),
            "--".to_string(),
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
        ];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(
            parsed.fallback_binary,
            Some(PathBuf::from("/usr/bin/system-probe"))
        );
        assert_eq!(parsed.system_probe_args, vec!["run"]);
        assert_eq!(parsed.config_path, None);
    }

    #[test]
    fn test_extract_paths_pid_space_separated() {
        let args = vec![
            "run".to_string(),
            "--pid".to_string(),
            "/var/run/sd-agent.pid".to_string(),
        ];
        let (config, pid) = extract_paths(&args);
        assert_eq!(config, None);
        assert_eq!(pid, Some(PathBuf::from("/var/run/sd-agent.pid")));
    }

    #[test]
    fn test_extract_paths_pid_equals() {
        let args = vec!["run".to_string(), "--pid=/var/run/sd-agent.pid".to_string()];
        let (config, pid) = extract_paths(&args);
        assert_eq!(config, None);
        assert_eq!(pid, Some(PathBuf::from("/var/run/sd-agent.pid")));
    }

    #[test]
    fn test_extract_paths_pid_not_present() {
        let args = vec!["run".to_string(), "--config=/etc/config.yaml".to_string()];
        let (config, pid) = extract_paths(&args);
        assert_eq!(config, Some(PathBuf::from("/etc/config.yaml")));
        assert_eq!(pid, None);
    }

    #[test]
    fn test_parse_with_pid_space_separated() {
        let args = vec![
            "sd-agent".to_string(),
            "--".to_string(),
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "--pid".to_string(),
            "/var/run/sd-agent.pid".to_string(),
        ];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(
            parsed.fallback_binary,
            Some(PathBuf::from("/usr/bin/system-probe"))
        );
        assert_eq!(
            parsed.system_probe_args,
            vec!["run", "--pid", "/var/run/sd-agent.pid"]
        );
        assert_eq!(
            parsed.pid_path,
            Some(PathBuf::from("/var/run/sd-agent.pid"))
        );
    }

    #[test]
    fn test_parse_with_pid_equals() {
        let args = vec![
            "sd-agent".to_string(),
            "--".to_string(),
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "--pid=/var/run/sd-agent.pid".to_string(),
        ];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(
            parsed.fallback_binary,
            Some(PathBuf::from("/usr/bin/system-probe"))
        );
        assert_eq!(
            parsed.system_probe_args,
            vec!["run", "--pid=/var/run/sd-agent.pid"]
        );
        assert_eq!(
            parsed.pid_path,
            Some(PathBuf::from("/var/run/sd-agent.pid"))
        );
    }

    #[test]
    fn test_parse_with_config_and_pid() {
        let args = vec![
            "sd-agent".to_string(),
            "--".to_string(),
            "/usr/bin/system-probe".to_string(),
            "run".to_string(),
            "--config".to_string(),
            "/etc/config.yaml".to_string(),
            "--pid".to_string(),
            "/var/run/sd-agent.pid".to_string(),
        ];
        let parsed = Args::parse(args.into_iter());
        assert_eq!(
            parsed.fallback_binary,
            Some(PathBuf::from("/usr/bin/system-probe"))
        );
        assert_eq!(
            parsed.system_probe_args,
            vec![
                "run",
                "--config",
                "/etc/config.yaml",
                "--pid",
                "/var/run/sd-agent.pid"
            ]
        );
        assert_eq!(parsed.config_path, Some(PathBuf::from("/etc/config.yaml")));
        assert_eq!(
            parsed.pid_path,
            Some(PathBuf::from("/var/run/sd-agent.pid"))
        );
    }
}
