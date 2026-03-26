use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, Float64Array, Int64Array, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;
use tokio::sync::Mutex;

use super::context_store::ContextStore;
use super::intern::StringInterner;
use super::parquet_helpers::{dict_utf8_type, interner_to_dict_array, BaseWriter};
use super::tags::{decompose_joined_into_interners, METRIC_RESERVED_KEYS};
use crate::generated::signals_generated::signals::MetricBatch;

/// Schema for inline mode (13 columns).
fn inline_schema() -> Arc<Schema> {
    let dt = dict_utf8_type();
    Arc::new(Schema::new(vec![
        Field::new("name", dt.clone(), false),
        Field::new("tag_host", dt.clone(), false),
        Field::new("tag_device", dt.clone(), false),
        Field::new("tag_source", dt.clone(), false),
        Field::new("tag_service", dt.clone(), false),
        Field::new("tag_env", dt.clone(), false),
        Field::new("tag_version", dt.clone(), false),
        Field::new("tag_team", dt.clone(), false),
        Field::new("tags_overflow", dt.clone(), false),
        Field::new("value", DataType::Float64, false),
        Field::new("timestamp_ns", DataType::Int64, false),
        Field::new("sample_rate", DataType::Float64, false),
        Field::new("source", dt, false),
    ]))
}

/// Schema for context_key mode (5 columns).
fn contextkey_schema() -> Arc<Schema> {
    Arc::new(Schema::new(vec![
        Field::new("context_key", DataType::UInt64, false),
        Field::new("value", DataType::Float64, false),
        Field::new("timestamp_ns", DataType::Int64, false),
        Field::new("sample_rate", DataType::Float64, false),
        Field::new("source", dict_utf8_type(), false),
    ]))
}

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
    pub base: BaseWriter,

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
        let output_dir = output_dir.as_ref();
        let flush_rows = flush_rows;
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
            base: BaseWriter::new(output_dir, flush_rows, flush_interval),

            sources: StringInterner::with_capacity(flush_rows),
            values: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
            sample_rates: Vec::with_capacity(flush_rows),

            mode,

            unresolved_count: 0,
            last_unresolved_log: Instant::now(),
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

        if self.base.should_flush(self.len()) {
            return self.flush().await.map(Some);
        }
        Ok(None)
    }

    /// Flush accumulated columns to a new Parquet file. Returns the file path.
    pub async fn flush(&mut self) -> Result<PathBuf> {
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no rows to flush");
        }

        let (source_vals, source_codes) = self.sources.take();
        let values = std::mem::take(&mut self.values);
        let timestamps = std::mem::take(&mut self.timestamps);
        let sample_rates = std::mem::take(&mut self.sample_rates);

        // Sort index by timestamp for better compression (delta encoding).
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        let source_array: ArrayRef =
            Arc::new(interner_to_dict_array(source_vals, source_codes, &order));

        let (schema, columns) = match &mut self.mode {
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

                let columns: Vec<ArrayRef> = vec![
                    Arc::new(interner_to_dict_array(name_vals, name_codes, &order)),
                    Arc::new(interner_to_dict_array(host_vals, host_codes, &order)),
                    Arc::new(interner_to_dict_array(device_vals, device_codes, &order)),
                    Arc::new(interner_to_dict_array(tsource_vals, tsource_codes, &order)),
                    Arc::new(interner_to_dict_array(service_vals, service_codes, &order)),
                    Arc::new(interner_to_dict_array(env_vals, env_codes, &order)),
                    Arc::new(interner_to_dict_array(version_vals, version_codes, &order)),
                    Arc::new(interner_to_dict_array(team_vals, team_codes, &order)),
                    Arc::new(interner_to_dict_array(
                        overflow_vals,
                        overflow_codes,
                        &order,
                    )),
                    Arc::new(Float64Array::from_iter_values(
                        order.iter().map(|&i| values[i]),
                    )),
                    Arc::new(Int64Array::from_iter_values(
                        order.iter().map(|&i| timestamps[i]),
                    )),
                    Arc::new(Float64Array::from_iter_values(
                        order.iter().map(|&i| sample_rates[i]),
                    )),
                    source_array,
                ];

                (inline_schema(), columns)
            }
            MetricsMode::ContextKey { context_keys, .. } => {
                let keys = std::mem::take(context_keys);

                let columns: Vec<ArrayRef> = vec![
                    Arc::new(UInt64Array::from_iter_values(
                        order.iter().map(|&i| keys[i]),
                    )),
                    Arc::new(Float64Array::from_iter_values(
                        order.iter().map(|&i| values[i]),
                    )),
                    Arc::new(Int64Array::from_iter_values(
                        order.iter().map(|&i| timestamps[i]),
                    )),
                    Arc::new(Float64Array::from_iter_values(
                        order.iter().map(|&i| sample_rates[i]),
                    )),
                    source_array,
                ];

                (contextkey_schema(), columns)
            }
        };

        let batch = RecordBatch::try_new(schema.clone(), columns)
            .context("building metrics RecordBatch")?;

        self.base.write_parquet("metrics", schema, batch)
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

