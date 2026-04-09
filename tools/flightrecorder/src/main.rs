#[global_allocator]
static GLOBAL: tikv_jemallocator::Jemalloc = tikv_jemallocator::Jemalloc;

mod config;
mod disk_tracker;
mod framing;
mod generated;
mod heap_prof;
mod janitor;
mod signal_files;
pub mod telemetry;
mod transport;
mod writers;

use std::sync::atomic::Ordering;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::Result;
use clap::Parser;
use tokio::io::BufReader;
use tokio::net::UnixStream;
use tokio::signal::unix::{signal, SignalKind};
use tracing::{error, info, warn};

use generated::signals_generated::signals;
use writers::thread::WriterHandle;

/// Ring buffer capacity for the writer threads.
/// Each slot holds a Vec<u8> (24 bytes). With 512 slots the ring overhead
/// is ~12 KB. Worst-case heap with full ring: 512 × ~500 KB = ~256 MB.
/// This provides ~5 seconds of buffering at 100 frames/sec while bounding
/// memory if the writer thread falls behind.
const WRITER_RING_CAPACITY: usize = 512;

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_ansi(atty::is(atty::Stream::Stdout))
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive(tracing::Level::INFO.into()),
        )
        .init();

    let cfg = config::Config::parse();

    heap_prof::init();

    info!(
        socket_path = %cfg.socket_path,
        output_dir = %cfg.output_dir,
        flush_rows = cfg.flush_rows,
        flush_interval_secs = cfg.flush_interval_secs,
        max_disk_mb = cfg.max_disk_mb,
        "flightrecorder starting"
    );

    std::fs::create_dir_all(&cfg.output_dir)?;

    let cancel = tokio_util::sync::CancellationToken::new();
    let tracker = Arc::new(disk_tracker::DiskTracker::new(
        std::path::Path::new(&cfg.output_dir),
        cfg.max_disk_mb * 1024 * 1024,
    )?);
    let janitor = janitor::Janitor::new(tracker.clone());
    let janitor_cancel = cancel.clone();
    let janitor_handle = tokio::spawn(async move { janitor.run(janitor_cancel).await });

    let result = run(cfg, tracker).await;

    cancel.cancel();
    let _ = janitor_handle.await;

    result
}

