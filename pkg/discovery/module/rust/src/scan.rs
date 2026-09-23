// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Whole-system scan combining SPL service discovery (process language,
//! listening TCP ports) with HTTP probing of the ports for
//! Prometheus/OpenMetrics endpoints.
//!
//! This backs the `disco scan` CLI, used to empirically verify the claim that
//! Go programs (in particular Kubernetes/CNCF ones) commonly expose a
//! `/metrics` endpoint out of the box.
//!
//! Ports are attributed to processes through their open socket file
//! descriptors (same as the service discovery logic). Because several
//! network namespaces may coexist on a host (one per pod with Kubernetes),
//! ports are grouped per network namespace: ports in a foreign namespace are
//! probed by re-execing this binary inside that namespace (see `netns_child`).

use std::collections::{BTreeMap, BTreeSet, HashMap, VecDeque};
use std::fs;
use std::net::{IpAddr, Ipv4Addr, SocketAddr};
use std::os::fd::AsRawFd;
use std::path::Path;
use std::process::{Command, Stdio};
use std::time::Duration;

use serde::Serialize;

use crate::comm;
use crate::http_probe::{self, Classification, EndpointProbeResult};
use crate::language::Language;
use crate::netns::{self, Ino};
use crate::ports::{self, ParsingContext};
use crate::procfs::{self, Cmdline, Exe};

// setns(2) flag to only change the network namespace.
const CLONE_NEWNET: i32 = 0x40000000;

/// Maximum number of probe targets (candidate addresses) per port.
const MAX_TARGETS_PER_PORT: usize = 3;