#[cfg(test)]
mod tests {
    use super::super::context_store::read_contexts_bin;
    use super::*;
    use arrow::array::{
        Array, AsArray, Float64Array, Int64Array, UInt64Array,
    };
    use arrow::datatypes::UInt32Type;
    use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
    use std::fs::File;
    use tempfile::tempdir;

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

    fn read_parquet(path: &Path) -> RecordBatch {
        let file = File::open(path).unwrap();
        let reader = ParquetRecordBatchReaderBuilder::try_new(file)
            .unwrap()
            .build()
            .unwrap();
        let batches: Vec<RecordBatch> = reader.collect::<Result<_, _>>().unwrap();
        assert_eq!(batches.len(), 1, "expected exactly one row group");
        batches.into_iter().next().unwrap()
    }

    fn read_dict_string_column(batch: &RecordBatch, name: &str) -> Vec<String> {
        let col = batch.column_by_name(name).unwrap_or_else(|| panic!("column {name} not found"));
        let dict = col.as_dictionary::<UInt32Type>();
        let values = dict.values().as_string::<i32>();
        let keys = dict.keys();
        (0..dict.len())
            .map(|i| {
                let key = keys.value(i) as usize;
                values.value(key).to_string()
            })
            .collect()
    }

    #[tokio::test]
    async fn test_inline_push_and_flush() {
        let dir = tempdir().unwrap();
        let mut w = make_inline_writer(dir.path());
        add_inline_rows(&mut w, 100);

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);

        // Verify we can read it back and it has 100 rows.
        let batch = read_parquet(&path);
        assert_eq!(batch.num_rows(), 100);
        assert_eq!(batch.num_columns(), 13);

        // Verify column names.
        let schema = batch.schema();
        let col_names: Vec<&str> = schema.fields().iter().map(|f| f.name().as_str()).collect();
        assert_eq!(
            col_names,
            vec![
                "name", "tag_host", "tag_device", "tag_source", "tag_service",
                "tag_env", "tag_version", "tag_team", "tags_overflow",
                "value", "timestamp_ns", "sample_rate", "source",
            ]
        );

        // All names should be "cpu.user".
        let names = read_dict_string_column(&batch, "name");
        assert!(names.iter().all(|n| n == "cpu.user"));

        // All sources should be "test".
        let sources = read_dict_string_column(&batch, "source");
        assert!(sources.iter().all(|s| s == "test"));
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
        let batch = read_parquet(&path);

        let schema = batch.schema();
        let col_names: Vec<&str> = schema
            .fields()
            .iter()
            .map(|f| f.name().as_str())
            .collect();
        assert_eq!(
            col_names,
            vec!["context_key", "value", "timestamp_ns", "sample_rate", "source"]
        );
        assert_eq!(batch.num_rows(), 3);

        // Verify context_key values (sorted by timestamp: 1000, 2000, 3000).
        let ckey_col = batch
            .column_by_name("context_key")
            .unwrap()
            .as_any()
            .downcast_ref::<UInt64Array>()
            .unwrap();
        let keys: Vec<u64> = ckey_col.values().iter().copied().collect();
        assert_eq!(keys, vec![100, 200, 100]);

        // Verify values are sorted by timestamp order.
        let val_col = batch
            .column_by_name("value")
            .unwrap()
            .as_any()
            .downcast_ref::<Float64Array>()
            .unwrap();
        let vals: Vec<f64> = val_col.values().iter().copied().collect();
        assert_eq!(vals, vec![42.0, 99.0, 43.0]);

        // Verify timestamps are sorted.
        let ts_col = batch
            .column_by_name("timestamp_ns")
            .unwrap()
            .as_any()
            .downcast_ref::<Int64Array>()
            .unwrap();
        let ts: Vec<i64> = ts_col.values().iter().copied().collect();
        assert_eq!(ts, vec![1000, 2000, 3000]);
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

    #[tokio::test]
    async fn test_telemetry_counters() {
        let dir = tempdir().unwrap();
        let mut w = make_inline_writer(dir.path());
        add_inline_rows(&mut w, 10);

        assert_eq!(w.base.flush_count, 0);
        assert_eq!(w.base.flush_bytes, 0);

        let path = w.flush().await.unwrap();
        assert_eq!(w.base.flush_count, 1);
        assert!(w.base.flush_bytes > 0);
        assert!(w.base.last_flush_duration_ns > 0);

        // flush_bytes should match the file size on disk.
        let file_size = std::fs::metadata(&path).unwrap().len();
        assert_eq!(w.base.flush_bytes, file_size);
    }
}
