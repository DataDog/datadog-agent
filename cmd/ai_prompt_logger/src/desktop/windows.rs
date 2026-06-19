use std::mem;
use std::path::Path;
use std::ptr;

use anyhow::{Context, Result};
use windows_sys::Win32::Foundation::{CloseHandle, FILETIME, INVALID_HANDLE_VALUE};
use windows_sys::Win32::System::Console::{AttachConsole, FreeConsole, GetConsoleTitleW};
use windows_sys::Win32::System::Diagnostics::ToolHelp::{
    CreateToolhelp32Snapshot, PROCESSENTRY32W, Process32FirstW, Process32NextW, TH32CS_SNAPPROCESS,
};
use windows_sys::Win32::System::Services::{
    CloseServiceHandle, OpenSCManagerW, OpenServiceW, QueryServiceStatusEx, SC_MANAGER_CONNECT,
    SC_STATUS_PROCESS_INFO, SERVICE_QUERY_STATUS, SERVICE_RUNNING, SERVICE_STATUS_PROCESS,
};
use windows_sys::Win32::System::Threading::{
    GetProcessIoCounters, GetProcessTimes, IO_COUNTERS, OpenProcess,
    PROCESS_QUERY_LIMITED_INFORMATION, QueryFullProcessImageNameW,
};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    GetForegroundWindow, GetWindowTextLengthW, GetWindowTextW, GetWindowThreadProcessId,
};

use crate::datadog::DesktopMonitoringConfig;
use crate::desktop::DesktopDetector;
use crate::desktop::config::builtin_console_title_host_process_names;
use crate::desktop::matcher::{
    ProcessActivity, ProcessInfo, ProcessSnapshot, find_hosted_ai_process,
};

const AGENT_SERVICE_NAME: &str = "DatadogAgent";

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
        let process_activity = query_process_activity(pid).ok();
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
            attached_console_title: None,
            process_group_id: None,
            terminal_foreground_process_group_id: None,
            has_controlling_terminal: false,
            terminal_name: None,
            terminal_access_time_seconds: None,
            terminal_activity_age_seconds: None,
            process_activity,
            process_activity_delta: None,
            process_read_write_activity_observed: false,
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
                let process_activity = query_process_activity(entry.th32ProcessID).ok();
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
                    attached_console_title: None,
                    process_group_id: None,
                    terminal_foreground_process_group_id: None,
                    has_controlling_terminal: false,
                    terminal_name: None,
                    terminal_access_time_seconds: None,
                    terminal_activity_age_seconds: None,
                    process_activity,
                    process_activity_delta: None,
                    process_read_write_activity_observed: false,
                });
            }
            ok = unsafe { Process32NextW(snapshot, &mut entry) };
        }

        Ok(ProcessSnapshot::from_processes(processes))
    }

    /// Report whether the Datadog Agent Windows service is running.
    fn agent_service_running(&self) -> bool {
        query_service_running(AGENT_SERVICE_NAME).unwrap_or(true)
    }

    /// Stay resident in the user session and resume scanning when the Agent starts again.
    fn should_idle_when_agent_service_stopped(&self) -> bool {
        true
    }
}

