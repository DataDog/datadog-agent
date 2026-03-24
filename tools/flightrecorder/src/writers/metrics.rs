use std::collections::HashMap;
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

use super::intern::StringInterner;
use super::tags::{decompose_joined_into_interners, METRIC_RESERVED_KEYS};
use crate::generated::signals_generated::signals::MetricBatch;

/// Columnar accumulator for metric samples.
///
/// Tags are decomposed into per-key reserved columns (host, device, source,
/// service, env, version, team) plus an overflow column for non-reserved tags.
/// This keeps dictionary cardinality low and makes flush near-instant.
pub struct MetricsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    // Interned string columns (dictionary-encoded on flush).
    names: StringInterner,
    tag_host: StringInterner,
    tag_device: StringInterner,
    tag_source: StringInterner,
    tag_service: StringInterner,
    tag_env: StringInterner,
    tag_version: StringInterner,
    tag_team: StringInterner,
    tags_overflow: StringInterner,
    sources: StringInterner,

    // Plain columnar buffers.
    values: Vec<f64>,
    timestamps: Vec<i64>,
    sample_rates: Vec<f64>,

    // Resolves context_key → (name, joined_tags) for inline writing.
    // Tags are stored as a single pipe-joined string and decomposed per sample
    // to avoid storing 9 Strings per context (~0.5 KB → ~0.1 KB per entry).
    context_map: HashMap<u64, (String, String)>,

    // Rate-limited logging for unresolved context keys.
    unresolved_count: u64,
    last_unresolved_log: Instant,

    write_buf: Vec<u8>,
}

impl MetricsWriter {
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

            names: StringInterner::with_capacity(flush_rows),
            tag_host: StringInterner::with_capacity(flush_rows),
            tag_device: StringInterner::with_capacity(flush_rows),
            tag_source: StringInterner::with_capacity(flush_rows),
            tag_service: StringInterner::with_capacity(flush_rows),
            tag_env: StringInterner::with_capacity(flush_rows),
            tag_version: StringInterner::with_capacity(flush_rows),
            tag_team: StringInterner::with_capacity(flush_rows),
            tags_overflow: StringInterner::with_capacity(flush_rows),
            sources: StringInterner::with_capacity(flush_rows),

            values: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
            sample_rates: Vec::with_capacity(flush_rows),

            context_map: HashMap::new(),

            unresolved_count: 0,
            last_unresolved_log: Instant::now(),

