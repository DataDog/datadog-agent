//! Fine-grained container resource monitoring for datadog-agent development.
//!
//! This tool captures detailed resource metrics (memory, CPU) from all containers
//! on a Kubernetes node and writes them to Parquet files for post-hoc analysis.
//! Files rotate on a time-based interval (default 90s) to ensure each file has
//! a valid footer and is immediately readable. The 90-second interval exceeds
//! the 60-second accumulator window, ensuring complete time slices per file.

use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, RwLock};
use std::time::Duration;

use anyhow::Context;
use chrono::Utc;
use clap::Parser;
use serde::Serialize;
use tokio::signal::unix::{signal, SignalKind};
use tokio::sync::watch;
use uuid::Uuid;

mod discovery;
mod index;
mod kubernetes;
mod observer;
mod sidecar;

use index::ContainerIndex;
use kubernetes::KubernetesClient;
use sidecar::{ContainerSidecar, SidecarContainer};

/// Maximum total Parquet file size before triggering graceful shutdown (1 GiB)
const MAX_TOTAL_BYTES: u64 = 1024 * 1024 * 1024;

/// Fine-grained container resource monitor
#[derive(Parser, Debug, Clone)]
#[command(name = "fine-grained-monitor")]
#[command(about = "Capture detailed container metrics to Parquet files")]
struct Args {
    /// Output directory for Parquet files
    #[arg(short, long, default_value = "/data")]
    output_dir: PathBuf,

    /// Sampling interval in milliseconds.
    ///
    /// NOTE: Currently only 1000ms (1Hz) is supported due to a limitation in
    /// lading-capture where the tick duration is hardcoded to 1 second. Sub-second
    /// sampling would collect data but timestamps would be bucketed to 1-second
    /// resolution, losing the intended granularity. This limitation is tracked
    /// and can be addressed by making lading-capture's TICK_DURATION_MS configurable.
    #[arg(short, long, default_value = "1000")]
    interval_ms: u64,

    /// File rotation interval in seconds (should exceed 60s accumulator window)
    #[arg(long, default_value = "90")]
    rotation_seconds: u64,

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

    /// Pod name for unique file identification (from Kubernetes downward API)
    #[arg(long, env = "POD_NAME")]
    pod_name: Option<String>,

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

/// Session manifest written on startup for run context
#[derive(Serialize)]
struct SessionManifest {
    run_id: String,
    identifier: String,
    start_time: String,
    config: SessionConfig,
    node_name: String,
    cluster_name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    git_rev: Option<String>,
}

#[derive(Serialize)]
struct SessionConfig {
    sampling_interval_ms: u64,
    rotation_seconds: u64,
    compression_level: i32,
    verbose_perf_risk: bool,
    flush_seconds: u64,
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

    // Validate interval_ms - currently only 1Hz (1000ms) is supported due to
    // lading-capture's hardcoded tick duration. See state_machine.rs:31 and
    // manager.rs:36 in lading-capture where TICK_DURATION_MS is set to 1000.
    //
    // Sub-second sampling would collect more data points, but lading-capture
    // buckets all samples within the same second into one tick, and timestamps
    // are calculated as tick * 1000ms. For gauges, only the last sample per
    // second would be preserved, defeating the purpose of higher frequency.
    //
    // This is NOT an insurmountable limitation - it requires making lading-capture's
    // tick duration configurable. Until then, we fail fast to prevent misconfiguration.
    if args.interval_ms != 1000 {
        eprintln!("ERROR: --interval-ms={} is not supported.", args.interval_ms);
        eprintln!();
        eprintln!("Currently only 1000ms (1Hz) sampling is supported due to a limitation");
        eprintln!("in lading-capture where the tick duration is hardcoded to 1 second.");
        eprintln!();
        eprintln!("Technical details:");
        eprintln!("  - lading-capture/src/manager/state_machine.rs:31 defines TICK_DURATION_MS = 1000");
        eprintln!("  - All metric samples within the same second are bucketed into one tick");
        eprintln!("  - Timestamps are calculated as: start_ms + (tick * 1000)");
        eprintln!("  - For gauges (most cgroup metrics), only the last sample per second is preserved");
        eprintln!();
        eprintln!("This limitation can be addressed by making lading-capture's TICK_DURATION_MS");
        eprintln!("configurable. Until then, sub-second sampling would collect data but lose");
        eprintln!("the intended timestamp granularity.");
        eprintln!();
        eprintln!("Use --interval-ms=1000 (the default) for now.");
        std::process::exit(1);
    }

