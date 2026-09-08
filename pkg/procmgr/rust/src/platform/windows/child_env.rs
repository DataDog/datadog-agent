// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::collections::HashMap;
use std::ffi::c_void;

use anyhow::Result;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::System::Environment::{
    CreateEnvironmentBlock, DestroyEnvironmentBlock, ExpandEnvironmentStringsForUserW,
};
use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};
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

pub(crate) fn apply_child_baseline_env(cmd: &mut tokio::process::Command) {
    if let Err(e) = try_apply_create_environment_block(cmd) {
        log::warn!("CreateEnvironmentBlock baseline failed ({e:#}); using process-env fallback");
        apply_fallback_process_env(cmd);
    }
}

fn try_apply_create_environment_block(cmd: &mut tokio::process::Command) -> Result<()> {
    use windows_sys::Win32::Security::{TOKEN_DUPLICATE, TOKEN_QUERY};

    let mut token: HANDLE = std::ptr::null_mut();
    let ok = unsafe {
        OpenProcessToken(
            GetCurrentProcess(),
            TOKEN_QUERY | TOKEN_DUPLICATE,
            &mut token,
        )
    };
    if ok == 0 {
        anyhow::bail!("OpenProcessToken: {}", std::io::Error::last_os_error());
    }

    let mut env_block: *mut c_void = std::ptr::null_mut();
    let ok = unsafe { CreateEnvironmentBlock(&mut env_block, token, 0) };
    if ok == 0 {
        unsafe {
            CloseHandle(token);
        }
        anyhow::bail!(
            "CreateEnvironmentBlock: {}",
            std::io::Error::last_os_error()
        );
    }

    merge_wide_env_block_into_cmd(cmd, env_block as *const u16);

    unsafe {
        let _ = DestroyEnvironmentBlock(env_block as *const c_void);
        CloseHandle(token);
    }
    Ok(())
}

fn merge_wide_env_block_into_cmd(cmd: &mut tokio::process::Command, block: *const u16) {
    if block.is_null() {
        return;
    }
    let mut p = block;
    loop {
        // SAFETY: `block` is a valid env block from CreateEnvironmentBlock (caller).
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
                cmd.env(k, v);
            }
        }
    }
}

fn apply_fallback_process_env(cmd: &mut tokio::process::Command) {
    for &key in FALLBACK_ENV_KEYS {
        if let Ok(val) = std::env::var(key)
            && !val.is_empty()
        {
            cmd.env(key, val);
        }
    }
}

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

pub(crate) fn merge_env_overrides(
    vars: &mut HashMap<String, String>,
    overrides: &[(String, String)],
) {
    for (key, value) in overrides {
        vars.retain(|existing, _| !existing.eq_ignore_ascii_case(key));
        vars.insert(key.clone(), value.clone());
    }
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