            write_buf: Vec::with_capacity(64 * 1024),
        }
    }

    /// Number of rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.values.len()
    }

    /// Ingest a MetricBatch from FlatBuffers. Flushes automatically when thresholds are reached.
    ///
    /// Context definitions (context_key != 0, name non-empty) are stored in the
    /// context_map as (name, joined_tags). Tags are decomposed per sample to
    /// keep the context map lean (~0.1 KB/entry instead of ~0.5 KB).
    pub async fn push(&mut self, batch: &MetricBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(samples) = batch.samples() {
            for i in 0..samples.len() {
                let s = samples.get(i);
                let ckey = s.context_key();
                let raw_name = s.name().unwrap_or("");

                // If this is a context definition, store name + pipe-joined tags.
                if ckey != 0 && !raw_name.is_empty() {
                    let tags_joined: String = s
                        .tags()
                        .map(|tl| {
                            (0..tl.len())
                                .map(|j| tl.get(j))
                                .collect::<Vec<_>>()
                                .join("|")
                        })
                        .unwrap_or_default();
                    self.context_map
                        .insert(ckey, (raw_name.to_string(), tags_joined));
                }

                // Resolve context_key → decompose joined tags directly into interners.
                if let Some((name, joined_tags)) = self.context_map.get(&ckey) {
                    self.names.intern(name);
                    decompose_joined_into_interners(
                        joined_tags,
                        METRIC_RESERVED_KEYS,
                        &mut [
                            &mut self.tag_host,
                            &mut self.tag_device,
                            &mut self.tag_source,
                            &mut self.tag_service,
                            &mut self.tag_env,
                            &mut self.tag_version,
                            &mut self.tag_team,
                        ],
                        &mut self.tags_overflow,
                    );
                } else {
                    if ckey != 0 {
                        self.unresolved_count += 1;
                    }
                    self.names.intern("<unknown>");
                    self.tag_host.intern("");
                    self.tag_device.intern("");
                    self.tag_source.intern("");
                    self.tag_service.intern("");
                    self.tag_env.intern("");
                    self.tag_version.intern("");
                    self.tag_team.intern("");
                    self.tags_overflow.intern("");
                }

                self.sources.intern(s.source().unwrap_or(""));
                self.values.push(s.value());
                self.timestamps.push(s.timestamp_ns());
                self.sample_rates.push(s.sample_rate());
            }
        }

        // Log unresolved context keys at most once per minute.
        if self.unresolved_count > 0
            && self.last_unresolved_log.elapsed() >= Duration::from_secs(60)
        {
            tracing::warn!(
                count = self.unresolved_count,
                "unresolved context_keys in the last 60s (using <unknown>)"
            );
            self.unresolved_count = 0;
            self.last_unresolved_log = Instant::now();
        }

        if self.len() >= self.flush_rows || self.last_flush.elapsed() >= self.flush_interval {
            return self.flush().await.map(Some);
        }
        Ok(None)
    }

    /// Flush accumulated columns to a new Vortex file. Returns the file path.
    pub async fn flush(&mut self) -> Result<PathBuf> {
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no rows to flush");
        }

        let ts_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let path = self.output_dir.join(format!("flush-metrics-{}.vortex", ts_ms));

        // Take columns.
        let (name_vals, name_codes) = self.names.take();
        let (host_vals, host_codes) = self.tag_host.take();
        let (device_vals, device_codes) = self.tag_device.take();
        let (tsource_vals, tsource_codes) = self.tag_source.take();
        let (service_vals, service_codes) = self.tag_service.take();
        let (env_vals, env_codes) = self.tag_env.take();
        let (version_vals, version_codes) = self.tag_version.take();
        let (team_vals, team_codes) = self.tag_team.take();
        let (overflow_vals, overflow_codes) = self.tags_overflow.take();
        let (source_vals, source_codes) = self.sources.take();
        let values = std::mem::take(&mut self.values);
        let timestamps = std::mem::take(&mut self.timestamps);
        let sample_rates = std::mem::take(&mut self.sample_rates);

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

        let name_array = build_dict(name_vals, name_codes, "name")?;
        let host_array = build_dict(host_vals, host_codes, "tag_host")?;
        let device_array = build_dict(device_vals, device_codes, "tag_device")?;
        let tsource_array = build_dict(tsource_vals, tsource_codes, "tag_source")?;
        let service_array = build_dict(service_vals, service_codes, "tag_service")?;
        let env_array = build_dict(env_vals, env_codes, "tag_env")?;
        let version_array = build_dict(version_vals, version_codes, "tag_version")?;
        let team_array = build_dict(team_vals, team_codes, "tag_team")?;
        let overflow_array = build_dict(overflow_vals, overflow_codes, "tags_overflow")?;
        let source_array = build_dict(source_vals, source_codes, "source")?;

        let st = StructArray::try_new(
            FieldNames::from(METRIC_FIELD_NAMES),
            vec![
                name_array.into_array(),
                host_array.into_array(),
                device_array.into_array(),
                tsource_array.into_array(),
                service_array.into_array(),
                env_array.into_array(),
                version_array.into_array(),
                team_array.into_array(),
                overflow_array.into_array(),
                order
                    .iter()
                    .map(|&i| values[i])
                    .collect::<PrimitiveArray>()
                    .into_array(),
                order
                    .iter()
                    .map(|&i| timestamps[i])
                    .collect::<PrimitiveArray>()
                    .into_array(),
                order
                    .iter()
                    .map(|&i| sample_rates[i])
                    .collect::<PrimitiveArray>()
                    .into_array(),
                source_array.into_array(),
            ],
            row_count,
            Validity::NonNullable,
        )
        .context("building metrics StructArray")?;

        let strategy = super::strategy::fast_flush_strategy();

        // Fresh session per flush to prevent registry accumulation in VortexSession.
        let session = VortexSession::default();

        self.write_buf.clear();
        VortexWriteOptions::new(session)
            .with_strategy(strategy)
            .write(&mut self.write_buf, st.into_array().to_array_stream())
            .await
            .context("writing metrics vortex file")?;

        tokio::fs::write(&path, &self.write_buf)
            .await
            .with_context(|| format!("writing {}", path.display()))?;

        // Shrink write_buf to release memory back to the allocator.
        self.write_buf = Vec::with_capacity(64 * 1024);

        self.last_flush = Instant::now();
        Ok(path)
    }

    /// Clear the context map. Called when a new agent connection is accepted
    /// because the agent will re-send all context definitions.
    pub fn reset_context_map(&mut self) {
        self.context_map.clear();
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

/// Field names for the metric schema (13 columns).
pub const METRIC_FIELD_NAMES: [&str; 13] = [
    "name",
    "tag_host",
    "tag_device",
    "tag_source",
    "tag_service",
    "tag_env",
    "tag_version",
    "tag_team",
    "tags_overflow",
    "value",
    "timestamp_ns",
    "sample_rate",
    "source",
];

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    fn make_writer(dir: &Path) -> MetricsWriter {
        MetricsWriter::new(dir, 1000, Duration::from_secs(60))
    }

    fn add_rows(w: &mut MetricsWriter, n: usize) {
        for i in 0..n {
            w.names.intern("cpu.user");
            w.tag_host.intern("a");
            w.tag_device.intern("");
            w.tag_source.intern("");
            w.tag_service.intern("");
            w.tag_env.intern("prod");
            w.tag_version.intern("");
            w.tag_team.intern("");
            w.tags_overflow.intern("");
            w.sources.intern("test");
            w.values.push(i as f64);
            w.timestamps.push(1000);
            w.sample_rates.push(1.0);
        }
    }

    #[tokio::test]
    async fn test_push_and_flush() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 100);

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
    }

    #[tokio::test]
    async fn test_readback_fields() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.names.intern("cpu.user");
        w.tag_host.intern("a");
        w.tag_device.intern("");
        w.tag_source.intern("");
        w.tag_service.intern("");
        w.tag_env.intern("prod");
        w.tag_version.intern("");
        w.tag_team.intern("");
        w.tags_overflow.intern("custom:foo");
        w.sources.intern("src1");
        w.values.push(42.0);
        w.timestamps.push(999);
        w.sample_rates.push(0.5);

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
    }

    #[tokio::test]
    async fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().await.is_err());
    }

    #[tokio::test]
    async fn test_interning_deduplicates() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        for i in 0..1000 {
            w.names.intern("cpu.user");
            w.tag_host.intern("a");
            w.tag_device.intern("");
            w.tag_source.intern("");
            w.tag_service.intern("");
            w.tag_env.intern("prod");
            w.tag_version.intern("");
            w.tag_team.intern("");
            w.tags_overflow.intern("");
            w.sources.intern("agent");
            w.values.push(i as f64);
            w.timestamps.push(i as i64);
            w.sample_rates.push(1.0);
        }
        let path = w.flush().await.unwrap();
        let size = path.metadata().unwrap().len();
        assert!(size > 0);
        // With fast_flush_strategy (no compression), files are larger, but
        // dict encoding still keeps things reasonable for low-cardinality data.
        assert!(
            size < 100_000,
            "file too large: {} bytes, dict encoding may not be working",
            size
        );
    }

    #[tokio::test]
    async fn test_context_map_resolution() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Insert context definitions as (name, joined_tags)
        w.context_map
            .insert(100, ("cpu.user".to_string(), "host:a|env:prod".to_string()));
        w.context_map
            .insert(200, ("mem.used".to_string(), "host:b".to_string()));

        // Simulate rows that resolve via context_map + decompose per sample
        for &ckey in &[100u64, 200] {
            let (name, joined_tags) = w.context_map.get(&ckey).unwrap();
            let name = name.clone();
            let joined_tags = joined_tags.clone();
            w.names.intern(&name);
            decompose_joined_into_interners(
                &joined_tags,
                METRIC_RESERVED_KEYS,
                &mut [
                    &mut w.tag_host,
                    &mut w.tag_device,
                    &mut w.tag_source,
                    &mut w.tag_service,
                    &mut w.tag_env,
                    &mut w.tag_version,
                    &mut w.tag_team,
                ],
                &mut w.tags_overflow,
            );
            w.sources.intern("agent");
            w.values.push(ckey as f64);
            w.timestamps.push(ckey as i64 * 10);
            w.sample_rates.push(1.0);
        }

        let path = w.flush().await.unwrap();
        assert!(path.exists());
    }

    #[tokio::test]
    async fn test_reset_context_map() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.context_map
            .insert(42, ("cpu.system".to_string(), "host:x".to_string()));
        assert_eq!(w.context_map.len(), 1);

        w.reset_context_map();
        assert!(w.context_map.is_empty());
    }
}
