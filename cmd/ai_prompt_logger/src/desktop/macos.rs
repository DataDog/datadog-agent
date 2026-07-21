use std::collections::{HashSet, VecDeque};
use std::ffi::{CStr, c_void};
use std::fs;
use std::os::unix::fs::MetadataExt;
use std::path::Path;
use std::process::Command;
use std::ptr;
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::Result;
use libc::{c_char, c_int, pid_t};

use crate::desktop::DesktopDetector;
use crate::desktop::matcher::{ProcessActivity, ProcessEdge, ProcessInfo, ProcessSnapshot};

const PROC_ALL_PIDS: u32 = 1;
const K_CG_WINDOW_LIST_OPTION_ON_SCREEN_ONLY: u32 = 1;
const K_CG_WINDOW_LIST_EXCLUDE_DESKTOP_ELEMENTS: u32 = 16;
const K_CF_NUMBER_SINT32_TYPE: c_int = 3;
const K_CF_STRING_ENCODING_UTF8: u32 = 0x0800_0100;

type CFTypeRef = *const c_void;
type CFArrayRef = *const c_void;
type CFDictionaryRef = *const c_void;
type CFStringRef = *const c_void;
type CFNumberRef = *const c_void;
type CFIndex = isize;
type Boolean = u8;
type CGWindowID = u32;
const RUSAGE_INFO_V2: c_int = 2;

#[repr(C)]
#[derive(Default)]
struct RUsageInfoV2 {
    ri_uuid: [u8; 16],
    ri_user_time: u64,
    ri_system_time: u64,
    ri_pkg_idle_wkups: u64,
    ri_interrupt_wkups: u64,
    ri_pageins: u64,
    ri_wired_size: u64,
    ri_resident_size: u64,
    ri_phys_footprint: u64,
    ri_proc_start_abstime: u64,
    ri_proc_exit_abstime: u64,
    ri_child_user_time: u64,
    ri_child_system_time: u64,
    ri_child_pkg_idle_wkups: u64,
    ri_child_interrupt_wkups: u64,
    ri_child_pageins: u64,
    ri_child_elapsed_abstime: u64,
    ri_diskio_bytesread: u64,
    ri_diskio_byteswritten: u64,
}

unsafe extern "C" {
    fn devname(dev: libc::dev_t, type_: libc::mode_t) -> *mut c_char;
    fn proc_pid_rusage(pid: c_int, flavor: c_int, buffer: *mut c_void) -> c_int;
}

#[link(name = "CoreGraphics", kind = "framework")]
unsafe extern "C" {
    fn CGWindowListCopyWindowInfo(option: u32, relative_to_window: CGWindowID) -> CFArrayRef;
    fn CGWindowListCreateDescriptionFromArray(window_array: CFArrayRef) -> CFArrayRef;
}

#[link(name = "CoreFoundation", kind = "framework")]
unsafe extern "C" {
    fn CFArrayCreate(
        allocator: *const c_void,
        values: *const *const c_void,
        num_values: CFIndex,
        callbacks: *const c_void,
    ) -> CFArrayRef;
    fn CFArrayGetCount(the_array: CFArrayRef) -> CFIndex;
    fn CFArrayGetValueAtIndex(the_array: CFArrayRef, idx: CFIndex) -> *const c_void;
    fn CFDictionaryGetValueIfPresent(
        the_dict: CFDictionaryRef,
        key: *const c_void,
        value: *mut *const c_void,
    ) -> Boolean;
    fn CFNumberGetValue(number: CFNumberRef, the_type: c_int, value_ptr: *mut c_void) -> Boolean;
    fn CFRelease(cf: CFTypeRef);
    fn CFStringCreateWithCString(
        alloc: *const c_void,
        c_str: *const c_char,
        encoding: u32,
    ) -> CFStringRef;
    fn CFStringGetCString(
        the_string: CFStringRef,
        buffer: *mut c_char,
        buffer_size: CFIndex,
        encoding: u32,
    ) -> Boolean;
}

pub struct MacosDesktopDetector;

