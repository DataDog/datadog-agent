// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Identity bootstrap. Before its first OPMS call, par-control runs the
//! Go PAR's ensure-enrollment command. Go owns hostname resolution, enrollment,
//! and persistence. Rust then loads operational config and the identity.

use crate::config::Config;
use anyhow::{Context, Result, bail};
use log::info;
use std::path::Path;
use std::process::Command;

/// Run ensure-enrollment, then load the control-plane config
/// from local YAML, environment, and Fleet policy.
pub fn load_config_with_bootstrap(
    config_path: &Path,
    ensure_enrollment_command: &[String],
) -> Result<Config> {
    if ensure_enrollment_command.is_empty() {
        bail!(
            "no ensure-enrollment command is configured; set \
             --ensure-enrollment-command"
        );
    }

    info!("ensuring runner enrollment: {ensure_enrollment_command:?}");
    run_ensure_enrollment(ensure_enrollment_command)?;

    Config::try_from_yaml_file(config_path)?
        .context("runner has no usable identity after ensure-enrollment completed")
}

fn run_ensure_enrollment(argv: &[String]) -> Result<()> {
    let status = Command::new(&argv[0])
        .args(&argv[1..])
        .status()
        .with_context(|| format!("failed to run ensure-enrollment command {argv:?}"))?;
    if !status.success() {
        bail!("ensure-enrollment command {argv:?} exited with status {status}");
    }
    Ok(())
}
