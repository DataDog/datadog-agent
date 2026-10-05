use std::collections::{HashMap, HashSet};
use std::path::Path;

use crate::datadog::{AiProcessConfig, AiProcessMatchScope, DesktopMonitoringConfig};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessInfo {
    pub pid: u32,
    pub parent_pid: u32,
    pub exe_name: String,
    pub bsd_name: Option<String>,
    pub bsd_comm: Option<String>,
    pub argv0: Option<String>,
    pub argv: Vec<String>,
    pub exe_path: Option<String>,
    pub window_title: Option<String>,
    pub attached_console_title: Option<String>,
    pub process_group_id: Option<u32>,
    pub terminal_foreground_process_group_id: Option<u32>,
    pub has_controlling_terminal: bool,
    pub terminal_name: Option<String>,
    pub terminal_access_time_seconds: Option<u64>,
    pub terminal_activity_age_seconds: Option<u64>,
    pub process_activity: Option<ProcessActivity>,
    pub process_activity_delta: Option<ProcessActivityDelta>,
    pub process_read_write_activity_observed: bool,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessActivity {
    pub process_start_key: u64,
    pub read_operation_count: Option<u64>,
    pub write_operation_count: Option<u64>,
    pub other_operation_count: Option<u64>,
    pub read_bytes: Option<u64>,
    pub write_bytes: Option<u64>,
    pub other_bytes: Option<u64>,
    pub user_time_ns: Option<u64>,
    pub system_time_ns: Option<u64>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct ProcessActivityDelta {
    pub read_operation_count: Option<u64>,
    pub write_operation_count: Option<u64>,
    pub other_operation_count: Option<u64>,
    pub read_bytes: Option<u64>,
    pub write_bytes: Option<u64>,
    pub other_bytes: Option<u64>,
    pub user_time_ns: Option<u64>,
    pub system_time_ns: Option<u64>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessEdge {
    pub pid: u32,
    pub parent_pid: u32,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessSnapshot {
    pub processes: Vec<ProcessInfo>,
    pub edges: Vec<ProcessEdge>,
}

impl ProcessSnapshot {
    #[cfg(any(test, windows))]
    /// Build a snapshot whose edge graph is derived from each process parent PID.
    pub fn from_processes(processes: Vec<ProcessInfo>) -> Self {
        let edges = processes
            .iter()
            .map(|process| ProcessEdge {
                pid: process.pid,
                parent_pid: process.parent_pid,
            })
            .collect();

        Self { processes, edges }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AiUsageDetection {
    pub tool: String,
    pub provider: String,
    pub approved: bool,
    pub matched_process_name: String,
    pub matched_pid: u32,
    pub foreground_process_name: String,
    pub foreground_pid: u32,
}

/// Detect all configured AI usage from the foreground process and process snapshot.
pub fn detect_ai_usages(
    foreground: &ProcessInfo,
    snapshot: &ProcessSnapshot,
    config: &DesktopMonitoringConfig,
) -> Vec<AiUsageDetection> {
    let foreground_is_host = matches_process_name(foreground, &config.host_process_names);
    let foreground_is_terminal_host = is_terminal_host_process(foreground);
    let process_by_pid: HashMap<u32, &ProcessInfo> = snapshot
        .processes
        .iter()
        .map(|process| (process.pid, process))
        .collect();
    let children_by_parent = children_by_parent(&snapshot.edges);
    let descendants = if foreground_is_host {
        descendants_of(foreground.pid, &children_by_parent, &process_by_pid)
    } else {
        Vec::new()
    };
    if let Some(tool) = find_ai_process_for_location(
        foreground,
        MatchLocation::ForegroundRoot,
        &config.ai_process_names,
    ) {
        return vec![detection_from_match(foreground, foreground, tool)];
    }

    let ai_candidates = ai_process_candidates(
        &snapshot.processes,
        MatchLocation::HostedDescendant,
        &config.ai_process_names,
    );
    let mut detections = Vec::new();
    for process in descendants {
        if let Some(tool) = ai_candidates.get(&process.pid) {
            let in_terminal_foreground_group =
                foreground_is_terminal_host && is_in_terminal_foreground_group(process);
            if foreground_is_terminal_host && !in_terminal_foreground_group {
                continue;
            }

            // Require hosted descendants to show read/write IO activity so hidden panels,
            // background helper windows, and stale AI agent processes do not count as
            // active usage. POSIX terminal foreground process groups are the exception:
            // the user may be actively reading output even while IO counters are idle.
            if !process.process_read_write_activity_observed && !in_terminal_foreground_group {
                continue;
            }
            detections.push(detection_from_match(foreground, process, tool));
        }
    }
    if !detections.is_empty() {
        return platform_descendant_detections(detections);
    }

    #[cfg(windows)]
    {
        let detections =
            windows_console_title_matches(foreground, &snapshot.processes, &ai_candidates);
        if !detections.is_empty() {
            return detections;
        }
    }

    #[cfg(windows)]
    return windows_activity_fallback(foreground, &snapshot.processes, &ai_candidates);
    #[cfg(not(windows))]
    return Vec::new();
}

#[cfg(windows)]
fn platform_descendant_detections(detections: Vec<AiUsageDetection>) -> Vec<AiUsageDetection> {
    detections
}

#[cfg(not(windows))]
fn platform_descendant_detections(detections: Vec<AiUsageDetection>) -> Vec<AiUsageDetection> {
    detections.into_iter().take(1).collect()
}

#[cfg(windows)]
fn windows_console_title_matches(
    foreground: &ProcessInfo,
    processes: &[ProcessInfo],
    ai_candidates: &HashMap<u32, &AiProcessConfig>,
) -> Vec<AiUsageDetection> {
    let Some(foreground_title) = foreground.window_title.as_deref() else {
        return Vec::new();
    };
    if foreground_title.is_empty() {
        return Vec::new();
    }

    let mut seen_tools = HashSet::new();
    processes
        .iter()
        .filter_map(|process| {
            let tool = ai_candidates.get(&process.pid)?;
            let console_title = process.attached_console_title.as_deref()?;
            if console_title.trim_end() != foreground_title.trim_end()
                || !seen_tools.insert(tool.tool.clone())
            {
                return None;
            }
            Some(detection_from_match(foreground, process, tool))
        })
        .collect()
}

#[cfg(windows)]
fn windows_activity_fallback(
    foreground: &ProcessInfo,
    processes: &[ProcessInfo],
    ai_candidates: &HashMap<u32, &AiProcessConfig>,
) -> Vec<AiUsageDetection> {
    let mut seen_tools = HashSet::new();
    processes
        .iter()
        .filter_map(|process| {
            let tool = ai_candidates.get(&process.pid)?;
            if tool.match_scope != AiProcessMatchScope::HostedChild {
                return None;
            }
            if !process.process_read_write_activity_observed
                || !seen_tools.insert(tool.tool.clone())
            {
                return None;
            }
            Some(detection_from_match(foreground, process, tool))
        })
        .collect()
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum MatchLocation {
    ForegroundRoot,
    HostedDescendant,
}

fn find_ai_process_for_location<'a>(
    process: &ProcessInfo,
    location: MatchLocation,
    ai_processes: &'a [AiProcessConfig],
) -> Option<&'a AiProcessConfig> {
    ai_processes
        .iter()
        .filter(|candidate| matches_location(candidate, location))
        .find(|candidate| matches_process_name(process, &candidate.process_names))
}

/// Return the hosted AI process config that matches a process identity, if any.
pub(crate) fn find_hosted_ai_process<'a>(
    process: &ProcessInfo,
    ai_processes: &'a [AiProcessConfig],
) -> Option<&'a AiProcessConfig> {
    find_ai_process_for_location(process, MatchLocation::HostedDescendant, ai_processes)
}

fn matches_location(process: &AiProcessConfig, location: MatchLocation) -> bool {
    match location {
        MatchLocation::ForegroundRoot => {
            !process.secondary
                && matches!(
                    process.match_scope,
                    AiProcessMatchScope::Direct | AiProcessMatchScope::Both
                )
        }
        MatchLocation::HostedDescendant => {
            matches!(
                process.match_scope,
                AiProcessMatchScope::HostedChild | AiProcessMatchScope::Both
            )
        }
    }
}

fn ai_process_candidates<'a>(
    processes: &'a [ProcessInfo],
    location: MatchLocation,
    ai_processes: &'a [AiProcessConfig],
) -> HashMap<u32, &'a AiProcessConfig> {
    let mut candidates = HashMap::new();
    for process in processes {
        if let Some(tool) = find_ai_process_for_location(process, location, ai_processes) {
            candidates.entry(process.pid).or_insert(tool);
        }
    }
    candidates
}

fn is_in_terminal_foreground_group(process: &ProcessInfo) -> bool {
    process.has_controlling_terminal
        && process.process_group_id.is_some()
        && process.process_group_id == process.terminal_foreground_process_group_id
}

#[cfg(windows)]
fn is_terminal_host_process(_process: &ProcessInfo) -> bool {
    // Windows terminals do not expose the POSIX foreground process-group signal
    // so foreground process group matching is unreliable.
    false
}

#[cfg(not(windows))]
fn is_terminal_host_process(process: &ProcessInfo) -> bool {
    process_identity_names(process).into_iter().any(|name| {
        matches!(
            normalize_process_name(name).as_str(),
            "terminal" | "iterm2" | "ghostty" | "wezterm" | "wezterm-gui" | "alacritty" | "kitty"
        )
    })
}

fn detection_from_match(
    foreground: &ProcessInfo,
    matched: &ProcessInfo,
    tool: &AiProcessConfig,
) -> AiUsageDetection {
    AiUsageDetection {
        tool: tool.tool.clone(),
        provider: tool.provider.clone(),
        approved: tool.approved,
        matched_process_name: matched.exe_name.clone(),
        matched_pid: matched.pid,
        foreground_process_name: foreground.exe_name.clone(),
        foreground_pid: foreground.pid,
    }
}

/// Match a process against configured names using executable, path, and argv identities.
pub(crate) fn matches_process_name(process: &ProcessInfo, candidates: &[String]) -> bool {
    process_identity_names(process)
        .into_iter()
        .any(|process_name| {
            let process_name = normalize_process_name(process_name);
            candidates
                .iter()
                .any(|candidate| normalize_process_name(candidate) == process_name)
        })
}

/// Return all process identity strings considered for name matching.
fn process_identity_names(process: &ProcessInfo) -> Vec<&str> {
    let mut names = vec![process.exe_name.as_str()];
    if let Some(argv0) = process.argv0.as_deref() {
        names.push(argv0);
    }
    names.extend(process.argv.iter().map(String::as_str));
    if let Some(bsd_name) = process.bsd_name.as_deref() {
        names.push(bsd_name);
    }
    if let Some(bsd_comm) = process.bsd_comm.as_deref() {
        names.push(bsd_comm);
    }
    if let Some(exe_path) = process.exe_path.as_deref() {
        names.push(exe_path);
    }
    names
}

/// Normalize executable names and paths for case-insensitive cross-platform matching.
fn normalize_process_name(name: &str) -> String {
    let trimmed = name.trim().trim_matches('"');
    let basename = Path::new(trimmed)
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or(trimmed);
    let lower = basename
        .trim_end_matches(|c: char| c.is_whitespace())
        .to_ascii_lowercase();
    lower.strip_suffix(".exe").unwrap_or(&lower).to_string()
}

/// Index PID edges by parent PID for descendant traversal.
pub(super) fn children_by_parent(edges: &[ProcessEdge]) -> HashMap<u32, Vec<u32>> {
    let mut children: HashMap<u32, Vec<u32>> = HashMap::new();
    for edge in edges {
        children.entry(edge.parent_pid).or_default().push(edge.pid);
    }
    children
}

/// Return enriched process descendants reachable through the PID edge graph.
fn descendants_of<'a>(
    root_pid: u32,
    children_by_parent: &HashMap<u32, Vec<u32>>,
    process_by_pid: &HashMap<u32, &'a ProcessInfo>,
) -> Vec<&'a ProcessInfo> {
    let mut visited = HashSet::new();
    let mut stack = children_by_parent
        .get(&root_pid)
        .cloned()
        .unwrap_or_default();
    let mut descendants = Vec::new();

    while let Some(pid) = stack.pop() {
        if !visited.insert(pid) {
            continue;
        }
        if let Some(process) = process_by_pid.get(&pid).copied() {
            descendants.push(process);
        }
        if let Some(children) = children_by_parent.get(&pid) {
            stack.extend(children);
        }
    }

    descendants
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::datadog::{AiProcessConfig, AiProcessMatchScope, DesktopMonitoringConfig};

    fn config() -> DesktopMonitoringConfig {
        DesktopMonitoringConfig {
            enabled: true,
            debug: 0,
            poll_interval_seconds: 60,
            process_activity_window_seconds: 600,
            ai_process_names: vec![
                AiProcessConfig {
                    process_names: vec!["Cursor.exe".to_string()],
                    tool: "Cursor".to_string(),
                    provider: "Anysphere".to_string(),
                    match_scope: AiProcessMatchScope::Both,
                    approved: false,
                    secondary: false,
                },
                AiProcessConfig {
                    process_names: vec!["Claude.exe".to_string()],
                    tool: "Claude".to_string(),
                    provider: "Anthropic".to_string(),
                    match_scope: AiProcessMatchScope::Direct,
                    approved: false,
                    secondary: false,
                },
                AiProcessConfig {
                    process_names: vec!["claude.exe".to_string()],
                    tool: "Claude Code".to_string(),
                    provider: "Anthropic".to_string(),
                    match_scope: AiProcessMatchScope::HostedChild,
                    approved: true,
                    secondary: false,
                },
            ],
            host_process_names: vec!["cmd.exe".to_string(), "Code.exe".to_string()],
        }
    }

    fn detect_ai_usage(
        foreground: &ProcessInfo,
        snapshot: &ProcessSnapshot,
        config: &DesktopMonitoringConfig,
    ) -> Option<AiUsageDetection> {
        detect_ai_usages(foreground, snapshot, config)
            .into_iter()
            .next()
    }

    fn process(pid: u32, parent_pid: u32, exe_name: &str) -> ProcessInfo {
        ProcessInfo {
            pid,
            parent_pid,
            exe_name: exe_name.to_string(),
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
            process_activity: None,
            process_activity_delta: None,
            process_read_write_activity_observed: false,
        }
    }

    fn process_with_title(pid: u32, parent_pid: u32, exe_name: &str, title: &str) -> ProcessInfo {
        ProcessInfo {
            pid,
            parent_pid,
            exe_name: exe_name.to_string(),
            bsd_name: None,
            bsd_comm: None,
            argv0: None,
            argv: Vec::new(),
            exe_path: None,
            window_title: Some(title.to_string()),
            attached_console_title: None,
            process_group_id: None,
            terminal_foreground_process_group_id: None,
            has_controlling_terminal: false,
            terminal_name: None,
            terminal_access_time_seconds: None,
            terminal_activity_age_seconds: None,
            process_activity: None,
            process_activity_delta: None,
            process_read_write_activity_observed: false,
        }
    }

    fn terminal_process(
        pid: u32,
        parent_pid: u32,
        exe_name: &str,
        process_group_id: u32,
        terminal_foreground_process_group_id: u32,
    ) -> ProcessInfo {
        ProcessInfo {
            pid,
            parent_pid,
            exe_name: exe_name.to_string(),
            bsd_name: Some(exe_name.to_string()),
            bsd_comm: Some(exe_name.to_string()),
            argv0: Some(exe_name.to_string()),
            argv: vec![exe_name.to_string()],
            exe_path: None,
            window_title: None,
            attached_console_title: None,
            process_group_id: Some(process_group_id),
            terminal_foreground_process_group_id: Some(terminal_foreground_process_group_id),
            has_controlling_terminal: true,
            terminal_name: Some(format!("ttys{pid:03}")),
            terminal_access_time_seconds: Some(1_000),
            terminal_activity_age_seconds: Some(0),
            process_activity: Some(activity(pid as u64, 0, 0)),
            process_activity_delta: Some(ProcessActivityDelta {
                read_bytes: Some(1),
                ..ProcessActivityDelta::default()
            }),
            process_read_write_activity_observed: true,
        }
    }

    fn process_with_identity(
        pid: u32,
        parent_pid: u32,
        exe_name: &str,
        bsd_name: Option<&str>,
        bsd_comm: Option<&str>,
        argv0: Option<&str>,
    ) -> ProcessInfo {
        ProcessInfo {
            pid,
            parent_pid,
            exe_name: exe_name.to_string(),
            bsd_name: bsd_name.map(str::to_string),
            bsd_comm: bsd_comm.map(str::to_string),
            argv0: argv0.map(str::to_string),
            argv: argv0.map(str::to_string).into_iter().collect(),
            exe_path: None,
            window_title: None,
            attached_console_title: None,
            process_group_id: None,
            terminal_foreground_process_group_id: None,
            has_controlling_terminal: false,
            terminal_name: None,
            terminal_access_time_seconds: None,
            terminal_activity_age_seconds: None,
            process_activity: None,
            process_activity_delta: None,
            process_read_write_activity_observed: false,
        }
    }

    fn process_with_argv(
        pid: u32,
        parent_pid: u32,
        exe_name: &str,
        argv: Vec<&str>,
    ) -> ProcessInfo {
        ProcessInfo {
            pid,
            parent_pid,
            exe_name: exe_name.to_string(),
            bsd_name: None,
            bsd_comm: None,
            argv0: argv.first().map(|value| value.to_string()),
            argv: argv.into_iter().map(str::to_string).collect(),
            exe_path: None,
            window_title: None,
            attached_console_title: None,
            process_group_id: None,
            terminal_foreground_process_group_id: None,
            has_controlling_terminal: false,
            terminal_name: None,
            terminal_access_time_seconds: None,
            terminal_activity_age_seconds: None,
            process_activity: None,
            process_activity_delta: None,
            process_read_write_activity_observed: false,
        }
    }

    fn activity(process_start_key: u64, read_bytes: u64, write_bytes: u64) -> ProcessActivity {
        ProcessActivity {
            process_start_key,
            read_operation_count: None,
            write_operation_count: None,
            other_operation_count: None,
            read_bytes: Some(read_bytes),
            write_bytes: Some(write_bytes),
            other_bytes: None,
            user_time_ns: None,
            system_time_ns: None,
        }
    }

    fn with_read_activity(mut process: ProcessInfo) -> ProcessInfo {
        process.process_activity = Some(activity(process.pid as u64, 10, 0));
        process.process_activity_delta = Some(ProcessActivityDelta {
            read_bytes: Some(1),
            ..ProcessActivityDelta::default()
        });
        process.process_read_write_activity_observed = true;
        process
    }

    fn with_write_activity(mut process: ProcessInfo) -> ProcessInfo {
        process.process_activity = Some(activity(process.pid as u64, 0, 10));
        process.process_activity_delta = Some(ProcessActivityDelta {
            write_bytes: Some(1),
            ..ProcessActivityDelta::default()
        });
        process.process_read_write_activity_observed = true;
        process
    }

    #[cfg(not(windows))]
    fn with_cpu_activity_only(mut process: ProcessInfo) -> ProcessInfo {
        process.process_activity = Some(ProcessActivity {
            process_start_key: process.pid as u64,
            read_operation_count: None,
            write_operation_count: None,
            other_operation_count: None,
            read_bytes: Some(10),
            write_bytes: Some(10),
            other_bytes: None,
            user_time_ns: Some(20),
            system_time_ns: Some(30),
        });
        process.process_activity_delta = Some(ProcessActivityDelta {
            user_time_ns: Some(10),
            system_time_ns: Some(10),
            ..ProcessActivityDelta::default()
        });
        process.process_read_write_activity_observed = false;
        process
    }

    fn with_attached_console_title(mut process: ProcessInfo, title: &str) -> ProcessInfo {
        process.attached_console_title = Some(title.to_string());
        process
    }

    fn snapshot(processes: Vec<ProcessInfo>) -> ProcessSnapshot {
        ProcessSnapshot::from_processes(processes)
    }

    fn snapshot_with_edges(processes: Vec<ProcessInfo>, edges: Vec<(u32, u32)>) -> ProcessSnapshot {
        ProcessSnapshot {
            processes,
            edges: edges
                .into_iter()
                .map(|(pid, parent_pid)| ProcessEdge { pid, parent_pid })
                .collect(),
        }
    }

    #[test]
    fn direct_match_is_case_insensitive_and_normalizes_exe_suffix() {
        let foreground = process(1, 0, "cursor");
        let detection =
            detect_ai_usage(&foreground, &snapshot(vec![foreground.clone()]), &config())
                .expect("expected direct Cursor match");

        assert_eq!(detection.tool, "Cursor");
        assert_eq!(detection.provider, "Anysphere");
        assert_eq!(detection.matched_pid, 1);
    }

    #[test]
    fn host_process_matches_descendant_ai_process() {
        let foreground = process_with_title(10, 1, "cmd.exe", "claude");
        let child = process(11, 10, "powershell.exe");
        let grandchild = with_read_activity(process(12, 11, "Claude.exe"));
        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child, grandchild]),
            &config(),
        )
        .expect("expected hosted Claude Code match");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 12);
        assert_eq!(detection.foreground_pid, 10);
    }

    #[test]
    fn host_process_matches_descendant_when_write_bytes_advance() {
        let foreground = process_with_title(10, 1, "cmd.exe", "claude");
        let child = with_write_activity(process(11, 10, "Claude.exe"));

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config(),
        )
        .expect("expected hosted Claude Code match from write activity");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn host_process_matches_descendant_by_argv0() {
        let foreground = process_with_title(10, 1, "Terminal", "claude");
        let child = with_read_activity(ProcessInfo {
            process_group_id: Some(11),
            terminal_foreground_process_group_id: Some(11),
            has_controlling_terminal: true,
            terminal_name: Some("ttys011".to_string()),
            terminal_access_time_seconds: Some(1_000),
            terminal_activity_age_seconds: Some(0),
            ..process_with_identity(11, 10, "2.1.172", None, None, Some("claude"))
        });
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config,
        )
        .expect("expected hosted Claude Code match from argv0");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
        assert_eq!(detection.matched_process_name, "2.1.172");
    }

    #[test]
    fn host_process_matches_descendant_by_bsd_name() {
        let foreground = process_with_title(10, 1, "Terminal", "claude");
        let child = with_read_activity(ProcessInfo {
            process_group_id: Some(11),
            terminal_foreground_process_group_id: Some(11),
            has_controlling_terminal: true,
            terminal_name: Some("ttys011".to_string()),
            terminal_access_time_seconds: Some(1_000),
            terminal_activity_age_seconds: Some(0),
            ..process_with_identity(11, 10, "2.1.172", Some("claude"), None, None)
        });
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config,
        )
        .expect("expected hosted Claude Code match from bsd_name");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn host_process_matches_descendant_by_bsd_comm() {
        let foreground = process_with_title(10, 1, "Terminal", "claude");
        let child = with_read_activity(ProcessInfo {
            process_group_id: Some(11),
            terminal_foreground_process_group_id: Some(11),
            has_controlling_terminal: true,
            terminal_name: Some("ttys011".to_string()),
            terminal_access_time_seconds: Some(1_000),
            terminal_activity_age_seconds: Some(0),
            ..process_with_identity(11, 10, "2.1.172", None, Some("claude"), None)
        });
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config,
        )
        .expect("expected hosted Claude Code match from bsd_comm");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn host_process_matches_descendant_by_command_line_arg_basename() {
        let foreground = process_with_title(10, 1, "Terminal", "hermes");
        let child = with_read_activity(ProcessInfo {
            process_group_id: Some(11),
            terminal_foreground_process_group_id: Some(11),
            has_controlling_terminal: true,
            terminal_name: Some("ttys011".to_string()),
            terminal_access_time_seconds: Some(1_000),
            terminal_activity_age_seconds: Some(0),
            ..process_with_argv(
                11,
                10,
                "Python",
                vec![
                    "/opt/homebrew/Cellar/python@3.11/3.11.9/Frameworks/Python.framework/Versions/3.11/Resources/Python.app/Contents/MacOS/Python",
                    "/Users/example/.hermes/hermes-agent/venv/bin/hermes",
                ],
            )
        });
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            approved: false,
            secondary: false,
        });

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config,
        )
        .expect("expected hosted Hermes match from command-line script");

        assert_eq!(detection.tool, "Hermes Agent");
        assert_eq!(detection.matched_process_name, "Python");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn host_process_matches_recent_terminal_activity_with_descriptive_title() {
        let foreground = process_with_title(10, 1, "Terminal", "work - hermes");
        let candidate = with_read_activity(ProcessInfo {
            process_group_id: Some(73059),
            terminal_foreground_process_group_id: Some(73059),
            has_controlling_terminal: true,
            terminal_name: Some("ttys009".to_string()),
            terminal_access_time_seconds: Some(1_000),
            terminal_activity_age_seconds: Some(30),
            ..process_with_argv(
                73059,
                10,
                "Python",
                vec![
                    "/opt/homebrew/Cellar/python@3.11/3.11.9/Frameworks/Python.framework/Versions/3.11/Resources/Python.app/Contents/MacOS/Python",
                    "/Users/example/.hermes/hermes-agent/venv/bin/hermes",
                ],
            )
        });
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            approved: false,
            secondary: false,
        });

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), candidate]),
            &config,
        )
        .expect("expected recent Hermes terminal activity match");

        assert_eq!(detection.tool, "Hermes Agent");
        assert_eq!(detection.matched_pid, 73059);
    }

    #[test]
    fn host_process_matches_recent_terminal_activity_ignoring_unrelated_title() {
        let foreground = process_with_title(10, 1, "Terminal", "work - vim");
        let candidate = with_read_activity(ProcessInfo {
            process_group_id: Some(73059),
            terminal_foreground_process_group_id: Some(73059),
            has_controlling_terminal: true,
            terminal_name: Some("ttys009".to_string()),
            terminal_access_time_seconds: Some(1_000),
            terminal_activity_age_seconds: Some(30),
            ..process_with_argv(
                73059,
                10,
                "Python",
                vec![
                    "/opt/homebrew/Cellar/python@3.11/3.11.9/Frameworks/Python.framework/Versions/3.11/Resources/Python.app/Contents/MacOS/Python",
                    "/Users/example/.hermes/hermes-agent/venv/bin/hermes",
                ],
            )
        });
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            approved: false,
            secondary: false,
        });

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), candidate]),
            &config,
        )
        .expect("expected recent Hermes terminal activity to match while ignoring title");

        assert_eq!(detection.tool, "Hermes Agent");
        assert_eq!(detection.matched_pid, 73059);
    }

    #[test]
    fn host_process_matches_candidate_through_opaque_process_edges() {
        let foreground = process_with_title(10, 1, "Terminal", "work - hermes");
        let candidate = with_read_activity(ProcessInfo {
            process_group_id: Some(73059),
            terminal_foreground_process_group_id: Some(73059),
            has_controlling_terminal: true,
            terminal_name: Some("ttys009".to_string()),
            terminal_access_time_seconds: Some(1_000),
            terminal_activity_age_seconds: Some(30),
            ..process_with_argv(
                73059,
                55775,
                "Python",
                vec![
                    "/opt/homebrew/Cellar/python@3.11/3.11.9/Frameworks/Python.framework/Versions/3.11/Resources/Python.app/Contents/MacOS/Python",
                    "/Users/example/.hermes/hermes-agent/venv/bin/hermes",
                ],
            )
        });
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            approved: false,
            secondary: false,
        });
        let process_snapshot = snapshot_with_edges(
            vec![foreground.clone(), candidate],
            vec![(55742, 10), (55775, 55742), (73059, 55775)],
        );

        let detection = detect_ai_usage(&foreground, &process_snapshot, &config)
            .expect("expected hosted Hermes match through opaque login and shell edges");

        assert_eq!(detection.tool, "Hermes Agent");
        assert_eq!(detection.matched_pid, 73059);
    }

    #[test]
    fn match_scope_disambiguates_direct_desktop_from_hosted_cli() {
        let foreground = process_with_title(40, 1, "Claude.exe", "Claude");
        let direct_detection =
            detect_ai_usage(&foreground, &snapshot(vec![foreground.clone()]), &config())
                .expect("expected direct Claude Desktop match");

        assert_eq!(direct_detection.tool, "Claude");

        let host = process_with_title(41, 1, "cmd.exe", "claude");
        let child = with_read_activity(process(42, 41, "Claude.exe"));
        let hosted_detection =
            detect_ai_usage(&host, &snapshot(vec![host.clone(), child]), &config())
                .expect("expected hosted Claude Code match");

        assert_eq!(hosted_detection.tool, "Claude Code");
    }

    #[test]
    fn hosted_child_scope_does_not_match_foreground_root() {
        let foreground = process(50, 1, "claude.exe");
        let mut config = config();
        config
            .ai_process_names
            .retain(|process| process.match_scope == AiProcessMatchScope::HostedChild);

        assert!(
            detect_ai_usage(&foreground, &snapshot(vec![foreground.clone()]), &config).is_none()
        );
    }

    #[test]
    fn direct_scope_does_not_match_hosted_descendant() {
        let foreground = process_with_title(60, 1, "cmd.exe", "claude");
        let child = process(61, 60, "Claude.exe");
        let mut config = config();
        config
            .ai_process_names
            .retain(|process| process.match_scope == AiProcessMatchScope::Direct);

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), child]),
                &config
            )
            .is_none()
        );
    }

    #[test]
    fn non_terminal_host_process_matches_descendant_without_title_signal() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "agent monitor");
        let child = with_read_activity(process(11, 10, "claude.exe"));
        let mut config = config();
        config
            .host_process_names
            .push("WindowsTerminal.exe".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config,
        )
        .expect("expected hosted descendant to match without reading title semantics");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn terminal_foreground_group_matches_without_window_title() {
        let foreground = process(10, 1, "iTerm2");
        let child = terminal_process(11, 10, "claude", 11, 11);
        let mut config = config();
        config.host_process_names.push("iTerm2".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config,
        )
        .expect("expected title-less terminal process to match");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn terminal_foreground_group_matches_with_empty_window_title() {
        let foreground = process_with_title(10, 1, "iTerm2", "  ");
        let child = terminal_process(11, 10, "claude", 11, 11);
        let mut config = config();
        config.host_process_names.push("iTerm2".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config,
        )
        .expect("expected empty-title terminal process to match");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn terminal_foreground_group_matches_while_ignoring_descriptive_window_title() {
        let foreground = process_with_title(10, 1, "iTerm2", "✳ Claude Code");
        let child = terminal_process(11, 10, "claude", 11, 11);
        let mut config = config();
        config.host_process_names.push("iTerm2".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config,
        )
        .expect("expected recent terminal activity to match");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn terminal_foreground_group_matches_recent_activity_ignoring_process_like_title() {
        let foreground = process_with_title(10, 1, "iTerm2", "Python");
        let candidate = with_read_activity(ProcessInfo {
            process_group_id: Some(73059),
            terminal_foreground_process_group_id: Some(73059),
            has_controlling_terminal: true,
            terminal_name: Some("ttys009".to_string()),
            terminal_access_time_seconds: Some(1_000),
            terminal_activity_age_seconds: Some(30),
            ..process_with_argv(
                73059,
                10,
                "Python",
                vec![
                    "/opt/homebrew/Cellar/python@3.11/bin/Python",
                    "/Users/example/.hermes/hermes-agent/venv/bin/hermes",
                ],
            )
        });
        let mut config = config();
        config.host_process_names.push("iTerm2".to_string());
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            approved: false,
            secondary: false,
        });

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), candidate]),
            &config,
        )
        .expect("expected recent Hermes terminal activity to match");

        assert_eq!(detection.tool, "Hermes Agent");
        assert_eq!(detection.matched_pid, 73059);
    }

    #[test]
    fn terminal_foreground_group_matches_recent_activity_ignoring_unrelated_title() {
        let foreground = process_with_title(10, 1, "Terminal", "release — sudo — monitor");
        let active_ai = terminal_process(11, 10, "claude", 11, 11);
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), active_ai]),
            &config,
        )
        .expect("expected recent terminal activity to match while ignoring title");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[cfg(not(windows))]
    #[test]
    fn terminal_foreground_group_matches_candidate_when_only_cpu_counters_advance() {
        let foreground = process_with_title(10, 1, "Terminal", "Claude Code");
        let cpu_only_ai = with_cpu_activity_only(terminal_process(11, 10, "claude", 11, 11));
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), cpu_only_ai]),
            &config,
        )
        .expect("expected terminal foreground group match without read/write activity");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn hosted_child_rejects_candidate_without_read_write_activity() {
        let foreground = process_with_title(10, 1, "Terminal", "Claude Code");
        let unchanged_ai = process(11, 10, "claude");
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), unchanged_ai]),
                &config
            )
            .is_none()
        );
    }

    #[test]
    fn non_terminal_host_rejects_candidate_without_read_write_activity() {
        let foreground =
            process_with_title(10, 1, "Code.exe", "datadog-agent - Visual Studio Code");
        let unchanged_ai = process(11, 10, "claude.exe");

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), unchanged_ai]),
                &config()
            )
            .is_none()
        );
    }

    #[cfg(not(windows))]
    #[test]
    fn terminal_foreground_group_ignores_inactive_tab_descendants() {
        let foreground = process(10, 1, "iTerm2");
        let active_shell = terminal_process(11, 10, "zsh", 11, 11);
        let inactive_ai = terminal_process(12, 10, "claude", 12, 11);
        let mut config = config();
        config.host_process_names.push("iTerm2".to_string());

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), active_shell, inactive_ai]),
                &config
            )
            .is_none()
        );
    }

    #[test]
    fn host_window_title_does_not_create_detection_without_descendant_ai_candidate() {
        let foreground = process_with_title(10, 1, "iTerm2", "✳ Claude Code");
        let active_runtime = terminal_process(11, 10, "node", 11, 11);
        let mut config = config();
        config.host_process_names.push("iTerm2".to_string());

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), active_runtime]),
                &config
            )
            .is_none()
        );
    }

    #[test]
    fn terminal_window_title_does_not_create_detection_when_no_descendant_matches() {
        let foreground =
            process_with_title(10, 1, "Terminal", "agent — ✳ Claude Code — claude — 184×44");
        let shell = process(11, 10, "zsh");
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), shell]),
                &config
            )
            .is_none()
        );
    }

    #[test]
    fn direct_match_takes_precedence_over_host_descendant_match() {
        let foreground = process(20, 1, "Cursor.exe");
        let child = process(21, 20, "claude.exe");
        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config(),
        )
        .expect("expected direct Cursor match");

        assert_eq!(detection.tool, "Cursor");
        assert_eq!(detection.matched_pid, 20);
    }

    #[cfg(windows)]
    #[test]
    fn windows_title_match_detects_hosted_candidate_before_activity_fallback() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "Claude Code");
        let inactive_title_match =
            with_attached_console_title(process(12, 30, "claude.exe"), "Claude Code");
        let active_fallback_candidate = with_read_activity(process(13, 31, "hermes.exe"));
        let mut config = config();
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes.exe".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            approved: false,
            secondary: false,
        });

        let detections = detect_ai_usages(
            &foreground,
            &snapshot(vec![
                foreground.clone(),
                inactive_title_match,
                active_fallback_candidate,
            ]),
            &config,
        );

        assert_eq!(detections.len(), 1);
        assert_eq!(detections[0].tool, "Claude Code");
        assert_eq!(detections[0].matched_pid, 12);
    }

    #[cfg(windows)]
    #[test]
    fn windows_title_match_ignores_different_console_title() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "Claude Code");
        let inactive_title_mismatch =
            with_attached_console_title(process(12, 30, "claude.exe"), "Other Tab");

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), inactive_title_mismatch]),
                &config()
            )
            .is_none()
        );
    }

    #[cfg(windows)]
    #[test]
    fn windows_title_match_ignores_trailing_foreground_padding() {
        let foreground = process_with_title(10, 1, "mintty.exe", "Claude Code\u{00a0}\u{00a0}");
        let title_match = with_attached_console_title(process(12, 30, "claude.exe"), "Claude Code");

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), title_match]),
            &config(),
        )
        .expect("expected title match with trailing padding ignored");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 12);
    }

    #[cfg(windows)]
    #[test]
    fn windows_title_match_coalesces_candidates_by_tool() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "Claude Code");
        let first_match = with_attached_console_title(process(12, 30, "claude.exe"), "Claude Code");
        let second_match =
            with_attached_console_title(process(13, 31, "claude.exe"), "Claude Code");

        let detections = detect_ai_usages(
            &foreground,
            &snapshot(vec![foreground.clone(), first_match, second_match]),
            &config(),
        );

        assert_eq!(detections.len(), 1);
        assert_eq!(detections[0].tool, "Claude Code");
        assert_eq!(detections[0].matched_pid, 12);
    }

    #[cfg(windows)]
    #[test]
    fn windows_fallback_matches_active_hosted_candidate_outside_foreground_ancestry() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "work");
        let unrelated_shell = process(11, 10, "cmd.exe");
        let active_ai = with_read_activity(process(12, 30, "claude.exe"));

        let detections = detect_ai_usages(
            &foreground,
            &snapshot(vec![foreground.clone(), unrelated_shell, active_ai]),
            &config(),
        );

        assert_eq!(detections.len(), 1);
        assert_eq!(detections[0].tool, "Claude Code");
        assert_eq!(detections[0].matched_pid, 12);
    }

    #[cfg(windows)]
    #[test]
    fn windows_fallback_emits_multiple_active_hosted_candidates() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "work");
        let active_cursor = with_read_activity(process(11, 29, "Cursor.exe"));
        let active_claude = with_read_activity(process(12, 30, "claude.exe"));
        let active_codex = with_write_activity(process(13, 31, "codex.exe"));
        let mut config = config();
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["codex.exe".to_string()],
            tool: "Codex".to_string(),
            provider: "OpenAI".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            approved: false,
            secondary: false,
        });

        let detections = detect_ai_usages(
            &foreground,
            &snapshot(vec![
                foreground.clone(),
                active_cursor,
                active_claude,
                active_codex,
            ]),
            &config,
        );

        assert_eq!(detections.len(), 2);
        assert_eq!(detections[0].tool, "Claude Code");
        assert_eq!(detections[0].matched_pid, 12);
        assert_eq!(detections[1].tool, "Codex");
        assert_eq!(detections[1].matched_pid, 13);
    }

    #[cfg(windows)]
    #[test]
    fn windows_fallback_coalesces_active_candidates_by_tool() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "work");
        let active_hermes_one = with_read_activity(process(11, 29, "hermes.exe"));
        let active_hermes_two = with_write_activity(process(12, 30, "hermes.exe"));
        let active_claude = with_read_activity(process(13, 31, "claude.exe"));
        let mut config = config();
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes.exe".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            approved: false,
            secondary: false,
        });

        let detections = detect_ai_usages(
            &foreground,
            &snapshot(vec![
                foreground.clone(),
                active_hermes_one,
                active_hermes_two,
                active_claude,
            ]),
            &config,
        );

        assert_eq!(detections.len(), 2);
        assert_eq!(detections[0].tool, "Hermes Agent");
        assert_eq!(detections[0].matched_pid, 11);
        assert_eq!(detections[1].tool, "Claude Code");
        assert_eq!(detections[1].matched_pid, 13);
    }

    #[cfg(windows)]
    #[test]
    fn windows_fallback_ignores_both_scope_desktop_app_candidates() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "work");
        let active_cursor = with_read_activity(process(11, 29, "Cursor.exe"));

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), active_cursor]),
                &config()
            )
            .is_none()
        );
    }

    #[cfg(windows)]
    #[test]
    fn windows_fallback_ignores_inactive_hosted_candidate() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "work");
        let inactive_ai = process(12, 30, "claude.exe");

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), inactive_ai]),
                &config()
            )
            .is_none()
        );
    }

    #[cfg(not(windows))]
    #[test]
    fn non_windows_rejects_active_hosted_candidate_outside_foreground_ancestry() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "work");
        let active_ai = with_read_activity(process(12, 30, "claude.exe"));

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), active_ai]),
                &config()
            )
            .is_none()
        );
    }

    #[test]
    fn unrelated_foreground_process_does_not_match() {
        let foreground = process(30, 1, "notepad.exe");

        assert!(
            detect_ai_usage(&foreground, &snapshot(vec![foreground.clone()]), &config()).is_none()
        );
    }
}
