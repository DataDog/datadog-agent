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
        merge_max_files = cfg.merge_max_files,
        merge_interval_secs = cfg.merge_interval_secs,
        merge_size_threshold_mb = cfg.merge_size_threshold_mb,
        "flightrecorder starting"
    );

    std::fs::create_dir_all(&cfg.output_dir)?;

    let cancel = tokio_util::sync::CancellationToken::new();
    let janitor = janitor::Janitor::new(
        &cfg.output_dir,
        Duration::from_secs(cfg.retention_hours * 3600),
        cfg.max_disk_mb * 1024 * 1024,
        cfg.merge_min_files,
        cfg.merge_max_files,
        Duration::from_secs(cfg.merge_interval_secs),
        cfg.merge_size_threshold_mb * 1024 * 1024,
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

    let mut metrics_writer = writers::metrics::MetricsWriter::new(
        &cfg.output_dir,
        cfg.flush_rows,
        flush_interval,
    );
    let mut logs_writer = writers::logs::LogsWriter::new(
        &cfg.output_dir,
        cfg.flush_rows,
        flush_interval,
    );

    let transport = transport::UnixSocketTransport::bind(&cfg.socket_path)?;
    info!(socket = %cfg.socket_path, "listening for connections");

    let mut sigterm = signal(SignalKind::terminate())?;
    let mut sigint = signal(SignalKind::interrupt())?;

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
        // New connection — agent will re-send all context definitions.
        metrics_writer.reset_context_map();

        // Read frames from this connection until EOF or shutdown.
        let mut reader = BufReader::new(stream);
        loop {
            let frame_result = tokio::select! {
                _ = sigterm.recv() => {
                    info!("received SIGTERM during connection");
                    break;
                }
                _ = sigint.recv() => {
                    info!("received SIGINT during connection");
                    break;
                }
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
                        if let Err(e) = metrics_writer.push(&batch).await {
                            warn!("metrics writer error: {}", e);
                        }
                    }
                }
                signals::SignalPayload::LogBatch => {
                    if let Some(batch) = env.payload_as_log_batch() {
                        if let Err(e) = logs_writer.push(&batch).await {
                            warn!("logs writer error: {}", e);
                        }
                    }
                }
                _ => {
                    warn!("unknown SignalPayload variant");
                }
            }
        }
    }

    // Final flush on shutdown.
    if let Err(e) = metrics_writer.flush_if_any().await {
        warn!("final metrics flush error: {}", e);
    }
    if let Err(e) = logs_writer.flush_if_any().await {
        warn!("final logs flush error: {}", e);
    }

    info!("flightrecorder stopped");
    Ok(())
}
