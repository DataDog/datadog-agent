// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::collections::HashMap;
use std::os::windows::ffi::OsStrExt;

use anyhow::Result;
use windows_sys::Win32::Foundation::HANDLE;

use super::super::merge_env_overrides;

fn legacy_scm_service_name(process_name: &str) -> Option<&'static str> {
    match process_name {
        "datadog-agent-process" => Some("datadog-process-agent"),
        "datadog-agent-action" => Some("datadog-agent-action"),
        "datadog-agent-ddot" => Some("datadog-otel-agent"),
        _ => None,
    }
}

const LEGACY_SCM_ENV_DENYLIST: &[&str] = &[
    "DD_FLEET_POLICIES_DIR",
    "DD_OTELCOLLECTOR_INSTALLATION_METHOD",
];

fn build_child_env_vars(
    process_name: &str,
    baseline: HashMap<String, String>,
    config_overrides: &[(String, String)],
) -> HashMap<String, String> {
    let mut vars = baseline;
    merge_legacy_scm_env(process_name, &mut vars);
    merge_env_overrides(&mut vars, config_overrides);
    vars
}

fn merge_legacy_scm_env(process_name: &str, vars: &mut HashMap<String, String>) {
    let Some(service_name) = legacy_scm_service_name(process_name) else {
        return;
    };

    let overrides = match legacy_scm_env_overrides(service_name) {
        Ok(overrides) => overrides,
        Err(e) => {
            log::warn!(
                "[{process_name}] failed to read legacy SCM Environment for {service_name}: {e:#}"
            );
            return;
        }
    };

    if overrides.is_empty() {
        return;
    }

    let names: Vec<&str> = overrides.iter().map(|(k, _)| k.as_str()).collect();
    log::info!(
        "[{process_name}] applying {} legacy SCM environment variable(s) from {service_name}: {}",
        names.len(),
        names.join(", ")
    );
    merge_env_overrides(vars, &overrides);
}

fn legacy_scm_env_overrides(service_name: &str) -> anyhow::Result<Vec<(String, String)>> {
    let entries = read_service_environment(service_name)?;
    Ok(filter_legacy_scm_env(&parse_scm_environment_entries(
        &entries,
    )))
}

fn read_service_environment(service_name: &str) -> anyhow::Result<Vec<String>> {
    use windows_registry::LOCAL_MACHINE;
    use windows_sys::Win32::System::Registry::KEY_WOW64_64KEY;

    let key = LOCAL_MACHINE
        .options()
        .read()
        .access(KEY_WOW64_64KEY)
        .open(format!(r"SYSTEM\CurrentControlSet\Services\{service_name}"))?;

    match key.get_multi_string("Environment") {
        Ok(entries) => Ok(entries),
        Err(_) => Ok(Vec::new()),
    }
}

fn parse_scm_environment_entries(entries: &[String]) -> Vec<(String, String)> {
    entries
        .iter()
        .filter_map(|entry| {
            if entry.trim().is_empty() {
                return None;
            }
            let (key, value) = entry.split_once('=')?;
            if key.is_empty() {
                return None;
            }
            Some((key.to_string(), value.to_string()))
        })
        .collect()
}

fn filter_legacy_scm_env(entries: &[(String, String)]) -> Vec<(String, String)> {
    entries
        .iter()
        .filter(|(key, _)| !is_denied_legacy_scm_env_key(key))
        .cloned()
        .collect()
}

fn is_denied_legacy_scm_env_key(key: &str) -> bool {
    LEGACY_SCM_ENV_DENYLIST
        .iter()
        .any(|denied| denied.eq_ignore_ascii_case(key))
}

pub(crate) fn build_windows_command_line(command: &str, args: &[String]) -> String {
    let mut cmdline = windows_command_line_arg(command);
    for arg in args {
        cmdline.push(' ');
        cmdline.push_str(&windows_command_line_arg(arg));
    }
    cmdline
}

pub(crate) fn env_block_from_baseline_plus_overrides(
    process_name: &str,
    token: HANDLE,
    overrides: &[(String, String)],
) -> Result<Vec<u16>> {
    let baseline = super::super::baseline_env_vars_for_spawn(process_name, token);
    let vars = build_child_env_vars(process_name, baseline, overrides);
    Ok(env_vars_to_wide_block(&vars))
}

pub(crate) fn env_vars_to_wide_block(vars: &HashMap<String, String>) -> Vec<u16> {
    let mut keys: Vec<&String> = vars.keys().collect();
    keys.sort_by(|a, b| {
        a.to_ascii_lowercase()
            .cmp(&b.to_ascii_lowercase())
            .then_with(|| a.cmp(b))
    });

    let mut block: Vec<u16> = Vec::new();
    for k in keys {
        let kv = format!("{k}={}", vars[k]);
        block.extend(std::ffi::OsStr::new(&kv).encode_wide());
        block.push(0);
    }
    block.push(0);
    block
}

