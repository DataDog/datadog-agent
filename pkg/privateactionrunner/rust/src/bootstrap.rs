// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Identity bootstrap. Before its first OPMS call, par-control must have a valid
//! runner identity. All enrollment and crypto stay in Go: if no identity is
//! persisted yet, the control plane runs the Go one-shot enroll (which creates and
//! persists the identity exactly as today) and then reads the persisted identity.
//! An already-enrolled runner is used as-is with no re-enrollment.

use crate::config::Config;
use anyhow::{bail, Context, Result};
use log::info;
use std::path::Path;
use std::process::Command;

/// Load the control-plane config, running the Go one-shot enroll first if the
/// runner has no identity yet. `enroll_command` is the argv of the Go enroll
/// one-shot, which the installed procmgr definition points at
/// `privateactionrunner rotate-identity --cfgpath <datadog.yaml>`; if empty,
/// bootstrap does not enroll and an existing identity is required.
///
/// `self_enroll` mirrors `private_action_runner.self_enroll`: an operator who
/// turned it off provisions the identity out of band, so par-control must not
/// enroll behind their back — same contract as the Go runner.
pub fn load_config_with_bootstrap(
    config_path: &Path,
    enroll_command: &[String],
    self_enroll: bool,
) -> Result<Config> {
    if let Some(cfg) = Config::try_from_yaml_file(config_path)? {
        return Ok(cfg);
    }
    if !self_enroll {
        bail!(
            "runner has no persisted identity and private_action_runner.self_enroll is false; \
             pre-provision the identity or enable self-enrollment"
        );
    }
    if enroll_command.is_empty() {
        bail!(
            "runner has no persisted identity and no enroll command is configured; \
             set --enroll-command or pre-provision the identity"
        );
    }

    info!("no runner identity found; running one-shot enroll: {enroll_command:?}");
    run_enroll(enroll_command)?;

    Config::try_from_yaml_file(config_path)?
        .context("runner still has no identity after running the enroll command")
}

fn run_enroll(argv: &[String]) -> Result<()> {
    let status = Command::new(&argv[0])
        .args(&argv[1..])
        .status()
        .with_context(|| format!("failed to run enroll command {argv:?}"))?;
    if !status.success() {
        bail!("enroll command {argv:?} exited with status {status}");
    }
    Ok(())
}
