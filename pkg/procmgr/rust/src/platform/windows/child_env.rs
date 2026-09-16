// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::collections::HashMap;
use std::ffi::c_void;

use anyhow::Result;
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::System::Environment::{
    CreateEnvironmentBlock, DestroyEnvironmentBlock, ExpandEnvironmentStringsForUserW,
};
use windows_sys::Win32::UI::Shell::GetUserProfileDirectoryW;

use super::wide;

const FALLBACK_ENV_KEYS: &[&str] = &[
    "SystemRoot",
    "WINDIR",
    "SystemDrive",
    "ProgramData",
    "ProgramFiles",
    "ProgramFiles(x86)",
    "ProgramW6432",
    "CommonProgramFiles",
    "CommonProgramFiles(x86)",
    "CommonProgramW6432",
    "PUBLIC",
    "TEMP",
    "TMP",
    "Path",
    "PATHEXT",
    "LOCALAPPDATA",
    "APPDATA",
    "USERPROFILE",
    "ComSpec",
];

pub(crate) fn baseline_env_vars_for_spawn(
    process_name: &str,
    token: HANDLE,
) -> HashMap<String, String> {
    match baseline_env_vars_from_token(token) {
        Ok(vars) => vars,
        Err(e) => {
            log::warn!(
                "[{process_name}] CreateEnvironmentBlock failed ({e:#}); using allowlisted process-env fallback"
            );
            fallback_env_vars_for_spawn(token)
        }
    }
}

pub(crate) fn baseline_env_vars_from_token(token: HANDLE) -> Result<HashMap<String, String>> {
    if token.is_null() {
        anyhow::bail!("baseline_env_vars_from_token: null token handle");
    }

    let mut env_block: *mut c_void = std::ptr::null_mut();
    let ok = unsafe { CreateEnvironmentBlock(&mut env_block, token, 0) };
    if ok == 0 {
        anyhow::bail!(
            "CreateEnvironmentBlock: {}",
            std::io::Error::last_os_error()
        );
    }

    let vars = wide_env_block_to_map(env_block as *const u16);

    unsafe {
        let _ = DestroyEnvironmentBlock(env_block as *const c_void);
    }
    Ok(vars)
}

/// Merge override entries into `vars`.
///
/// Most keys replace the previous value. `Path` is special on Windows: overrides are appended
/// with `;`, matching how SCM and user PATH entries compose on top of the system PATH.
pub(crate) fn merge_env_overrides(
    vars: &mut HashMap<String, String>,
    overrides: &[(String, String)],
) {
    for (key, value) in overrides {
        if key.eq_ignore_ascii_case("path") {
            merge_path_override(vars, key, value);
            continue;
        }
        vars.retain(|existing, _| !existing.eq_ignore_ascii_case(key));
        vars.insert(key.clone(), value.clone());
    }
}

fn merge_path_override(vars: &mut HashMap<String, String>, key: &str, value: &str) {
    let existing_key = vars
        .keys()
        .find(|existing| existing.eq_ignore_ascii_case(key))
        .cloned();
    if let Some(existing_key) = existing_key {
        let existing_value = vars.get(&existing_key).cloned().unwrap_or_default();
        vars.insert(existing_key, append_path_segment(&existing_value, value));
        return;
    }
    vars.insert(key.to_string(), value.to_string());
}

fn append_path_segment(existing: &str, addition: &str) -> String {
    let existing = existing.trim();
    let addition = addition.trim();
    if existing.is_empty() {
        return addition.to_string();
    }
    if addition.is_empty() {
        return existing.to_string();
    }
    format!("{existing};{addition}")
}

const LEGACY_SCM_ENV_DENYLIST: &[&str] = &[
    "DD_FLEET_POLICIES_DIR",
    "DD_OTELCOLLECTOR_INSTALLATION_METHOD",
];