impl DesktopDetector for MacosDesktopDetector {
    /// Return the owner process for the frontmost visible macOS window.
    fn foreground_process(&self) -> Result<Option<ProcessInfo>> {
        let Some(window) = frontmost_window() else {
            return Ok(None);
        };

        let bsd_info = process_bsd_info(window.pid);
        let exe_path = query_process_path(window.pid);
        let argv = query_process_args(window.pid);
        let argv0 = argv.first().cloned();
        let terminal_name = bsd_info.as_ref().and_then(terminal_name);
        let terminal_access_time_seconds = terminal_name
            .as_deref()
            .and_then(terminal_access_time_seconds);
        let terminal_activity_age_seconds =
            terminal_access_time_seconds.and_then(terminal_activity_age_seconds);
        let process_activity = process_activity(window.pid);
        let exe_name = exe_path
            .as_deref()
            .and_then(exe_name_from_path)
            .or(window.owner_name)
            .or_else(|| bsd_info.as_ref().and_then(process_name));

        Ok(exe_name.map(|exe_name| ProcessInfo {
            pid: window.pid,
            parent_pid: bsd_info
                .as_ref()
                .map(|info| info.pbi_ppid)
                .unwrap_or_default(),
            exe_name,
            bsd_name: bsd_info.as_ref().and_then(process_bsd_name),
            bsd_comm: bsd_info.as_ref().and_then(process_bsd_comm),
            argv0,
            argv,
            exe_path,
            window_title: window.title,
            attached_console_title: None,
            process_group_id: bsd_info.as_ref().map(|info| info.pbi_pgid),
            terminal_foreground_process_group_id: bsd_info
                .as_ref()
                .and_then(terminal_foreground_process_group_id),
            has_controlling_terminal: bsd_info.as_ref().is_some_and(has_controlling_terminal),
            terminal_name,
            terminal_access_time_seconds,
            terminal_activity_age_seconds,
            process_activity,
            process_activity_delta: None,
            process_read_write_activity_observed: false,
        }))
    }

    /// Return enriched process metadata plus opaque PID edges under the foreground process.
    fn process_snapshot(&self, foreground_pid: u32) -> Result<ProcessSnapshot> {
        Ok(process_snapshot(foreground_pid))
    }

    /// Report whether launchd currently considers the system Agent service running.
    fn agent_service_running(&self) -> bool {
        let Ok(output) = Command::new("/bin/launchctl")
            .args(["print", "system/com.datadoghq.agent"])
            .output()
        else {
            return true;
        };
        output.status.success() && launchctl_print_has_pid(&String::from_utf8_lossy(&output.stdout))
    }

    /// Stay resident in the user session and resume scanning when the Agent starts again.
    fn should_idle_when_agent_service_stopped(&self) -> bool {
        true
    }
}

#[derive(Debug)]
struct ForegroundWindow {
    pid: u32,
    owner_name: Option<String>,
    title: Option<String>,
}

struct CFStringKey(CFStringRef);

impl CFStringKey {
    fn new(name: &'static CStr) -> Option<Self> {
        let value = unsafe {
            CFStringCreateWithCString(ptr::null(), name.as_ptr(), K_CF_STRING_ENCODING_UTF8)
        };
        if value.is_null() {
            None
        } else {
            Some(Self(value))
        }
    }

    fn as_void(&self) -> *const c_void {
        self.0
    }
}

impl Drop for CFStringKey {
    fn drop(&mut self) {
        unsafe {
            CFRelease(self.0);
        }
    }
}

struct CFObject(CFTypeRef);

impl Drop for CFObject {
    fn drop(&mut self) {
        unsafe {
            CFRelease(self.0);
        }
    }
}