// Declaration of the libc setns(2) wrapper.
// SAFETY: this is a plain C ABI declaration; the function itself is only
// called through a documented unsafe block in `netns_child`.
unsafe extern "C" {
    fn setns(fd: i32, nstype: i32) -> i32;
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum LanguageFilter {
    Go,
    Any,
}

#[derive(Debug, Clone)]
pub struct ProbeOptions {
    pub path: String,
    pub connect_timeout: Duration,
    pub request_timeout: Duration,
    pub max_body_bytes: usize,
}

#[derive(Debug, Clone)]
pub struct ScanOptions {
    pub dry_run: bool,
    pub language_filter: LanguageFilter,
    pub probe: ProbeOptions,
    /// When true (default), foreign network namespaces are entered (via a
    /// re-exec child) to probe ports that live in them. When false, every
    /// port is probed from the current network namespace.
    pub netns_enter: bool,
    /// When true, processes ignored by the service discovery logic (kubelet,
    /// containerd, datadog-* ...) are excluded.
    pub exclude_infra: bool,
    pub workers: usize,
    pub max_concurrent_children: usize,
}

#[derive(Debug, Serialize)]
pub struct ScanReport {
    pub timestamp_unix_seconds: u64,
    pub hostname: Option<String>,
    pub dry_run: bool,
    pub proc_root: String,
    pub language_filter: LanguageFilter,
    pub metrics_path: String,
    /// Network namespace of the scanning process itself.
    pub own_netns_ino: Option<u64>,
    pub processes: Vec<ProcessReport>,
    pub summary: ScanSummary,
}

#[derive(Debug, Serialize)]
pub struct ProcessReport {
    pub pid: i32,
    pub comm: String,
    pub exe: Option<String>,
    pub cmdline: String,
    pub language: Option<Language>,
    pub netns_ino: Option<u64>,
    pub own_netns: Option<bool>,
    /// True when the standard service discovery logic would ignore this
    /// process (kubelet, containerd, datadog-*, ...). Those are kept in the
    /// scan by default since they are exactly the kind of Go infra daemons
    /// relevant to the OpenMetrics endpoint study.
    pub ignored_by_service_discovery: bool,
    pub ports: Vec<PortReport>,
}

#[derive(Debug, Serialize)]
pub struct PortReport {
    pub port: u16,
    /// Raw bind addresses of the listening socket(s) in the process's
    /// network namespace ("0.0.0.0", "127.0.0.1", "10.128.68.59", "::", ...).
    pub listen_addrs: Vec<String>,
    /// Probe attempts for this port; empty in dry-run mode.
    pub probes: Vec<EndpointProbeResult>,
}

#[derive(Debug, Default, Serialize)]
pub struct ScanSummary {
    pub processes_found: usize,
    pub processes_scanned: usize,
    pub unreadable_processes: usize,
    pub language_counts: BTreeMap<String, usize>,
    /// Processes matching the language filter (before the infra filter).
    pub matched_processes: usize,
    pub excluded_infra_processes: usize,
    pub matched_processes_with_tcp_ports: usize,
    /// Distinct (network namespace, port) pairs examined.
    pub tcp_ports_examined: usize,
    pub netns_entered: usize,
    pub netns_enter_failures: usize,
    pub probes_performed: usize,
    pub prometheus_endpoints_found: usize,
    pub processes_with_prometheus_endpoint: usize,
    pub processes_with_go_runtime_metrics: usize,
}

#[derive(Debug)]
struct RawProcess {
    pid: i32,
    comm: String,
    exe: Option<String>,
    cmdline: String,
    language: Option<Language>,
    netns_ino: Option<u64>,
    ports: Vec<u16>,
    ignored_by_sd: bool,
}

#[derive(Debug, Clone, Copy)]
struct ProbeTask {
    key: (Ino, u16),
    addr: SocketAddr,
}

/// Runs the full scan and returns the report. No probing happens in dry-run
/// mode: the report lists processes and their listening ports only.
pub fn run_scan(opts: &ScanOptions) -> ScanReport {
    let root = procfs::root_path().to_path_buf();
    let own_netns = netns::get_self_netns_ino().ok();

    // ------------------------------------------------------------------
    // Phase 1: enumerate processes.
    // ------------------------------------------------------------------
    let pids = list_pids(&root);
    let mut summary = ScanSummary {
        processes_found: pids.len(),
        ..Default::default()
    };

    let mut context = ParsingContext::new();
    let mut raw: Vec<RawProcess> = Vec::new();

    for pid in pids {
        summary.processes_scanned += 1;
        match scan_process(pid, &mut context) {
            Some(p) => {
                if let Some(lang) = p.language {
                    *summary
                        .language_counts
                        .entry(lang.as_str().to_string())
                        .or_default() += 1;
                }
                raw.push(p);
            }
            None => summary.unreadable_processes += 1,
        }
    }

    let mut matched: Vec<RawProcess> = raw
        .into_iter()
        .filter(|p| match opts.language_filter {
            LanguageFilter::Go => p.language == Some(Language::Go),
            LanguageFilter::Any => true,
        })
        .collect();
    summary.matched_processes = matched.len();

    if opts.exclude_infra {
        let before = matched.len();
        matched.retain(|p| !p.ignored_by_sd);
        summary.excluded_infra_processes = before - matched.len();
    }

    // ------------------------------------------------------------------
    // Phase 2: map ports to network namespaces and candidate addresses.
    // ------------------------------------------------------------------
    // Key: (netns inode, or 0 when unknown), port.
    let mut port_keys: BTreeSet<(Ino, u16)> = BTreeSet::new();
    for p in &matched {
        for port in &p.ports {
            port_keys.insert((p.netns_ino.unwrap_or(0), *port));
        }
    }
    summary.tcp_ports_examined = port_keys.len();
    summary.matched_processes_with_tcp_ports = matched.iter().filter(|p| !p.ports.is_empty()).count();

    // Listening addresses per network namespace (socket tables are shared
    // per namespace, so we parse them once per namespace).
    let mut listen_addrs_by_netns: BTreeMap<Ino, HashMap<u16, Vec<IpAddr>>> = BTreeMap::new();
    let mut enter_pid_by_netns: BTreeMap<Ino, i32> = BTreeMap::new();
    for p in &matched {
        if let Some(ino) = p.netns_ino {
            enter_pid_by_netns.entry(ino).or_insert(p.pid);
        }
    }
    for (ino, pid) in &enter_pid_by_netns {
        listen_addrs_by_netns
            .entry(*ino)
            .or_insert_with(|| netns::get_listen_addrs(*pid));
    }

    // ------------------------------------------------------------------
    // Phase 3: probe (unless dry-run).
    // ------------------------------------------------------------------
    // Results keyed by (netns, port), one entry per probe attempt.
    let mut results: HashMap<(Ino, u16), Vec<EndpointProbeResult>> = HashMap::new();

    if !opts.dry_run {
        let mut direct_tasks: VecDeque<ProbeTask> = VecDeque::new();
        // One child plan per foreign network namespace.
        let mut child_plans: BTreeMap<Ino, (i32, Vec<ProbeTask>)> = BTreeMap::new();

        for key in &port_keys {
            let addrs = listen_addrs_by_netns
                .get(&key.0)
                .and_then(|m| m.get(&key.1))
                .map(Vec::as_slice)
                .unwrap_or(&[]);
            let targets = probe_targets_for(addrs);
            let targets = if targets.is_empty() {
                // No parseable bind address: try loopback as a best effort.
                vec![SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), key.1)]
            } else {
                targets
                    .iter()
                    .map(|ip| SocketAddr::new(*ip, key.1))
                    .collect()
            };
            for target in targets {
                let task = ProbeTask { key: *key, addr: target };
                let foreign_netns = opts.netns_enter
                    && key.0 != 0
                    && own_netns.is_some_and(|own| own != key.0);
                if foreign_netns {
                    if let Some(enter_pid) = enter_pid_by_netns.get(&key.0) {
                        child_plans
                            .entry(key.0)
                            .or_insert((*enter_pid, Vec::new()))
                            .1
                            .push(task);
                    } else {
                        direct_tasks.push_back(task);
                    }
                } else {
                    direct_tasks.push_back(task);
                }
            }
        }

        let results_mutex = std::sync::Mutex::new(results);
        let queue = std::sync::Mutex::new(direct_tasks);
        let child_stats = std::sync::Mutex::new((0usize, 0usize)); // (ok, failed)
        let probe_opts = &opts.probe;

        let worker_count = opts.workers.clamp(1, 64);
        let child_cap = opts.max_concurrent_children.max(1);
        let child_plans: Vec<(Ino, (i32, Vec<ProbeTask>))> = child_plans.into_iter().collect();

        std::thread::scope(|scope| {
            // Shared state is referenced (not moved) by the worker closures.
            let queue = &queue;
            let results_mutex = &results_mutex;
            let child_stats = &child_stats;

            // Direct probe pool (current network namespace).
            let worker_handles: Vec<_> = (0..worker_count)
                .map(|_| {
                    scope.spawn(move || loop {
                        let task = queue.lock().ok().and_then(|mut q| q.pop_front());
                        let Some(task) = task else {
                            break;
                        };
                        let r = http_probe::probe_endpoint(
                            task.addr,
                            &probe_opts.path,
                            probe_opts.connect_timeout,
                            probe_opts.request_timeout,
                            probe_opts.max_body_bytes,
                        );
                        if let Ok(mut all) = results_mutex.lock() {
                            all.entry(task.key).or_default().push(r);
                        }
                    })
                })
                .collect();

            // Foreign network namespaces: waves of child processes.
            for wave in child_plans.chunks(child_cap) {
                let child_handles: Vec<_> = wave
                    .iter()
                    .map(|(_netns, (enter_pid, tasks))| {
                        scope.spawn(move || {
                            let targets: Vec<SocketAddr> =
                                tasks.iter().map(|t| t.addr).collect();
                            match spawn_netns_child(*enter_pid, &targets, probe_opts) {
                                Ok(child_results) => {
                                    if let Ok(mut stats) = child_stats.lock() {
                                        stats.0 += 1;
                                    }
                                    if let Ok(mut all) = results_mutex.lock() {
                                        for (task, r) in
                                            tasks.iter().zip(child_results)
                                        {
                                            all.entry(task.key).or_default().push(r);
                                        }
                                    }
                                }
                                Err(err) => {
                                    if let Ok(mut stats) = child_stats.lock() {
                                        stats.1 += 1;
                                    }
                                    // Fallback: loopback addresses cannot be
                                    // reached from our namespace, but pod IPs
                                    // may be. Record enter failures for the
                                    // rest.
                                    for task in tasks {
                                        let r = if task.addr.ip().is_loopback() {
                                            enter_failed_result(task.addr, probe_opts, &err)
                                        } else {
                                            http_probe::probe_endpoint(
                                                task.addr,
                                                &probe_opts.path,
                                                probe_opts.connect_timeout,
                                                probe_opts.request_timeout,
                                                probe_opts.max_body_bytes,
                                            )
                                        };
                                        if let Ok(mut all) = results_mutex.lock() {
                                            all.entry(task.key).or_default().push(r);
                                        }
                                    }
                                }
                            }
                        })
                    })
                    .collect();
                for h in child_handles {
                    let _ = h.join();
                }
            }

            for h in worker_handles {
                let _ = h.join();
            }
        });

        results = results_mutex.into_inner().unwrap_or_default();
        if let Ok(stats) = child_stats.lock() {
            summary.netns_entered = stats.0;
            summary.netns_enter_failures = stats.1;
        }
    }

    // ------------------------------------------------------------------
    // Phase 4: assemble the report.
    // ------------------------------------------------------------------
    let processes: Vec<ProcessReport> = matched
        .iter()
        .map(|p| {
            let ports = p
                .ports
                .iter()
                .map(|port| {
                    let key = (p.netns_ino.unwrap_or(0), *port);
                    let listen_addrs = listen_addrs_by_netns
                        .get(&key.0)
                        .and_then(|m| m.get(port))
                        .map(|v| v.iter().map(ToString::to_string).collect())
                        .unwrap_or_default();
                    let probes = results.get(&key).cloned().unwrap_or_default();
                    PortReport {
                        port: *port,
                        listen_addrs,
                        probes,
                    }
                })
                .collect();
            ProcessReport {
                pid: p.pid,
                comm: p.comm.clone(),
                exe: p.exe.clone(),
                cmdline: p.cmdline.clone(),
                language: p.language,
                netns_ino: p.netns_ino,
                own_netns: p.netns_ino.map(|ino| Some(ino) == own_netns),
                ignored_by_service_discovery: p.ignored_by_sd,
                ports,
            }
        })
        .collect();

    // Summary counters on probe outcomes.
    summary.probes_performed = results.values().map(Vec::len).sum();
    let mut prometheus_ports: BTreeSet<(Ino, u16)> = BTreeSet::new();
    let mut go_runtime_ports: BTreeSet<(Ino, u16)> = BTreeSet::new();
    for (key, probes) in &results {
        if probes
            .iter()
            .any(|r| r.classification == Classification::PrometheusMetrics)
        {
            prometheus_ports.insert(*key);
        }
        if probes.iter().any(|r| {
            r.analysis
                .as_ref()
                .is_some_and(|a| a.has_go_runtime_metrics)
        }) {
            go_runtime_ports.insert(*key);
        }
    }
    summary.prometheus_endpoints_found = prometheus_ports.len();
    summary.processes_with_prometheus_endpoint = processes
        .iter()
        .filter(|p| {
            p.ports
                .iter()
                .any(|port| prometheus_ports.contains(&(p.netns_ino.unwrap_or(0), port.port)))
        })
        .count();
    summary.processes_with_go_runtime_metrics = processes
        .iter()
        .filter(|p| {
            p.ports
                .iter()
                .any(|port| go_runtime_ports.contains(&(p.netns_ino.unwrap_or(0), port.port)))
        })
        .count();

    ScanReport {
        timestamp_unix_seconds: unix_timestamp(),
        hostname: read_hostname(),
        dry_run: opts.dry_run,
        proc_root: root.to_string_lossy().into_owned(),
        language_filter: opts.language_filter,
        metrics_path: opts.probe.path.clone(),
        own_netns_ino: own_netns,
        processes,
        summary,
    }
}