pub(super) fn enrich_attached_console_titles(
    foreground: &ProcessInfo,
    snapshot: &mut ProcessSnapshot,
    config: &DesktopMonitoringConfig,
) {
    if foreground
        .window_title
        .as_deref()
        .is_none_or(|title| title.is_empty())
        || !is_console_title_host(foreground)
    {
        return;
    }

    for process in &mut snapshot.processes {
        if find_hosted_ai_process(process, &config.ai_process_names).is_none() {
            continue;
        }
        if let Ok(title) = attached_console_title(process.pid)
            && !title.is_empty()
        {
            process.attached_console_title = Some(title);
        }
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

struct AttachedConsoleGuard;

impl Drop for AttachedConsoleGuard {
    fn drop(&mut self) {
        unsafe {
            FreeConsole();
        }
    }
}

struct ServiceHandleGuard(windows_sys::Win32::System::Services::SC_HANDLE);

impl Drop for ServiceHandleGuard {
    fn drop(&mut self) {
        unsafe {
            CloseServiceHandle(self.0);
        }
    }
}

fn query_service_running(service_name: &str) -> Result<bool> {
    let manager = unsafe { OpenSCManagerW(ptr::null(), ptr::null(), SC_MANAGER_CONNECT) };
    if manager.is_null() {
        return Err(std::io::Error::last_os_error()).context("OpenSCManagerW failed");
    }
    let _manager_guard = ServiceHandleGuard(manager);

    let service_name_wide: Vec<u16> = service_name
        .encode_utf16()
        .chain(std::iter::once(0))
        .collect();
    let service =
        unsafe { OpenServiceW(manager, service_name_wide.as_ptr(), SERVICE_QUERY_STATUS) };
    if service.is_null() {
        return Err(std::io::Error::last_os_error()).context("OpenServiceW failed");
    }
    let _service_guard = ServiceHandleGuard(service);

    let mut status = SERVICE_STATUS_PROCESS::default();
    let mut bytes_needed = 0u32;
    let ok = unsafe {
        QueryServiceStatusEx(
            service,
            SC_STATUS_PROCESS_INFO,
            &mut status as *mut SERVICE_STATUS_PROCESS as *mut u8,
            mem::size_of::<SERVICE_STATUS_PROCESS>() as u32,
            &mut bytes_needed,
        )
    };
    if ok == 0 {
        return Err(std::io::Error::last_os_error()).context("QueryServiceStatusEx failed");
    }

    Ok(status.dwCurrentState == SERVICE_RUNNING)
}

fn attached_console_title(pid: u32) -> Result<String> {
    unsafe {
        FreeConsole();
    }
    let attached = unsafe { AttachConsole(pid) };
    if attached == 0 {
        return Err(std::io::Error::last_os_error()).context("AttachConsole failed");
    }
    let _guard = AttachedConsoleGuard;

    let mut buffer = vec![0u16; 32768];
    let copied = unsafe { GetConsoleTitleW(buffer.as_mut_ptr(), buffer.len() as u32) };
    if copied == 0 {
        return Ok(String::new());
    }

    Ok(String::from_utf16_lossy(&buffer[..copied as usize]))
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

/// Query process read/write counters and CPU timing for activity detection.
fn query_process_activity(pid: u32) -> Result<ProcessActivity> {
    let handle = unsafe { OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid) };
    if handle.is_null() {
        return Err(std::io::Error::last_os_error()).context("OpenProcess failed");
    }
    let _guard = HandleGuard(handle);

    let mut creation_time = FILETIME {
        dwLowDateTime: 0,
        dwHighDateTime: 0,
    };
    let mut exit_time = FILETIME {
        dwLowDateTime: 0,
        dwHighDateTime: 0,
    };
    let mut kernel_time = FILETIME {
        dwLowDateTime: 0,
        dwHighDateTime: 0,
    };
    let mut user_time = FILETIME {
        dwLowDateTime: 0,
        dwHighDateTime: 0,
    };
    let ok = unsafe {
        GetProcessTimes(
            handle,
            &mut creation_time,
            &mut exit_time,
            &mut kernel_time,
            &mut user_time,
        )
    };
    if ok == 0 {
        return Err(std::io::Error::last_os_error()).context("GetProcessTimes failed");
    }

    let mut counters = IO_COUNTERS {
        ReadOperationCount: 0,
        WriteOperationCount: 0,
        OtherOperationCount: 0,
        ReadTransferCount: 0,
        WriteTransferCount: 0,
        OtherTransferCount: 0,
    };
    let ok = unsafe { GetProcessIoCounters(handle, &mut counters) };
    if ok == 0 {
        return Err(std::io::Error::last_os_error()).context("GetProcessIoCounters failed");
    }

    Ok(ProcessActivity {
        process_start_key: filetime_to_u64(creation_time),
        read_operation_count: Some(counters.ReadOperationCount),
        write_operation_count: Some(counters.WriteOperationCount),
        other_operation_count: Some(counters.OtherOperationCount),
        read_bytes: Some(counters.ReadTransferCount),
        write_bytes: Some(counters.WriteTransferCount),
        other_bytes: Some(counters.OtherTransferCount),
        user_time_ns: Some(filetime_to_u64(user_time).saturating_mul(100)),
        system_time_ns: Some(filetime_to_u64(kernel_time).saturating_mul(100)),
    })
}

fn filetime_to_u64(value: FILETIME) -> u64 {
    ((value.dwHighDateTime as u64) << 32) | value.dwLowDateTime as u64
}

/// Return the executable basename from a full image path.
fn exe_name_from_path(path: &str) -> Option<String> {
    Path::new(path)
        .file_name()
        .and_then(|name| name.to_str())
        .filter(|name| !name.is_empty())
        .map(str::to_string)
}

fn is_console_title_host(process: &ProcessInfo) -> bool {
    let title_host_names = builtin_console_title_host_process_names();
    process_identity_names(process)
        .into_iter()
        .any(|name| matches_normalized_name(name, &title_host_names))
}

fn process_identity_names(process: &ProcessInfo) -> Vec<&str> {
    let mut names = Vec::new();
    names.push(process.exe_name.as_str());
    if let Some(exe_path) = process.exe_path.as_deref() {
        names.push(exe_path);
    }
    names
}

fn normalize_process_name(name: &str) -> String {
    let mut base_name = Path::new(name)
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or(name)
        .trim_matches('"')
        .to_ascii_lowercase();
    if let Some(stripped) = base_name.strip_suffix(".exe") {
        base_name = stripped.to_string();
    }
    base_name
}

fn matches_normalized_name(process_name: &str, candidates: &[String]) -> bool {
    let process_name = normalize_process_name(process_name);
    candidates
        .iter()
        .any(|candidate| process_name == normalize_process_name(candidate))
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
