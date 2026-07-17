// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! One-shot privileged child execution as LocalSystem with captured stdout/stderr.

use anyhow::{Context, Result, bail};
use std::collections::HashMap;
use std::ffi::c_void;
use std::os::windows::ffi::OsStrExt;
use std::ptr;
use windows_sys::Win32::Foundation::{
    CloseHandle, ERROR_BROKEN_PIPE, HANDLE, HANDLE_FLAG_INHERIT, INVALID_HANDLE_VALUE,
    SetHandleInformation,
};
use windows_sys::Win32::Security::{
    DuplicateTokenEx, SECURITY_ATTRIBUTES, SecurityDelegation, TOKEN_DUPLICATE, TOKEN_QUERY,
    TokenPrimary,
};
use windows_sys::Win32::Storage::FileSystem::{
    CreateFileW, FILE_ATTRIBUTE_NORMAL, FILE_GENERIC_READ, FILE_GENERIC_WRITE, FILE_SHARE_READ,
    FILE_SHARE_WRITE, OPEN_EXISTING, ReadFile,
};
use windows_sys::Win32::System::Pipes::CreatePipe;
use windows_sys::Win32::System::SystemServices::MAXIMUM_ALLOWED;
use windows_sys::Win32::System::Threading::{
    CREATE_NEW_CONSOLE, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW, CREATE_UNICODE_ENVIRONMENT,
    CreateProcessAsUserW, GetCurrentProcess, GetExitCodeProcess, INFINITE, OpenProcessToken,
    PROCESS_INFORMATION, STARTF_USESTDHANDLES, STARTUPINFOW, WAIT_FAILED, WaitForSingleObject,
};

use super::wide;

const MAX_CAPTURE_BYTES: usize = 4 * 1024 * 1024;

use crate::privileged::PrivilegedCommandOutput;
pub fn run_privileged_command(
    command: &str,
    args: &[String],
    env: &HashMap<String, String>,
) -> Result<PrivilegedCommandOutput> {
    let _console_guard = super::console_lock();

    let mut stdout_pipe = AnonymousPipe::new()?;
    let mut stderr_pipe = AnonymousPipe::new()?;
    let stdin_handle = open_nul_handle(FILE_GENERIC_READ | FILE_GENERIC_WRITE)?;

    let command_line = build_command_line(command, args);
    let application_name_w = wide::null_terminated(command);
    let mut command_line_w: Vec<u16> = std::ffi::OsStr::new(&command_line)
        .encode_wide()
        .chain([0])
        .collect();

    let env_block = env_block_from_current_plus_overrides(env)?;
    let env_block_ptr = env_block.as_ptr() as *const c_void;

    let primary_token = local_system_primary_token()?;
    let _primary_token_guard = TokenHandle(primary_token);

    let mut si: STARTUPINFOW = unsafe { std::mem::zeroed() };
    si.cb = std::mem::size_of::<STARTUPINFOW>() as u32;
    si.dwFlags = STARTF_USESTDHANDLES;
    si.hStdInput = stdin_handle;
    si.hStdOutput = stdout_pipe.write;
    si.hStdError = stderr_pipe.write;

    let mut pi: PROCESS_INFORMATION = unsafe { std::mem::zeroed() };
    let creation_flags = CREATE_NEW_PROCESS_GROUP
        | CREATE_NEW_CONSOLE
        | CREATE_NO_WINDOW
        | CREATE_UNICODE_ENVIRONMENT;

    let ok = unsafe {
        CreateProcessAsUserW(
            primary_token,
            application_name_w.as_ptr(),
            command_line_w.as_mut_ptr(),
            ptr::null(),
            ptr::null(),
            1,
            creation_flags,
            env_block_ptr,
            ptr::null(),
            &si,
            &mut pi,
        )
    };

    unsafe {
        let _ = CloseHandle(stdin_handle);
        stdout_pipe.close_write();
        stderr_pipe.close_write();
    }

    if ok == 0 {
        bail!(
            "CreateProcessAsUserW failed: {}",
            std::io::Error::last_os_error()
        );
    }

    let process_handle = pi.hProcess;
    unsafe {
        let _ = CloseHandle(pi.hThread);
    }

    let stdout_read = stdout_pipe.take_read();
    let stderr_read = stderr_pipe.take_read();
    let stdout_reader =
        std::thread::spawn(move || read_handle_to_string(stdout_read, MAX_CAPTURE_BYTES));
    let stderr_reader =
        std::thread::spawn(move || read_handle_to_string(stderr_read, MAX_CAPTURE_BYTES));

    let wait = unsafe { WaitForSingleObject(process_handle, INFINITE) };
    if wait == WAIT_FAILED {
        bail!(
            "WaitForSingleObject failed: {}",
            std::io::Error::last_os_error()
        );
    }

    let mut code: u32 = 0;
    let ok = unsafe { GetExitCodeProcess(process_handle, &mut code) };
    unsafe {
        CloseHandle(process_handle);
    }
    if ok == 0 {
        bail!(
            "GetExitCodeProcess failed: {}",
            std::io::Error::last_os_error()
        );
    }

    let stdout = stdout_reader
        .join()
        .map_err(|_| anyhow::anyhow!("stdout reader thread panicked"))??;
    let stderr = stderr_reader
        .join()
        .map_err(|_| anyhow::anyhow!("stderr reader thread panicked"))??;

    Ok(PrivilegedCommandOutput {
        exit_code: windows_exit_code_to_i32(code),
        stdout,
        stderr,
    })
}

