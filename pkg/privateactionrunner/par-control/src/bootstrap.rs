// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Configuration bootstrap. Before doing anything else, par-control runs
//! `privateactionrunner bootstrap-par-control`, which loads the canonical Agent
//! configuration, ensures the runner is enrolled, and returns the resolved
//! control-plane configuration.
//!
//! The configuration line carries the runner private key and may carry proxy
//! credentials, so it is never logged and never included in an error. Ordinary
//! stdout and stderr lines are forwarded as bootstrap logs.

use crate::config::BootstrapConfig;
use anyhow::{Context, Result, bail};
use std::ffi::OsStr;
use std::process::{Command, Output};

/// Prefixes the single stdout line carrying the configuration. Must match
/// `ConfigPrefix` in `cmd/privateactionrunner/subcommands/bootstrapparcontrol`.
const CONFIG_PREFIX: &str = "PAR_CONTROL_CONFIG=";

/// Run the bootstrap command and parse the configuration it returns.
pub fn run_bootstrap(argv: &[String]) -> Result<BootstrapConfig> {
    let Some((program, args)) = argv.split_first() else {
        bail!("no bootstrap command is configured; set --bootstrap-command");
    };

    // Only the executable is named. The argv may carry operator-supplied
    // arguments, which should not be echoed into logs or errors.
    log::info!("running the par-control bootstrap command: {program}");

    let output = Command::new(program)
        .args(args)
        .output()
        .with_context(|| format!("failed to run the bootstrap command {program}"))?;

    parse_bootstrap_output(program, &output)
}

fn parse_bootstrap_output(program: &str, output: &Output) -> Result<BootstrapConfig> {
    let stdout = String::from_utf8_lossy(&output.stdout);
    let stderr = String::from_utf8_lossy(&output.stderr);

    // Forwarded even on failure: these lines explain why bootstrap failed.
    forward_logs(&stdout, &stderr);

    if !output.status.success() {
        // Deliberately carries no captured output: stdout may contain the
        // configuration line.
        bail!(
            "the bootstrap command {program} exited with status {}",
            output.status
        );
    }

    let mut payloads = stdout
        .lines()
        .filter_map(|line| line.trim_end_matches('\r').strip_prefix(CONFIG_PREFIX));
    let payload = payloads
        .next()
        .with_context(|| format!("the bootstrap command {program} returned no configuration"))?;
    if payloads.next().is_some() {
        bail!("the bootstrap command {program} returned more than one configuration");
    }

    // The parse error would quote the offending input, which is the payload.
    serde_json::from_str::<BootstrapConfig>(payload).map_err(|_| {
        anyhow::anyhow!("the bootstrap command {program} returned malformed configuration")
    })
}

/// Forward bootstrap output, dropping the configuration line.
fn forward_logs(stdout: &str, stderr: &str) {
    for line in stdout.lines() {
        if !line.trim_end_matches('\r').starts_with(CONFIG_PREFIX) {
            log_line(line);
        }
    }
    for line in stderr.lines() {
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
            std::path::Path::new(program)
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

    const CONFIG_LINE: &str = r#"PAR_CONTROL_CONFIG={"split_mode":true,"log_level":"debug","identity":{"urn":"urn:dd:apps:on-prem-runner:us1:42:r","private_key":"super-secret-key","org_id":42,"runner_id":"r"}}"#;

    #[test]
    fn parses_the_configuration_line() {
        let out = output(true, &format!("starting up\n{CONFIG_LINE}\ndone\n"), "");

        let cfg = parse_bootstrap_output("privateactionrunner", &out).unwrap();

        assert!(cfg.split_mode);
        assert_eq!(cfg.log_level(), log::LevelFilter::Debug);
    }

    #[test]
    fn parses_a_disabled_split_mode_gate() {
        let out = output(
            true,
            r#"PAR_CONTROL_CONFIG={"split_mode":false,"log_level":"info"}"#,
            "",
        );

        let cfg = parse_bootstrap_output("privateactionrunner", &out).unwrap();

        assert!(!cfg.split_mode);
    }

    /// Windows-style line endings must not defeat prefix matching.
    #[test]
    fn tolerates_carriage_returns() {
        let out = output(true, &format!("log line\r\n{CONFIG_LINE}\r\n"), "");

        assert!(
            parse_bootstrap_output("privateactionrunner", &out)
                .unwrap()
                .split_mode
        );
    }

    #[test]
    fn rejects_a_nonzero_exit_status() {
        let out = output(false, "", "enrollment failed\n");

        let error = parse_bootstrap_output("privateactionrunner", &out)
            .unwrap_err()
            .to_string();

        assert!(error.contains("exited with status"), "{error}");
    }

    #[test]
    fn rejects_missing_configuration() {
        let out = output(true, "started\nfinished\n", "");

        let error = parse_bootstrap_output("privateactionrunner", &out)
            .unwrap_err()
            .to_string();

        assert!(error.contains("no configuration"), "{error}");
    }

    #[test]
    fn rejects_duplicate_configuration() {
        let out = output(true, &format!("{CONFIG_LINE}\n{CONFIG_LINE}\n"), "");

        let error = parse_bootstrap_output("privateactionrunner", &out)
            .unwrap_err()
            .to_string();

        assert!(error.contains("more than one configuration"), "{error}");
    }

    #[test]
    fn rejects_malformed_configuration() {
        let out = output(true, "PAR_CONTROL_CONFIG={\"split_mode\":\n", "");

        let error = parse_bootstrap_output("privateactionrunner", &out)
            .unwrap_err()
            .to_string();

        assert!(error.contains("malformed configuration"), "{error}");
    }

    /// The payload holds the private key, so no error may quote it — including
    /// the malformed-input case, where a serde error would.
    #[test]
    fn errors_never_carry_the_configuration_payload() {
        let malformed = r#"PAR_CONTROL_CONFIG={"identity":{"private_key":"super-secret-key"#;
        for out in [
            output(true, &format!("{CONFIG_LINE}\n{CONFIG_LINE}\n"), ""),
            output(false, &format!("{CONFIG_LINE}\n"), "failed\n"),
            output(true, malformed, ""),
        ] {
            let error = format!(
                "{:#}",
                parse_bootstrap_output("privateactionrunner", &out).unwrap_err()
            );
            assert!(!error.contains("super-secret-key"), "{error}");
            assert!(!error.contains(CONFIG_PREFIX), "{error}");
        }
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