/// Scans a single process: language, listening TCP ports, network namespace.
fn scan_process(pid: i32, context: &mut ParsingContext) -> Option<RawProcess> {
    // Kernel threads have no exe; processes of other users may be unreadable.
    // Either way Exe::get / get_open_files_info fail and we skip them.
    let exe = Exe::get(pid).ok()?;
    let open_files = procfs::fd::get_open_files_info(pid).ok()?;
    let maps_info = procfs::maps::read_maps_info(pid).unwrap_or_default();
    let cmdline = Cmdline::get(pid).ok()?;

    let language = Language::detect(pid, &exe, &cmdline, &open_files, &maps_info);

    let (tcp_ports, _) = ports::get(context, pid, &open_files.sockets);
    let ports_vec = tcp_ports.unwrap_or_default();

    let comm = fs::read_to_string(procfs::root_path().join(pid.to_string()).join("comm"))
        .map(|s| s.trim().to_string())
        .unwrap_or_default();

    Some(RawProcess {
        pid,
        comm,
        exe: Some(exe.0.to_string_lossy().into_owned()),
        cmdline: cmdline.args().collect::<Vec<&str>>().join(" "),
        language,
        netns_ino: netns::get_netns_ino(pid).ok(),
        ports: ports_vec,
        ignored_by_sd: comm::should_ignore_comm(pid),
    })
}

