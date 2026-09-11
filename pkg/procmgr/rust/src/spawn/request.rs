// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;
use std::ffi::OsString;
use std::path::PathBuf;
#[cfg(not(windows))]
use tokio::process::Command;

use crate::config::ProcessConfig;
use crate::env::{expand_env_vars, parse_environment_file, try_expand_env_vars};

#[cfg(not(windows))]
use super::stdio::to_command_stdio;
use super::stdio::{StdioSetting, parse_stdio_setting};

pub(crate) struct SpawnRequest {
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

    #[cfg(not(windows))]
    pub(crate) fn to_command(&self, stdout_inheritable: bool, stderr_inheritable: bool) -> Command {
        let mut cmd = Command::new(&self.command);
        cmd.args(&self.args);
        cmd.env_clear();
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
    // Parent environment inheritance is opt-in so host-managed children stay isolated.
    let prefixes = env_list("DD_PM_INHERIT_ENV_PREFIXES");
    let exact_names = env_list("DD_PM_INHERIT_ENV_NAMES");
    let mut env = if prefixes.is_empty() && exact_names.is_empty() {
        Vec::new()
    } else {
        collect_inherited_env(std::env::vars_os(), &prefixes, &exact_names)
    };

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

fn collect_inherited_env(
    vars: impl IntoIterator<Item = (OsString, OsString)>,
    prefixes: &[String],
    exact_names: &[String],
) -> Vec<(String, String)> {
    vars.into_iter()
        .filter_map(|(name, value)| {
            let name = name.into_string().ok()?;
            let matches = prefixes.iter().any(|prefix| name.starts_with(prefix))
                || exact_names.contains(&name);
            if !matches {
                return None;
            }
            Some((name, value.into_string().ok()?))
        })
        .collect()
}

fn env_list(name: &str) -> Vec<String> {
    std::env::var(name)
        .unwrap_or_default()
        .split(',')
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
        .collect()
}

#[cfg(all(test, unix))]
mod inherited_env_tests {
    use super::collect_inherited_env;
    use std::ffi::OsString;
    use std::os::unix::ffi::OsStringExt;

    #[test]
    fn inherited_env_skips_non_unicode_entries() {
        let vars = [
            (OsString::from_vec(vec![0xff]), OsString::from("value")),
            (OsString::from("DD_BAD"), OsString::from_vec(vec![0xff])),
            (OsString::from("DD_GOOD"), OsString::from("value")),
        ];

        assert_eq!(
            collect_inherited_env(vars, &["DD_".to_string()], &[]),
            vec![("DD_GOOD".to_string(), "value".to_string())]
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::ProcessConfig;

    #[test]
    fn from_config_collects_environment_file_vars() {
        let dir = tempfile::tempdir().unwrap();
        let env_file = dir.path().join("env");
        std::fs::write(&env_file, "EXIT_CODE=42\n").unwrap();

        let config = ProcessConfig {
            environment_file: Some(env_file.to_str().unwrap().to_string()),
            ..Default::default()
        };

        let request = SpawnRequest::from_config("envfile", &config).unwrap();
        assert!(
            request
                .env
                .iter()
                .any(|(k, v)| k == "EXIT_CODE" && v == "42"),
            "environment_file vars must be included in spawn env: {:?}",
            request.env
        );
    }

    #[test]
    fn from_config_env_overrides_environment_file() {
        let dir = tempfile::tempdir().unwrap();
        let env_file = dir.path().join("env");
        std::fs::write(&env_file, "MY_VAR=from_file\n").unwrap();

        let config = ProcessConfig {
            environment_file: Some(env_file.to_str().unwrap().to_string()),
            env: [("MY_VAR".to_string(), "overridden".to_string())]
                .into_iter()
                .collect(),
            ..Default::default()
        };

        let request = SpawnRequest::from_config("override", &config).unwrap();
        assert_eq!(
            request
                .env
                .iter()
                .rev()
                .find(|(k, _)| k == "MY_VAR")
                .map(|(_, v)| v.as_str()),
            Some("overridden")
        );
    }
}
