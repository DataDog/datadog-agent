// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! E2e oracles for spawn identity. Query the OS directly; do not call
//! `intended_spawn_user` or `runtime_user_for_pid`.

pub fn expected_agent_spawn_user() -> String {
    agent_spawn_user_oracle().expect("agent spawn user oracle")
}

pub fn expected_runtime_user_for_pid(pid: u32) -> String {
    runtime_user_oracle(pid).unwrap_or_default()
}

#[cfg(unix)]
fn agent_spawn_user_oracle() -> Option<String> {
    nix::unistd::User::from_uid(nix::unistd::Uid::effective())
        .ok()
        .flatten()
        .map(|user| user.name)
}

#[cfg(windows)]
fn agent_spawn_user_oracle() -> Option<String> {
    use windows_registry::LOCAL_MACHINE;
    use windows_sys::Win32::System::Registry::KEY_WOW64_64KEY;

    let key = LOCAL_MACHINE
        .options()
        .read()
        .access(KEY_WOW64_64KEY)
        .open(r"SOFTWARE\Datadog\Datadog Agent")
        .ok()?;
    let user = key.get_string("installedUser").ok()?;
    if user.is_empty() {
        return None;
    }
    let domain = key.get_string("installedDomain").unwrap_or_default();
    Some(format_install_account(&domain, &user))
}

#[cfg(windows)]
fn format_install_account(domain: &str, user: &str) -> String {
    let domain = domain.trim();
    if domain.is_empty() || domain == "." {
        format!(r".\{user}")
    } else {
        format!(r"{domain}\{user}")
    }
}

#[cfg(target_os = "linux")]
fn runtime_user_oracle(pid: u32) -> Option<String> {
    use nix::unistd::{Uid, User};
    use std::fs::File;
    use std::io::BufReader;

    let status_path = format!("/proc/{pid}/status");
    let file = File::open(status_path).ok()?;
    let uid = parse_effective_uid(BufReader::new(file))?;
    User::from_uid(Uid::from_raw(uid))
        .ok()
        .flatten()
        .map(|user| user.name)
}

#[cfg(target_os = "linux")]
fn parse_effective_uid<R: std::io::BufRead>(reader: R) -> Option<u32> {
    for line in reader.lines().filter_map(Result::ok) {
        if let Some(rest) = line.strip_prefix("Uid:") {
            let effective = rest.split_whitespace().nth(1)?;
            return effective.parse().ok();
        }
    }
    None
}

#[cfg(target_os = "macos")]
fn runtime_user_oracle(pid: u32) -> Option<String> {
    use libc::{PROC_PIDTBSDINFO, c_int, proc_bsdinfo, proc_pidinfo};
    use nix::unistd::{Uid, User};

    let mut info = unsafe { std::mem::zeroed::<proc_bsdinfo>() };
    let size = std::mem::size_of::<proc_bsdinfo>() as c_int;
    let result = unsafe {
        proc_pidinfo(
            pid as c_int,
            PROC_PIDTBSDINFO,
            0,
            (&raw mut info).cast(),
            size,
        )
    };
    if result != size {
        return None;
    }

    User::from_uid(Uid::from_raw(info.pbi_uid))
        .ok()
        .flatten()
        .map(|user| user.name)
}

#[cfg(not(any(target_os = "linux", target_os = "macos", windows)))]
fn runtime_user_oracle(_pid: u32) -> Option<String> {
    None
}

#[cfg(windows)]
fn runtime_user_oracle(pid: u32) -> Option<String> {
    use std::ptr;
    use windows_sys::Win32::Foundation::CloseHandle;
    use windows_sys::Win32::Foundation::HANDLE;
    use windows_sys::Win32::Security::{
        GetLengthSid, GetTokenInformation, LookupAccountSidW, TOKEN_USER, TokenUser,
    };
    use windows_sys::Win32::System::Threading::{
        OpenProcess, OpenProcessToken, PROCESS_QUERY_LIMITED_INFORMATION,
    };

    unsafe {
        let process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid);
        if process.is_null() {
            return None;
        }

        let mut token = ptr::null_mut();
        let token_opened = OpenProcessToken(
            process,
            windows_sys::Win32::Security::TOKEN_QUERY,
            &mut token,
        );
        if token_opened == 0 {
            CloseHandle(process);
            return None;
        }

        let account = lookup_token_account(token);
        CloseHandle(token);
        CloseHandle(process);
        account
    }
}

#[cfg(windows)]
unsafe fn lookup_token_account(token: windows_sys::Win32::Foundation::HANDLE) -> Option<String> {
    use std::ptr;
    use windows_sys::Win32::Security::{
        GetTokenInformation, LookupAccountSidW, TOKEN_USER, TokenUser,
    };

    let mut needed = 0u32;
    let _ = GetTokenInformation(token, TokenUser, ptr::null_mut(), 0, &mut needed);
    if needed == 0 {
        return None;
    }

    let mut buffer = vec![0u8; needed as usize];
    if GetTokenInformation(
        token,
        TokenUser,
        buffer.as_mut_ptr().cast(),
        needed,
        &mut needed,
    ) == 0
    {
        return None;
    }

    let token_user = ptr::read_unaligned(buffer.as_ptr().cast::<TOKEN_USER>());
    let sid_ptr = token_user.User.Sid;
    if sid_ptr.is_null() {
        return None;
    }

    let sid_len = windows_sys::Win32::Security::GetLengthSid(sid_ptr);
    if sid_len == 0 {
        return None;
    }
    let mut sid = vec![0u8; sid_len as usize];
    ptr::copy_nonoverlapping(sid_ptr.cast(), sid.as_mut_ptr(), sid_len as usize);
    lookup_account_display(&sid)
}

#[cfg(windows)]
unsafe fn lookup_account_display(sid: &[u8]) -> Option<String> {
    use std::ptr;
    use windows_sys::Win32::Security::LookupAccountSidW;

    let sid_ptr = sid.as_ptr().cast();
    let mut name_size = 0u32;
    let mut domain_size = 0u32;
    let mut sid_type = 0i32;
    let _ = LookupAccountSidW(
        ptr::null(),
        sid_ptr,
        ptr::null_mut(),
        &mut name_size,
        ptr::null_mut(),
        &mut domain_size,
        &mut sid_type,
    );

    let mut name = vec![0u16; name_size as usize];
    let mut domain = vec![0u16; domain_size as usize];
    if LookupAccountSidW(
        ptr::null(),
        sid_ptr,
        name.as_mut_ptr(),
        &mut name_size,
        domain.as_mut_ptr(),
        &mut domain_size,
        &mut sid_type,
    ) == 0
    {
        return None;
    }

    let user = trim_wide_nul(&name);
    let domain = trim_wide_nul(&domain);
    if domain.is_empty() {
        Some(user)
    } else {
        Some(format!("{domain}\\{user}"))
    }
}

#[cfg(windows)]
fn trim_wide_nul(wide: &[u16]) -> String {
    let end = wide.iter().position(|&c| c == 0).unwrap_or(wide.len());
    std::ffi::OsString::from_wide(&wide[..end])
        .to_string_lossy()
        .into_owned()
}
