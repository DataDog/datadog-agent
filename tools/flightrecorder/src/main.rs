#[global_allocator]
static GLOBAL: tikv_jemallocator::Jemalloc = tikv_jemallocator::Jemalloc;

mod archive;
mod config;
mod context_parquet;
mod disk_tracker;
mod framing;
mod generated;
mod heap_prof;
mod janitor;
#[cfg(feature = "s3")]
mod s3_uploader;
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

/// Signal type determined by peeking at the FlatBuffers envelope.
enum Signal {
    Metrics,
    Logs,
    TraceStats,
    Empty,
}

use flightrecorder::BufferPool;

/// Handle a single client connection: read frames and route to writer threads.
async fn handle_connection(
    stream: UnixStream,
    mh: Arc<Mutex<WriterHandle>>,
    lh: Arc<Mutex<WriterHandle>>,
    th: Arc<Mutex<WriterHandle>>,
    token: tokio_util::sync::CancellationToken,
    cs: Arc<telemetry::ConnectionStats>,
    pool: BufferPool,
) {
    let mut reader = BufReader::new(stream);
    let mut buf = pool.take();
    loop {
        let frame_result = tokio::select! {
            _ = token.cancelled() => break,
            r = framing::read_frame(&mut reader, &mut buf) => r,
        };

        match frame_result {
            Ok(None) => {
                info!("client disconnected");
                break;
            }
            Ok(Some(_len)) => {}
            Err(e) => {
                warn!("frame read error: {}", e);
                break;
            }
        }

        // Peek at the envelope to determine the signal type, then drop
        // the borrow so we can move the buffer into the writer thread.
        let signal = match flatbuffers::root::<signals::SignalEnvelope>(&buf) {
            Ok(env) => {
                if env.metric_batch().is_some() {
                    Signal::Metrics
                } else if env.log_batch().is_some() {
                    Signal::Logs
                } else if env.trace_stats_batch().is_some() {
                    Signal::TraceStats
                } else {
                    Signal::Empty
                }
            }
            Err(e) => {
                warn!("failed to decode SignalEnvelope: {}", e);
                continue;
            }
        };

        cs.frames_received.fetch_add(1, Ordering::Relaxed);

        // Move the buffer into the ring — take a fresh one from the pool.
        let frame = std::mem::replace(&mut buf, pool.take());
        match signal {
            Signal::Metrics => mh.lock().unwrap().send_frame(frame),
            Signal::Logs => lh.lock().unwrap().send_frame(frame),
            Signal::TraceStats => th.lock().unwrap().send_frame(frame),
            Signal::Empty => warn!("empty SignalEnvelope (no batch field set)"),
        }
    }
    // Return our last buffer to the pool.
    pool.put(buf);
    cs.active_connections.fetch_sub(1, Ordering::Relaxed);
}

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

    if let Some(config::Commands::Archive { input_dir, output }) = &cfg.command {
        return archive::run(
            input_dir
                .as_deref()
                .unwrap_or_else(|| std::path::Path::new(&cfg.output_dir)),
            output.as_deref(),
        );
    }

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

    // Periodic heap profiling (every 30s when MALLOC_CONF=prof:true).
    let prof_cancel = cancel.clone();
    let prof_output_dir = cfg.output_dir.clone();
    let prof_handle = tokio::spawn(async move {
        let mut tick = 0u64;
        loop {
            tokio::select! {
                _ = prof_cancel.cancelled() => break,
                _ = tokio::time::sleep(std::time::Duration::from_secs(30)) => {
                    tick += 1;
                    heap_prof::dump_heap_profile(
                        std::path::Path::new(&prof_output_dir),
                        &format!("heap-{tick:04}"),
                    );
                    // Log jemalloc memory stats via raw ctl reads.
                    if let (Ok(allocated), Ok(resident)) = unsafe {(
                        tikv_jemalloc_ctl::raw::read::<usize>(b"stats.allocated\0"),
                        tikv_jemalloc_ctl::raw::read::<usize>(b"stats.resident\0"),
                    )} {
                        info!(
                            allocated_mb = allocated / (1024 * 1024),
                            resident_mb = resident / (1024 * 1024),
                            "jemalloc stats"
                        );
                    }
                }
            }
        }
    });

    let result = run(cfg, tracker).await;

    cancel.cancel();
    let _ = janitor_handle.await;
    let _ = prof_handle.await;

    result
}

