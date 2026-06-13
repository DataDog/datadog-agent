use std::path::Path;

use anyhow::{Context, Result};
use windows_sys::Win32::Foundation::{CloseHandle, INVALID_HANDLE_VALUE};
use windows_sys::Win32::System::Diagnostics::ToolHelp::{
    CreateToolhelp32Snapshot, PROCESSENTRY32W, Process32FirstW, Process32NextW, TH32CS_SNAPPROCESS,
};
use windows_sys::Win32::System::Threading::{
    OpenProcess, PROCESS_QUERY_LIMITED_INFORMATION, QueryFullProcessImageNameW,
};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    GetForegroundWindow, GetWindowTextLengthW, GetWindowTextW, GetWindowThreadProcessId,
};

use crate::desktop::DesktopDetector;
use crate::desktop::matcher::{ProcessInfo, ProcessSnapshot};

pub struct WindowsDesktopDetector;

impl DesktopDetector for WindowsDesktopDetector {
    /// Return the process that owns the current foreground window.
    fn foreground_process(&self) -> Result<Option<ProcessInfo>> {
        let hwnd = unsafe { GetForegroundWindow() };
        if hwnd.is_null() {
            return Ok(None);
        }

        let mut pid = 0u32;
        let thread_id = unsafe { GetWindowThreadProcessId(hwnd, &mut pid) };
        if thread_id == 0 || pid == 0 {
            return Ok(None);
        }

        let exe_path = query_process_image_path(pid).ok();
        let exe_name = exe_path
            .as_deref()
            .and_then(exe_name_from_path)
            .or_else(|| {
                self.process_snapshot(pid).ok().and_then(|snapshot| {
                    snapshot
                        .processes
                        .into_iter()
                        .find(|process| process.pid == pid)
                        .map(|process| process.exe_name)
                })
            });

        Ok(exe_name.map(|exe_name| ProcessInfo {
            pid,
            parent_pid: 0,
            exe_name,
            bsd_name: None,
            bsd_comm: None,
            argv0: None,
            argv: Vec::new(),
            exe_path,
            window_title: window_title(hwnd),
            process_group_id: None,
            terminal_foreground_process_group_id: None,
            has_controlling_terminal: false,
        }))
    }

    /// Return Toolhelp process metadata and parent PID edges for Windows processes.
    fn process_snapshot(&self, _foreground_pid: u32) -> Result<ProcessSnapshot> {
        let snapshot = unsafe { CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0) };
        if snapshot == INVALID_HANDLE_VALUE {
            return Err(std::io::Error::last_os_error()).context("CreateToolhelp32Snapshot failed");
        }
        let _guard = HandleGuard(snapshot);

        let mut entry = PROCESSENTRY32W {
            dwSize: std::mem::size_of::<PROCESSENTRY32W>() as u32,
            cntUsage: 0,
            th32ProcessID: 0,
            th32DefaultHeapID: 0,
            th32ModuleID: 0,
            cntThreads: 0,
            th32ParentProcessID: 0,
            pcPriClassBase: 0,
            dwFlags: 0,
            szExeFile: [0; 260],
        };

        let mut processes = Vec::new();
        let mut ok = unsafe { Process32FirstW(snapshot, &mut entry) };
        while ok != 0 {
            let exe_name = utf16_to_string(&entry.szExeFile);
            if !exe_name.is_empty() {
                processes.push(ProcessInfo {
                    pid: entry.th32ProcessID,
                    parent_pid: entry.th32ParentProcessID,
                    exe_name,
                    bsd_name: None,
                    bsd_comm: None,
                    argv0: None,
                    argv: Vec::new(),
                    exe_path: None,
                    window_title: None,
                    process_group_id: None,
                    terminal_foreground_process_group_id: None,
                    has_controlling_terminal: false,
                });
            }
            ok = unsafe { Process32NextW(snapshot, &mut entry) };
        }

        Ok(ProcessSnapshot::from_processes(processes))
    }
}

struct HandleGuard(windows_sys::Win32::Foundation::HANDLE);

impl Drop for HandleGuard {
    fn drop(&mut self) {
        unsafe {
            CloseHandle(self.0);
        }
    }
}

/// Query the full executable image path for a process.
fn query_process_image_path(pid: u32) -> Result<String> {
    let handle = unsafe { OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid) };
    if handle.is_null() {
        return Err(std::io::Error::last_os_error()).context("OpenProcess failed");
    }
    let _guard = HandleGuard(handle);

    let mut buffer = vec![0u16; 32768];
    let mut size = buffer.len() as u32;
    let ok = unsafe { QueryFullProcessImageNameW(handle, 0, buffer.as_mut_ptr(), &mut size) };
    if ok == 0 {
        return Err(std::io::Error::last_os_error()).context("QueryFullProcessImageNameW failed");
    }

    Ok(String::from_utf16_lossy(&buffer[..size as usize]))
}

/// Return the executable basename from a full image path.
fn exe_name_from_path(path: &str) -> Option<String> {
    Path::new(path)
        .file_name()
        .and_then(|name| name.to_str())
        .filter(|name| !name.is_empty())
        .map(str::to_string)
}

/// Read the foreground window title text.
fn window_title(hwnd: windows_sys::Win32::Foundation::HWND) -> Option<String> {
    let len = unsafe { GetWindowTextLengthW(hwnd) };
    if len <= 0 {
        return None;
    }

    let mut buffer = vec![0u16; len as usize + 1];
    let copied = unsafe { GetWindowTextW(hwnd, buffer.as_mut_ptr(), buffer.len() as i32) };
    if copied <= 0 {
        return None;
    }

    Some(String::from_utf16_lossy(&buffer[..copied as usize]))
}

/// Decode a nul-terminated UTF-16 buffer into a Rust string.
fn utf16_to_string(buf: &[u16]) -> String {
    let len = buf.iter().position(|ch| *ch == 0).unwrap_or(buf.len());
    String::from_utf16_lossy(&buf[..len])
}
