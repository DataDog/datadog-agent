use std::path::{Path, PathBuf};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use vortex::array::arrays::{DictArray, PrimitiveArray, StructArray, VarBinArray};
use vortex::array::dtype::FieldNames;
use vortex::array::validity::Validity;
use vortex::array::IntoArray;
use vortex::file::VortexWriteOptions;
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

use super::apply_permutation;
use super::intern::StringInterner;
use crate::generated::signals_generated::signals::TraceStatsBatch;

/// Columnar accumulator for trace stats entries.
///
/// Trace stats are already aggregated (low volume: ~10-100 entries per 10s),
/// so no context-key deduplication or split ring buffers are needed.
pub struct TraceStatsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    // Interned string columns (dictionary-encoded on flush).
    services: StringInterner,
    names: StringInterner,
    resources: StringInterner,
    types: StringInterner,
    span_kinds: StringInterner,
    hostnames: StringInterner,
    envs: StringInterner,
    versions: StringInterner,

    // Numeric columns.
    http_status_codes: Vec<u32>,
    hits: Vec<u64>,
    errors: Vec<u64>,
    durations: Vec<u64>,
    top_level_hits: Vec<u64>,
    bucket_starts: Vec<i64>,
    bucket_durations: Vec<i64>,
    timestamps: Vec<i64>,

    // Binary columns (DDSketch protobuf).
    ok_summaries: Vec<Vec<u8>>,
    error_summaries: Vec<Vec<u8>>,

    write_buf: Vec<u8>,

    // Telemetry counters (read by the telemetry reporter).
    pub flush_count: u64,
    pub flush_bytes: u64,
    pub last_flush_duration_ns: u64,
}

impl TraceStatsWriter {
    pub fn new(
        output_dir: impl AsRef<Path>,
        flush_rows: usize,
        flush_interval: Duration,
    ) -> Self {
        let output_dir = output_dir.as_ref().to_path_buf();
        Self {
            output_dir,
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),

            services: StringInterner::with_capacity(flush_rows),
            names: StringInterner::with_capacity(flush_rows),
            resources: StringInterner::with_capacity(flush_rows),
            types: StringInterner::with_capacity(flush_rows),
            span_kinds: StringInterner::with_capacity(flush_rows),
            hostnames: StringInterner::with_capacity(flush_rows),
            envs: StringInterner::with_capacity(flush_rows),
            versions: StringInterner::with_capacity(flush_rows),

            http_status_codes: Vec::with_capacity(flush_rows),
            hits: Vec::with_capacity(flush_rows),
            errors: Vec::with_capacity(flush_rows),
            durations: Vec::with_capacity(flush_rows),
            top_level_hits: Vec::with_capacity(flush_rows),
            bucket_starts: Vec::with_capacity(flush_rows),
            bucket_durations: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),

            ok_summaries: Vec::with_capacity(flush_rows),
            error_summaries: Vec::with_capacity(flush_rows),

            write_buf: Vec::with_capacity(64 * 1024),

