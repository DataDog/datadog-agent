// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;
use std::path::PathBuf;
use std::process::Stdio;

use crate::config::ProcessConfig;
use crate::env::{expand_env_vars, parse_environment_file};

use super::stdio;

/// Platform-agnostic spawn inputs for procmgr managed processes.
///
/// The platform backend is responsible for translating this into the OS-
/// specific spawn mechanism (Unix: `Command::spawn`; Windows: impersonation or
/// primary-token APIs).
pub struct SpawnRequest {
    pub command: String,
    pub args: Vec<String>,
    pub env: Vec<(String, String)>,
    pub working_dir: Option<PathBuf>,
    /// Raw stdout value from process config (inherit, null, or file path).
    #[cfg_attr(not(windows), allow(dead_code))]
    pub stdout_config: String,
    /// Raw stderr value from process config (inherit, null, or file path).
    #[cfg_attr(not(windows), allow(dead_code))]
    pub stderr_config: String,
    pub stdout: Stdio,
    pub stderr: Stdio,
}

impl SpawnRequest {
    pub(crate) fn from_config(process_name: &str, config: &ProcessConfig) -> Result<Self> {
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
            stdout_config: config.stdout.clone(),
            stderr_config: config.stderr.clone(),
            stdout: stdio::stdout_from_config(&config.stdout),
            stderr: stdio::stderr_from_config(&config.stderr),
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
