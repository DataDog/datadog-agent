// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use clap::{CommandFactory, Parser, Subcommand};
use dd_discovery::{Params, get_services};

use dd_discovery::{LanguageFilter, ProbeOptions, ScanOptions, netns_child, run_scan};

#[derive(Parser, Debug)]
#[command(name = "disco")]
#[command(
    about = "Service discovery tool - detects service information from processes",
    long_about = None
)]
struct Args {
    /// Process ID to analyze (legacy single-process mode)
    #[arg(short, long)]
    pid: Option<i32>,

    #[command(subcommand)]
    command: Option<Command>,
}

#[derive(Subcommand, Debug)]
enum Command {
    /// Scan all processes on the system, find the ones matching the language
    /// filter, list their listening TCP ports and probe them for
    /// Prometheus/OpenMetrics endpoints.
    Scan(ScanArgs),
    /// Internal: probe targets from inside the network namespace of a
    /// process. Invoked by `disco scan` through a re-exec.
    #[command(hide = true)]
    NetnsProbe(NetnsProbeArgs),
}

#[derive(clap::Args, Debug)]
struct ScanArgs {
    /// Only list processes and ports, do not send any HTTP request.
    #[arg(long)]
    dry_run: bool,

    /// Language filter: `go` (default) or `any`.
    #[arg(long, default_value = "go")]
    language: String,

    /// Path to probe on every port.
    #[arg(long, default_value = "/metrics")]
    path: String,

    /// TCP connect timeout per probe, in milliseconds.
    #[arg(long, default_value_t = 300)]
    connect_timeout_ms: u64,

    /// Overall deadline per probe request, in milliseconds.
    #[arg(long, default_value_t = 1500)]
    request_timeout_ms: u64,

    /// Maximum response body bytes read per probe.
    #[arg(long, default_value_t = 65536)]
    max_body_bytes: usize,

    /// Probe ports living in other network namespaces from this namespace
    /// instead of entering them (entering requires root/CAP_SYS_ADMIN).
    #[arg(long)]
    no_netns_enter: bool,

    /// Exclude processes ignored by the service discovery logic (kubelet,
    /// containerd, datadog-*, ...). By default they are included since they
    /// are relevant Go programs for the endpoint study.
    #[arg(long)]
    exclude_infra: bool,

    /// Number of concurrent probe workers for ports in the local network
    /// namespace.
    #[arg(long, default_value_t = 16)]
    workers: usize,

    /// Maximum number of concurrent netns-entering child processes.
    #[arg(long, default_value_t = 8)]
    max_children: usize,

    /// Root of the proc filesystem to scan. Defaults to HOST_PROC, or
    /// /host/proc when it exists (agent containers), or /proc.
    #[arg(long)]
    proc_root: Option<String>,
}

#[derive(clap::Args, Debug)]
struct NetnsProbeArgs {
    /// PID whose network namespace should be entered.
    #[arg(long)]
    enter_pid: i32,

    /// Comma-separated list of `addr:port` targets to probe.
    #[arg(long)]
    targets: String,

    #[arg(long, default_value = "/metrics")]
    path: String,

    #[arg(long, default_value_t = 300)]
    connect_timeout_ms: u64,

    #[arg(long, default_value_t = 1500)]
    request_timeout_ms: u64,

    #[arg(long, default_value_t = 65536)]
    max_body_bytes: usize,

    #[arg(long)]
    proc_root: Option<String>,
}

// Applies the proc root override. Must run before any /proc access and
// before spawning threads.
fn apply_proc_root(root: &Option<String>) {
    let root = root.clone().or_else(detect_host_proc);
    if let Some(root) = root {
        // SAFETY: single-threaded at this point in the CLI; no other thread
        // reads the environment.
        unsafe { std::env::set_var("HOST_PROC", root) };
    }
}

// In agent containers the host proc filesystem is mounted at /host/proc. Use
// it when present so the scan sees host processes.
fn detect_host_proc() -> Option<String> {
    let host_proc = std::path::Path::new("/host/proc");
    if host_proc.join("1").exists() {
        return Some("/host/proc".to_string());
    }
    None
}