/// Return the first on-screen layer-0 window as the foreground application window.
fn frontmost_window() -> Option<ForegroundWindow> {
    let window_number_key = CFStringKey::new(c"kCGWindowNumber")?;
    let owner_pid_key = CFStringKey::new(c"kCGWindowOwnerPID")?;
    let owner_name_key = CFStringKey::new(c"kCGWindowOwnerName")?;
    let window_name_key = CFStringKey::new(c"kCGWindowName")?;
    let window_layer_key = CFStringKey::new(c"kCGWindowLayer")?;

    let windows = unsafe {
        CGWindowListCopyWindowInfo(
            K_CG_WINDOW_LIST_OPTION_ON_SCREEN_ONLY | K_CG_WINDOW_LIST_EXCLUDE_DESKTOP_ELEMENTS,
            0,
        )
    };
    if windows.is_null() {
        return None;
    }
    let _windows_guard = CFObject(windows);

    let count = unsafe { CFArrayGetCount(windows) };
    for idx in 0..count {
        let window = unsafe { CFArrayGetValueAtIndex(windows, idx) as CFDictionaryRef };
        if window.is_null() {
            continue;
        }

        let layer = dictionary_i32(window, &window_layer_key).unwrap_or_default();
        if layer != 0 {
            continue;
        }

        let Some(pid) = dictionary_i32(window, &owner_pid_key) else {
            continue;
        };
        if pid <= 0 || pid as u32 == std::process::id() {
            continue;
        }

        let window_id = dictionary_i32(window, &window_number_key).unwrap_or_default() as u32;
        let title = non_empty_string(dictionary_string(window, &window_name_key))
            .or_else(|| window_title_from_description(window_id, &window_name_key));

        return Some(ForegroundWindow {
            pid: pid as u32,
            owner_name: dictionary_string(window, &owner_name_key),
            title,
        });
    }

    None
}

/// Query CoreGraphics for one window's description and return its title, if published.
fn window_title_from_description(
    window_id: CGWindowID,
    window_name_key: &CFStringKey,
) -> Option<String> {
    if window_id == 0 {
        return None;
    }

    let window_id_value = window_id as usize as *const c_void;
    let window_values = [window_id_value];
    let window_array = unsafe {
        CFArrayCreate(
            ptr::null(),
            window_values.as_ptr(),
            window_values.len() as CFIndex,
            ptr::null(),
        )
    };
    if window_array.is_null() {
        return None;
    }
    let _window_array_guard = CFObject(window_array);

    let window_list = unsafe { CGWindowListCreateDescriptionFromArray(window_array) };
    if window_list.is_null() {
        return None;
    }
    let _window_list_guard = CFObject(window_list);

    if unsafe { CFArrayGetCount(window_list) } <= 0 {
        return None;
    }

    let window = unsafe { CFArrayGetValueAtIndex(window_list, 0) as CFDictionaryRef };
    if window.is_null() {
        return None;
    }

    non_empty_string(dictionary_string(window, window_name_key))
}

fn dictionary_i32(dictionary: CFDictionaryRef, key: &CFStringKey) -> Option<i32> {
    let value = dictionary_value(dictionary, key)?;
    let mut number = 0i32;
    let ok = unsafe {
        CFNumberGetValue(
            value as CFNumberRef,
            K_CF_NUMBER_SINT32_TYPE,
            (&mut number as *mut i32).cast(),
        )
    };
    (ok != 0).then_some(number)
}

fn dictionary_string(dictionary: CFDictionaryRef, key: &CFStringKey) -> Option<String> {
    let value = dictionary_value(dictionary, key)?;
    cf_string_to_string(value as CFStringRef)
}

fn non_empty_string(value: Option<String>) -> Option<String> {
    value.filter(|value| !value.trim().is_empty())
}

fn dictionary_value(dictionary: CFDictionaryRef, key: &CFStringKey) -> Option<*const c_void> {
    let mut value = ptr::null();
    let ok = unsafe { CFDictionaryGetValueIfPresent(dictionary, key.as_void(), &mut value) };
    (ok != 0 && !value.is_null()).then_some(value)
}

fn cf_string_to_string(value: CFStringRef) -> Option<String> {
    let mut buffer = vec![0 as c_char; 4096];
    let ok = unsafe {
        CFStringGetCString(
            value,
            buffer.as_mut_ptr(),
            buffer.len() as CFIndex,
            K_CF_STRING_ENCODING_UTF8,
        )
    };
    if ok == 0 {
        return None;
    }
    c_string_from_ptr(buffer.as_ptr())
}

/// Build a snapshot from global enriched processes and foreground-rooted opaque edges.
fn process_snapshot(foreground_pid: u32) -> ProcessSnapshot {
    let pids = process_pids();
    let processes: Vec<ProcessInfo> = pids.into_iter().filter_map(process_info_from_pid).collect();

    let mut edges = Vec::new();
    let mut seen_edges = HashSet::new();
    for process in &processes {
        push_edge(&mut edges, &mut seen_edges, process.pid, process.parent_pid);
    }
    for edge in foreground_child_edges(foreground_pid) {
        push_edge(&mut edges, &mut seen_edges, edge.pid, edge.parent_pid);
    }

    ProcessSnapshot { processes, edges }
}

