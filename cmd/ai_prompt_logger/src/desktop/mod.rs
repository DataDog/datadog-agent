mod logger;
pub mod matcher;

use std::collections::{HashMap, HashSet};
use std::thread;
use std::time::Duration;

use anyhow::Result;

use crate::datadog::{AiUsageEvent, DatadogClient, DesktopMonitoringConfig, resolve_hostname};
use crate::desktop::logger::DesktopLogger;
use crate::desktop::matcher::{
    ProcessEdge, ProcessInfo, ProcessSnapshot, detect_ai_usage, find_hosted_ai_process,
    matches_process_name,
};

#[cfg(windows)]
mod windows;

#[cfg(target_os = "macos")]
mod macos;

#[cfg(target_os = "macos")]
use macos::MacosDesktopDetector as PlatformDesktopDetector;
#[cfg(windows)]
use windows::WindowsDesktopDetector as PlatformDesktopDetector;

trait DesktopDetector {
    /// Return the current foreground desktop process, including any available window title.
    fn foreground_process(&self) -> Result<Option<ProcessInfo>>;

    /// Return enriched processes plus PID edges rooted around the foreground process.
    fn process_snapshot(&self, foreground_pid: u32) -> Result<ProcessSnapshot>;

    /// Wait for the local Agent service dependency before starting the monitor loop.
    fn wait_for_agent_service(&self) -> bool {
        true
    }

    /// Report whether the local Agent service dependency is still running.
    fn agent_service_running(&self) -> bool {
        true
    }
}

/// Write an early desktop-monitor warning before the normal monitor logger exists.
pub(crate) fn log_startup_warning(message: impl AsRef<str>) {
    DesktopLogger::new(1).warn(message);
}

/// Run the desktop monitor polling loop and submit detected AI usage events.
pub fn run(dd_client: &DatadogClient, config: DesktopMonitoringConfig) -> Result<()> {
    let logger = DesktopLogger::new(config.debug);
    if !config.enabled {
        logger.info("desktop monitoring disabled by config");
        return Ok(());
    }

    #[cfg(any(windows, target_os = "macos"))]
    {
        let detector = PlatformDesktopDetector;
        if !detector.wait_for_agent_service() {
            logger.info("desktop monitor exiting because agent service is not running");
            return Ok(());
        }
        logger.info(format!(
            "desktop monitor started poll_interval_seconds={}",
            config.poll_interval_seconds
        ));
        loop {
            if !detector.agent_service_running() {
                logger.info("desktop monitor exiting because agent service stopped");
                return Ok(());
            }
            poll_once(dd_client, &detector, &config, &logger);
            thread::sleep(Duration::from_secs(config.poll_interval_seconds.max(1)));
        }
    }

    #[cfg(not(any(windows, target_os = "macos")))]
    {
        anyhow::bail!("desktop monitoring is not implemented on this platform yet");
    }
}

/// Inspect the current foreground process once and send an event when AI usage is detected.
fn poll_once(
    dd_client: &DatadogClient,
    detector: &impl DesktopDetector,
    config: &DesktopMonitoringConfig,
    logger: &DesktopLogger,
) {
    let foreground = match detector.foreground_process() {
        Ok(Some(process)) => process,
        Ok(None) => {
            logger.info("desktop scan foreground_process=\"none\" foreground_pid=0 foreground_window_title=\"\"");
            return;
        }
        Err(err) => {
            logger.warn(format!(
                "failed to inspect foreground process error=\"{err}\""
            ));
            return;
        }
    };

    logger.info(format!(
        "desktop scan foreground_process=\"{}\" foreground_pid={} foreground_window_title=\"{}\"",
        foreground.exe_name,
        foreground.pid,
        foreground.window_title.as_deref().unwrap_or("")
    ));

    let snapshot = match detector.process_snapshot(foreground.pid) {
        Ok(snapshot) => snapshot,
        Err(err) => {
            logger.warn(format!(
                "failed to inspect process snapshot error=\"{err}\""
            ));
            return;
        }
    };

    log_process_tree_diagnostics(&foreground, &snapshot, config, logger);

    let Some(detection) = detect_ai_usage(&foreground, &snapshot, config) else {
        return;
    };

    logger.info(format!(
        "detected AI usage tool=\"{}\" provider=\"{}\" matched_process=\"{}\" matched_pid={} foreground_process=\"{}\" foreground_pid={}",
        detection.tool,
        detection.provider,
        detection.matched_process_name,
        detection.matched_pid,
        detection.foreground_process_name,
        detection.foreground_pid
    ));

    let mut event = AiUsageEvent::new_with_source(
        "observed",
        "desktop_app",
        detection.tool,
        resolve_user_id(),
        resolve_hostname(),
        detection.approved,
    );
    event.provider = Some(detection.provider);

    if dd_client.send_event(&event) {
        logger.info(format!("sent AI usage event tool=\"{}\"", event.tool));
    } else {
        logger.warn(format!(
            "failed to send AI usage event tool=\"{}\"",
            event.tool
        ));
    }
}