fn windows_command_line_arg(s: &str) -> String {
    if s.is_empty() {
        return "\"\"".to_string();
    }
    if !s.chars().any(|ch| ch.is_whitespace() || ch == '"') {
        return s.to_string();
    }

    let mut out = String::new();
    out.push('"');
    let mut backslashes = 0usize;
    for ch in s.chars() {
        match ch {
            '\\' => backslashes += 1,
            '"' => {
                out.push_str(&"\\".repeat(backslashes * 2 + 1));
                out.push('"');
                backslashes = 0;
            }
            _ => {
                out.push_str(&"\\".repeat(backslashes));
                out.push(ch);
                backslashes = 0;
            }
        }
    }
    out.push_str(&"\\".repeat(backslashes * 2));
    out.push('"');
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn command_line_quotes_only_when_needed_for_cmd_c() {
        let line = build_windows_command_line("cmd.exe", &["/C".to_string(), "exit 1".to_string()]);
        assert_eq!(line, r#"cmd.exe /C "exit 1""#);
    }

    #[test]
    fn command_line_quotes_paths_with_spaces() {
        let line = build_windows_command_line(r"C:\Program Files\app.exe", &["--flag".to_string()]);
        assert_eq!(line, r#""C:\Program Files\app.exe" --flag"#);
    }

    #[test]
    fn env_block_is_sorted_case_insensitively() {
        let mut vars = HashMap::new();
        vars.insert("ZZZ".to_string(), "1".to_string());
        vars.insert("aaa".to_string(), "2".to_string());
        vars.insert("BBB".to_string(), "3".to_string());

        let block = env_vars_to_wide_block(&vars);
        let entries = wide_block_entries(&block);
        assert_eq!(entries, ["aaa=2", "BBB=3", "ZZZ=1"]);
    }

    fn wide_block_entries(block: &[u16]) -> Vec<String> {
        let mut entries = Vec::new();
        let mut start = 0usize;
        for (i, &unit) in block.iter().enumerate() {
            if unit == 0 {
                if i == start {
                    break;
                }
                entries.push(String::from_utf16_lossy(&block[start..i]));
                start = i + 1;
            }
        }
        entries
    }

    #[test]
    fn parse_scm_environment_entries_skips_empty_and_malformed() {
        let entries = vec![
            "DD_PROXY_HTTP=http://proxy.example.com".to_string(),
            "MALFORMED".to_string(),
            "".to_string(),
            "   ".to_string(),
            "  DD_LOG_LEVEL=debug  ".to_string(),
        ];
        let parsed = parse_scm_environment_entries(&entries);
        assert_eq!(
            parsed,
            [
                (
                    "DD_PROXY_HTTP".to_string(),
                    "http://proxy.example.com".to_string()
                ),
                ("  DD_LOG_LEVEL".to_string(), "debug  ".to_string()),
            ]
        );
    }

    #[test]
    fn parse_scm_environment_entries_preserves_value_whitespace() {
        let entries = vec!["DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED=true ".to_string()];
        let parsed = parse_scm_environment_entries(&entries);
        assert_eq!(
            parsed,
            [(
                "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED".to_string(),
                "true ".to_string()
            )]
        );
    }

    #[test]
    fn filter_legacy_scm_env_drops_denylisted_keys_case_insensitively() {
        let entries = vec![
            ("DD_PROXY_HTTP".to_string(), "http://x".to_string()),
            ("dd_fleet_policies_dir".to_string(), r"C:\stale".to_string()),
            (
                "DD_OTELCOLLECTOR_INSTALLATION_METHOD".to_string(),
                "bare-metal".to_string(),
            ),
            ("DD_LOG_LEVEL".to_string(), "debug".to_string()),
        ];
        let filtered = filter_legacy_scm_env(&entries);
        assert_eq!(
            filtered,
            [
                ("DD_PROXY_HTTP".to_string(), "http://x".to_string()),
                ("DD_LOG_LEVEL".to_string(), "debug".to_string()),
            ]
        );
    }

    #[test]
    fn build_child_env_vars_applies_legacy_before_processes_d() {
        let mut vars = HashMap::from([("BASE".to_string(), "1".to_string())]);
        merge_env_overrides(
            &mut vars,
            &[("DD_CUSTOM".to_string(), "from-legacy".to_string())],
        );
        merge_env_overrides(
            &mut vars,
            &[("DD_CUSTOM".to_string(), "from-yaml".to_string())],
        );
        assert_eq!(vars.get("DD_CUSTOM").unwrap(), "from-yaml");
        assert_eq!(vars.get("BASE").unwrap(), "1");
    }

    #[test]
    fn legacy_scm_service_name_maps_procmgr_managed_processes() {
        assert_eq!(
            legacy_scm_service_name("datadog-agent-process"),
            Some("datadog-process-agent")
        );
        assert_eq!(
            legacy_scm_service_name("datadog-agent-action"),
            Some("datadog-agent-action")
        );
        assert_eq!(
            legacy_scm_service_name("datadog-agent-ddot"),
            Some("datadog-otel-agent")
        );
        assert_eq!(legacy_scm_service_name("datadog-agent-trace"), None);
    }
}