    tracing::info!(
        output_dir = %args.output_dir.display(),
        interval_ms = args.interval_ms,
        rotation_seconds = args.rotation_seconds,
        compression_level = args.compression_level,
        flush_seconds = args.flush_seconds,
        verbose_perf_risk = args.verbose_perf_risk,
        "Starting fine-grained-monitor"
    );

    run(args).await
}

/// Get unique identifier for filenames (pod name > node name > hostname)
fn get_unique_identifier(args: &Args) -> String {
    args.pod_name
        .clone()
        .or_else(|| args.node_name.clone())
        .or_else(|| hostname::get().ok().and_then(|h| h.into_string().ok()))
        .unwrap_or_else(|| "unknown".to_string())
}

/// Generate the partitioned directory path for this rotation
fn get_partition_dir(output_dir: &std::path::Path, identifier: &str) -> PathBuf {
    let date = Utc::now().format("%Y-%m-%d");
    output_dir
        .join(format!("dt={}", date))
        .join(format!("identifier={}", identifier))
}

/// Generate a unique filename for this rotation period
fn generate_filename() -> String {
    format!("metrics-{}.parquet", Utc::now().format("%Y%m%dT%H%M%SZ"))
}

/// Write container sidecar file for a completed parquet file.
///
/// The sidecar contains the list of containers active at rotation time,
/// enabling the viewer to discover containers by reading tiny sidecar files
/// instead of decompressing parquet row groups.
fn write_container_sidecar(parquet_path: &PathBuf, index: &ContainerIndex) {
    let sidecar_path = sidecar::sidecar_path_for_parquet(parquet_path);

    // Convert index entries to sidecar format
    let containers: Vec<SidecarContainer> = index
        .containers
        .iter()
        .map(|(short_id, entry)| SidecarContainer {
            container_id: short_id.clone(),
            pod_name: entry.pod_name.clone(),
            container_name: entry.container_name.clone(),
            namespace: entry.namespace.clone(),
            pod_uid: entry.pod_uid.clone(),
            qos_class: entry.qos_class.clone(),
            labels: entry.labels.clone(),
        })
        .collect();

    let sidecar = ContainerSidecar::new(containers);

    match sidecar.write(&sidecar_path) {
        Ok(()) => {
            tracing::debug!(
                path = %sidecar_path.display(),
                containers = sidecar.containers.len(),
                "Wrote container sidecar"
            );
        }
        Err(e) => {
            tracing::warn!(
                error = %e,
                path = %sidecar_path.display(),
                "Failed to write container sidecar"
            );
        }
    }
}

/// Write session manifest to output directory
async fn write_session_manifest(
    args: &Args,
    identifier: &str,
    run_id: &str,
    node_name: &str,
    cluster_name: &str,
) -> anyhow::Result<()> {
    let manifest = SessionManifest {
        run_id: run_id.to_string(),
        identifier: identifier.to_string(),
        start_time: Utc::now().to_rfc3339(),
        config: SessionConfig {
            sampling_interval_ms: args.interval_ms,
            rotation_seconds: args.rotation_seconds,
            compression_level: args.compression_level,
            verbose_perf_risk: args.verbose_perf_risk,
            flush_seconds: args.flush_seconds,
            expiration_seconds: args.expiration_seconds,
        },
        node_name: node_name.to_string(),
        cluster_name: cluster_name.to_string(),
        git_rev: option_env!("GIT_REV").map(String::from),
    };

    let manifest_path = args.output_dir.join("session.json");
    let json = serde_json::to_string_pretty(&manifest)?;
    tokio::fs::write(&manifest_path, json).await?;

    tracing::info!(path = %manifest_path.display(), "Wrote session manifest");
    Ok(())
}

async fn run(args: Args) -> anyhow::Result<()> {
    // Ensure output directory exists
    tokio::fs::create_dir_all(&args.output_dir)
        .await
        .context("Failed to create output directory")?;

    let run_id = Uuid::new_v4().to_string();
    let identifier = get_unique_identifier(&args);
    let node_name = args
        .node_name
        .clone()
        .or_else(|| hostname::get().ok().and_then(|h| h.into_string().ok()))
        .unwrap_or_else(|| "unknown".to_string());
    let cluster_name = args
        .cluster_name
        .clone()
        .unwrap_or_else(|| "unknown".to_string());

    tracing::info!(
        run_id = %run_id,
        identifier = %identifier,
        node = %node_name,
        cluster = %cluster_name,
        "Configured identifiers and labels"
    );

    // Write session manifest
    write_session_manifest(&args, &identifier, &run_id, &node_name, &cluster_name).await?;

    // Container metadata for sidecar generation (in-memory only, no persistence)
    let container_index = Arc::new(RwLock::new(ContainerIndex::new(args.rotation_seconds)));

    // REQ-MV-015: Initialize Kubernetes API client for pod metadata enrichment
    // Gracefully degrades if API unavailable (returns None)
    let k8s_client = KubernetesClient::try_new().await;
    let k8s_client = k8s_client.map(Arc::new);

    // Set up external shutdown coordination
    let (external_shutdown_tx, external_shutdown_rx) = watch::channel(false);
    let shutdown_requested = Arc::new(AtomicBool::new(false));
    let shutdown_requested_clone = shutdown_requested.clone();

    // Set up OS signal handlers
    let mut sigint = signal(SignalKind::interrupt())?;
    let mut sigterm = signal(SignalKind::terminate())?;

    // Spawn signal handler task
    let external_shutdown_tx_clone = external_shutdown_tx.clone();
    tokio::spawn(async move {
        tokio::select! {
            _ = sigint.recv() => {
                tracing::info!("Received SIGINT, initiating graceful shutdown");
            }
            _ = sigterm.recv() => {
                tracing::info!("Received SIGTERM, initiating graceful shutdown");
            }
        }
        shutdown_requested_clone.store(true, Ordering::SeqCst);
        let _ = external_shutdown_tx_clone.send(true);
    });

    // Spawn observer loop (runs continuously across rotations)
    let observer_shutdown = external_shutdown_rx.clone();
    let interval_ms = args.interval_ms;
    let verbose_perf_risk = args.verbose_perf_risk;
    let observer_index = container_index.clone();
    let observer_k8s_client = k8s_client.clone();
    tokio::spawn(async move {
        observer_loop(
            interval_ms,
            verbose_perf_risk,
            observer_shutdown,
            observer_index,
            observer_k8s_client,
        )
        .await;
    });

    // Run rotation loop
    run_rotation_loop(
        args,
        identifier,
        node_name,
        cluster_name,
        external_shutdown_rx,
        shutdown_requested,
        container_index,
    )
    .await
}

#[allow(clippy::too_many_arguments)]
async fn run_rotation_loop(
    args: Args,
    identifier: String,
    node_name: String,
    cluster_name: String,
    mut external_shutdown_rx: watch::Receiver<bool>,
    shutdown_requested: Arc<AtomicBool>,
    container_index: Arc<RwLock<ContainerIndex>>,
) -> anyhow::Result<()> {
    let rotation_interval = Duration::from_secs(args.rotation_seconds);
    let mut total_bytes: u64 = 0;

    // Create initial partitioned directory structure
    let partition_dir = get_partition_dir(&args.output_dir, &identifier);
    tokio::fs::create_dir_all(&partition_dir)
        .await
        .context("Failed to create partition directory")?;

    let initial_filename = generate_filename();
    let initial_output_path = partition_dir.join(&initial_filename);

    tracing::info!(
        file = %initial_output_path.display(),
        "Starting capture with initial file"
    );

    // Create signal pairs for the CaptureManager
    let (shutdown_watcher, shutdown_broadcaster) = lading_signal::signal();
    let (experiment_watcher, experiment_broadcaster) = lading_signal::signal();
    let (target_watcher, target_broadcaster) = lading_signal::signal();

    // Create a single CaptureManager with rotation support
    let mut capture_manager = lading_capture::manager::CaptureManager::new_parquet(
        initial_output_path.clone(),
        args.flush_seconds,
        args.compression_level,
        shutdown_watcher,
        experiment_watcher,
        target_watcher,
        Duration::from_secs(args.expiration_seconds),
    )
    .await
    .context("Failed to create CaptureManager")?;

    // Add global labels
    capture_manager.add_global_label("node_name", &node_name);
    capture_manager.add_global_label("cluster_name", &cluster_name);

    // Signal that experiment has started and target is running
    experiment_broadcaster.signal();
    target_broadcaster.signal();

    // Start CaptureManager with rotation support - this spawns the event loop
    // internally and returns the RotationSender and JoinHandle immediately
    let (rotation_sender, capture_manager_handle) = capture_manager
        .start_with_rotation()
        .await
        .context("Failed to start CaptureManager")?;

    tracing::info!("CaptureManager started with rotation support");

    // Run rotation loop
    let mut rotation_count = 0u64;
    let mut rotation_timer = tokio::time::interval(rotation_interval);
    rotation_timer.tick().await; // First tick is immediate, skip it

    // Track current output path for size calculations
    let mut current_output_path = initial_output_path;

    loop {
        tokio::select! {
            _ = rotation_timer.tick() => {
                rotation_count += 1;

                // Calculate size of completed file before rotation
                let file_size = match tokio::fs::metadata(&current_output_path).await {
                    Ok(metadata) => metadata.len(),
                    Err(e) => {
                        tracing::warn!(
                            error = %e,
                            file = %current_output_path.display(),
                            "Failed to get file size before rotation"
                        );
                        0
                    }
                };

                // Check total size limit before rotation
                total_bytes += file_size;
                if total_bytes >= MAX_TOTAL_BYTES {
                    tracing::warn!(
                        total_bytes = total_bytes,
                        limit_bytes = MAX_TOTAL_BYTES,
                        "Total size limit reached, initiating shutdown"
                    );
                    break;
                }

                // Prepare new file path (may cross date boundary)
                let partition_dir = get_partition_dir(&args.output_dir, &identifier);
                if let Err(e) = tokio::fs::create_dir_all(&partition_dir).await {
                    tracing::error!(error = %e, "Failed to create partition directory");
                    continue;
                }

                let new_filename = generate_filename();
                let new_output_path = partition_dir.join(&new_filename);

                // Send rotation request and wait for completion
                let (response_tx, response_rx) = tokio::sync::oneshot::channel();
                let rotation_request = lading_capture::manager::RotationRequest {
                    path: new_output_path.clone(),
                    response: response_tx,
                };

                if let Err(e) = rotation_sender.send(rotation_request).await {
                    tracing::error!(error = %e, "Failed to send rotation request - CaptureManager may have exited");
                    break;
                }

                // Wait for rotation to complete
                match response_rx.await {
                    Ok(Ok(())) => {
                        tracing::info!(
                            rotation = rotation_count,
                            old_file = %current_output_path.display(),
                            new_file = %new_output_path.display(),
                            total_bytes = total_bytes,
                            "File rotation completed"
                        );

                        // Write container sidecar for the completed file
                        // (must happen before reassigning current_output_path)
                        if let Ok(index) = container_index.read() {
                            write_container_sidecar(&current_output_path, &index);
                        }

                        current_output_path = new_output_path;
                    }
                    Ok(Err(e)) => {
                        tracing::error!(
                            error = %e,
                            rotation = rotation_count,
                            "Rotation failed"
                        );
                        // Continue with old file, next rotation will retry
                    }
                    Err(_) => {
                        tracing::error!("Rotation response channel closed - CaptureManager may have exited");
                        break;
                    }
                }
            }

            _ = external_shutdown_rx.changed() => {
                if *external_shutdown_rx.borrow() {
                    tracing::info!("External shutdown received");
                    break;
                }
            }
        }

        // Check external shutdown
        if shutdown_requested.load(Ordering::SeqCst) {
            tracing::info!("Shutdown requested, exiting rotation loop");
            break;
        }
    }

    // Graceful shutdown: signal CaptureManager and wait for it to drain all
    // buffered metrics. This ensures short-lived container data in the 60-tick
    // accumulator window is flushed to disk rather than lost on shutdown.
    tracing::info!("Signaling CaptureManager shutdown and waiting for drain");
    shutdown_broadcaster.signal_and_wait().await;

    // Also await the JoinHandle to ensure the spawned task has fully exited
    if let Err(e) = capture_manager_handle.await {
        tracing::warn!(error = %e, "CaptureManager task panicked during shutdown");
    }
    tracing::info!("CaptureManager shutdown complete");

    // Final file size calculation
    if let Ok(metadata) = tokio::fs::metadata(&current_output_path).await {
        total_bytes += metadata.len();
    }

    tracing::info!(
        total_bytes = total_bytes,
        rotations = rotation_count,
        "Shutdown complete"
    );
    Ok(())
}

/// Kubernetes metadata refresh interval (30 seconds per design.md)
const K8S_REFRESH_INTERVAL_SECS: u64 = 30;

/// Minimum time between eager K8s refreshes (to avoid hammering the API)
const K8S_EAGER_REFRESH_COOLDOWN_SECS: u64 = 5;

/// Main observer loop: discovers containers and samples metrics at the configured interval
async fn observer_loop(
    interval_ms: u64,
    verbose_perf_risk: bool,
    mut shutdown_rx: watch::Receiver<bool>,
    container_index: Arc<RwLock<ContainerIndex>>,
    k8s_client: Option<Arc<KubernetesClient>>,
) {
    let mut interval = tokio::time::interval(Duration::from_millis(interval_ms));
    let mut observer = observer::Observer::new();

    // REQ-MV-015: Track last Kubernetes metadata refresh
    let mut last_k8s_refresh = std::time::Instant::now()
        .checked_sub(Duration::from_secs(K8S_REFRESH_INTERVAL_SECS))
        .unwrap_or_else(std::time::Instant::now);

    // Track last eager refresh to enforce cooldown
    let mut last_eager_refresh = std::time::Instant::now()
        .checked_sub(Duration::from_secs(K8S_EAGER_REFRESH_COOLDOWN_SECS))
        .unwrap_or_else(std::time::Instant::now);

    loop {
        tokio::select! {
            _ = interval.tick() => {
                // REQ-MV-015: Refresh Kubernetes metadata periodically
                if let Some(ref client) = k8s_client {
                    let now = std::time::Instant::now();
                    if now.duration_since(last_k8s_refresh) >= Duration::from_secs(K8S_REFRESH_INTERVAL_SECS) {
                        if let Err(e) = client.refresh().await {
                            tracing::warn!(error = %e, "Failed to refresh Kubernetes pod metadata");
                        }
                        last_k8s_refresh = now;
                    }
                }

                // Discover all running containers via cgroup scan
                let mut containers = discovery::scan_cgroups();

                if containers.is_empty() {
                    tracing::debug!("No containers discovered");
                    continue;
                }

                // REQ-MV-015: Enrich containers with Kubernetes metadata
                if let Some(ref client) = k8s_client {
                    let metadata_cache = client.get_cache().await;
                    for container in &mut containers {
                        if let Some(metadata) = metadata_cache.get(&container.id) {
                            container.pod_name = Some(metadata.pod_name.clone());
                            container.container_name = Some(metadata.container_name.clone());
                            container.namespace = Some(metadata.namespace.clone());
                            container.labels = Some(metadata.labels.clone());
                        }
                    }

                    // Check for unenriched containers (new containers not yet in K8s cache)
                    let unenriched_count = containers
                        .iter()
                        .filter(|c| c.pod_name.is_none())
                        .count();

                    // Eager refresh: if we have unenriched containers and cooldown has passed,
                    // immediately query K8s API to get their metadata
                    if unenriched_count > 0 {
                        let now = std::time::Instant::now();
                        if now.duration_since(last_eager_refresh)
                            >= Duration::from_secs(K8S_EAGER_REFRESH_COOLDOWN_SECS)
                        {
                            tracing::debug!(
                                unenriched = unenriched_count,
                                "Triggering eager K8s refresh for new containers"
                            );
                            if let Err(e) = client.refresh().await {
                                tracing::warn!(error = %e, "Failed eager K8s refresh");
                            } else {
                                last_eager_refresh = now;
                                last_k8s_refresh = now;

                                // Re-enrich containers with fresh metadata
                                let fresh_cache = client.get_cache().await;
                                let mut enriched = 0;
                                for container in &mut containers {
                                    if container.pod_name.is_none() {
                                        if let Some(metadata) = fresh_cache.get(&container.id) {
                                            container.pod_name = Some(metadata.pod_name.clone());
                                            container.container_name =
                                                Some(metadata.container_name.clone());
                                            container.namespace = Some(metadata.namespace.clone());
                                            container.labels = Some(metadata.labels.clone());
                                            enriched += 1;
                                        }
                                    }
                                }
                                tracing::debug!(
                                    enriched = enriched,
                                    remaining = unenriched_count - enriched,
                                    "Eager refresh completed"
                                );
                            }
                        }
                    }
                }

                tracing::debug!(count = containers.len(), "Discovered containers");

                // Update container index with discovered containers (for sidecar generation)
                if let Ok(mut index) = container_index.write() {
                    index.update(&containers);
                }

                // Sample metrics for all containers
                observer.sample(&containers, verbose_perf_risk).await;
            }

            _ = shutdown_rx.changed() => {
                if *shutdown_rx.borrow() {
                    tracing::info!("Observer loop shutting down");
                    break;
                }
            }
        }
    }
}