/// Return all process IDs visible through libproc.
fn process_pids() -> Vec<pid_t> {
    let bytes = unsafe { libc::proc_listpids(PROC_ALL_PIDS, 0, ptr::null_mut(), 0) };
    if bytes <= 0 {
        return Vec::new();
    }

    let mut pids = vec![0 as pid_t; (bytes as usize / std::mem::size_of::<pid_t>()) + 256];
    let bytes = unsafe {
        libc::proc_listpids(
            PROC_ALL_PIDS,
            0,
            pids.as_mut_ptr().cast(),
            (pids.len() * std::mem::size_of::<pid_t>()) as c_int,
        )
    };
    if bytes <= 0 {
        return Vec::new();
    }

    let count = bytes as usize / std::mem::size_of::<pid_t>();
    pids.truncate(count);

    pids.into_iter().filter(|pid| *pid > 0).collect()
}

/// Recursively collect child PID edges below the foreground process without enrichment.
fn foreground_child_edges(foreground_pid: u32) -> Vec<ProcessEdge> {
    let mut edges = Vec::new();
    let mut seen_pids = HashSet::new();
    let mut queue = VecDeque::from([foreground_pid]);

    while let Some(parent_pid) = queue.pop_front() {
        if !seen_pids.insert(parent_pid) {
            continue;
        }
        for child_pid in child_pids(parent_pid) {
            edges.push(ProcessEdge {
                pid: child_pid,
                parent_pid,
            });
            queue.push_back(child_pid);
        }
    }

    edges
}

/// Return direct child PIDs for a parent using libproc's low-requirement child listing.
fn child_pids(parent_pid: u32) -> Vec<u32> {
    let count = unsafe { libc::proc_listchildpids(parent_pid as pid_t, ptr::null_mut(), 0) };
    if count <= 0 {
        return Vec::new();
    }

    let mut pids = vec![0 as pid_t; count as usize + 16];
    let count = unsafe {
        libc::proc_listchildpids(
            parent_pid as pid_t,
            pids.as_mut_ptr().cast(),
            (pids.len() * std::mem::size_of::<pid_t>()) as c_int,
        )
    };
    if count <= 0 {
        return Vec::new();
    }

    pids.truncate((count as usize).min(pids.len()));
    pids.into_iter()
        .filter(|pid| *pid > 0)
        .map(|pid| pid as u32)
        .collect()
}

/// Add one valid PID edge while de-duplicating repeated edge sources.
fn push_edge(
    edges: &mut Vec<ProcessEdge>,
    seen_edges: &mut HashSet<(u32, u32)>,
    pid: u32,
    parent_pid: u32,
) {
    if pid == 0 || parent_pid == 0 || pid == parent_pid {
        return;
    }
    if seen_edges.insert((pid, parent_pid)) {
        edges.push(ProcessEdge { pid, parent_pid });
    }
}

/// Enrich a PID with process identity, executable path, argv, and terminal state.
fn process_info_from_pid(pid: pid_t) -> Option<ProcessInfo> {
    let info = process_bsd_info(pid as u32)?;
    let exe_path = query_process_path(pid as u32);
    let argv = query_process_args(pid as u32);
    let argv0 = argv.first().cloned();
    let terminal_name = terminal_name(&info);
    let terminal_access_time_seconds = terminal_name
        .as_deref()
        .and_then(terminal_access_time_seconds);
    let terminal_activity_age_seconds =
        terminal_access_time_seconds.and_then(terminal_activity_age_seconds);
    let process_activity = process_activity(pid as u32);
    let exe_name = exe_path
        .as_deref()
        .and_then(exe_name_from_path)
        .or_else(|| process_name(&info))?;

    Some(ProcessInfo {
        pid: info.pbi_pid,
        parent_pid: info.pbi_ppid,
        exe_name,
        bsd_name: process_bsd_name(&info),
        bsd_comm: process_bsd_comm(&info),
        argv0,
        argv,
        exe_path,
        window_title: None,
        attached_console_title: None,
        process_group_id: Some(info.pbi_pgid),
        terminal_foreground_process_group_id: terminal_foreground_process_group_id(&info),
        has_controlling_terminal: has_controlling_terminal(&info),
        terminal_name,
        terminal_access_time_seconds,
        terminal_activity_age_seconds,
        process_activity,
        process_activity_delta: None,
        process_read_write_activity_observed: false,
    })
}