fn list_pids(root: &Path) -> Vec<i32> {
    let mut pids = Vec::new();
    let Ok(entries) = fs::read_dir(root) else {
        return pids;
    };
    for entry in entries.flatten() {
        let name = entry.file_name();
        let Some(name) = name.to_str() else {
            continue;
        };
        if let Ok(pid) = name.parse::<i32>() {
            pids.push(pid);
        }
    }
    pids.sort_unstable();
    pids
}

/// Maps the bind addresses of a listening port to the actual probe targets:
/// wildcard addresses are probed through loopback, IPv4-mapped IPv6
/// addresses are probed through IPv4. Loopback targets come first.
fn probe_targets_for(addrs: &[IpAddr]) -> Vec<IpAddr> {
    let mut targets: Vec<IpAddr> = Vec::new();
    let mut push = |ip: IpAddr| {
        if !targets.contains(&ip) && targets.len() < MAX_TARGETS_PER_PORT {
            targets.push(ip);
        }
    };
    for addr in addrs {
        match addr {
            IpAddr::V4(v4) if v4.is_unspecified() => push(IpAddr::V4(Ipv4Addr::LOCALHOST)),
            IpAddr::V4(_) => push(*addr),
            IpAddr::V6(v6) if v6.is_unspecified() => push(IpAddr::V4(Ipv4Addr::LOCALHOST)),
            IpAddr::V6(v6) if v6.is_loopback() => push(*addr),
            IpAddr::V6(v6) => {
                if let Some(mapped) = v6.to_ipv4_mapped() {
                    push(IpAddr::V4(mapped));
                } else {
                    push(*addr);
                }
            }
        }
    }
    // Loopback targets first (cheapest and most likely for /metrics).
    targets.sort_by_key(|t| !t.is_loopback());
    targets
}

