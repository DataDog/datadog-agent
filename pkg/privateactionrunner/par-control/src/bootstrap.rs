// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::BootstrapConfig;
use anyhow::{Context, Result, bail};
use std::path::Path;
use std::process::{Command, Output};

pub fn run_bootstrap(argv: &[String]) -> Result<BootstrapConfig> {
    let Some((program, args)) = argv.split_first() else {
        bail!("no bootstrap command is configured; set --bootstrap-command");
    };
    let name = Path::new(program)
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or(program);
    log::info!("running bootstrap command: {name}");

    let output = Command::new(program)
        .args(args)
        .output()
        .with_context(|| format!("failed to run bootstrap command {name}"))?;
    parse_output(name, &output)
}

fn parse_output(program: &str, output: &Output) -> Result<BootstrapConfig> {
    for line in String::from_utf8_lossy(&output.stderr).lines() {
        if !line.trim().is_empty() {
            log::info!("bootstrap: {line}");
        }
    }

    if !output.status.success() {
        bail!(
            "bootstrap command {program} exited with status {}",
            output.status
        );
    }
    if output.stdout.is_empty() {
        bail!("bootstrap command {program} returned no configuration");
    }

    serde_json::from_slice(&output.stdout).map_err(|_| {
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

    fn output(success: bool, stdout: &str) -> Output {
        Output {
            status: status(success),
            stdout: stdout.as_bytes().to_vec(),
            stderr: Vec::new(),
        }
    }

    #[test]
    fn parses_config() {
        let config = parse_output(
            "bootstrap",
            &output(true, r#"{"split_mode":false,"log_level":"debug"}"#),
        )
        .unwrap();

        assert!(!config.split_mode);
        assert_eq!(config.log_level(), log::LevelFilter::Debug);
    }

    #[test]
    fn reports_command_failure() {
        let error = parse_output("bootstrap", &output(false, "secret"))
            .unwrap_err()
            .to_string();

        assert!(error.contains("exited with status"));
        assert!(!error.contains("secret"));
    }

    #[test]
    fn malformed_config_is_not_exposed() {
        let error = parse_output("bootstrap", &output(true, r#"{"private_key":"secret""#))
            .unwrap_err()
            .to_string();

        assert!(error.contains("malformed configuration"));
        assert!(!error.contains("secret"));
    }
}