/// Query BSD process metadata for a PID.
fn process_bsd_info(pid: u32) -> Option<libc::proc_bsdinfo> {
    let mut info = unsafe { std::mem::zeroed::<libc::proc_bsdinfo>() };
    let size = std::mem::size_of::<libc::proc_bsdinfo>() as c_int;
    let result = unsafe {
        libc::proc_pidinfo(
            pid as c_int,
            libc::PROC_PIDTBSDINFO,
            0,
            (&mut info as *mut libc::proc_bsdinfo).cast(),
            size,
        )
    };
    (result == size).then_some(info)
}

/// Read the process argv vector through `KERN_PROCARGS2`.
fn query_process_args(pid: u32) -> Vec<String> {
    let mut mib = [libc::CTL_KERN, libc::KERN_PROCARGS2, pid as libc::c_int];
    let mut size = 8192usize;
    let mut buffer = vec![0u8; size];
    let ok = unsafe {
        libc::sysctl(
            mib.as_mut_ptr(),
            mib.len() as u32,
            buffer.as_mut_ptr().cast(),
            &mut size,
            ptr::null_mut(),
            0,
        )
    };
    if ok != 0 || size < std::mem::size_of::<c_int>() {
        return Vec::new();
    }
    buffer.truncate(size);

    let Some(argc) = buffer[..std::mem::size_of::<c_int>()]
        .try_into()
        .ok()
        .map(i32::from_ne_bytes)
    else {
        return Vec::new();
    };
    if argc <= 0 {
        return Vec::new();
    }

    let mut offset = std::mem::size_of::<c_int>();
    if skip_c_string(&buffer, &mut offset).is_none() {
        return Vec::new();
    }
    skip_nul_bytes(&buffer, &mut offset);

    let mut args = Vec::new();
    for _ in 0..argc {
        let Some(arg) = read_c_string(&buffer, offset) else {
            break;
        };
        offset += arg.len() + 1;
        args.push(arg);
    }
    args
}

/// Query the executable path for a PID.
fn query_process_path(pid: u32) -> Option<String> {
    let mut buffer = vec![0u8; libc::PROC_PIDPATHINFO_MAXSIZE as usize];
    let len = unsafe {
        libc::proc_pidpath(
            pid as c_int,
            buffer.as_mut_ptr().cast(),
            buffer.len() as u32,
        )
    };
    if len <= 0 {
        return None;
    }
    buffer.truncate(len as usize);
    String::from_utf8(buffer)
        .ok()
        .filter(|path| !path.is_empty())
}

fn process_name(info: &libc::proc_bsdinfo) -> Option<String> {
    process_bsd_name(info).or_else(|| process_bsd_comm(info))
}

fn process_bsd_name(info: &libc::proc_bsdinfo) -> Option<String> {
    c_string_from_ptr(info.pbi_name.as_ptr())
}

fn process_bsd_comm(info: &libc::proc_bsdinfo) -> Option<String> {
    c_string_from_ptr(info.pbi_comm.as_ptr())
}

fn process_activity(pid: u32) -> Option<ProcessActivity> {
    let mut usage = RUsageInfoV2::default();
    let result = unsafe {
        proc_pid_rusage(
            pid as c_int,
            RUSAGE_INFO_V2,
            (&mut usage as *mut RUsageInfoV2).cast(),
        )
    };
    if result != 0 {
        return None;
    }

    Some(ProcessActivity {
        process_start_key: usage.ri_proc_start_abstime,
        read_operation_count: None,
        write_operation_count: None,
        other_operation_count: None,
        read_bytes: Some(usage.ri_diskio_bytesread),
        write_bytes: Some(usage.ri_diskio_byteswritten),
        other_bytes: None,
        user_time_ns: Some(usage.ri_user_time),
        system_time_ns: Some(usage.ri_system_time),
    })
}