            flush_count: 0,
            flush_bytes: 0,
            last_flush_duration_ns: 0,
        }
    }

    /// Number of rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.timestamps.len()
    }

    /// Ingest a TraceStatsBatch from FlatBuffers. Flushes automatically when thresholds are reached.
    pub async fn push(&mut self, batch: &TraceStatsBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(entries) = batch.entries() {
            for i in 0..entries.len() {
                let e = entries.get(i);

                self.services.intern(e.service().unwrap_or(""));
                self.names.intern(e.name().unwrap_or(""));
                self.resources.intern(e.resource().unwrap_or(""));
                self.types.intern(e.type_().unwrap_or(""));
                self.span_kinds.intern(e.span_kind().unwrap_or(""));
                self.hostnames.intern(e.hostname().unwrap_or(""));
                self.envs.intern(e.env().unwrap_or(""));
                self.versions.intern(e.version().unwrap_or(""));

                self.http_status_codes.push(e.http_status_code());
                self.hits.push(e.hits());
                self.errors.push(e.errors());
                self.durations.push(e.duration_ns());
                self.top_level_hits.push(e.top_level_hits());
                self.bucket_starts.push(e.bucket_start_ns());
                self.bucket_durations.push(e.bucket_duration_ns());
                self.timestamps.push(e.timestamp_ns());

                self.ok_summaries.push(
                    e.ok_summary().map(|v| v.bytes().to_vec()).unwrap_or_default(),
                );
                self.error_summaries.push(
                    e.error_summary().map(|v| v.bytes().to_vec()).unwrap_or_default(),
                );
            }
        }

        if self.len() >= self.flush_rows || self.last_flush.elapsed() >= self.flush_interval {
            return self.flush().await.map(Some);
        }
        Ok(None)
    }

    /// Flush accumulated columns to a new Vortex file. Returns the file path.
    pub async fn flush(&mut self) -> Result<PathBuf> {
        let flush_start = Instant::now();
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no rows to flush");
        }

        let ts_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let path = self.output_dir.join(format!("flush-trace_stats-{}.vortex", ts_ms));

        // Take columns.
        let (service_vals, service_codes) = self.services.take();
        let (name_vals, name_codes) = self.names.take();
        let (resource_vals, resource_codes) = self.resources.take();
        let (type_vals, type_codes) = self.types.take();
        let (span_kind_vals, span_kind_codes) = self.span_kinds.take();
        let (hostname_vals, hostname_codes) = self.hostnames.take();
        let (env_vals, env_codes) = self.envs.take();
        let (version_vals, version_codes) = self.versions.take();

        let http_status_codes = std::mem::take(&mut self.http_status_codes);
        let hits = std::mem::take(&mut self.hits);
        let errors = std::mem::take(&mut self.errors);
        let durations = std::mem::take(&mut self.durations);
        let top_level_hits = std::mem::take(&mut self.top_level_hits);
        let bucket_starts = std::mem::take(&mut self.bucket_starts);
        let bucket_durations = std::mem::take(&mut self.bucket_durations);
        let timestamps = std::mem::take(&mut self.timestamps);
        let ok_summaries = std::mem::take(&mut self.ok_summaries);
        let error_summaries = std::mem::take(&mut self.error_summaries);

        // Sort index by timestamp for better compression (delta encoding).
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        // Helper: build a DictArray from interner output + sort order.
        let build_dict =
            |vals: Vec<String>, codes: Vec<u32>, label: &str| -> Result<DictArray> {
                DictArray::try_new(
                    order
                        .iter()
                        .map(|&i| codes[i])
                        .collect::<PrimitiveArray>()
                        .into_array(),
                    VarBinArray::from(vals).into_array(),
                )
                .with_context(|| format!("building {label} DictArray"))
            };

        let service_array = build_dict(service_vals, service_codes, "service")?;
        let name_array = build_dict(name_vals, name_codes, "name")?;
        let resource_array = build_dict(resource_vals, resource_codes, "resource")?;
        let type_array = build_dict(type_vals, type_codes, "type")?;
        let span_kind_array = build_dict(span_kind_vals, span_kind_codes, "span_kind")?;
        let hostname_array = build_dict(hostname_vals, hostname_codes, "hostname")?;
        let env_array = build_dict(env_vals, env_codes, "env")?;
        let version_array = build_dict(version_vals, version_codes, "version")?;

        // Reorder binary columns in-place (avoids cloning every Vec<u8>).
        let sorted_ok = apply_permutation(ok_summaries, &order);
        let sorted_err = apply_permutation(error_summaries, &order);

        let st = StructArray::try_new(
            FieldNames::from(TRACE_STATS_FIELD_NAMES),
            vec![
                service_array.into_array(),
                name_array.into_array(),
                resource_array.into_array(),
                type_array.into_array(),
                span_kind_array.into_array(),
                hostname_array.into_array(),
                env_array.into_array(),
                version_array.into_array(),
                order.iter().map(|&i| http_status_codes[i]).collect::<PrimitiveArray>().into_array(),
                order.iter().map(|&i| hits[i]).collect::<PrimitiveArray>().into_array(),
                order.iter().map(|&i| errors[i]).collect::<PrimitiveArray>().into_array(),
                order.iter().map(|&i| durations[i]).collect::<PrimitiveArray>().into_array(),
                order.iter().map(|&i| top_level_hits[i]).collect::<PrimitiveArray>().into_array(),
                VarBinArray::from(sorted_ok).into_array(),
                VarBinArray::from(sorted_err).into_array(),
                order.iter().map(|&i| bucket_starts[i]).collect::<PrimitiveArray>().into_array(),
                order.iter().map(|&i| bucket_durations[i]).collect::<PrimitiveArray>().into_array(),
                order.iter().map(|&i| timestamps[i]).collect::<PrimitiveArray>().into_array(),
            ],
            row_count,
            Validity::NonNullable,
        )
        .context("building trace_stats StructArray")?;

        let strategy = super::strategy::fast_flush_strategy();

        // Fresh session per flush to prevent registry accumulation in VortexSession.
        let session = VortexSession::default();

        self.write_buf.clear();
        VortexWriteOptions::new(session)
            .with_strategy(strategy)
            .write(&mut self.write_buf, st.into_array().to_array_stream())
            .await
            .context("writing trace_stats vortex file")?;

        tokio::fs::write(&path, &self.write_buf)
            .await
            .with_context(|| format!("writing {}", path.display()))?;

        // Shrink write_buf to release memory back to the allocator.
        let bytes_written = self.write_buf.len() as u64;
        self.write_buf = Vec::with_capacity(64 * 1024);

        self.flush_count += 1;
        self.flush_bytes += bytes_written;
        self.last_flush_duration_ns = flush_start.elapsed().as_nanos() as u64;
        self.last_flush = Instant::now();
        Ok(path)
    }

    /// Flush if any rows are buffered. Used on shutdown.
    pub async fn flush_if_any(&mut self) -> Result<Option<PathBuf>> {
        if self.len() == 0 {
            Ok(None)
        } else {
            self.flush().await.map(Some)
        }
    }
}

/// Field names for the trace stats schema (18 columns).
pub const TRACE_STATS_FIELD_NAMES: [&str; 18] = [
    "service",
    "name",
    "resource",
    "type",
    "span_kind",
    "hostname",
    "env",
    "version",
    "http_status_code",
    "hits",
    "errors",
    "duration_ns",
    "top_level_hits",
    "ok_summary",
    "error_summary",
    "bucket_start_ns",
    "bucket_duration_ns",
    "timestamp_ns",
];