fn parse_language_filter(s: &str) -> Result<LanguageFilter, String> {
    match s {
        "go" => Ok(LanguageFilter::Go),
        "any" => Ok(LanguageFilter::Any),
        other => Err(format!("invalid language filter {other:?}: expected 'go' or 'any'")),
    }
}

#[allow(clippy::print_stdout, clippy::print_stderr)]
fn main() {
    let args = Args::parse();

    // Legacy single-PID mode.
    if let Some(pid) = args.pid {
        let params = Params {
            new_pids: Some(vec![pid]),
            heartbeat_pids: None,
        };
        let response = get_services(params);
        match serde_json::to_string_pretty(&response) {
            Ok(json) => println!("{json}"),
            Err(e) => eprintln!("Error serializing response: {e}"),
        }
        return;
    }

    match args.command {
        Some(Command::Scan(scan_args)) => {
            apply_proc_root(&scan_args.proc_root);

            let language_filter = match parse_language_filter(&scan_args.language) {
                Ok(f) => f,
                Err(e) => {
                    eprintln!("{e}");
                    std::process::exit(2);
                }
            };

            let opts = ScanOptions {
                dry_run: scan_args.dry_run,
                language_filter,
                probe: ProbeOptions {
                    path: scan_args.path,
                    connect_timeout: std::time::Duration::from_millis(
                        scan_args.connect_timeout_ms,
                    ),
                    request_timeout: std::time::Duration::from_millis(
                        scan_args.request_timeout_ms,
                    ),
                    max_body_bytes: scan_args.max_body_bytes,
                },
                netns_enter: !scan_args.no_netns_enter,
                exclude_infra: scan_args.exclude_infra,
                workers: scan_args.workers,
                max_concurrent_children: scan_args.max_children,
            };

            let report = run_scan(&opts);

            // Human-readable summary on stderr for quick reading; the full
            // report goes to stdout as JSON.
            let s = &report.summary;
            eprintln!(
                "disco scan: {} processes found, {} scanned, languages: {}",
                s.processes_found,
                s.processes_scanned,
                serde_json::to_string(&s.language_counts).unwrap_or_default()
            );
            eprintln!(
                "  matched processes: {} ({} with TCP ports), ports examined: {}",
                s.matched_processes, s.matched_processes_with_tcp_ports, s.tcp_ports_examined
            );
            if !report.dry_run {
                eprintln!(
                    "  probes: {}, prometheus endpoints: {} ({} processes, {} with go_* runtime metrics)",
                    s.probes_performed,
                    s.prometheus_endpoints_found,
                    s.processes_with_prometheus_endpoint,
                    s.processes_with_go_runtime_metrics
                );
            } else {
                eprintln!("  dry-run: no HTTP requests were sent");
            }

            match serde_json::to_string_pretty(&report) {
                Ok(json) => println!("{json}"),
                Err(e) => {
                    eprintln!("Error serializing report: {e}");
                    std::process::exit(1);
                }
            }
        }
        Some(Command::NetnsProbe(np_args)) => {
            apply_proc_root(&np_args.proc_root);

            let mut targets = Vec::new();
            for target in np_args.targets.split(',') {
                match target.parse() {
                    Ok(addr) => targets.push(addr),
                    Err(e) => {
                        eprintln!("invalid target {target:?}: {e}");
                        std::process::exit(2);
                    }
                }
            }

            let opts = ProbeOptions {
                path: np_args.path,
                connect_timeout: std::time::Duration::from_millis(np_args.connect_timeout_ms),
                request_timeout: std::time::Duration::from_millis(
                    np_args.request_timeout_ms,
                ),
                max_body_bytes: np_args.max_body_bytes,
            };

            match netns_child(np_args.enter_pid, &targets, &opts) {
                Ok(results) => match serde_json::to_string(&results) {
                    Ok(json) => {
                        println!("{json}");
                    }
                    Err(e) => {
                        eprintln!("Error serializing results: {e}");
                        std::process::exit(1);
                    }
                },
                Err(e) => {
                    eprintln!("{e}");
                    std::process::exit(1);
                }
            }
        }
        None => {
            let mut cmd = Args::command();
            cmd.print_help().ok();
        }
    }
}
