// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;
use std::path::PathBuf;

use crate::config::ProcessConfig;
use crate::env::{expand_env_vars, parse_environment_file};

use super::profile::SpawnProfile;
use super::stdio_setting::{self, StdioSetting};

/// Platform-agnostic spawn inputs for procmgr managed processes.
///
/// The platform backend is responsible for translating this into the OS-
/// specific spawn mechanism (Unix: `Command::spawn`; Windows: `CreateProcessAsUserW`).
pub struct SpawnRequest {
    pub command: String,
    pub args: Vec<String>,
    pub env: Vec<(String, String)>,
    pub working_dir: Option<PathBuf>,
    pub stdout_setting: StdioSetting,
    pub stderr_setting: StdioSetting,
}

impl SpawnRequest {
    pub(crate) fn from_config(
        process_name: &str,
        config: &ProcessConfig,
        profile: SpawnProfile,
    ) -> Result<Self> {
        let stdout = stdio_setting::parse_stdio_setting(&config.stdout);
        let stderr = stdio_setting::parse_stdio_setting(&config.stderr);

        if matches!(profile, SpawnProfile::Privileged) {
            stdio_setting::require_inherit_or_null(process_name, &stdout, &stderr)?;
        }

        Ok(Self {
            command: expand_env_vars(&config.command),
            args: config
                .args
                .iter()
                .map(|a| expand_env_vars(a))
                .collect(),
            env: collect_env(process_name, config)?,
            working_dir: config
                .working_dir
                .as_ref()
                .map(|dir| PathBuf::from(expand_env_vars(dir))),
            stdout_setting: stdout,
            stderr_setting: stderr,
        })
    }
}

/// Merge environment-file variables with inline config env (config wins on conflict).
fn collect_env(process_name: &str, config: &ProcessConfig) -> Result<Vec<(String, String)>> {
    // Platform backends apply baseline env and clear the process env (e.g. Windows
    // `apply_child_baseline_env` after `env_clear`).
    let mut env = Vec::new();

    if let Some(ref raw_path) = config.environment_file {
        let raw_path = expand_env_vars(raw_path);
        let (optional, path) = if let Some(stripped) = raw_path.strip_prefix('-') {
            (true, stripped)
        } else {
            (false, raw_path.as_str())
        };

        if optional && !std::path::Path::new(path).exists() {
            info!("[{process_name}] optional environment file not found, skipping: {path}");
        } else {
            let vars = parse_environment_file(path).with_context(|| {
                format!("[{process_name}] failed to read environment file: {path}")
            })?;
            env.extend(vars);
        }
    }

    for (k, v) in &config.env {
        env.push((k.clone(), expand_env_vars(v)));
    }

    Ok(env)
}