fn build_command_line(command: &str, args: &[String]) -> String {
    let mut cmdline = windows_crt_escape_arg(command);
    for arg in args {
        cmdline.push(' ');
        cmdline.push_str(&windows_crt_escape_arg(arg));
    }
    cmdline
}

fn windows_crt_escape_arg(s: &str) -> String {
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

fn env_block_from_current_plus_overrides(overrides: &HashMap<String, String>) -> Result<Vec<u16>> {
    let mut vars: HashMap<String, String> = std::env::vars().collect();
    for (k, v) in overrides {
        vars.insert(k.clone(), v.clone());
    }

    let mut block = Vec::new();
    for (k, v) in vars {
        let kv = format!("{k}={v}");
        block.extend(std::ffi::OsStr::new(&kv).encode_wide());
        block.push(0);
    }
    block.push(0);
    Ok(block)
}

fn local_system_primary_token() -> Result<HANDLE> {
    let mut process_token: HANDLE = ptr::null_mut();
    let ok = unsafe {
        OpenProcessToken(
            GetCurrentProcess(),
            TOKEN_QUERY | TOKEN_DUPLICATE,
            &mut process_token,
        )
    };
    if ok == 0 {
        bail!(
            "OpenProcessToken failed: {}",
            std::io::Error::last_os_error()
        );
    }
    let process_token_guard = TokenHandle(process_token);

    let mut primary_token: HANDLE = ptr::null_mut();
    let ok = unsafe {
        DuplicateTokenEx(
            process_token_guard.0,
            MAXIMUM_ALLOWED,
            ptr::null(),
            SecurityDelegation,
            TokenPrimary,
            &mut primary_token,
        )
    };
    if ok == 0 {
        bail!(
            "DuplicateTokenEx failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(primary_token)
}

fn open_nul_handle(access: u32) -> Result<HANDLE> {
    let nul = wide::null_terminated("NUL");
    let h = unsafe {
        CreateFileW(
            nul.as_ptr(),
            access,
            FILE_SHARE_READ | FILE_SHARE_WRITE,
            ptr::null(),
            OPEN_EXISTING,
            FILE_ATTRIBUTE_NORMAL,
            ptr::null_mut(),
        )
    };
    if h == INVALID_HANDLE_VALUE || h.is_null() {
        bail!(
            "CreateFileW(NUL) failed: {}",
            std::io::Error::last_os_error()
        );
    }
    Ok(h)
}

struct AnonymousPipe {
    read: HANDLE,
    write: HANDLE,
}

impl AnonymousPipe {
    fn new() -> Result<Self> {
        let mut sa: SECURITY_ATTRIBUTES = unsafe { std::mem::zeroed() };
        sa.nLength = std::mem::size_of::<SECURITY_ATTRIBUTES>() as u32;
        sa.bInheritHandle = 1;

        let mut read = ptr::null_mut();
        let mut write = ptr::null_mut();
        let ok = unsafe { CreatePipe(&mut read, &mut write, &sa, 0) };
        if ok == 0 {
            bail!("CreatePipe failed: {}", std::io::Error::last_os_error());
        }

        let read_guard = HandleGuard(read);
        let write_guard = HandleGuard(write);

        // Child inherits write ends only.
        read_guard.clear_inherit()?;
        write_guard.set_inherit()?;

        let read = read_guard.0;
        let write = write_guard.0;
        std::mem::forget(read_guard);
        std::mem::forget(write_guard);

        Ok(Self { read, write })
    }

    fn close_write(&mut self) {
        if !self.write.is_null() {
            unsafe {
                CloseHandle(self.write);
            }
            self.write = ptr::null_mut();
        }
    }

    fn take_read(&mut self) -> HANDLE {
        let read = self.read;
        self.read = ptr::null_mut();
        read
    }

    fn read_to_string(&self, max_bytes: usize) -> Result<String> {
        read_handle_to_string(self.read, max_bytes)
    }
}

fn windows_exit_code_to_i32(code: u32) -> i32 {
    // Windows exit codes are DWORDs; preserve the full bit pattern in proto int32.
    code as i32
}

fn read_handle_to_string(handle: HANDLE, max_bytes: usize) -> Result<String> {
    if handle.is_null() {
        return Ok(String::new());
    }
    let _guard = HandleGuard(handle);
    let mut buf = Vec::new();
    let mut chunk = [0u8; 4096];
    loop {
        if buf.len() >= max_bytes {
            bail!("privileged command output exceeded {max_bytes} byte cap");
        }
        let mut nbytes = 0u32;
        let to_read = std::cmp::min(chunk.len(), max_bytes - buf.len()) as u32;
        let ok = unsafe {
            ReadFile(
                handle,
                chunk.as_mut_ptr(),
                to_read,
                &mut nbytes,
                ptr::null_mut(),
            )
        };
        if ok == 0 {
            let err = std::io::Error::last_os_error();
            if err.raw_os_error() == Some(ERROR_BROKEN_PIPE as i32) {
                break;
            }
            bail!("ReadFile failed: {err}");
        }
        if nbytes == 0 {
            break;
        }
        buf.extend_from_slice(&chunk[..nbytes as usize]);
    }
    String::from_utf8(buf).context("privileged command output was not valid UTF-8")
}

impl Drop for AnonymousPipe {
    fn drop(&mut self) {
        for h in [self.read, self.write] {
            if !h.is_null() {
                unsafe {
                    CloseHandle(h);
                }
            }
        }
    }
}

struct HandleGuard(HANDLE);

impl HandleGuard {
    fn set_inherit(&self) -> Result<()> {
        let ok = unsafe { SetHandleInformation(self.0, HANDLE_FLAG_INHERIT, HANDLE_FLAG_INHERIT) };
        if ok == 0 {
            bail!(
                "SetHandleInformation(set inherit) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        Ok(())
    }

    fn clear_inherit(&self) -> Result<()> {
        let ok = unsafe { SetHandleInformation(self.0, HANDLE_FLAG_INHERIT, 0) };
        if ok == 0 {
            bail!(
                "SetHandleInformation(clear inherit) failed: {}",
                std::io::Error::last_os_error()
            );
        }
        Ok(())
    }
}

struct TokenHandle(HANDLE);

impl Drop for TokenHandle {
    fn drop(&mut self) {
        if !self.0.is_null() {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn windows_exit_code_preserves_high_bit() {
        assert_eq!(windows_exit_code_to_i32(0x8007_0005), -2_147_024_891);
    }

    #[test]
    fn privileged_echo_runs_as_system() {
        unsafe {
            std::env::set_var("DD_PM_PRIVILEGED_COMMANDS_ENABLED", "1");
        }
        let out = run_privileged_command(
            r"C:\Windows\System32\cmd.exe",
            &["/C".into(), "echo".into(), "procmgr-privileged-ok".into()],
            &HashMap::new(),
        )
        .expect("catalog test command should run");
        assert_eq!(out.exit_code, 0);
        assert!(
            out.stdout.contains("procmgr-privileged-ok"),
            "stdout={:?}",
            out.stdout
        );
    }
}
