use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use tokio::sync::Mutex;
use vortex::array::arrays::{DictArray, PrimitiveArray, StructArray, VarBinArray};
use vortex::array::dtype::FieldNames;
use vortex::array::validity::Validity;
use vortex::array::IntoArray;
use vortex::file::VortexWriteOptions;
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

use super::context_store::ContextStore;
use super::intern::StringInterner;
use super::tags::{decompose_joined_into_interners, METRIC_RESERVED_KEYS};
use crate::generated::signals_generated::signals::MetricBatch;

/// Columnar accumulator for metric samples.
///
/// Operates in one of two modes controlled by `inline`:
///
/// - **Inline mode** (`inline = true`): tags are decomposed into per-key
///   columns (host, device, source, service, env, version, team, overflow).
///   Each flush file is self-contained but requires a `context_map` HashMap
///   in memory (unbounded growth with cardinality).
///
/// - **Context-key mode** (`inline = false`, default): flush files store only
///   a `context_key` (u64) column. Context definitions are written to a shared
///   `contexts.bin` file via a [`ContextStore`] with bloom-filter dedup.
///   Much lower RSS since no `context_map` or tag StringInterners are needed.
pub struct MetricsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    // --- Common columns (both modes) ---
    sources: StringInterner,
    values: Vec<f64>,
    timestamps: Vec<i64>,
    sample_rates: Vec<f64>,

    // --- Mode-specific state ---
    mode: MetricsMode,

    // Rate-limited logging for unresolved context keys (inline mode only).
    unresolved_count: u64,
    last_unresolved_log: Instant,

    write_buf: Vec<u8>,

    // Telemetry counters (read by the telemetry reporter).
    pub flush_count: u64,
    pub flush_bytes: u64,
    pub last_flush_duration_ns: u64,
}

enum MetricsMode {
    /// Inline tags: decompose into 9 StringInterners, context_map for resolution.
    Inline {
        names: StringInterner,
        tag_host: StringInterner,
        tag_device: StringInterner,
        tag_source: StringInterner,
        tag_service: StringInterner,
        tag_env: StringInterner,
        tag_version: StringInterner,
        tag_team: StringInterner,
        tags_overflow: StringInterner,
        context_map: HashMap<u64, (String, String)>,
    },
    /// Context-key: store only u64 keys, write contexts to shared ContextStore.
    ContextKey {
        context_keys: Vec<u64>,
        context_store: Arc<Mutex<ContextStore>>,
    },
}

