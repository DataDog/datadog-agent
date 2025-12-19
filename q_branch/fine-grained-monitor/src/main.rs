//! Fine-grained container resource monitoring for datadog-agent development.
//!
//! This tool captures detailed resource metrics (memory, CPU) from all containers
//! on a Kubernetes node and writes them to a Parquet file for post-hoc analysis.

use std::path::PathBuf;
use std::time::Duration;

use anyhow::Context;
use clap::Parser;
use tokio::signal::unix::{signal, SignalKind};

mod discovery;
mod observer;

/// Maximum Parquet file size before triggering graceful shutdown (1 GiB)
const MAX_FILE_SIZE_BYTES: u64 = 1024 * 1024 * 1024;

/// Fine-grained container resource monitor
#[derive(Parser, Debug)]
#[command(name = "fine-grained-monitor")]
#[command(about = "Capture detailed container metrics to Parquet")]
struct Args {
    /// Output path for the Parquet file
    #[arg(short, long, default_value = "metrics.parquet")]
    output: PathBuf,

    /// Sampling interval in milliseconds
    #[arg(short, long, default_value = "1000")]
    interval_ms: u64,

    /// ZSTD compression level (1-22)
    #[arg(short, long, default_value = "3")]
    compression_level: i32,

    /// Enable potentially invasive metrics collection (e.g., /proc/<pid>/smaps)
    ///
    /// WARNING: Reading smaps acquires the kernel mm lock which may impact
    /// the monitored process. Only enable when detailed per-region memory
    /// breakdown is needed.
    #[arg(long, default_value = "false")]
    verbose_perf_risk: bool,

    /// Node name for labeling metrics
    #[arg(long, env = "NODE_NAME")]
    node_name: Option<String>,

    /// Cluster name for labeling metrics
    #[arg(long, env = "CLUSTER_NAME")]
    cluster_name: Option<String>,

    /// Flush interval in seconds (how often to write to disk)
    #[arg(long, default_value = "10")]
    flush_seconds: u64,

    /// Metric expiration in seconds (accumulator window size)
    #[arg(long, default_value = "60")]
    expiration_seconds: u64,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Initialize tracing - RUST_LOG takes precedence, fallback to info
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let args = Args::parse();

    tracing::info!(
        output = %args.output.display(),
        interval_ms = args.interval_ms,
        compression_level = args.compression_level,
        flush_seconds = args.flush_seconds,
        verbose_perf_risk = args.verbose_perf_risk,
        "Starting fine-grained-monitor"
    );

    run(args).await
}

