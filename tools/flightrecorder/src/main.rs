#[global_allocator]
static GLOBAL: tikv_jemallocator::Jemalloc = tikv_jemallocator::Jemalloc;

mod config;
mod framing;
mod generated;
mod heap_prof;
mod janitor;
mod signal_files;
pub mod telemetry;
mod transport;
mod writers;

use std::time::Duration;

use std::sync::atomic::Ordering;
use std::sync::Arc;

use anyhow::Result;
use clap::Parser;
use tokio::io::BufReader;
use tokio::net::UnixStream;
use tokio::signal::unix::{signal, SignalKind};
use tracing::{error, info, warn};

use generated::signals_generated::signals;

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
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
        retention_hours = cfg.retention_hours,
        max_disk_mb = cfg.max_disk_mb,
        "flightrecorder starting"
    );

    std::fs::create_dir_all(&cfg.output_dir)?;

    let cancel = tokio_util::sync::CancellationToken::new();
    let janitor = janitor::Janitor::new(
        &cfg.output_dir,
        Duration::from_secs(cfg.retention_hours * 3600),
        cfg.max_disk_mb * 1024 * 1024,
    );
    let janitor_cancel = cancel.clone();
    let janitor_handle = tokio::spawn(async move { janitor.run(janitor_cancel).await });

    let result = run(cfg).await;

    cancel.cancel();
    let _ = janitor_handle.await;

    result
}

pub async fn run(cfg: config::Config) -> Result<()> {
    let flush_interval = Duration::from_secs(cfg.flush_interval_secs);

    let context_store = if !cfg.inline_contexts {
        Some(Arc::new(tokio::sync::Mutex::new(
            writers::context_store::ContextStore::new(&cfg.output_dir)?,
        )))
    } else {
        None
    };

    let metrics_writer = std::sync::Arc::new(tokio::sync::Mutex::new(
        writers::metrics::MetricsWriter::new(
            &cfg.output_dir,
            cfg.flush_rows,
            flush_interval,
            cfg.inline_contexts,
            context_store.clone(),
        ),
    ));
    let logs_writer = std::sync::Arc::new(tokio::sync::Mutex::new(
        writers::logs::LogsWriter::new(&cfg.output_dir, cfg.flush_rows, flush_interval),
    ));
    let trace_stats_writer = std::sync::Arc::new(tokio::sync::Mutex::new(
        writers::trace_stats::TraceStatsWriter::new(&cfg.output_dir, cfg.flush_rows, flush_interval),
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
        let mw = metrics_writer.clone();
        let lw = logs_writer.clone();
        let tw = trace_stats_writer.clone();
        let cs = conn_stats.clone();
        Some(tokio::spawn(async move {
            reporter.run(tc, mw, lw, tw, cs).await;
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

        let mw = metrics_writer.clone();
        let lw = logs_writer.clone();
        let tw = trace_stats_writer.clone();
        let token = cancel.clone();
        let cs = conn_stats.clone();
        let ctx_store = context_store.clone();

        cs.active_connections.fetch_add(1, Ordering::Relaxed);

        client_tasks.push(tokio::spawn(async move {
            // New connection — agent will re-send all context definitions.
            mw.lock().await.reset_context_map();
            if let Some(store) = &ctx_store {
                if let Err(e) = store.lock().await.reset() {
                    warn!("context store reset error: {}", e);
                }
            }

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
                        if let Some(batch) = env.payload_as_metric_batch() {
                            if let Err(e) = mw.lock().await.push(&batch).await {
                                warn!("metrics writer error: {}", e);
                            }
                        }
                    }
                    signals::SignalPayload::LogBatch => {
                        if let Some(batch) = env.payload_as_log_batch() {
                            if let Err(e) = lw.lock().await.push(&batch).await {
                                warn!("logs writer error: {}", e);
                            }
                        }
                    }
                    signals::SignalPayload::TraceStatsBatch => {
                        if let Some(batch) = env.payload_as_trace_stats_batch() {
                            if let Err(e) = tw.lock().await.push(&batch).await {
                                warn!("trace stats writer error: {}", e);
                            }
                        }
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

    // Final flush on shutdown.
    if let Err(e) = metrics_writer.lock().await.flush_if_any().await {
        warn!("final metrics flush error: {}", e);
    }
    if let Err(e) = logs_writer.lock().await.flush_if_any().await {
        warn!("final logs flush error: {}", e);
    }
    if let Err(e) = trace_stats_writer.lock().await.flush_if_any().await {
        warn!("final trace stats flush error: {}", e);
    }

    info!("flightrecorder stopped");
    Ok(())
}