fn c_string_from_ptr(ptr: *const c_char) -> Option<String> {
    if ptr.is_null() {
        return None;
    }
    let value = unsafe { CStr::from_ptr(ptr) }
        .to_string_lossy()
        .into_owned();
    (!value.is_empty()).then_some(value)
}

fn exe_name_from_path(path: &str) -> Option<String> {
    Path::new(path)
        .file_name()
        .and_then(|name| name.to_str())
        .filter(|name| !name.is_empty())
        .map(str::to_string)
}

fn skip_c_string(buffer: &[u8], offset: &mut usize) -> Option<()> {
    while *offset < buffer.len() {
        let value = buffer[*offset];
        *offset += 1;
        if value == 0 {
            return Some(());
        }
    }
    None
}

fn skip_nul_bytes(buffer: &[u8], offset: &mut usize) {
    while *offset < buffer.len() && buffer[*offset] == 0 {
        *offset += 1;
    }
}

fn read_c_string(buffer: &[u8], offset: usize) -> Option<String> {
    if offset >= buffer.len() {
        return None;
    }
    let end = buffer[offset..]
        .iter()
        .position(|value| *value == 0)
        .map(|end| offset + end)
        .unwrap_or(buffer.len());
    if end == offset {
        return None;
    }
    Some(String::from_utf8_lossy(&buffer[offset..end]).into_owned())
}

/// Return whether BSD metadata indicates a controlling terminal.
fn has_controlling_terminal(info: &libc::proc_bsdinfo) -> bool {
    info.e_tdev != 0 && info.e_tdev != u32::MAX
}

/// Return the controlling terminal device name, for example `ttys009`.
fn terminal_name(info: &libc::proc_bsdinfo) -> Option<String> {
    if !has_controlling_terminal(info) {
        return None;
    }

    let name = unsafe { devname(info.e_tdev as libc::dev_t, libc::S_IFCHR) };
    c_string_from_ptr(name)
}

/// Return the tty device node's last access timestamp in Unix seconds.
fn terminal_access_time_seconds(terminal_name: &str) -> Option<u64> {
    let metadata = fs::metadata(format!("/dev/{terminal_name}")).ok()?;
    terminal_access_time_seconds_from_atime(metadata.atime())
}

fn terminal_access_time_seconds_from_atime(atime: i64) -> Option<u64> {
    if atime < 0 {
        return None;
    }

    Some(atime as u64)
}

fn terminal_activity_age_seconds(access_time_seconds: u64) -> Option<u64> {
    let now = SystemTime::now().duration_since(UNIX_EPOCH).ok()?.as_secs();
    Some(now.saturating_sub(access_time_seconds))
}

/// Return the terminal foreground process group ID when terminal state is available.
fn terminal_foreground_process_group_id(info: &libc::proc_bsdinfo) -> Option<u32> {
    if has_controlling_terminal(info) && info.e_tpgid != 0 && info.e_tpgid != u32::MAX {
        Some(info.e_tpgid)
    } else {
        None
    }
}

/// Parse `launchctl print` output and report whether a running PID is present.
fn launchctl_print_has_pid(output: &str) -> bool {
    output
        .lines()
        .any(|line| line.trim_start().starts_with("pid = "))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn launchctl_print_has_pid_detects_running_service() {
        assert!(launchctl_print_has_pid(
            r#"
system/com.datadoghq.agent = {
    active count = 1
    pid = 123
}
"#
        ));
    }

    #[test]
    fn launchctl_print_has_pid_rejects_loaded_but_stopped_service() {
        assert!(!launchctl_print_has_pid(
            r#"
system/com.datadoghq.agent = {
    active count = 0
}
"#
        ));
    }

    #[test]
    fn terminal_activity_age_uses_access_timestamp() {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system clock should be after epoch")
            .as_secs() as i64;

        let access_time = terminal_access_time_seconds_from_atime(now - 5)
            .expect("positive access timestamp should parse");
        let age = terminal_activity_age_seconds(access_time)
            .expect("positive timestamps should produce an age");

        assert!(age <= 5);
    }
}