async fn run(args: Args) -> anyhow::Result<()> {
    // Create signal pairs for lifecycle management
    // shutdown: triggers graceful shutdown (SIGINT/SIGTERM or file size limit)
    // experiment_started: not used, signal immediately
    // target_running: marks time-zero, signal immediately to start capture
    let (shutdown_watcher, shutdown_broadcaster) = lading_signal::signal();
    let (experiment_watcher, experiment_broadcaster) = lading_signal::signal();
    let (target_watcher, target_broadcaster) = lading_signal::signal();

    // Create the CaptureManager with Parquet output
    let mut capture_manager = lading_capture::manager::CaptureManager::new_parquet(
        args.output.clone(),
        args.flush_seconds,
        args.compression_level,
        shutdown_watcher,
        experiment_watcher,
        target_watcher,
        Duration::from_secs(args.expiration_seconds),
    )
    .await
    .context("Failed to create CaptureManager")?;

    // Add global labels for all metrics
    let node_name = args
        .node_name
        .clone()
        .or_else(|| hostname::get().ok().and_then(|h| h.into_string().ok()))
        .unwrap_or_else(|| "unknown".to_string());

    let cluster_name = args
        .cluster_name
        .clone()
        .unwrap_or_else(|| "unknown".to_string());

    capture_manager.add_global_label("node", &node_name);
    capture_manager.add_global_label("cluster", &cluster_name);

    // Note: CaptureManager::start() calls install() internally to set up
    // the global metrics recorder. Don't call install() explicitly here.

    tracing::info!(
        node = %node_name,
        cluster = %cluster_name,
        "Configured global labels"
    );

    // Signal that experiment has started and target is running
    // In our use case, we start immediately - no warmup phase
    experiment_broadcaster.signal();
    target_broadcaster.signal();

    // Channel for file size monitor to request shutdown
    let (size_limit_tx, mut size_limit_rx) = tokio::sync::oneshot::channel::<()>();
    let output_path = args.output.clone();

    // Spawn file size monitor task
    let file_size_monitor = tokio::spawn(async move {
        monitor_file_size(output_path, size_limit_tx).await
    });

    // Set up OS signal handlers for graceful shutdown
    let mut sigint = signal(SignalKind::interrupt())?;
    let mut sigterm = signal(SignalKind::terminate())?;

    // Run the main event loop
    // We need to signal shutdown from exactly one place since Broadcaster::signal() consumes self
    let shutdown_reason = tokio::select! {
        // CaptureManager runs until shutdown signal
        result = capture_manager.start() => {
            match result {
                Ok(()) => tracing::info!("CaptureManager completed normally"),
                Err(e) => tracing::error!(error = %e, "CaptureManager error"),
            }
            // CaptureManager already received shutdown from somewhere, don't signal again
            None
        }

        // Observer loop: discover containers and sample metrics
        () = observer_loop(args.interval_ms, args.verbose_perf_risk) => {
            tracing::info!("Observer loop completed unexpectedly");
            Some("observer_exit")
        }

        // Handle SIGINT (Ctrl+C)
        _ = sigint.recv() => {
            tracing::info!("Received SIGINT, initiating graceful shutdown");
            Some("sigint")
        }

        // Handle SIGTERM
        _ = sigterm.recv() => {
            tracing::info!("Received SIGTERM, initiating graceful shutdown");
            Some("sigterm")
        }

        // File size limit exceeded
        _ = &mut size_limit_rx => {
            tracing::warn!("File size limit exceeded, initiating graceful shutdown");
            Some("size_limit")
        }
    };

    // Signal shutdown if we haven't already
    if shutdown_reason.is_some() {
        shutdown_broadcaster.signal();
    }

    // Clean up file size monitor
    file_size_monitor.abort();

    tracing::info!("Shutdown complete");
    Ok(())
}

/// Main observer loop: discovers containers and samples metrics at the configured interval
async fn observer_loop(interval_ms: u64, verbose_perf_risk: bool) {
    let mut interval = tokio::time::interval(Duration::from_millis(interval_ms));
    let mut observer = observer::Observer::new();

    loop {
        interval.tick().await;

        // Discover all running containers via cgroup scan
        let containers = discovery::scan_cgroups();

        if containers.is_empty() {
            tracing::debug!("No containers discovered");
            continue;
        }

        tracing::debug!(count = containers.len(), "Discovered containers");

        // Sample metrics for all containers
        observer.sample(&containers, verbose_perf_risk).await;
    }
}

/// Monitor file size and notify when it exceeds the limit
async fn monitor_file_size(path: PathBuf, notify: tokio::sync::oneshot::Sender<()>) {
    let mut interval = tokio::time::interval(Duration::from_secs(10));

    loop {
        interval.tick().await;

        match tokio::fs::metadata(&path).await {
            Ok(metadata) => {
                let size = metadata.len();
                if size > MAX_FILE_SIZE_BYTES {
                    tracing::warn!(
                        size_bytes = size,
                        limit_bytes = MAX_FILE_SIZE_BYTES,
                        "Parquet file size limit exceeded"
                    );
                    // Notify main loop - ignore error if receiver dropped
                    let _ = notify.send(());
                    return;
                }

                tracing::debug!(
                    size_bytes = size,
                    limit_bytes = MAX_FILE_SIZE_BYTES,
                    "File size check"
                );
            }
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
                // File doesn't exist yet, that's fine during startup
                tracing::debug!("Output file not yet created");
            }
            Err(e) => {
                tracing::warn!(error = %e, "Failed to check file size");
            }
        }
    }
}
