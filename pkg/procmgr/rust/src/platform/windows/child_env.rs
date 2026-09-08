// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ffi::c_void;

use anyhow::Result;
use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
use windows_sys::Win32::Security::{TOKEN_DUPLICATE, TOKEN_QUERY};
use windows_sys::Win32::System::Environment::{
    CreateEnvironmentBlock, DestroyEnvironmentBlock,
};
use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};

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

pub fn apply_child_baseline_env(cmd: &mut tokio::process::Command) {
    if let Err(e) = try_apply_create_environment_block(cmd) {
        log::warn!("CreateEnvironmentBlock baseline failed ({e:#}); using process-env fallback");
        apply_fallback_process_env(cmd);
    }
}

fn try_apply_create_environment_block(cmd: &mut tokio::process::Command) -> Result<()> {
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
        // SAFETY: `block` is valid until `DestroyEnvironmentBlock` (caller guarantees).
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
