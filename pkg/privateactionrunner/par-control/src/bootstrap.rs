// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Configuration bootstrap. Before doing anything else, par-control runs
//! `privateactionrunner bootstrap-par-control`, which loads the canonical Agent
//! configuration, ensures the runner is enrolled, and returns the resolved
//! control-plane configuration through a private temporary file.

use crate::config::BootstrapConfig;
use anyhow::{Context, Result, bail};
use std::ffi::OsStr;
use std::path::Path;
use std::process::{Command, Output};

/// Must match `ConfigPathEnv` in
/// `cmd/privateactionrunner/subcommands/bootstrapparcontrol`.
const CONFIG_PATH_ENV: &str = "DD_PAR_CONTROL_CONFIG_PATH";

/// Run the bootstrap command and parse the configuration it returns.
pub fn run_bootstrap(argv: &[String]) -> Result<BootstrapConfig> {
    let Some((program, args)) = argv.split_first() else {
        bail!("no bootstrap command is configured; set --bootstrap-command");
    };
    let program_name = program_name(argv);

    log::info!("running the par-control bootstrap command: {program_name}");

    // NamedTempFile creates the file with permissions restricted to this user.
    let config_file = tempfile::NamedTempFile::new()
        .context("failed to create the bootstrap configuration file")?;
    let output = Command::new(program)
        .args(args)
        .env(CONFIG_PATH_ENV, config_file.path())
        .output()
        .with_context(|| format!("failed to run the bootstrap command {program_name}"))?;

    parse_bootstrap_output(program_name, &output, config_file.path())
}

fn parse_bootstrap_output(
    program: &str,
    output: &Output,
    config_path: &Path,
) -> Result<BootstrapConfig> {
    forward_logs(&output.stdout, &output.stderr);

    if !output.status.success() {
        bail!(
            "the bootstrap command {program} exited with status {}",
            output.status
        );
    }

    let payload = std::fs::read(config_path).map_err(|_| {
        anyhow::anyhow!("the bootstrap command {program} returned no configuration")
    })?;
    if payload.is_empty() {
        bail!("the bootstrap command {program} returned no configuration");
    }

    // Serde errors can quote the input, which carries secrets.
    serde_json::from_slice::<BootstrapConfig>(&payload).map_err(|_| {
        anyhow::anyhow!("the bootstrap command {program} returned malformed configuration")
    })
}

fn forward_logs(stdout: &[u8], stderr: &[u8]) {
    for line in String::from_utf8_lossy(stdout).lines() {
        log_line(line);
    }
    for line in String::from_utf8_lossy(stderr).lines() {
        log_line(line);
    }
}

fn log_line(line: &str) {
    if !line.trim().is_empty() {
        log::info!("bootstrap: {line}");
    }
}

/// Name of the bootstrap executable, for logs and errors. Never the full argv.
pub fn program_name(argv: &[String]) -> &str {
    argv.first()
        .map(|program| {
            Path::new(program)
                .file_name()
                .and_then(OsStr::to_str)
                .unwrap_or(program.as_str())
        })
        .unwrap_or("bootstrap-par-control")
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::process::ExitStatus;

    /// `ExitStatus::from_raw` takes a platform-specific wait status.
    fn exit_status(success: bool) -> ExitStatus {
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

    fn output(success: bool, stdout: &str, stderr: &str) -> Output {
        Output {
            status: exit_status(success),
            stdout: stdout.as_bytes().to_vec(),
            stderr: stderr.as_bytes().to_vec(),
        }
    }

    const CONFIG: &str = r#"{"split_mode":true,"log_level":"debug","identity":{"urn":"urn:dd:apps:on-prem-runner:us1:42:r","private_key":"super-secret-key","org_id":42,"runner_id":"r"}}"#;

    fn config_file(contents: &str) -> tempfile::NamedTempFile {
        let file = tempfile::NamedTempFile::new().unwrap();
        std::fs::write(file.path(), contents).unwrap();
        file
    }

    #[test]
    fn parses_the_configuration_file() {
        let file = config_file(CONFIG);
        let cfg = parse_bootstrap_output(
            "privateactionrunner",
            &output(true, "starting up\ndone\n", ""),
            file.path(),
        )
        .unwrap();

        assert!(cfg.split_mode);
        assert_eq!(cfg.log_level(), log::LevelFilter::Debug);
    }

    #[test]
    fn parses_a_disabled_split_mode_gate() {
        let file = config_file(r#"{"split_mode":false,"log_level":"info"}"#);
        let cfg = parse_bootstrap_output("privateactionrunner", &output(true, "", ""), file.path())
            .unwrap();

        assert!(!cfg.split_mode);
    }

    #[test]
    fn rejects_a_nonzero_exit_status() {
        let file = config_file(CONFIG);
        let error = parse_bootstrap_output(
            "privateactionrunner",
            &output(false, "", "enrollment failed\n"),
            file.path(),
        )
        .unwrap_err()
        .to_string();

        assert!(error.contains("exited with status"), "{error}");
    }

    #[test]
    fn rejects_missing_configuration() {
        let dir = tempfile::tempdir().unwrap();
        let error = parse_bootstrap_output(
            "privateactionrunner",
            &output(true, "started\nfinished\n", ""),
            &dir.path().join("missing.json"),
        )
        .unwrap_err()
        .to_string();

        assert!(error.contains("no configuration"), "{error}");
    }

    #[test]
    fn rejects_empty_configuration() {
        let file = config_file("");
        let error =
            parse_bootstrap_output("privateactionrunner", &output(true, "", ""), file.path())
                .unwrap_err()
                .to_string();

        assert!(error.contains("no configuration"), "{error}");
    }

    #[test]
    fn rejects_malformed_configuration_without_exposing_it() {
        let file = config_file(r#"{"identity":{"private_key":"super-secret-key""#);
        let error = format!(
            "{:#}",
            parse_bootstrap_output("privateactionrunner", &output(true, "", ""), file.path(),)
                .unwrap_err()
        );

        assert!(error.contains("malformed configuration"), "{error}");
        assert!(!error.contains("super-secret-key"), "{error}");
    }

    #[cfg(unix)]
    #[test]
    fn temporary_configuration_file_is_private() {
        use std::os::unix::fs::PermissionsExt;

        let file = tempfile::NamedTempFile::new().unwrap();
        let mode = file.as_file().metadata().unwrap().permissions().mode();

        assert_eq!(mode & 0o077, 0);
    }

    #[test]
    fn requires_a_bootstrap_command() {
        let error = run_bootstrap(&[]).unwrap_err().to_string();

        assert!(error.contains("--bootstrap-command"), "{error}");
    }

    /// Errors name the executable only; the argv may carry operator arguments.
    #[test]
    fn reports_the_executable_without_the_argv() {
        let argv = [
            "/opt/datadog-agent/embedded/bin/privateactionrunner".to_string(),
            "bootstrap-par-control".to_string(),
            "--cfgpath".to_string(),
            "/etc/datadog-agent/datadog.yaml".to_string(),
        ];

        assert_eq!(program_name(&argv), "privateactionrunner");
        assert_eq!(program_name(&[]), "bootstrap-par-control");
    }
}
