// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::BootstrapConfig;
use anyhow::{Context, Result, bail};
use std::path::Path;
use std::process::{Command, Output};

const CONFIG_PATH_ENV: &str = "DD_PAR_CONTROL_CONFIG_PATH";

pub fn run_bootstrap(argv: &[String]) -> Result<BootstrapConfig> {
    let Some((program, args)) = argv.split_first() else {
        bail!("no bootstrap command is configured; set --bootstrap-command");
    };
    let name = Path::new(program)
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or(program);
    log::info!("running bootstrap command: {name}");

    let config_file = tempfile::NamedTempFile::new()
        .context("failed to create the bootstrap configuration file")?;
    let output = Command::new(program)
        .args(args)
        .env(CONFIG_PATH_ENV, config_file.path())
        .output()
        .with_context(|| format!("failed to run bootstrap command {name}"))?;

    parse_output(name, &output, config_file.path())
}

fn parse_output(program: &str, output: &Output, config_path: &Path) -> Result<BootstrapConfig> {
    for bytes in [&output.stdout, &output.stderr] {
        for line in String::from_utf8_lossy(bytes).lines() {
            if !line.trim().is_empty() {
                log::info!("bootstrap: {line}");
            }
        }
    }

    if !output.status.success() {
        bail!(
            "bootstrap command {program} exited with status {}",
            output.status
        );
    }

    let payload = std::fs::read(config_path)
        .map_err(|_| anyhow::anyhow!("bootstrap command {program} returned no configuration"))?;
    if payload.is_empty() {
        bail!("bootstrap command {program} returned no configuration");
    }
    serde_json::from_slice(&payload).map_err(|_| {
        anyhow::anyhow!("bootstrap command {program} returned malformed configuration")
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::process::ExitStatus;

    fn status(success: bool) -> ExitStatus {
        #[cfg(unix)]
        use std::os::unix::process::ExitStatusExt;
        #[cfg(windows)]
        use std::os::windows::process::ExitStatusExt;

        #[cfg(unix)]
        let raw = if success { 0 } else { 256 };
        #[cfg(windows)]
        let raw = if success { 0 } else { 1 };
        ExitStatus::from_raw(raw)
    }

    fn output(success: bool) -> Output {
        Output {
            status: status(success),
            stdout: Vec::new(),
            stderr: Vec::new(),
        }
    }

    #[test]
    fn parses_config() {
        let file = tempfile::NamedTempFile::new().unwrap();
        std::fs::write(file.path(), r#"{"split_mode":false,"log_level":"debug"}"#).unwrap();

        let config = parse_output("bootstrap", &output(true), file.path()).unwrap();

        assert!(!config.split_mode);
        assert_eq!(config.log_level(), log::LevelFilter::Debug);
    }

    #[test]
    fn reports_command_failure() {
        let file = tempfile::NamedTempFile::new().unwrap();
        let error = parse_output("bootstrap", &output(false), file.path())
            .unwrap_err()
            .to_string();

        assert!(error.contains("exited with status"));
    }

    #[test]
    fn malformed_config_is_not_exposed() {
        let file = tempfile::NamedTempFile::new().unwrap();
        std::fs::write(file.path(), r#"{"private_key":"secret""#).unwrap();

        let error = parse_output("bootstrap", &output(true), file.path())
            .unwrap_err()
            .to_string();

        assert!(error.contains("malformed configuration"));
        assert!(!error.contains("secret"));
    }
}