/// Resolve the user identifier attached to observed desktop usage events.
fn resolve_user_id() -> String {
    std::env::var("USERNAME")
        .or_else(|_| std::env::var("USER"))
        .ok()
        .filter(|user| !user.is_empty())
        .unwrap_or_else(|| "unknown".to_string())
}

/// Emit debug details about hosted AI candidates and their foreground process ancestry.
fn log_process_tree_diagnostics(
    foreground: &ProcessInfo,
    snapshot: &ProcessSnapshot,
    config: &DesktopMonitoringConfig,
    logger: &DesktopLogger,
) {
    if !matches_process_name(foreground, &config.host_process_names) {
        return;
    }

    let context = ProcessTreeDiagnosticContext::new(snapshot, foreground.pid);
    let candidates = hosted_ai_candidates(snapshot, config);
    log_process_tree_summary(foreground, snapshot, &context, candidates.len(), logger);
    log_ai_candidate_diagnostics(&candidates, &context, logger);
}

struct ProcessTreeDiagnosticContext<'a> {
    process_by_pid: HashMap<u32, &'a ProcessInfo>,
    parent_by_pid: HashMap<u32, u32>,
    descendant_pids: HashSet<u32>,
    enriched_descendant_count: usize,
    opaque_descendant_count: usize,
}

impl<'a> ProcessTreeDiagnosticContext<'a> {
    /// Build reusable lookup tables and descendant counts for process-tree diagnostics.
    fn new(snapshot: &'a ProcessSnapshot, foreground_pid: u32) -> Self {
        let process_by_pid: HashMap<u32, &ProcessInfo> = snapshot
            .processes
            .iter()
            .map(|process| (process.pid, process))
            .collect();
        let parent_by_pid: HashMap<u32, u32> = snapshot
            .edges
            .iter()
            .map(|edge| (edge.pid, edge.parent_pid))
            .collect();
        let children_by_parent = children_by_parent(&snapshot.edges);
        let descendant_pids = descendant_pids_of(foreground_pid, &children_by_parent);
        let enriched_descendant_count = descendant_pids
            .iter()
            .filter(|pid| process_by_pid.contains_key(pid))
            .count();
        let opaque_descendant_count = descendant_pids
            .len()
            .saturating_sub(enriched_descendant_count);

        Self {
            process_by_pid,
            parent_by_pid,
            descendant_pids,
            enriched_descendant_count,
            opaque_descendant_count,
        }
    }
}

/// Return hosted AI candidates with their matched tool names.
fn hosted_ai_candidates<'a>(
    snapshot: &'a ProcessSnapshot,
    config: &'a DesktopMonitoringConfig,
) -> Vec<(&'a ProcessInfo, &'a str)> {
    snapshot
        .processes
        .iter()
        .filter_map(|process| {
            find_hosted_ai_process(process, &config.ai_process_names)
                .map(|tool| (process, tool.tool.as_str()))
        })
        .collect()
}

/// Log one compact process-tree summary before per-candidate details.
fn log_process_tree_summary(
    foreground: &ProcessInfo,
    snapshot: &ProcessSnapshot,
    context: &ProcessTreeDiagnosticContext,
    ai_candidate_count: usize,
    logger: &DesktopLogger,
) {
    logger.info_at(1, format!(
        "process_tree summary foreground=\"{}\" pid={} processes={} edges={} descendants={} enriched={} opaque={} ai_candidates={}",
        foreground.exe_name,
        foreground.pid,
        snapshot.processes.len(),
        snapshot.edges.len(),
        context.descendant_pids.len(),
        context.enriched_descendant_count,
        context.opaque_descendant_count,
        ai_candidate_count
    ));

    if context.enriched_descendant_count == 0 && !context.descendant_pids.is_empty() {
        logger.info_at(
            2,
            format!(
                "process_tree warning all_descendants_opaque=true foreground_pid={} descendants={}",
                foreground.pid,
                context.descendant_pids.len(),
            ),
        );
    }
}