pub async fn run(cfg: config::Config, tracker: Arc<disk_tracker::DiskTracker>) -> Result<()> {
    let flush_interval = Duration::from_secs(cfg.flush_interval_secs);
    let s3_cancel = tokio_util::sync::CancellationToken::new();

    // Initialize S3 uploader (if configured).
    #[cfg(feature = "s3")]
    let s3_upload_handle = if cfg.s3_enabled() {
        let key_prefix = cfg.s3_key_prefix();
        info!(
            bucket = %cfg.s3_bucket,
            region = %cfg.s3_region,
            key_prefix = %key_prefix,
            "S3 upload enabled"
        );
        let (uploader, handle) = s3_uploader::new_s3_uploader(
            cfg.s3_bucket.clone(),
            cfg.s3_region.clone(),
            key_prefix,
            tracker.clone(),
        )?;

        tracker.set_upload_handle(handle.clone());

        let sc = s3_cancel.clone();
        tokio::spawn(async move { uploader.run(sc).await });

        Some(handle)
    } else {
        info!("S3 upload disabled (no bucket configured)");
        None
    };

    // Build context upload config (if S3 enabled).
    #[cfg(feature = "s3")]
    let ctx_upload_config = s3_upload_handle.map(|h| {
        writers::context_writer::ContextUploadConfig {
            upload_handle: h,
            output_dir: std::path::PathBuf::from(&cfg.output_dir),
        }
    });
    #[cfg(not(feature = "s3"))]
    let ctx_upload_config: Option<writers::context_writer::ContextUploadConfig> = None;

    // Create shared context writer thread (serves both metrics and logs).
    let context_store = writers::context_store::ContextStore::new(&cfg.output_dir)?;
    let (mut ctx_handle, ctx_prod_metrics, ctx_prod_logs) =
        writers::context_writer::ContextWriterHandle::spawn(context_store, 1024, ctx_upload_config);

    // Create signal writers and spawn dedicated threads.
    let rotation_interval = Duration::from_secs(cfg.rotation_secs);
    let metrics_writer = writers::metrics::MetricsWriter::new(
        &cfg.output_dir,
        cfg.flush_rows,
        flush_interval,
        rotation_interval,
        ctx_prod_metrics,
        tracker.clone(),
    );
    let buffer_pool = BufferPool::new();
    let logs_writer = writers::logs::LogsWriter::new(
        &cfg.output_dir,
        cfg.flush_rows,
        flush_interval,
        rotation_interval,
        ctx_prod_logs,
        tracker.clone(),
        buffer_pool.clone(),
    );
    let trace_stats_writer =
        writers::trace_stats::TraceStatsWriter::new(&cfg.output_dir, cfg.flush_rows, flush_interval, rotation_interval, tracker.clone());

    let metrics_handle = Arc::new(Mutex::new(
        WriterHandle::spawn(metrics_writer, WRITER_RING_CAPACITY, "metrics", buffer_pool.clone()),
    ));
    let logs_handle = Arc::new(Mutex::new(
        WriterHandle::spawn(logs_writer, WRITER_RING_CAPACITY, "logs", buffer_pool.clone()),
    ));
    let traces_handle = Arc::new(Mutex::new(
        WriterHandle::spawn(trace_stats_writer, WRITER_RING_CAPACITY, "traces", buffer_pool.clone()),
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

        client_tasks.push(tokio::spawn(
            handle_connection(stream, mh, lh, th, token, cs, buffer_pool.clone()),
        ));

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

    // Shut down signal writer threads first (drain rings, flush, close files).
    metrics_handle.lock().unwrap().shutdown();
    logs_handle.lock().unwrap().shutdown();
    traces_handle.lock().unwrap().shutdown();
    // Context writer last — signal writers may have sent final context records.
    ctx_handle.shutdown();

    // Cancel S3 uploader after all writers have flushed their final files.
    s3_cancel.cancel();

    info!("flightrecorder stopped");
    Ok(())
}
