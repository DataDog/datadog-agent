// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};

use crate::spawn::{SpawnRequest, is_inherit_or_null};

use super::super::{fleet_policies_dir_for_managed_process, install_root, program_data_root};

/// Reject privileged spawn requests that don't exactly match our embedded catalog spec.
pub(super) fn validate_privileged_process_request(
    process_name: &str,
    request: &SpawnRequest,
) -> Result<()> {
    let install_root = install_root();
    let etc_root = program_data_root();

    let spec = privileged_process_spec(process_name, &install_root, &etc_root)?;

    validate_privileged_stdio(process_name, request)?;
    validate_privileged_working_dir(process_name, &spec, request)?;
    validate_privileged_command_args(process_name, &spec, request)?;
    validate_privileged_env(process_name, &spec, request)?;

    Ok(())
}

fn validate_privileged_stdio(process_name: &str, request: &SpawnRequest) -> Result<()> {
    if !is_inherit_or_null(&request.stdout_config) || !is_inherit_or_null(&request.stderr_config) {
        bail!("[{process_name}] refusing privileged spawn: stdout/stderr must be inherit or null");
    }
    Ok(())
}

fn validate_privileged_working_dir(
    process_name: &str,
    spec: &PrivilegedProcessSpec,
    request: &SpawnRequest,
) -> Result<()> {
    if spec.disallow_working_dir && request.working_dir.is_some() {
        bail!("[{process_name}] refusing privileged spawn: working_dir is not allowed");
    }
    Ok(())
}

fn validate_privileged_command_args(
    process_name: &str,
    spec: &PrivilegedProcessSpec,
    request: &SpawnRequest,
) -> Result<()> {
    let norm_cmd = normalize_win_path(&request.command);
    let expected_cmd = normalize_win_path(spec.expected_command.as_str());
    if norm_cmd != expected_cmd {
        bail!(
            "[{process_name}] refusing privileged spawn: unexpected command (got {}, expected {})",
            request.command,
            spec.expected_command
        );
    }

    let norm_args: Vec<_> = request.args.iter().map(|a| normalize_win_path(a)).collect();
    let expected_args: Vec<_> = spec
        .expected_args
        .iter()
        .map(|a| normalize_win_path(a.as_str()))
        .collect();

    if norm_args != expected_args {
        bail!(
            "[{process_name}] refusing privileged spawn: unexpected args {:?} (expected {:?})",
            request.args,
            spec.expected_args
        );
    }
    Ok(())
}

fn validate_privileged_env(
    process_name: &str,
    spec: &PrivilegedProcessSpec,
    request: &SpawnRequest,
) -> Result<()> {
    for (key, expected) in &spec.required_env {
        match request.env.iter().find(|(k, _)| k == key) {
            None => {
                bail!("[{process_name}] refusing privileged spawn: missing required env var {key}");
            }
            Some((_, actual)) if normalize_win_path(actual) != normalize_win_path(expected) => {
                bail!(
                    "[{process_name}] refusing privileged spawn: unexpected value for {key} (got {actual}, expected {expected})"
                );
            }
            _ => {}
        }
    }

    for (k, _) in &request.env {
        if !spec.allowed_env.contains(&k.as_str()) {
            bail!(
                "[{process_name}] refusing privileged spawn: disallowed env var for privileged process: {k}"
            );
        }
    }

    Ok(())
}

struct PrivilegedProcessSpec {
    expected_command: String,
    expected_args: Vec<String>,
    allowed_env: &'static [&'static str],
    required_env: Vec<(String, String)>,
    disallow_working_dir: bool,
}

fn privileged_process_spec(
    process_name: &str,
    install_root: &std::path::PathBuf,
    etc_root: &std::path::PathBuf,
) -> Result<PrivilegedProcessSpec> {
    use crate::spawn::DATADOG_AGENT_PROCESS;

    match process_name {
        DATADOG_AGENT_PROCESS => {
            let fleet_policies_dir = fleet_policies_dir_for_managed_process()
                .to_string_lossy()
                .into_owned();
            Ok(PrivilegedProcessSpec {
                expected_command: install_root
                    .join(r"bin\agent\process-agent.exe")
                    .to_string_lossy()
                    .into_owned(),
                expected_args: vec![
                    "--cfgpath".to_string(),
                    etc_root.join("datadog.yaml").to_string_lossy().into_owned(),
                    "--sysprobe-config".to_string(),
                    etc_root
                        .join("system-probe.yaml")
                        .to_string_lossy()
                        .into_owned(),
                    "--pid".to_string(),
                    install_root
                        .join(r"run\process-agent.pid")
                        .to_string_lossy()
                        .into_owned(),
                ],
                allowed_env: &["DD_FLEET_POLICIES_DIR"],
                required_env: vec![("DD_FLEET_POLICIES_DIR".to_string(), fleet_policies_dir)],
                disallow_working_dir: true,
            })
        }
        other => bail!(
            "[{other}] refusing privileged spawn: no privileged catalog template (internal error?)"
        ),
    }
}

fn normalize_win_path(s: &str) -> String {
    s.replace('/', "\\").to_ascii_lowercase()
}