/// Log AI candidates as readable identity, metadata, and ancestry lines.
fn log_ai_candidate_diagnostics(
    candidates: &[(&ProcessInfo, &str)],
    context: &ProcessTreeDiagnosticContext,
    logger: &DesktopLogger,
) {
    let total = candidates.len();
    for (index, (process, tool)) in candidates.iter().take(50).enumerate() {
        let label = format!("{}/{}", index + 1, total);
        let is_descendant = context.descendant_pids.contains(&process.pid);
        logger.info_at(2, format!(
            "process_tree candidate[{label}] tool=\"{}\" descendant={} pid={} ppid={} exe=\"{}\" path=\"{}\"",
            tool,
            is_descendant,
            process.pid,
            process.parent_pid,
            process.exe_name,
            process.exe_path.as_deref().unwrap_or("")
        ));
        logger.info_at(2, format!(
            "process_tree candidate[{label}] names bsd_name=\"{}\" bsd_comm=\"{}\" argv0=\"{}\" terminal=\"pgid={} tpgid={} has_ctty={}\"",
            process.bsd_name.as_deref().unwrap_or(""),
            process.bsd_comm.as_deref().unwrap_or(""),
            process.argv0.as_deref().unwrap_or(""),
            format_optional_u32(process.process_group_id),
            format_optional_u32(process.terminal_foreground_process_group_id),
            process.has_controlling_terminal
        ));
        logger.info_at(
            2,
            format!(
                "process_tree candidate[{label}] ancestry=\"{}\"",
                format_ancestry(process, &context.process_by_pid, &context.parent_by_pid)
            ),
        );
    }

    if total > 50 {
        logger.info_at(
            2,
            format!("process_tree candidates_truncated displayed=50 total={total}"),
        );
    }
}

/// Format ancestry through enriched and opaque PID graph nodes.
fn format_ancestry(
    process: &ProcessInfo,
    process_by_pid: &HashMap<u32, &ProcessInfo>,
    parent_by_pid: &HashMap<u32, u32>,
) -> String {
    let mut seen = Vec::new();
    let mut current_pid = Some(process.pid);
    while let Some(pid) = current_pid {
        if seen.len() >= 16 {
            seen.push("...".to_string());
            break;
        }
        if let Some(process) = process_by_pid.get(&pid) {
            seen.push(format!("{}:{}", process.pid, process.exe_name));
            current_pid = parent_by_pid.get(&process.pid).copied().or_else(|| {
                (process.parent_pid != 0 && process.parent_pid != process.pid)
                    .then_some(process.parent_pid)
            });
        } else {
            seen.push(format!("{pid}:opaque"));
            current_pid = parent_by_pid.get(&pid).copied();
        }
    }
    seen.join(" <- ")
}

/// Render optional process IDs in log-friendly form.
fn format_optional_u32(value: Option<u32>) -> String {
    value
        .map(|value| value.to_string())
        .unwrap_or_else(|| "none".to_string())
}

/// Index PID edges by parent PID for descendant traversal.
fn children_by_parent(edges: &[ProcessEdge]) -> HashMap<u32, Vec<u32>> {
    let mut children: HashMap<u32, Vec<u32>> = HashMap::new();
    for edge in edges {
        children.entry(edge.parent_pid).or_default().push(edge.pid);
    }
    children
}

/// Return all descendant PIDs reachable from a root PID in the edge graph.
fn descendant_pids_of(
    root_pid: u32,
    children_by_parent: &HashMap<u32, Vec<u32>>,
) -> std::collections::HashSet<u32> {
    let mut visited = std::collections::HashSet::new();
    let mut stack = children_by_parent
        .get(&root_pid)
        .cloned()
        .unwrap_or_default();

    while let Some(pid) = stack.pop() {
        if !visited.insert(pid) {
            continue;
        }
        if let Some(children) = children_by_parent.get(&pid) {
            stack.extend(children);
        }
    }

    visited
}
