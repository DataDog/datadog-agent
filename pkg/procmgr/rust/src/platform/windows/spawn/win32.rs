// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::collections::HashMap;
use std::os::windows::ffi::OsStrExt;
use std::ptr;

use anyhow::{bail, Result};
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::Security::{DuplicateTokenEx, SecurityDelegation, TokenPrimary};
use windows_sys::Win32::System::SystemServices::MAXIMUM_ALLOWED;
use windows_sys::Win32::System::Threading::ResumeThread;
use windows_sys::Win32::System::Threading::{
    CREATE_BREAKAWAY_FROM_JOB, CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW,
    CREATE_SUSPENDED, CREATE_UNICODE_ENVIRONMENT, EXTENDED_STARTUPINFO_PRESENT,
};

use super::super::child_env::merge_legacy_scm_env;
use super::super::merge_env_overrides;

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

pub(crate) fn duplicate_primary_token(context: &str, token: HANDLE) -> Result<HANDLE> {
    let mut primary_token: HANDLE = ptr::null_mut();
    let ok = unsafe {
        DuplicateTokenEx(
            token,
            MAXIMUM_ALLOWED,
            ptr::null(),
            SecurityDelegation,
            TokenPrimary,
            &mut primary_token,
        )
    };
    if ok == 0 {
        bail!(
            "[{context}] DuplicateTokenEx failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(primary_token)
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

/// `CreateProcess*` flags shared by managed child spawn paths.
///
/// `CREATE_NO_WINDOW` spawns hidden console children. `CREATE_NEW_CONSOLE` is omitted:
/// with both set, Windows ignores `CREATE_NO_WINDOW` and the child gets a visible console.
pub(crate) fn managed_process_creation_flags() -> u32 {
    CREATE_NEW_PROCESS_GROUP
        | CREATE_NO_WINDOW
        | CREATE_UNICODE_ENVIRONMENT
        | EXTENDED_STARTUPINFO_PRESENT
}

/// Flags for create-time `PROC_THREAD_ATTRIBUTE_JOB_LIST` assignment.
///
/// When the supervisor already belongs to a job (GitLab CI sets
/// `FF_USE_WINDOWS_JOB_OBJECT`), the child must break away before joining our job.
pub(crate) fn managed_process_creation_flags_for_job_list() -> u32 {
    managed_process_creation_flags() | CREATE_BREAKAWAY_FROM_JOB
}

/// Flags for post-create job assignment when the parent is already in a foreign job.
///
/// Matches the pre-`JOB_LIST` spawn path: create suspended, assign to our job, then resume.
pub(crate) fn managed_process_creation_flags_for_post_assign() -> u32 {
    CREATE_SUSPENDED
        | CREATE_NEW_PROCESS_GROUP
        | CREATE_NEW_CONSOLE
        | CREATE_NO_WINDOW
        | CREATE_UNICODE_ENVIRONMENT
        | EXTENDED_STARTUPINFO_PRESENT
}

/// Resumes the primary thread of a child created with `CREATE_SUSPENDED`.
pub(crate) fn resume_child_primary_thread(
    process_name: &str,
    pid: u32,
    thread: HANDLE,
) -> Result<()> {
    let previous_count = unsafe { ResumeThread(thread) };
    if previous_count == u32::MAX {
        bail!(
            "[{process_name}] ResumeThread({pid}) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(())
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
}
