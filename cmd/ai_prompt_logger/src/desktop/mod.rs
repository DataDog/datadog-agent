pub(crate) mod config;
mod logger;
pub mod matcher;

use std::collections::{HashMap, HashSet};
use std::thread;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use anyhow::Result;

use crate::datadog::{
    AiProcessConfig, AiUsageEvent, DatadogClient, DesktopMonitoringConfig, resolve_hostname,
};
use crate::desktop::logger::DesktopLogger;
use crate::desktop::matcher::{
    ProcessActivity, ProcessActivityDelta, ProcessInfo, ProcessSnapshot, children_by_parent,
    detect_ai_usages, find_hosted_ai_process, matches_process_name,
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

    /// Return true to keep the monitor process alive while the Agent service is stopped.
    ///
    /// The default preserves the existing behavior: exit when the service stops.
    fn should_idle_when_agent_service_stopped(&self) -> bool {
        false
    }
}

/// Write an early desktop-monitor warning before the normal monitor logger exists.
pub(crate) fn log_startup_warning(message: impl AsRef<str>) {
    DesktopLogger::new(1).warn(message);
}

/// Run the desktop monitor polling loop and submit detected AI usage events.
pub fn run(dd_client: &DatadogClient, mut config: DesktopMonitoringConfig) -> Result<()> {
    let mut logger = DesktopLogger::new(config.debug);
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
        let mut process_activity_tracker = ProcessActivityTracker::default();
        let mut idle_for_agent_service = false;
        loop {
            reload_config(&mut config, &mut logger);
            if !config.enabled {
                logger.info_at(1, "desktop monitoring disabled by config");
                thread::sleep(Duration::from_secs(config.poll_interval_seconds.max(1)));
                continue;
            }
            if !detector.agent_service_running() {
                if !detector.should_idle_when_agent_service_stopped() {
                    logger.info("desktop monitor exiting because agent service stopped");
                    return Ok(());
                }
                if !idle_for_agent_service {
                    logger.info("desktop monitor idling because agent service stopped");
                    process_activity_tracker = ProcessActivityTracker::default();
                    idle_for_agent_service = true;
                }
                thread::sleep(Duration::from_secs(config.poll_interval_seconds.max(1)));
                continue;
            }
            if idle_for_agent_service {
                logger.info("desktop monitor resumed because agent service is running");
                idle_for_agent_service = false;
            }
            poll_once(
                dd_client,
                &detector,
                &config,
                &logger,
                &mut process_activity_tracker,
            );
            thread::sleep(Duration::from_secs(config.poll_interval_seconds.max(1)));
        }
    }

    #[cfg(not(any(windows, target_os = "macos")))]
    {
        anyhow::bail!("desktop monitoring is not implemented on this platform yet");
    }
}

fn reload_config(config: &mut DesktopMonitoringConfig, logger: &mut DesktopLogger) {
    let next_config = config::reload_desktop_monitoring_config();
    let debug_changed = next_config.debug != config.debug;
    let poll_interval_changed = next_config.poll_interval_seconds != config.poll_interval_seconds;
    *config = next_config;
    if debug_changed {
        *logger = DesktopLogger::new(config.debug);
    }
    if debug_changed || poll_interval_changed {
        logger.info(format!(
            "desktop monitor config reloaded debug={} poll_interval_seconds={}",
            config.debug, config.poll_interval_seconds
        ));
    }
}

