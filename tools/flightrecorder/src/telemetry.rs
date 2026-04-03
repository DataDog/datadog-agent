//! DogStatsD telemetry reporter for the flightrecorder sidecar.
//!
//! Periodically reads writer stats (from lock-free atomics) and jemalloc
//! memory stats, then emits them as DogStatsD metrics to the agent's
//! DogStatsD server (typically at 127.0.0.1:8125 within the same pod).

use std::net::UdpSocket;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;

use cadence::prelude::*;
use cadence::{StatsdClient, UdpMetricSink};
use tikv_jemalloc_ctl::raw;
use tracing::warn;

use crate::writers::thread::WriterTelemetry;

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
    ///
    /// All writer stats are read from lock-free [`WriterTelemetry`] atomics —
    /// no mutex acquisition needed.
    pub async fn run(
        self,
        cancel: tokio_util::sync::CancellationToken,
        metrics_telemetry: Arc<WriterTelemetry>,
        logs_telemetry: Arc<WriterTelemetry>,
        trace_stats_telemetry: Arc<WriterTelemetry>,
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
        let mut prev_metrics_rows: u64 = 0;
        let mut prev_logs_rows: u64 = 0;
        let mut prev_tss_rows: u64 = 0;
        let mut prev_frames: u64 = 0;
        let mut jemalloc_warned = false;

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                _ = interval.tick() => {}
            }

            // --- Jemalloc memory stats ---
            unsafe { let _ = raw::write(b"epoch\0", 1u64); }

            let mut jemalloc_ok = true;
            if let Ok(v) = unsafe { raw::read::<usize>(b"stats.resident\0") } {
                let _ = self.client.gauge("memory.resident", v as u64);
            } else {
                jemalloc_ok = false;
            }
            if let Ok(v) = unsafe { raw::read::<usize>(b"stats.allocated\0") } {
                let _ = self.client.gauge("memory.allocated", v as u64);
            } else {
                jemalloc_ok = false;
            }
            if let Ok(v) = unsafe { raw::read::<usize>(b"stats.active\0") } {
                let _ = self.client.gauge("memory.active", v as u64);
            } else {
                jemalloc_ok = false;
            }
            if !jemalloc_ok && !jemalloc_warned {
                warn!("jemalloc stats unavailable (built without stats feature?), memory metrics disabled");
                jemalloc_warned = true;
            }

            // --- Connection stats ---
            let conns = conn_stats.active_connections.load(Ordering::Relaxed);
            let _ = self.client.gauge("connections", conns);

            let frames = conn_stats.frames_received.load(Ordering::Relaxed);
            let _ = self.client.count("frames_received", (frames - prev_frames) as i64);
            prev_frames = frames;

            // --- Per-writer stats (read from lock-free atomics) ---
            Self::report_writer(
                &self.client, "metrics", &metrics_telemetry,
                &mut prev_metrics_flush_count, &mut prev_metrics_flush_bytes, &mut prev_metrics_rows,
            );
            Self::report_writer(
                &self.client, "logs", &logs_telemetry,
                &mut prev_logs_flush_count, &mut prev_logs_flush_bytes, &mut prev_logs_rows,
            );
            Self::report_writer(
                &self.client, "trace_stats", &trace_stats_telemetry,
                &mut prev_tss_flush_count, &mut prev_tss_flush_bytes, &mut prev_tss_rows,
            );
        }
    }

    fn report_writer(
        client: &StatsdClient,
        tag: &str,
        telemetry: &WriterTelemetry,
        prev_flush_count: &mut u64,
        prev_flush_bytes: &mut u64,
        prev_rows: &mut u64,
    ) {
        let flush_count = telemetry.flush_count.load(Ordering::Relaxed);
        let flush_bytes = telemetry.flush_bytes.load(Ordering::Relaxed);
        let rows = telemetry.rows_written.load(Ordering::Relaxed);
        let buffered = telemetry.buffered_rows.load(Ordering::Relaxed);
        let flush_ns = telemetry.last_flush_duration_ns.load(Ordering::Relaxed);

        let _ = client.gauge_with_tags("buffered_rows", buffered)
            .with_tag("writer", tag).send();
        let _ = client.count_with_tags("flush_count", (flush_count - *prev_flush_count) as i64)
            .with_tag("writer", tag).send();
        let _ = client.count_with_tags("flush_bytes", (flush_bytes - *prev_flush_bytes) as i64)
            .with_tag("writer", tag).send();
        let _ = client.count_with_tags("rows_written", (rows - *prev_rows) as i64)
            .with_tag("writer", tag).send();
        let _ = client.gauge_with_tags("flush_duration_ns", flush_ns)
            .with_tag("writer", tag).send();

        *prev_flush_count = flush_count;
        *prev_flush_bytes = flush_bytes;
        *prev_rows = rows;
    }
}
