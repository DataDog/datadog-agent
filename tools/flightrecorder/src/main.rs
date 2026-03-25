#[global_allocator]
static GLOBAL: tikv_jemallocator::Jemalloc = tikv_jemallocator::Jemalloc;

mod config;
mod framing;
mod generated;
mod heap_prof;
mod janitor;
mod merge;
mod transport;
mod vortex_files;
mod writers;

use std::time::Duration;

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
        merge_min_files = cfg.merge_min_files,
        merge_interval_secs = cfg.merge_interval_secs,
        "flightrecorder starting"
    );

    std::fs::create_dir_all(&cfg.output_dir)?;

    let cancel = tokio_util::sync::CancellationToken::new();
    let janitor = janitor::Janitor::new(
        &cfg.output_dir,
        Duration::from_secs(cfg.retention_hours * 3600),
        cfg.max_disk_mb * 1024 * 1024,
        cfg.merge_enabled,
        cfg.merge_min_files,
        Duration::from_secs(cfg.merge_interval_secs),
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

    let metrics_writer = std::sync::Arc::new(tokio::sync::Mutex::new(
        writers::metrics::MetricsWriter::new(&cfg.output_dir, cfg.flush_rows, flush_interval),
    ));
    let logs_writer = std::sync::Arc::new(tokio::sync::Mutex::new(
        writers::logs::LogsWriter::new(&cfg.output_dir, cfg.flush_rows, flush_interval),
    ));
    let trace_stats_writer = std::sync::Arc::new(tokio::sync::Mutex::new(
        writers::trace_stats::TraceStatsWriter::new(&cfg.output_dir, cfg.flush_rows, flush_interval),
    ));

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

        client_tasks.push(tokio::spawn(async move {
            // New connection — agent will re-send all context definitions.
            mw.lock().await.reset_context_map();

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
        }));

        // Clean up completed client tasks.
        client_tasks.retain(|t| !t.is_finished());
    }

    // Signal all client tasks to stop.
    cancel.cancel();
    for task in client_tasks {
        let _ = task.await;
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