/// Inspect the current foreground process once and send an event when AI usage is detected.
fn poll_once(
    dd_client: &DatadogClient,
    detector: &impl DesktopDetector,
    config: &DesktopMonitoringConfig,
    logger: &DesktopLogger,
    process_activity_tracker: &mut ProcessActivityTracker,
) {
    let foreground = match detector.foreground_process() {
        Ok(Some(process)) => process,
        Ok(None) => {
            logger.info("desktop scan foreground_process=<none> foreground_pid=0 foreground_window_title=<>");
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
        "desktop scan foreground_process=<{}> foreground_pid={} foreground_window_title=<{}>",
        foreground.exe_name,
        foreground.pid,
        foreground.window_title.as_deref().unwrap_or("")
    ));

    let mut snapshot = match detector.process_snapshot(foreground.pid) {
        Ok(snapshot) => snapshot,
        Err(err) => {
            logger.warn(format!(
                "failed to inspect process snapshot error=\"{err}\""
            ));
            return;
        }
    };
    process_activity_tracker.mark_observed_activity(&mut snapshot, config);
    #[cfg(windows)]
    windows::enrich_attached_console_titles(&foreground, &mut snapshot, config);

    log_process_tree_diagnostics(&foreground, &snapshot, config, logger);

    let detections = detect_ai_usages(&foreground, &snapshot, config);
    if detections.is_empty() {
        return;
    }

    let user_id = resolve_user_id();
    let hostname = resolve_hostname();
    for detection in detections {
        logger.info(format!(
            "detected AI usage tool=<{}> provider=<{}> matched_process=<{}> matched_pid={} foreground_process=<{}> foreground_pid={}",
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
            user_id.clone(),
            hostname.clone(),
            detection.approved,
        );
        event.provider = Some(detection.provider);

        if dd_client.send_event(&event) {
            logger.info(format!("sent AI usage event tool=<{}>", event.tool));
        } else {
            logger.warn(format!(
                "failed to send AI usage event tool=<{}>",
                event.tool
            ));
        }
    }
}

#[derive(Debug, Clone, Copy, Hash, PartialEq, Eq)]
struct ProcessActivityKey {
    pid: u32,
    process_start_key: u64,
}

#[derive(Default)]
struct ProcessActivityTracker {
    activity_by_process: HashMap<ProcessActivityKey, ObservedProcessActivity>,
}

#[derive(Clone)]
struct ObservedProcessActivity {
    activity: ProcessActivity,
    observed_at_seconds: u64,
}

impl ProcessActivityTracker {
    /// Mark processes whose read/write counters advanced since the previous poll.
    fn mark_observed_activity(
        &mut self,
        snapshot: &mut ProcessSnapshot,
        config: &DesktopMonitoringConfig,
    ) {
        let Some(now) = current_unix_seconds() else {
            return;
        };
        let mut latest_activity = HashMap::new();
        for process in &mut snapshot.processes {
            process.process_activity_delta = None;
            process.process_read_write_activity_observed = false;
            let Some(activity) = process.process_activity.as_ref() else {
                continue;
            };
            let key = ProcessActivityKey {
                pid: process.pid,
                process_start_key: activity.process_start_key,
            };

            if let Some(previous) = self.activity_by_process.get(&key) {
                let delta = process_activity_delta(&previous.activity, activity);
                process.process_read_write_activity_observed = read_write_activity_observed(&delta);
                process.process_activity_delta = Some(delta);
            }
            latest_activity.insert(
                key,
                ObservedProcessActivity {
                    activity: activity.clone(),
                    observed_at_seconds: now,
                },
            );
        }

        self.activity_by_process.extend(latest_activity);
        self.activity_by_process.retain(|_, observed| {
            now.saturating_sub(observed.observed_at_seconds)
                <= config.process_activity_window_seconds
        });
    }
}

fn current_unix_seconds() -> Option<u64> {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .map(|duration| duration.as_secs())
}

fn process_activity_delta(
    previous: &ProcessActivity,
    current: &ProcessActivity,
) -> ProcessActivityDelta {
    ProcessActivityDelta {
        read_operation_count: counter_delta(
            previous.read_operation_count,
            current.read_operation_count,
        ),
        write_operation_count: counter_delta(
            previous.write_operation_count,
            current.write_operation_count,
        ),
        other_operation_count: counter_delta(
            previous.other_operation_count,
            current.other_operation_count,
        ),
        read_bytes: counter_delta(previous.read_bytes, current.read_bytes),
        write_bytes: counter_delta(previous.write_bytes, current.write_bytes),
        other_bytes: counter_delta(previous.other_bytes, current.other_bytes),
        user_time_ns: counter_delta(previous.user_time_ns, current.user_time_ns),
        system_time_ns: counter_delta(previous.system_time_ns, current.system_time_ns),
    }
}

fn counter_delta(previous: Option<u64>, current: Option<u64>) -> Option<u64> {
    match (previous, current) {
        (Some(previous), Some(current)) if current >= previous => Some(current - previous),
        _ => None,
    }
}

fn read_write_activity_observed(delta: &ProcessActivityDelta) -> bool {
    [
        delta.read_operation_count,
        delta.write_operation_count,
        delta.read_bytes,
        delta.write_bytes,
    ]
    .into_iter()
    .flatten()
    .any(|delta| delta > 0)
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
) -> Vec<(&'a ProcessInfo, &'a AiProcessConfig)> {
    snapshot
        .processes
        .iter()
        .filter_map(|process| {
            find_hosted_ai_process(process, &config.ai_process_names).map(|tool| (process, tool))
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
    candidates: &[(&ProcessInfo, &AiProcessConfig)],
    context: &ProcessTreeDiagnosticContext,
    logger: &DesktopLogger,
) {
    let total = candidates.len();
    for (index, (process, tool)) in candidates.iter().take(50).enumerate() {
        let label = format!("{}/{}", index + 1, total);
        let is_descendant = context.descendant_pids.contains(&process.pid);
        logger.info_at(2, format!(
            "process_tree candidate[{label}] tool=\"{}\" descendant={} pid={} ppid={} exe=\"{}\" path=\"{}\"",
            tool.tool,
            is_descendant,
            process.pid,
            process.parent_pid,
            process.exe_name,
            process.exe_path.as_deref().unwrap_or("")
        ));
        logger.info_at(2, format!(
            "process_tree candidate[{label}] names bsd_name=\"{}\" bsd_comm=\"{}\" argv0=\"{}\" terminal=\"pgid={} tpgid={} has_ctty={} tty={} tty_access_time_seconds={} tty_activity_age_seconds={}\"",
            process.bsd_name.as_deref().unwrap_or(""),
            process.bsd_comm.as_deref().unwrap_or(""),
            process.argv0.as_deref().unwrap_or(""),
            format_optional_u32(process.process_group_id),
            format_optional_u32(process.terminal_foreground_process_group_id),
            process.has_controlling_terminal,
            process.terminal_name.as_deref().unwrap_or(""),
            format_optional_u64(process.terminal_access_time_seconds),
            format_optional_u64(process.terminal_activity_age_seconds),
        ));
        let activity = process.process_activity.as_ref();
        let delta = process.process_activity_delta.as_ref();
        logger.info_at(2, format!(
            "process_tree candidate[{label}] activity start_key={} read_ops={} write_ops={} other_ops={} read_bytes={} write_bytes={} other_bytes={} user_time_ns={} system_time_ns={} delta_read_ops={} delta_write_ops={} delta_other_ops={} delta_read_bytes={} delta_write_bytes={} delta_other_bytes={} delta_user_time_ns={} delta_system_time_ns={} read_write_activity_observed={}",
            activity.map(|activity| activity.process_start_key).map_or_else(|| "none".to_string(), |value| value.to_string()),
            format_optional_u64(activity.and_then(|activity| activity.read_operation_count)),
            format_optional_u64(activity.and_then(|activity| activity.write_operation_count)),
            format_optional_u64(activity.and_then(|activity| activity.other_operation_count)),
            format_optional_u64(activity.and_then(|activity| activity.read_bytes)),
            format_optional_u64(activity.and_then(|activity| activity.write_bytes)),
            format_optional_u64(activity.and_then(|activity| activity.other_bytes)),
            format_optional_u64(activity.and_then(|activity| activity.user_time_ns)),
            format_optional_u64(activity.and_then(|activity| activity.system_time_ns)),
            format_optional_u64(delta.and_then(|delta| delta.read_operation_count)),
            format_optional_u64(delta.and_then(|delta| delta.write_operation_count)),
            format_optional_u64(delta.and_then(|delta| delta.other_operation_count)),
            format_optional_u64(delta.and_then(|delta| delta.read_bytes)),
            format_optional_u64(delta.and_then(|delta| delta.write_bytes)),
            format_optional_u64(delta.and_then(|delta| delta.other_bytes)),
            format_optional_u64(delta.and_then(|delta| delta.user_time_ns)),
            format_optional_u64(delta.and_then(|delta| delta.system_time_ns)),
            process.process_read_write_activity_observed,
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

/// Render optional age values in log-friendly form.
fn format_optional_u64(value: Option<u64>) -> String {
    value
        .map(|value| value.to_string())
        .unwrap_or_else(|| "none".to_string())
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::datadog::DesktopMonitoringConfig;

    fn config() -> DesktopMonitoringConfig {
        DesktopMonitoringConfig {
            enabled: true,
            debug: 0,
            poll_interval_seconds: 60,
            process_activity_window_seconds: 600,
            ai_process_names: Vec::new(),
            host_process_names: Vec::new(),
        }
    }

    fn process(pid: u32, start_key: u64, read_bytes: u64, write_bytes: u64) -> ProcessInfo {
        ProcessInfo {
            pid,
            parent_pid: 1,
            exe_name: "claude".to_string(),
            bsd_name: Some("claude".to_string()),
            bsd_comm: Some("claude".to_string()),
            argv0: Some("claude".to_string()),
            argv: vec!["claude".to_string()],
            exe_path: None,
            window_title: None,
            attached_console_title: None,
            process_group_id: Some(pid),
            terminal_foreground_process_group_id: Some(pid),
            has_controlling_terminal: true,
            terminal_name: None,
            terminal_access_time_seconds: None,
            terminal_activity_age_seconds: Some(0),
            process_activity: Some(ProcessActivity {
                process_start_key: start_key,
                read_operation_count: None,
                write_operation_count: None,
                other_operation_count: None,
                read_bytes: Some(read_bytes),
                write_bytes: Some(write_bytes),
                other_bytes: None,
                user_time_ns: None,
                system_time_ns: None,
            }),
            process_activity_delta: None,
            process_read_write_activity_observed: false,
        }
    }

    fn process_with_cpu_time(pid: u32, start_key: u64, user_time_ns: u64) -> ProcessInfo {
        ProcessInfo {
            process_activity: Some(ProcessActivity {
                user_time_ns: Some(user_time_ns),
                ..process(pid, start_key, 10, 10)
                    .process_activity
                    .expect("process should have activity")
            }),
            ..process(pid, start_key, 10, 10)
        }
    }

    fn snapshot(process: ProcessInfo) -> ProcessSnapshot {
        ProcessSnapshot {
            edges: vec![ProcessEdge {
                pid: process.pid,
                parent_pid: process.parent_pid,
            }],
            processes: vec![process],
        }
    }

    #[test]
    fn process_activity_tracker_requires_new_read_write_delta() {
        let mut tracker = ProcessActivityTracker::default();
        let config = config();

        let mut first_snapshot = snapshot(process(11, 100, 10, 10));
        tracker.mark_observed_activity(&mut first_snapshot, &config);
        assert!(!first_snapshot.processes[0].process_read_write_activity_observed);
        assert!(first_snapshot.processes[0].process_activity_delta.is_none());

        let mut unchanged_snapshot = snapshot(process(11, 100, 10, 10));
        tracker.mark_observed_activity(&mut unchanged_snapshot, &config);
        assert!(!unchanged_snapshot.processes[0].process_read_write_activity_observed);
        assert_eq!(
            unchanged_snapshot.processes[0]
                .process_activity_delta
                .as_ref()
                .and_then(|delta| delta.read_bytes),
            Some(0)
        );

        let mut advanced_snapshot = snapshot(process(11, 100, 11, 10));
        tracker.mark_observed_activity(&mut advanced_snapshot, &config);
        assert!(advanced_snapshot.processes[0].process_read_write_activity_observed);
        assert_eq!(
            advanced_snapshot.processes[0]
                .process_activity_delta
                .as_ref()
                .and_then(|delta| delta.read_bytes),
            Some(1)
        );
    }

    #[test]
    fn process_activity_tracker_ignores_cpu_only_deltas() {
        let mut tracker = ProcessActivityTracker::default();
        let config = config();

        let mut first_snapshot = snapshot(process_with_cpu_time(11, 100, 10));
        tracker.mark_observed_activity(&mut first_snapshot, &config);

        let mut cpu_snapshot = snapshot(process_with_cpu_time(11, 100, 20));
        tracker.mark_observed_activity(&mut cpu_snapshot, &config);

        assert!(!cpu_snapshot.processes[0].process_read_write_activity_observed);
        assert_eq!(
            cpu_snapshot.processes[0]
                .process_activity_delta
                .as_ref()
                .and_then(|delta| delta.user_time_ns),
            Some(10)
        );
    }

    #[test]
    fn process_activity_tracker_uses_start_key_to_avoid_pid_reuse() {
        let mut tracker = ProcessActivityTracker::default();
        let config = config();

        let mut first_snapshot = snapshot(process(11, 100, 10, 10));
        tracker.mark_observed_activity(&mut first_snapshot, &config);

        let mut reused_pid_snapshot = snapshot(process(11, 200, 11, 10));
        tracker.mark_observed_activity(&mut reused_pid_snapshot, &config);

        assert!(!reused_pid_snapshot.processes[0].process_read_write_activity_observed);
        assert!(
            reused_pid_snapshot.processes[0]
                .process_activity_delta
                .is_none()
        );
    }
}