pub async fn run(cfg: config::Config, tracker: Arc<disk_tracker::DiskTracker>) -> Result<()> {
    let flush_interval = Duration::from_secs(cfg.flush_interval_secs);

    // Create writers and spawn dedicated threads.
    let context_store = writers::context_store::ContextStore::new(&cfg.output_dir)?;
    let mut metrics_writer = writers::metrics::MetricsWriter::new(
        &cfg.output_dir,
        cfg.flush_rows,
        flush_interval,
        context_store,
    );
    let mut logs_writer =
        writers::logs::LogsWriter::new(&cfg.output_dir, cfg.flush_rows, flush_interval);
    let mut trace_stats_writer =
        writers::trace_stats::TraceStatsWriter::new(&cfg.output_dir, cfg.flush_rows, flush_interval);

    // Wire disk tracker into writers so file rotations are tracked in-memory.
    metrics_writer.base.set_disk_tracker(tracker.clone());
    logs_writer.base.set_disk_tracker(tracker.clone());
    trace_stats_writer.base.set_disk_tracker(tracker.clone());

    let metrics_handle = Arc::new(Mutex::new(
        WriterHandle::spawn(metrics_writer, WRITER_RING_CAPACITY, "metrics"),
    ));
    let logs_handle = Arc::new(Mutex::new(
        WriterHandle::spawn(logs_writer, WRITER_RING_CAPACITY, "logs"),
    ));
    let traces_handle = Arc::new(Mutex::new(
        WriterHandle::spawn(trace_stats_writer, WRITER_RING_CAPACITY, "traces"),
    ));

    let conn_stats = Arc::new(telemetry::ConnectionStats::new());

    // Start telemetry reporter (DogStatsD) if configured.
    let telemetry_cancel = tokio_util::sync::CancellationToken::new();
    let telemetry_handle = if let Some(reporter) =
        telemetry::TelemetryReporter::new(&cfg.statsd_host, cfg.statsd_port)
    {
        info!(
            host = %cfg.statsd_host,
            port = cfg.statsd_port,
            "telemetry reporter started"
        );
        let tc = telemetry_cancel.clone();
        let mt = metrics_handle.lock().unwrap().telemetry.clone();
        let lt = logs_handle.lock().unwrap().telemetry.clone();
        let tt = traces_handle.lock().unwrap().telemetry.clone();
        let cs = conn_stats.clone();
        Some(tokio::spawn(async move {
            reporter.run(tc, mt, lt, tt, cs).await;
        }))
    } else {
        info!("telemetry reporter disabled (empty statsd_host)");
        None
    };

    let transport = transport::UnixSocketTransport::bind(&cfg.socket_path)?;
    info!(socket = %cfg.socket_path, "listening for connections");

    let mut sigterm = signal(SignalKind::terminate())?;
    let mut sigint = signal(SignalKind::interrupt())?;

    let cancel = tokio_util::sync::CancellationToken::new();
    let mut client_tasks: Vec<tokio::task::JoinHandle<()>> = Vec::new();

    loop {
        // Wait for next connection or shutdown signal.
        let stream: UnixStream = tokio::select! {
            _ = sigterm.recv() => {
                info!("received SIGTERM, flushing and shutting down");
                break;
            }
            _ = sigint.recv() => {
                info!("received SIGINT, flushing and shutting down");
                break;
            }
            result = transport.accept_stream() => {
                match result {
                    Ok(s) => s,
                    Err(e) => {
                        error!("accept error: {}", e);
                        continue;
                    }
                }
            }
        };

        info!("client connected");

        let mh = metrics_handle.clone();
        let lh = logs_handle.clone();
        let th = traces_handle.clone();
        let token = cancel.clone();
        let cs = conn_stats.clone();
        cs.active_connections.fetch_add(1, Ordering::Relaxed);

        client_tasks.push(tokio::spawn(async move {
            // Context keys are deterministic hashes — the bloom filter
            // persists across connections. Duplicate contexts from
            // reconnecting agents are silently deduplicated.

            let mut reader = BufReader::new(stream);
            loop {
                let frame_result = tokio::select! {
                    _ = token.cancelled() => break,
                    r = framing::read_frame(&mut reader) => r,
                };

                let buf = match frame_result {
                    Ok(Some(b)) => b,
                    Ok(None) => {
                        info!("client disconnected");
                        break;
                    }
                    Err(e) => {
                        warn!("frame read error: {}", e);
                        break;
                    }
                };

                // Peek at the payload type to route to the correct writer
                // thread. The full frame (buf) is moved into the ring — the
                // writer thread decodes it.
                let env = match flatbuffers::root::<signals::SignalEnvelope>(&buf) {
                    Ok(e) => e,
                    Err(e) => {
                        warn!("failed to decode SignalEnvelope: {}", e);
                        continue;
                    }
                };

                cs.frames_received.fetch_add(1, Ordering::Relaxed);

                match env.payload_type() {
                    signals::SignalPayload::MetricBatch => {
                        mh.lock().unwrap().send_frame(buf);
                    }
                    signals::SignalPayload::LogBatch => {
                        lh.lock().unwrap().send_frame(buf);
                    }
                    signals::SignalPayload::TraceStatsBatch => {
                        th.lock().unwrap().send_frame(buf);
                    }
                    _ => {
                        warn!("unknown SignalPayload variant");
                    }
                }
            }
            cs.active_connections.fetch_sub(1, Ordering::Relaxed);
        }));

        // Clean up completed client tasks.
        client_tasks.retain(|t| !t.is_finished());
    }

    // Signal all client tasks and telemetry to stop.
    cancel.cancel();
    telemetry_cancel.cancel();
    for task in client_tasks {
        let _ = task.await;
    }
    if let Some(h) = telemetry_handle {
        let _ = h.await;
    }

    // Shut down writer threads (drain rings, flush, close files).
    metrics_handle.lock().unwrap().shutdown();
    logs_handle.lock().unwrap().shutdown();
    traces_handle.lock().unwrap().shutdown();

    info!("flightrecorder stopped");
    Ok(())
}