impl MetricsWriter {
    pub fn new(
        output_dir: impl AsRef<Path>,
        flush_rows: usize,
        flush_interval: Duration,
        inline: bool,
        context_store: Option<Arc<Mutex<ContextStore>>>,
    ) -> Self {
        let output_dir = output_dir.as_ref().to_path_buf();
        let mode = if inline {
            MetricsMode::Inline {
                names: StringInterner::with_capacity(flush_rows),
                tag_host: StringInterner::with_capacity(flush_rows),
                tag_device: StringInterner::with_capacity(flush_rows),
                tag_source: StringInterner::with_capacity(flush_rows),
                tag_service: StringInterner::with_capacity(flush_rows),
                tag_env: StringInterner::with_capacity(flush_rows),
                tag_version: StringInterner::with_capacity(flush_rows),
                tag_team: StringInterner::with_capacity(flush_rows),
                tags_overflow: StringInterner::with_capacity(flush_rows),
                context_map: HashMap::new(),
            }
        } else {
            MetricsMode::ContextKey {
                context_keys: Vec::with_capacity(flush_rows),
                context_store: context_store.expect("context_store required when inline=false"),
            }
        };

        Self {
            output_dir,
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),

            sources: StringInterner::with_capacity(flush_rows),
            values: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
            sample_rates: Vec::with_capacity(flush_rows),

            mode,

            unresolved_count: 0,
            last_unresolved_log: Instant::now(),

            write_buf: Vec::with_capacity(64 * 1024),

            flush_count: 0,
            flush_bytes: 0,
            last_flush_duration_ns: 0,
        }
    }

    /// Number of rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.values.len()
    }

    /// Ingest a MetricBatch from FlatBuffers. Flushes automatically when thresholds are reached.
    pub async fn push(&mut self, batch: &MetricBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(samples) = batch.samples() {
            for i in 0..samples.len() {
                let s = samples.get(i);
                let ckey = s.context_key();
                let raw_name = s.name().unwrap_or("");

                match &mut self.mode {
                    MetricsMode::Inline {
                        names,
                        tag_host,
                        tag_device,
                        tag_source,
                        tag_service,
                        tag_env,
                        tag_version,
                        tag_team,
                        tags_overflow,
                        context_map,
                    } => {
                        // Store context definition in memory.
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
                            context_map.insert(ckey, (raw_name.to_string(), tags_joined));
                        }

                        // Resolve and decompose tags.
                        if let Some((name, joined_tags)) = context_map.get(&ckey) {
                            names.intern(name);
                            decompose_joined_into_interners(
                                joined_tags,
                                METRIC_RESERVED_KEYS,
                                &mut [
                                    tag_host,
                                    tag_device,
                                    tag_source,
                                    tag_service,
                                    tag_env,
                                    tag_version,
                                    tag_team,
                                ],
                                tags_overflow,
                            );
                        } else {
                            if ckey != 0 {
                                self.unresolved_count += 1;
                            }
                            names.intern("<unknown>");
                            tag_host.intern("");
                            tag_device.intern("");
                            tag_source.intern("");
                            tag_service.intern("");
                            tag_env.intern("");
                            tag_version.intern("");
                            tag_team.intern("");
                            tags_overflow.intern("");
                        }
                    }
                    MetricsMode::ContextKey {
                        context_keys,
                        context_store,
                    } => {
                        // Write context definition to shared file if new.
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
                            let mut store = context_store.lock().await;
                            let _ = store.try_record(ckey, raw_name, &tags_joined);
                        }
                        context_keys.push(ckey);
                    }
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
        let flush_start = Instant::now();
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no rows to flush");
        }

        let ts_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let path = self.output_dir.join(format!("flush-metrics-{}.vortex", ts_ms));

        let (source_vals, source_codes) = self.sources.take();
        let values = std::mem::take(&mut self.values);
        let timestamps = std::mem::take(&mut self.timestamps);
        let sample_rates = std::mem::take(&mut self.sample_rates);

        // Sort index by timestamp for better compression (delta encoding).
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

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

        let source_array = build_dict(source_vals, source_codes, "source")?;

        let st = match &mut self.mode {
            MetricsMode::Inline {
                names,
                tag_host,
                tag_device,
                tag_source,
                tag_service,
                tag_env,
                tag_version,
                tag_team,
                tags_overflow,
                ..
            } => {
                let (name_vals, name_codes) = names.take();
                let (host_vals, host_codes) = tag_host.take();
                let (device_vals, device_codes) = tag_device.take();
                let (tsource_vals, tsource_codes) = tag_source.take();
                let (service_vals, service_codes) = tag_service.take();
                let (env_vals, env_codes) = tag_env.take();
                let (version_vals, version_codes) = tag_version.take();
                let (team_vals, team_codes) = tag_team.take();
                let (overflow_vals, overflow_codes) = tags_overflow.take();

                StructArray::try_new(
                    FieldNames::from(METRIC_FIELD_NAMES_INLINE),
                    vec![
                        build_dict(name_vals, name_codes, "name")?.into_array(),
                        build_dict(host_vals, host_codes, "tag_host")?.into_array(),
                        build_dict(device_vals, device_codes, "tag_device")?.into_array(),
                        build_dict(tsource_vals, tsource_codes, "tag_source")?.into_array(),
                        build_dict(service_vals, service_codes, "tag_service")?.into_array(),
                        build_dict(env_vals, env_codes, "tag_env")?.into_array(),
                        build_dict(version_vals, version_codes, "tag_version")?.into_array(),
                        build_dict(team_vals, team_codes, "tag_team")?.into_array(),
                        build_dict(overflow_vals, overflow_codes, "tags_overflow")?.into_array(),
                        order.iter().map(|&i| values[i]).collect::<PrimitiveArray>().into_array(),
                        order.iter().map(|&i| timestamps[i]).collect::<PrimitiveArray>().into_array(),
                        order.iter().map(|&i| sample_rates[i]).collect::<PrimitiveArray>().into_array(),
                        source_array.into_array(),
                    ],
                    row_count,
                    Validity::NonNullable,
                )
                .context("building metrics StructArray (inline)")?
            }
            MetricsMode::ContextKey { context_keys, .. } => {
                let keys = std::mem::take(context_keys);
                StructArray::try_new(
                    FieldNames::from(METRIC_FIELD_NAMES_CONTEXTKEY),
                    vec![
                        order.iter().map(|&i| keys[i]).collect::<PrimitiveArray>().into_array(),
                        order.iter().map(|&i| values[i]).collect::<PrimitiveArray>().into_array(),
                        order.iter().map(|&i| timestamps[i]).collect::<PrimitiveArray>().into_array(),
                        order.iter().map(|&i| sample_rates[i]).collect::<PrimitiveArray>().into_array(),
                        source_array.into_array(),
                    ],
                    row_count,
                    Validity::NonNullable,
                )
                .context("building metrics StructArray (context_key)")?
            }
        };

        let strategy = super::strategy::fast_flush_strategy();
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

        let bytes_written = self.write_buf.len() as u64;
        self.write_buf = Vec::with_capacity(64 * 1024);

        self.flush_count += 1;
        self.flush_bytes += bytes_written;
        self.last_flush_duration_ns = flush_start.elapsed().as_nanos() as u64;
        self.last_flush = Instant::now();
        Ok(path)
    }

    /// Clear the context map (inline mode) or no-op (context_key mode).
    /// Called when a new agent connection is accepted.
    pub fn reset_context_map(&mut self) {
        if let MetricsMode::Inline { context_map, .. } = &mut self.mode {
            context_map.clear();
        }
        // Context-key mode: ContextStore.reset() is called separately by main.rs.
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

/// Field names for the inline metric schema (13 columns).
pub const METRIC_FIELD_NAMES_INLINE: [&str; 13] = [
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

/// Field names for the context_key metric schema (5 columns).
pub const METRIC_FIELD_NAMES_CONTEXTKEY: [&str; 5] = [
    "context_key",
    "value",
    "timestamp_ns",
    "sample_rate",
    "source",
];

// Keep the old name as an alias for backward compat in tests/readers.
pub const METRIC_FIELD_NAMES: [&str; 13] = METRIC_FIELD_NAMES_INLINE;

#[cfg(test)]
mod tests {
    use super::*;
    use super::super::context_store::read_contexts_bin;
    use tempfile::tempdir;
    use vortex::array::stream::ArrayStreamExt;
    use vortex::array::Canonical;
    use vortex::file::OpenOptionsSessionExt;

    fn make_inline_writer(dir: &Path) -> MetricsWriter {
        MetricsWriter::new(dir, 1000, Duration::from_secs(60), true, None)
    }

    fn make_contextkey_writer(dir: &Path, store: Arc<Mutex<ContextStore>>) -> MetricsWriter {
        MetricsWriter::new(dir, 1000, Duration::from_secs(60), false, Some(store))
    }

    fn add_inline_rows(w: &mut MetricsWriter, n: usize) {
        if let MetricsMode::Inline {
            names,
            tag_host,
            tag_device,
            tag_source,
            tag_service,
            tag_env,
            tag_version,
            tag_team,
            tags_overflow,
            ..
        } = &mut w.mode
        {
            for i in 0..n {
                names.intern("cpu.user");
                tag_host.intern("a");
                tag_device.intern("");
                tag_source.intern("");
                tag_service.intern("");
                tag_env.intern("prod");
                tag_version.intern("");
                tag_team.intern("");
                tags_overflow.intern("");
                w.sources.intern("test");
                w.values.push(i as f64);
                w.timestamps.push(1000);
                w.sample_rates.push(1.0);
            }
        } else {
            panic!("add_inline_rows called on context_key writer");
        }
    }

    #[tokio::test]
    async fn test_inline_push_and_flush() {
        let dir = tempdir().unwrap();
        let mut w = make_inline_writer(dir.path());
        add_inline_rows(&mut w, 100);

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
    }

    #[tokio::test]
    async fn test_inline_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_inline_writer(dir.path());
        assert!(w.flush().await.is_err());
    }

    #[tokio::test]
    async fn test_contextkey_flush_schema() {
        let dir = tempdir().unwrap();
        let store = Arc::new(Mutex::new(ContextStore::new(dir.path()).unwrap()));
        let mut w = make_contextkey_writer(dir.path(), store.clone());

        // Simulate context definition + samples.
        {
            let mut s = store.lock().await;
            s.try_record(100, "cpu.user", "host:a|env:prod").unwrap();
            s.try_record(200, "mem.usage", "host:b").unwrap();
        }

        if let MetricsMode::ContextKey { context_keys, .. } = &mut w.mode {
            context_keys.push(100);
            context_keys.push(200);
            context_keys.push(100);
        }
        w.sources.intern("agent");
        w.sources.intern("agent");
        w.sources.intern("agent");
        w.values.extend_from_slice(&[42.0, 99.0, 43.0]);
        w.timestamps.extend_from_slice(&[1000, 2000, 3000]);
        w.sample_rates.extend_from_slice(&[1.0, 1.0, 1.0]);

        let path = w.flush().await.unwrap();
        assert!(path.exists());

        // Read back and verify schema has 5 columns.
        let session = VortexSession::default();
        let array = session
            .open_options()
            .open_path(path)
            .await
            .unwrap()
            .scan()
            .unwrap()
            .into_array_stream()
            .unwrap()
            .read_all()
            .await
            .unwrap();
        let canonical = array.to_canonical().unwrap();
        let st = canonical.into_struct();

        let col_names: Vec<String> = st.names().iter().map(|s| s.as_ref().to_string()).collect();
        assert_eq!(
            col_names,
            vec!["context_key", "value", "timestamp_ns", "sample_rate", "source"]
        );
        assert_eq!(st.len(), 3);

        // Verify context_key values (sorted by timestamp).
        let ckey_arr = st.unmasked_field_by_name("context_key").unwrap();
        let ckey_canon = ckey_arr.to_canonical().unwrap();
        if let Canonical::Primitive(prim) = ckey_canon {
            let keys: Vec<u64> = prim.as_slice::<u64>().to_vec();
            assert_eq!(keys, vec![100, 200, 100]); // sorted by timestamp: 1000, 2000, 3000
        } else {
            panic!("expected Primitive u64 for context_key");
        }
    }

    #[tokio::test]
    async fn test_contextkey_roundtrip_with_contexts_bin() {
        let dir = tempdir().unwrap();
        let store = Arc::new(Mutex::new(ContextStore::new(dir.path()).unwrap()));
        let mut w = make_contextkey_writer(dir.path(), store.clone());

        // Record contexts.
        {
            let mut s = store.lock().await;
            s.try_record(100, "cpu.user", "host:a|env:prod").unwrap();
            s.try_record(200, "mem.usage", "host:b").unwrap();
        }

        if let MetricsMode::ContextKey { context_keys, .. } = &mut w.mode {
            context_keys.push(100);
            context_keys.push(200);
        }
        w.sources.intern("agent");
        w.sources.intern("agent");
        w.values.extend_from_slice(&[42.0, 99.0]);
        w.timestamps.extend_from_slice(&[1000, 2000]);
        w.sample_rates.extend_from_slice(&[1.0, 1.0]);

        let _path = w.flush().await.unwrap();

        // Read back contexts.bin and verify.
        let contexts = read_contexts_bin(&dir.path().join("contexts.bin")).unwrap();
        assert_eq!(contexts.len(), 2);

        let ctx_map: HashMap<u64, (&str, &str)> = contexts
            .iter()
            .map(|(k, n, t)| (*k, (n.as_str(), t.as_str())))
            .collect();

        assert_eq!(ctx_map[&100], ("cpu.user", "host:a|env:prod"));
        assert_eq!(ctx_map[&200], ("mem.usage", "host:b"));
    }
}
