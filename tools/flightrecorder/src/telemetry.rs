//! DogStatsD telemetry reporter for the flightrecorder sidecar.
//!
//! Periodically reads writer stats (from lock-free atomics) and jemalloc
//! memory stats, then emits them as DogStatsD metrics to the agent's
//! DogStatsD server (typically at 127.0.0.1:8125 within the same pod).
//!
//! Origin detection: reads `DD_ENTITY_ID` (injected by the Datadog admission
//! controller) and includes `dd.internal.entity_id:<pod-uid>` on every metric
//! so the agent can enrich with pod_name, kube_namespace, etc.

use std::net::UdpSocket;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;

use cadence::prelude::*;
use cadence::{StatsdClient, UdpMetricSink};
use tikv_jemalloc_ctl::raw;
use tracing::{info, warn};

use crate::writers::thread::WriterTelemetry;

/// DogStatsD tag prefix recognized by the agent's origin detection.
const ENTITY_ID_TAG_PREFIX: &str = "dd.internal.entity_id:";

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
    /// Origin detection tag: `dd.internal.entity_id:<pod-uid>`.
    /// Empty if DD_ENTITY_ID is not set (origin detection disabled).
    entity_id_tag: String,
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

        // Read DD_ENTITY_ID (injected by Datadog admission controller) for
        // origin detection. Without this, metrics show pod_name:N/A.
        let entity_id_tag = match std::env::var("DD_ENTITY_ID") {
            Ok(uid) if !uid.is_empty() => {
                let tag = format!("{ENTITY_ID_TAG_PREFIX}{uid}");
                info!(tag = %tag, "origin detection enabled via DD_ENTITY_ID");
                tag
            }
            _ => {
                warn!("DD_ENTITY_ID not set — sidecar metrics will not have pod_name tags");
                String::new()
            }
        };

        Some(Self {
            client,
            interval: Duration::from_secs(10),
            entity_id_tag,
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
                self.send_gauge("memory.resident", v as u64);
            } else {
                jemalloc_ok = false;
            }
            if let Ok(v) = unsafe { raw::read::<usize>(b"stats.allocated\0") } {
                self.send_gauge("memory.allocated", v as u64);
            } else {
                jemalloc_ok = false;
            }
            if let Ok(v) = unsafe { raw::read::<usize>(b"stats.active\0") } {
                self.send_gauge("memory.active", v as u64);
            } else {
                jemalloc_ok = false;
            }
            if !jemalloc_ok && !jemalloc_warned {
                warn!("jemalloc stats unavailable (built without stats feature?), memory metrics disabled");
                jemalloc_warned = true;
            }

            // --- Connection stats ---
            let conns = conn_stats.active_connections.load(Ordering::Relaxed);
            self.send_gauge("connections", conns);

            let frames = conn_stats.frames_received.load(Ordering::Relaxed);
            self.send_count("frames_received", (frames - prev_frames) as i64);
            prev_frames = frames;

            // --- Per-writer stats (read from lock-free atomics) ---
            self.report_writer(
                "metrics", &metrics_telemetry,
                &mut prev_metrics_flush_count, &mut prev_metrics_flush_bytes, &mut prev_metrics_rows,
            );
            self.report_writer(
                "logs", &logs_telemetry,
                &mut prev_logs_flush_count, &mut prev_logs_flush_bytes, &mut prev_logs_rows,
            );
            self.report_writer(
                "trace_stats", &trace_stats_telemetry,
                &mut prev_tss_flush_count, &mut prev_tss_flush_bytes, &mut prev_tss_rows,
            );
        }
    }

    /// Send a gauge with the entity ID tag for origin detection.
    fn send_gauge(&self, key: &str, value: u64) {
        let mut b = self.client.gauge_with_tags(key, value);
        if !self.entity_id_tag.is_empty() {
            b = b.with_tag_value(&self.entity_id_tag);
        }
        let _ = b.send();
    }

    /// Send a count with the entity ID tag for origin detection.
    fn send_count(&self, key: &str, value: i64) {
        let mut b = self.client.count_with_tags(key, value);
        if !self.entity_id_tag.is_empty() {
            b = b.with_tag_value(&self.entity_id_tag);
        }
        let _ = b.send();
    }

    fn report_writer(
        &self,
        writer_tag: &str,
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

        let eid = &self.entity_id_tag;
        let has_eid = !eid.is_empty();

        let mut b = self.client.gauge_with_tags("buffered_rows", buffered)
            .with_tag("writer", writer_tag);
        if has_eid { b = b.with_tag_value(eid); }
        let _ = b.send();

        let mut b = self.client.count_with_tags("flush_count", (flush_count - *prev_flush_count) as i64)
            .with_tag("writer", writer_tag);
        if has_eid { b = b.with_tag_value(eid); }
        let _ = b.send();

        let mut b = self.client.count_with_tags("flush_bytes", (flush_bytes - *prev_flush_bytes) as i64)
            .with_tag("writer", writer_tag);
        if has_eid { b = b.with_tag_value(eid); }
        let _ = b.send();

        let mut b = self.client.count_with_tags("rows_written", (rows - *prev_rows) as i64)
            .with_tag("writer", writer_tag);
        if has_eid { b = b.with_tag_value(eid); }
        let _ = b.send();

        let mut b = self.client.gauge_with_tags("flush_duration_ns", flush_ns)
            .with_tag("writer", writer_tag);
        if has_eid { b = b.with_tag_value(eid); }
        let _ = b.send();

        let fill_pct = telemetry.rtrb_ring_fill_pct.load(Ordering::Relaxed);
        let mut b = self.client.gauge_with_tags("rtrb_ring_fill_pct", fill_pct)
            .with_tag("writer", writer_tag);
        if has_eid { b = b.with_tag_value(eid); }
        let _ = b.send();

        *prev_flush_count = flush_count;
        *prev_flush_bytes = flush_bytes;
        *prev_rows = rows;
    }
}
