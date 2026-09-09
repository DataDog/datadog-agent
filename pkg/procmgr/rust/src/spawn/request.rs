// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;
use std::path::PathBuf;
use tokio::process::Command;

use crate::config::ProcessConfig;
use crate::env::{expand_env_vars, parse_environment_file, try_expand_env_vars};

use super::stdio::{StdioSetting, parse_stdio_setting, to_command_stdio};

pub(crate) struct SpawnRequest {
    process_name: String,
    command: String,
    args: Vec<String>,
    env: Vec<(String, String)>,
    working_dir: Option<PathBuf>,
    stdout_setting: StdioSetting,
    stderr_setting: StdioSetting,
}

impl SpawnRequest {
    #[cfg(windows)]
    pub(crate) fn command(&self) -> &str {
        &self.command
    }

    #[cfg(windows)]
    pub(crate) fn args(&self) -> &[String] {
        &self.args
    }

    #[cfg(windows)]
    pub(crate) fn env(&self) -> &[(String, String)] {
        &self.env
    }

    #[cfg(windows)]
    pub(crate) fn working_dir(&self) -> Option<&PathBuf> {
        self.working_dir.as_ref()
    }

    #[cfg(windows)]
    pub(crate) fn stdout_setting(&self) -> &StdioSetting {
        &self.stdout_setting
    }

    #[cfg(windows)]
    pub(crate) fn stderr_setting(&self) -> &StdioSetting {
        &self.stderr_setting
    }

    pub(crate) fn from_config(process_name: &str, config: &ProcessConfig) -> Result<Self> {
        Ok(Self {
            process_name: process_name.to_string(),
            command: expand_env_vars(&config.command),
            args: config.args.iter().map(|a| expand_env_vars(a)).collect(),
            env: collect_env(process_name, config)?,
            working_dir: config
                .working_dir
                .as_ref()
                .map(|dir| PathBuf::from(expand_env_vars(dir))),
            stdout_setting: parse_stdio_setting(&config.stdout),
            stderr_setting: parse_stdio_setting(&config.stderr),
        })
    }

    pub(crate) fn to_command(&self, stdout_inheritable: bool, stderr_inheritable: bool) -> Command {
        let mut cmd = Command::new(&self.command);
        cmd.args(&self.args);
        cmd.env_clear();
        #[cfg(windows)]
        {
            crate::platform::apply_child_baseline_env(&mut cmd);
            crate::platform::apply_legacy_scm_env(&mut cmd, &self.process_name);
        }
        for (k, v) in &self.env {
            cmd.env(k, v);
        }
        if let Some(dir) = &self.working_dir {
            cmd.current_dir(dir);
        }
        cmd.stdout(to_command_stdio(&self.stdout_setting, stdout_inheritable));
        cmd.stderr(to_command_stdio(&self.stderr_setting, stderr_inheritable));
        cmd
    }
}

fn collect_env(process_name: &str, config: &ProcessConfig) -> Result<Vec<(String, String)>> {
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
        match v.strip_prefix('-') {
            Some(template) => match try_expand_env_vars(template) {
                Some(val) => env.push((k.clone(), val)),
                None => info!(
                    "[{process_name}] optional env var {k} references an unset variable, omitting"
                ),
            },
            None => env.push((k.clone(), expand_env_vars(v))),
        }
    }

    Ok(env)
}
