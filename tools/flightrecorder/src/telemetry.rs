//! DogStatsD telemetry reporter for the flightrecorder sidecar.
//!
//! Periodically reads writer stats and jemalloc memory stats, then emits them
//! as DogStatsD metrics to the agent's DogStatsD server (typically at
//! 127.0.0.1:8125 within the same pod).

use std::net::UdpSocket;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;

use cadence::prelude::*;
use cadence::{StatsdClient, UdpMetricSink};
use tikv_jemalloc_ctl::raw;
use tokio::sync::Mutex;
use tracing::warn;

use crate::writers::logs::LogsWriter;
use crate::writers::metrics::MetricsWriter;
use crate::writers::trace_stats::TraceStatsWriter;

/// Shared counters for connection-level telemetry.
pub struct ConnectionStats {
    pub active_connections: AtomicU64,
    pub frames_received: AtomicU64,
}

impl ConnectionStats {
    pub fn new() -> Self {
        Self {
            active_connections: AtomicU64::new(0),
            frames_received: AtomicU64::new(0),
        }
    }
}

/// Background task that emits sidecar telemetry via DogStatsD.
pub struct TelemetryReporter {
    client: StatsdClient,
    interval: Duration,
}

impl TelemetryReporter {
    /// Create a new reporter targeting the given DogStatsD host:port.
    /// Returns None if the host is empty (telemetry disabled).
    pub fn new(host: &str, port: u16) -> Option<Self> {
        if host.is_empty() {
            return None;
        }
        let addr = format!("{host}:{port}");
        let socket = match UdpSocket::bind("0.0.0.0:0") {
            Ok(s) => s,
            Err(e) => {
                warn!("telemetry: failed to bind UDP socket: {e}");
                return None;
            }
        };
        // Non-blocking so we never stall the reporter loop.
        socket.set_nonblocking(true).ok();
        let sink = UdpMetricSink::from(addr.as_str(), socket).ok()?;
        let client = StatsdClient::from_sink("flightrecorder.sidecar", sink);
        Some(Self {
            client,
            interval: Duration::from_secs(10),
        })
    }

    /// Run the reporter until the cancellation token fires.
    pub async fn run(
        self,
        cancel: tokio_util::sync::CancellationToken,
        metrics_writer: Arc<Mutex<MetricsWriter>>,
        logs_writer: Arc<Mutex<LogsWriter>>,
        trace_stats_writer: Arc<Mutex<TraceStatsWriter>>,
        conn_stats: Arc<ConnectionStats>,
    ) {
        let mut interval = tokio::time::interval(self.interval);
        // Track previous counter values for delta reporting.
        let mut prev_metrics_flush_count: u64 = 0;
        let mut prev_logs_flush_count: u64 = 0;
        let mut prev_tss_flush_count: u64 = 0;
        let mut prev_metrics_flush_bytes: u64 = 0;
        let mut prev_logs_flush_bytes: u64 = 0;
        let mut prev_tss_flush_bytes: u64 = 0;
        let mut prev_frames: u64 = 0;

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                _ = interval.tick() => {}
            }

            // --- Jemalloc memory stats ---
            // These don't require profiling to be enabled.
            if let Ok(resident) = unsafe { raw::read::<usize>(b"stats.resident\0") } {
                let _ = self.client.gauge("memory.resident", resident as u64);
            }
            if let Ok(allocated) = unsafe { raw::read::<usize>(b"stats.allocated\0") } {
                let _ = self.client.gauge("memory.allocated", allocated as u64);
            }
            if let Ok(active) = unsafe { raw::read::<usize>(b"stats.active\0") } {
                let _ = self.client.gauge("memory.active", active as u64);
            }

            // Need to call epoch.advance() to refresh jemalloc cached stats.
            // This is a global operation but lightweight.
            unsafe { let _ = raw::write(b"epoch\0", 1u64); }

            // --- Connection stats ---
            let conns = conn_stats.active_connections.load(Ordering::Relaxed);
            let _ = self.client.gauge("connections", conns);

            let frames = conn_stats.frames_received.load(Ordering::Relaxed);
            let _ = self.client.count("frames_received", (frames - prev_frames) as i64);
            prev_frames = frames;

            // --- Per-writer stats (lock briefly to read counters) ---
            {
                let mw = metrics_writer.lock().await;
                let _ = self.client.gauge_with_tags("buffered_rows", mw.len() as u64)
                    .with_tag("writer", "metrics").send();
                let _ = self.client.count_with_tags("flush_count", (mw.flush_count - prev_metrics_flush_count) as i64)
                    .with_tag("writer", "metrics").send();
                let _ = self.client.count_with_tags("flush_bytes", (mw.flush_bytes - prev_metrics_flush_bytes) as i64)
                    .with_tag("writer", "metrics").send();
                let _ = self.client.gauge_with_tags("flush_duration_ns", mw.last_flush_duration_ns)
                    .with_tag("writer", "metrics").send();
                prev_metrics_flush_count = mw.flush_count;
                prev_metrics_flush_bytes = mw.flush_bytes;
            }
            {
                let lw = logs_writer.lock().await;
                let _ = self.client.gauge_with_tags("buffered_rows", lw.len() as u64)
                    .with_tag("writer", "logs").send();
                let _ = self.client.count_with_tags("flush_count", (lw.flush_count - prev_logs_flush_count) as i64)
                    .with_tag("writer", "logs").send();
                let _ = self.client.count_with_tags("flush_bytes", (lw.flush_bytes - prev_logs_flush_bytes) as i64)
                    .with_tag("writer", "logs").send();
                let _ = self.client.gauge_with_tags("flush_duration_ns", lw.last_flush_duration_ns)
                    .with_tag("writer", "logs").send();
                prev_logs_flush_count = lw.flush_count;
                prev_logs_flush_bytes = lw.flush_bytes;
            }
            {
                let tw = trace_stats_writer.lock().await;
                let _ = self.client.gauge_with_tags("buffered_rows", tw.len() as u64)
                    .with_tag("writer", "trace_stats").send();
                let _ = self.client.count_with_tags("flush_count", (tw.flush_count - prev_tss_flush_count) as i64)
                    .with_tag("writer", "trace_stats").send();
                let _ = self.client.count_with_tags("flush_bytes", (tw.flush_bytes - prev_tss_flush_bytes) as i64)
                    .with_tag("writer", "trace_stats").send();
                let _ = self.client.gauge_with_tags("flush_duration_ns", tw.last_flush_duration_ns)
                    .with_tag("writer", "trace_stats").send();
                prev_tss_flush_count = tw.flush_count;
                prev_tss_flush_bytes = tw.flush_bytes;
            }
        }
    }
}