fn enter_failed_result(
    addr: SocketAddr,
    opts: &ProbeOptions,
    error: &str,
) -> EndpointProbeResult {
    let path = opts.path.as_str();
    EndpointProbeResult {
        url: format!("http://{addr}{path}"),
        classification: Classification::NetnsEnterFailed,
        status: None,
        content_type: None,
        location: None,
        body_bytes: 0,
        body_truncated: false,
        analysis: None,
        error: Some(error.to_string()),
        elapsed_ms: 0,
    }
}

/// Probes the given targets from inside the network namespace of `enter_pid`.
///
/// This is the body of the hidden `disco netns-probe` child process: the
/// namespace switch must happen before any thread is spawned, so the parent
/// re-executes this binary with that subcommand.
pub fn netns_child(
    enter_pid: i32,
    targets: &[SocketAddr],
    opts: &ProbeOptions,
) -> Result<Vec<EndpointProbeResult>, String> {
    // The ns path must be resolved through the configured proc root (which
    // may point at the host /proc when running in a container).
    let ns_path = procfs::root_path()
        .join(enter_pid.to_string())
        .join("ns/net");
    let file =
        fs::File::open(&ns_path).map_err(|e| format!("open {}: {e}", ns_path.display()))?;
    // SAFETY: setns(2) moves the calling thread into the network namespace
    // referred to by `fd`. This function runs before any other thread exists
    // (the caller is a fresh process) and CLONE_NEWNET restricts the change
    // to the network namespace. `fd` is a valid, open namespace file.
    let rc = unsafe { setns(file.as_raw_fd(), CLONE_NEWNET) };
    if rc != 0 {
        return Err(format!(
            "setns({}): {}",
            ns_path.display(),
            std::io::Error::last_os_error()
        ));
    }

    Ok(targets
        .iter()
        .map(|addr| {
            http_probe::probe_endpoint(
                *addr,
                &opts.path,
                opts.connect_timeout,
                opts.request_timeout,
                opts.max_body_bytes,
            )
        })
        .collect())
}

fn spawn_netns_child(
    enter_pid: i32,
    targets: &[SocketAddr],
    opts: &ProbeOptions,
) -> Result<Vec<EndpointProbeResult>, String> {
    let exe = std::env::current_exe().map_err(|e| format!("current_exe: {e}"))?;
    let targets_str = targets
        .iter()
        .map(ToString::to_string)
        .collect::<Vec<_>>()
        .join(",");

    let output = Command::new(exe)
        .arg("netns-probe")
        .arg("--enter-pid")
        .arg(enter_pid.to_string())
        .arg("--targets")
        .arg(targets_str)
        .arg("--path")
        .arg(&opts.path)
        .arg("--connect-timeout-ms")
        .arg(opts.connect_timeout.as_millis().to_string())
        .arg("--request-timeout-ms")
        .arg(opts.request_timeout.as_millis().to_string())
        .arg("--max-body-bytes")
        .arg(opts.max_body_bytes.to_string())
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|e| format!("spawn child: {e}"))?;

    if !output.status.success() {
        return Err(format!(
            "child exited with {}: {}",
            output.status,
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }
    serde_json::from_slice(&output.stdout).map_err(|e| format!("parse child output: {e}"))
}

fn unix_timestamp() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

fn read_hostname() -> Option<String> {
    fs::read_to_string("/proc/sys/kernel/hostname")
        .ok()
        .map(|h| h.trim().to_string())
}
