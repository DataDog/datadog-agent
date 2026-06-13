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
    pub process_group_id: Option<u32>,
    pub terminal_foreground_process_group_id: Option<u32>,
    pub has_controlling_terminal: bool,
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

/// Detect configured AI usage from the foreground process and process snapshot.
pub fn detect_ai_usage(
    foreground: &ProcessInfo,
    snapshot: &ProcessSnapshot,
    config: &DesktopMonitoringConfig,
) -> Option<AiUsageDetection> {
    let foreground_is_host = matches_process_name(foreground, &config.host_process_names);
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
    let has_terminal_foreground_signal = descendants
        .iter()
        .any(|process| process.terminal_foreground_process_group_id.is_some());

    if let Some(tool) = find_ai_process_for_location(
        foreground,
        MatchLocation::ForegroundRoot,
        &config.ai_process_names,
    ) {
        return Some(detection_from_match(foreground, foreground, tool));
    }

    let ai_candidates = ai_process_candidates(
        &snapshot.processes,
        MatchLocation::HostedDescendant,
        &config.ai_process_names,
    );
    for process in descendants {
        if has_terminal_foreground_signal && !is_in_terminal_foreground_group(process) {
            continue;
        }
        if let Some(tool) = ai_candidates.get(&process.pid) {
            if foreground_has_title(foreground)
                && !foreground_title_matches_non_empty_tool_hint(foreground, tool)
            {
                continue;
            }
            return Some(detection_from_match(foreground, process, tool));
        }
    }

    if foreground_is_host && foreground_has_title(foreground) {
        for process in &snapshot.processes {
            if !is_in_terminal_foreground_group(process) {
                continue;
            }
            if let Some(tool) = ai_candidates.get(&process.pid) {
                if foreground_title_matches_non_empty_tool_hint(foreground, tool) {
                    return Some(detection_from_match(foreground, process, tool));
                }
            }
        }
    }

    None
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

/// Check whether the foreground window title contains a configured non-empty tool hint.
fn foreground_title_matches_non_empty_tool_hint(
    foreground: &ProcessInfo,
    tool: &AiProcessConfig,
) -> bool {
    let Some(title) = foreground.window_title.as_deref() else {
        return false;
    };
    let title = title.to_ascii_lowercase();
    tool.window_title_hints
        .iter()
        .filter(|hint| !hint.trim().is_empty())
        .map(|hint| hint.to_ascii_lowercase())
        .any(|hint| title.contains(&hint))
}

/// Return whether foreground window metadata contains a non-empty title.
fn foreground_has_title(foreground: &ProcessInfo) -> bool {
    foreground
        .window_title
        .as_deref()
        .is_some_and(|title| !title.trim().is_empty())
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
fn children_by_parent(edges: &[ProcessEdge]) -> HashMap<u32, Vec<u32>> {
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
            ai_process_names: vec![
                AiProcessConfig {
                    process_names: vec!["Cursor.exe".to_string()],
                    tool: "Cursor".to_string(),
                    provider: "Anysphere".to_string(),
                    match_scope: AiProcessMatchScope::Both,
                    window_title_hints: vec!["cursor".to_string()],
                    approved: false,
                    secondary: false,
                },
                AiProcessConfig {
                    process_names: vec!["Claude.exe".to_string()],
                    tool: "Claude".to_string(),
                    provider: "Anthropic".to_string(),
                    match_scope: AiProcessMatchScope::Direct,
                    window_title_hints: vec!["claude".to_string()],
                    approved: false,
                    secondary: false,
                },
                AiProcessConfig {
                    process_names: vec!["claude.exe".to_string()],
                    tool: "Claude Code".to_string(),
                    provider: "Anthropic".to_string(),
                    match_scope: AiProcessMatchScope::HostedChild,
                    window_title_hints: vec!["claude".to_string()],
                    approved: true,
                    secondary: false,
                },
            ],
            host_process_names: vec!["cmd.exe".to_string(), "Code.exe".to_string()],
        }
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
            process_group_id: None,
            terminal_foreground_process_group_id: None,
            has_controlling_terminal: false,
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
            process_group_id: None,
            terminal_foreground_process_group_id: None,
            has_controlling_terminal: false,
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
            process_group_id: Some(process_group_id),
            terminal_foreground_process_group_id: Some(terminal_foreground_process_group_id),
            has_controlling_terminal: true,
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
            process_group_id: None,
            terminal_foreground_process_group_id: None,
            has_controlling_terminal: false,
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
            process_group_id: None,
            terminal_foreground_process_group_id: None,
            has_controlling_terminal: false,
        }
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
        let grandchild = process(12, 11, "Claude.exe");
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
    fn host_process_matches_descendant_by_argv0() {
        let foreground = process_with_title(10, 1, "Terminal", "claude");
        let child = process_with_identity(11, 10, "2.1.172", None, None, Some("claude"));
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
        let child = process_with_identity(11, 10, "2.1.172", Some("claude"), None, None);
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
        let child = process_with_identity(11, 10, "2.1.172", None, Some("claude"), None);
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
        let child = process_with_argv(
            11,
            10,
            "Python",
            vec![
                "/opt/homebrew/Cellar/python@3.11/3.11.9/Frameworks/Python.framework/Versions/3.11/Resources/Python.app/Contents/MacOS/Python",
                "/Users/example/.hermes/hermes-agent/venv/bin/hermes",
            ],
        );
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            window_title_hints: vec!["hermes".to_string()],
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
    fn host_process_falls_back_to_title_confirmed_terminal_foreground_candidate() {
        let foreground = process_with_title(10, 1, "Terminal", "work - hermes");
        let candidate = ProcessInfo {
            process_group_id: Some(73059),
            terminal_foreground_process_group_id: Some(73059),
            has_controlling_terminal: true,
            ..process_with_argv(
                73059,
                55775,
                "Python",
                vec![
                    "/opt/homebrew/Cellar/python@3.11/3.11.9/Frameworks/Python.framework/Versions/3.11/Resources/Python.app/Contents/MacOS/Python",
                    "/Users/example/.hermes/hermes-agent/venv/bin/hermes",
                ],
            )
        };
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            window_title_hints: vec!["hermes".to_string()],
            approved: false,
            secondary: false,
        });

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), candidate]),
            &config,
        )
        .expect("expected title-confirmed Hermes fallback match");

        assert_eq!(detection.tool, "Hermes Agent");
        assert_eq!(detection.matched_pid, 73059);
    }

    #[test]
    fn host_process_title_fallback_rejects_unrelated_terminal_foreground_candidate() {
        let foreground = process_with_title(10, 1, "Terminal", "work - vim");
        let candidate = ProcessInfo {
            process_group_id: Some(73059),
            terminal_foreground_process_group_id: Some(73059),
            has_controlling_terminal: true,
            ..process_with_argv(
                73059,
                55775,
                "Python",
                vec![
                    "/opt/homebrew/Cellar/python@3.11/3.11.9/Frameworks/Python.framework/Versions/3.11/Resources/Python.app/Contents/MacOS/Python",
                    "/Users/example/.hermes/hermes-agent/venv/bin/hermes",
                ],
            )
        };
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            window_title_hints: vec!["hermes".to_string()],
            approved: false,
            secondary: false,
        });

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), candidate]),
                &config
            )
            .is_none()
        );
    }

    #[test]
    fn host_process_matches_candidate_through_opaque_process_edges() {
        let foreground = process_with_title(10, 1, "Terminal", "work - hermes");
        let candidate = ProcessInfo {
            process_group_id: Some(73059),
            terminal_foreground_process_group_id: Some(73059),
            has_controlling_terminal: true,
            ..process_with_argv(
                73059,
                55775,
                "Python",
                vec![
                    "/opt/homebrew/Cellar/python@3.11/3.11.9/Frameworks/Python.framework/Versions/3.11/Resources/Python.app/Contents/MacOS/Python",
                    "/Users/example/.hermes/hermes-agent/venv/bin/hermes",
                ],
            )
        };
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());
        config.ai_process_names.push(AiProcessConfig {
            process_names: vec!["hermes".to_string()],
            tool: "Hermes Agent".to_string(),
            provider: "Nous Research".to_string(),
            match_scope: AiProcessMatchScope::HostedChild,
            window_title_hints: vec!["hermes".to_string()],
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
        let child = process(42, 41, "Claude.exe");
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
    fn host_process_requires_matching_window_title_hint() {
        let foreground = process_with_title(10, 1, "WindowsTerminal.exe", "agent monitor");
        let child = process(11, 10, "claude.exe");
        let mut config = config();
        config
            .host_process_names
            .push("WindowsTerminal.exe".to_string());

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
    fn terminal_foreground_group_matches_with_window_title_hint() {
        let foreground = process_with_title(10, 1, "iTerm2", "✳ Claude Code");
        let child = terminal_process(11, 10, "claude", 11, 11);
        let mut config = config();
        config.host_process_names.push("iTerm2".to_string());

        let detection = detect_ai_usage(
            &foreground,
            &snapshot(vec![foreground.clone(), child]),
            &config,
        )
        .expect("expected title-confirmed terminal process to match");

        assert_eq!(detection.tool, "Claude Code");
        assert_eq!(detection.matched_pid, 11);
    }

    #[test]
    fn terminal_foreground_group_rejects_candidate_when_title_is_unrelated() {
        let foreground = process_with_title(10, 1, "Terminal", "release — sudo — monitor");
        let inactive_ai = terminal_process(11, 10, "claude", 11, 11);
        let mut config = config();
        config.host_process_names.push("Terminal".to_string());

        assert!(
            detect_ai_usage(
                &foreground,
                &snapshot(vec![foreground.clone(), inactive_ai]),
                &config
            )
            .is_none()
        );
    }

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
    fn host_window_title_does_not_match_without_descendant_ai_candidate() {
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
    fn terminal_window_title_does_not_match_when_no_descendant_matches() {
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

    #[test]
    fn unrelated_foreground_process_does_not_match() {
        let foreground = process(30, 1, "notepad.exe");

        assert!(
            detect_ai_usage(&foreground, &snapshot(vec![foreground.clone()]), &config()).is_none()
        );
    }
}