fn legacy_scm_service_name(process_name: &str) -> Option<&'static str> {
    match process_name {
        "datadog-agent-process" => Some("datadog-process-agent"),
        "datadog-agent-action" => Some("datadog-agent-action"),
        "datadog-agent-ddot" => Some("datadog-otel-agent"),
        _ => None,
    }
}

pub(crate) fn merge_legacy_scm_env(process_name: &str, vars: &mut HashMap<String, String>) {
    let Some(service_name) = legacy_scm_service_name(process_name) else {
        return;
    };
    let overrides = legacy_scm_env_overrides_for_service(process_name, service_name);
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

fn legacy_scm_env_overrides_for_service(
    process_name: &str,
    service_name: &str,
) -> Vec<(String, String)> {
    match read_service_environment(service_name) {
        Ok(entries) => filter_legacy_scm_env(&parse_scm_environment_entries(&entries)),
        Err(e) => {
            log::warn!(
                "[{process_name}] failed to read legacy SCM Environment for {service_name}: {e:#}"
            );
            Vec::new()
        }
    }
}

fn read_service_environment(service_name: &str) -> Result<Vec<String>> {
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

fn wide_env_block_to_map(block: *const u16) -> HashMap<String, String> {
    if block.is_null() {
        return HashMap::new();
    }
    let mut vars = HashMap::new();
    let mut p = block;
    loop {
        unsafe {
            if *p == 0 {
                break;
            }
            let entry_start = p;
            while *p != 0 {
                p = p.add(1);
            }
            let len = (p as usize - entry_start as usize) / std::mem::size_of::<u16>();
            let slice = std::slice::from_raw_parts(entry_start, len);
            p = p.add(1);
            if let Some((k, v)) = wide::split_env_entry_wide(slice) {
                vars.insert(
                    k.to_string_lossy().into_owned(),
                    v.to_string_lossy().into_owned(),
                );
            }
        }
    }
    vars
}

fn supervisor_machine_env_vars() -> HashMap<String, String> {
    let mut vars = HashMap::new();
    for &key in FALLBACK_ENV_KEYS {
        if let Ok(val) = std::env::var(key)
            && !val.is_empty()
        {
            vars.insert(key.to_string(), val);
        }
    }
    vars
}

fn fallback_env_vars_for_spawn(token: HANDLE) -> HashMap<String, String> {
    let mut vars = supervisor_machine_env_vars();
    vars.extend(token_profile_env_vars(token));
    vars
}

fn token_profile_env_vars(token: HANDLE) -> HashMap<String, String> {
    let mut vars = HashMap::new();
    if let Ok(Some(userprofile)) = user_profile_directory_for_token(token) {
        vars.insert("USERPROFILE".to_string(), userprofile);
    }
    for (key, pattern) in [("LOCALAPPDATA", "%LOCALAPPDATA%"), ("APPDATA", "%APPDATA%")] {
        if let Ok(Some(value)) = expand_environment_string_for_user(token, pattern)
            && !value.is_empty()
        {
            vars.insert(key.to_string(), value);
        }
    }
    vars
}

fn user_profile_directory_for_token(token: HANDLE) -> Result<Option<String>> {
    if token.is_null() {
        return Ok(None);
    }

    use windows_sys::Win32::Foundation::ERROR_INSUFFICIENT_BUFFER;

    unsafe {
        let mut size = 260u32;
        loop {
            let mut buf = vec![0u16; size as usize];
            let mut size_inout = size;
            if GetUserProfileDirectoryW(token, buf.as_mut_ptr(), &mut size_inout) != 0 {
                return Ok(Some(wide::from_ptr(buf.as_ptr())));
            }
            let err = std::io::Error::last_os_error();
            if err.raw_os_error() == Some(ERROR_INSUFFICIENT_BUFFER as i32) {
                size = size_inout;
                continue;
            }
            log::debug!("GetUserProfileDirectoryW failed: {err}");
            return Ok(None);
        }
    }
}

fn expand_environment_string_for_user(token: HANDLE, src: &str) -> Result<Option<String>> {
    if token.is_null() {
        return Ok(None);
    }

    use windows_sys::Win32::Foundation::ERROR_INSUFFICIENT_BUFFER;

    let src_w = wide::null_terminated(src);
    unsafe {
        let mut size = 256u32;
        loop {
            let mut buf = vec![0u16; size as usize];
            if ExpandEnvironmentStringsForUserW(token, src_w.as_ptr(), buf.as_mut_ptr(), size) != 0
            {
                let value = wide::from_ptr(buf.as_ptr());
                return Ok(if value.is_empty() { None } else { Some(value) });
            }
            let err = std::io::Error::last_os_error();
            if err.raw_os_error() == Some(ERROR_INSUFFICIENT_BUFFER as i32) {
                size = size.saturating_mul(2).max(size + 64);
                continue;
            }
            log::debug!("ExpandEnvironmentStringsForUserW({src}) failed: {err}");
            return Ok(None);
        }
    }
}

#[cfg(test)]
mod legacy_scm_tests {
    use super::*;

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

    #[test]
    fn merge_legacy_scm_env_leaves_processes_d_overrides_winning() {
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
}

#[cfg(test)]
mod merge_env_override_tests {
    use super::*;

    #[test]
    fn append_path_segment_joins_with_semicolon() {
        assert_eq!(
            append_path_segment(r"C:\Windows\System32", r"C:\Datadog\bin"),
            r"C:\Windows\System32;C:\Datadog\bin"
        );
    }

    #[test]
    fn append_path_segment_handles_empty_existing() {
        assert_eq!(
            append_path_segment("", r"C:\Datadog\bin"),
            r"C:\Datadog\bin"
        );
    }

    #[test]
    fn append_path_segment_handles_empty_addition() {
        assert_eq!(
            append_path_segment(r"C:\Windows\System32", ""),
            r"C:\Windows\System32"
        );
    }

    #[test]
    fn merge_env_overrides_appends_path_preserving_baseline_key_casing() {
        let mut vars = HashMap::from([("Path".to_string(), r"C:\Windows\System32".to_string())]);
        merge_env_overrides(
            &mut vars,
            &[("PATH".to_string(), r"C:\Datadog\bin".to_string())],
        );
        assert_eq!(
            vars.get("Path").unwrap(),
            r"C:\Windows\System32;C:\Datadog\bin"
        );
        assert_eq!(vars.len(), 1);
    }

    #[test]
    fn merge_env_overrides_sets_path_when_missing() {
        let mut vars = HashMap::from([("DD_LOG_LEVEL".to_string(), "debug".to_string())]);
        merge_env_overrides(
            &mut vars,
            &[("Path".to_string(), r"C:\Datadog\bin".to_string())],
        );
        assert_eq!(vars.get("Path").unwrap(), r"C:\Datadog\bin");
        assert_eq!(vars.get("DD_LOG_LEVEL").unwrap(), "debug");
    }

    #[test]
    fn merge_env_overrides_still_replaces_non_path_keys() {
        let mut vars = HashMap::from([("DD_LOG_LEVEL".to_string(), "info".to_string())]);
        merge_env_overrides(
            &mut vars,
            &[("DD_LOG_LEVEL".to_string(), "debug".to_string())],
        );
        assert_eq!(vars.get("DD_LOG_LEVEL").unwrap(), "debug");
    }

    #[test]
    fn merge_env_overrides_layers_path_overrides_in_order() {
        let mut vars = HashMap::from([("Path".to_string(), r"C:\Windows\System32".to_string())]);
        merge_env_overrides(
            &mut vars,
            &[("Path".to_string(), r"C:\Datadog\bin".to_string())],
        );
        merge_env_overrides(
            &mut vars,
            &[("Path".to_string(), r"C:\Datadog\embedded".to_string())],
        );
        assert_eq!(
            vars.get("Path").unwrap(),
            r"C:\Windows\System32;C:\Datadog\bin;C:\Datadog\embedded"
        );
    }
}
