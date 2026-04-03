use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, Float64Array, Int64Array, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;

use super::context_store::ContextStore;
use super::intern::StringInterner;
use super::parquet_helpers::{dict_utf8_type, interner_to_dict_array, BaseWriter};
use super::thread::{SignalWriter, WriterStats};
use crate::generated::signals_generated::signals;

/// Schema for context-key mode (5 columns).
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
/// Stores a `context_key` (u64) per row. Context definitions (name + tags)
/// are written to `contexts.bin` via an owned [`ContextStore`] with
/// bloom-filter dedup. The writer lives on a dedicated thread, so no mutex
/// is needed for the context store.
pub struct MetricsWriter {
    pub base: BaseWriter,

    // Accumulation columns.
    sources: StringInterner,
    values: Vec<f64>,
    timestamps: Vec<i64>,
    sample_rates: Vec<f64>,
    context_keys: Vec<u64>,

    // Context deduplication (owned — single-threaded access on writer thread).
    context_store: ContextStore,
}

impl MetricsWriter {
    pub fn new(
        output_dir: impl AsRef<Path>,
        flush_rows: usize,
        flush_interval: Duration,
        context_store: ContextStore,
    ) -> Self {
        Self {
            base: BaseWriter::new(output_dir.as_ref(), flush_rows, flush_interval),

            sources: StringInterner::with_capacity(flush_rows),
            values: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
            sample_rates: Vec::with_capacity(flush_rows),
            context_keys: Vec::with_capacity(flush_rows),

            context_store,
        }
    }

    /// Number of rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.values.len()
    }

    /// Ingest a MetricBatch from FlatBuffers. Flushes automatically when thresholds are reached.
    pub fn push(&mut self, batch: &signals::MetricBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(samples) = batch.samples() {
            for i in 0..samples.len() {
                let s = samples.get(i);
                let ckey = s.context_key();
                let raw_name = s.name().unwrap_or("");

                // Write context definition to contexts.bin if new.
                // The bloom filter handles the common case without I/O.
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
                    let _ = self.context_store.try_record(ckey, raw_name, &tags_joined);
                }
                self.context_keys.push(ckey);

                self.sources.intern(s.source().unwrap_or(""));
                self.values.push(s.value());
                self.timestamps.push(s.timestamp_ns());
                self.sample_rates.push(s.sample_rate());
            }
            // Flush the BufWriter once per frame (not per-context).
            let _ = self.context_store.flush();
        }

        if self.base.should_flush(self.len()) {
            return self.flush().map(Some);
        }
        Ok(None)
    }

    /// Flush accumulated columns to a Parquet row group. Returns the file path.
    pub fn flush(&mut self) -> Result<PathBuf> {
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no rows to flush");
        }

        let (source_vals, source_codes) = self.sources.take();
        let values = std::mem::take(&mut self.values);
        let timestamps = std::mem::take(&mut self.timestamps);
        let sample_rates = std::mem::take(&mut self.sample_rates);
        let keys = std::mem::take(&mut self.context_keys);

        // Sort index by timestamp for better compression (delta encoding).
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        let source_array: ArrayRef =
            Arc::new(interner_to_dict_array(source_vals, source_codes, &order));

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

        let schema = contextkey_schema();
        let batch = RecordBatch::try_new(schema.clone(), columns)
            .context("building metrics RecordBatch")?;

        self.base.write_batch("metrics", schema, batch)
    }
}

impl SignalWriter for MetricsWriter {
    fn process_frame(&mut self, buf: &[u8]) -> Result<()> {
        let env = flatbuffers::root::<signals::SignalEnvelope>(buf)
            .map_err(|e| anyhow::anyhow!("decode error: {e}"))?;
        if let Some(batch) = env.payload_as_metric_batch() {
            self.push(&batch)?;
        }
        Ok(())
    }

    fn flush_and_close(&mut self) -> Result<()> {
        if self.len() > 0 {
            self.flush()?;
        }
        self.base.close()
    }

    fn stats(&self) -> WriterStats {
        WriterStats {
            buffered_rows: self.len() as u64,
            flush_count: self.base.flush_count,
            flush_bytes: self.base.flush_bytes,
            rows_written: self.base.rows_written,
            last_flush_duration_ns: self.base.last_flush_duration_ns,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::super::context_store::read_contexts_bin;
    use super::*;
    use arrow::array::{Float64Array, Int64Array, UInt64Array};
    use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
    use std::fs::File;
    use tempfile::tempdir;

    fn make_writer(dir: &Path) -> MetricsWriter {
        let store = ContextStore::new(dir).unwrap();
        MetricsWriter::new(dir, 1000, Duration::from_secs(60), store)
    }

    fn read_parquet(path: &Path) -> RecordBatch {
        let file = File::open(path).unwrap();
        let reader = ParquetRecordBatchReaderBuilder::try_new(file)
            .unwrap()
            .build()
            .unwrap();
        let batches: Vec<RecordBatch> = reader.collect::<Result<_, _>>().unwrap();
        assert!(!batches.is_empty(), "no row groups in parquet file");
        if batches.len() == 1 {
            return batches.into_iter().next().unwrap();
        }
        arrow::compute::concat_batches(&batches[0].schema(), &batches).unwrap()
    }

    #[test]
    fn test_contextkey_flush_schema() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Record contexts directly.
        w.context_store
            .try_record(100, "cpu.user", "host:a|env:prod")
            .unwrap();
        w.context_store
            .try_record(200, "mem.usage", "host:b")
            .unwrap();

        w.context_keys.extend_from_slice(&[100, 200, 100]);
        w.sources.intern("agent");
        w.sources.intern("agent");
        w.sources.intern("agent");
        w.values.extend_from_slice(&[42.0, 99.0, 43.0]);
        w.timestamps.extend_from_slice(&[1000, 2000, 3000]);
        w.sample_rates.extend_from_slice(&[1.0, 1.0, 1.0]);

        let path = w.flush().unwrap();
        assert!(path.exists());

        w.base.close().unwrap();
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

        let ckey_col = batch
            .column_by_name("context_key")
            .unwrap()
            .as_any()
            .downcast_ref::<UInt64Array>()
            .unwrap();
        let keys: Vec<u64> = ckey_col.values().iter().copied().collect();
        assert_eq!(keys, vec![100, 200, 100]);

        let val_col = batch
            .column_by_name("value")
            .unwrap()
            .as_any()
            .downcast_ref::<Float64Array>()
            .unwrap();
        let vals: Vec<f64> = val_col.values().iter().copied().collect();
        assert_eq!(vals, vec![42.0, 99.0, 43.0]);

        let ts_col = batch
            .column_by_name("timestamp_ns")
            .unwrap()
            .as_any()
            .downcast_ref::<Int64Array>()
            .unwrap();
        let ts: Vec<i64> = ts_col.values().iter().copied().collect();
        assert_eq!(ts, vec![1000, 2000, 3000]);
    }

    #[test]
    fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().is_err());
    }

    #[test]
    fn test_contextkey_roundtrip_with_contexts_bin() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        w.context_store
            .try_record(100, "cpu.user", "host:a|env:prod")
            .unwrap();
        w.context_store
            .try_record(200, "mem.usage", "host:b")
            .unwrap();
        w.context_store.flush().unwrap();

        w.context_keys.extend_from_slice(&[100, 200]);
        w.sources.intern("agent");
        w.sources.intern("agent");
        w.values.extend_from_slice(&[42.0, 99.0]);
        w.timestamps.extend_from_slice(&[1000, 2000]);
        w.sample_rates.extend_from_slice(&[1.0, 1.0]);

        let _path = w.flush().unwrap();

        let contexts = read_contexts_bin(&dir.path().join("contexts.bin")).unwrap();
        assert_eq!(contexts.len(), 2);

        let ctx_map: std::collections::HashMap<u64, (&str, &str)> = contexts
            .iter()
            .map(|(k, n, t)| (*k, (n.as_str(), t.as_str())))
            .collect();

        assert_eq!(ctx_map[&100], ("cpu.user", "host:a|env:prod"));
        assert_eq!(ctx_map[&200], ("mem.usage", "host:b"));
    }

    #[test]
    fn test_telemetry_counters() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        w.context_store
            .try_record(1, "cpu.user", "host:a")
            .unwrap();
        for i in 0..10 {
            w.context_keys.push(1);
            w.sources.intern("test");
            w.values.push(i as f64);
            w.timestamps.push(1000);
            w.sample_rates.push(1.0);
        }

        assert_eq!(w.base.flush_count, 0);
        assert_eq!(w.base.flush_bytes, 0);

        let path = w.flush().unwrap();
        assert_eq!(w.base.flush_count, 1);
        assert!(w.base.last_flush_duration_ns > 0);

        w.base.close().unwrap();
        assert!(w.base.flush_bytes > 0);
        let file_size = std::fs::metadata(&path).unwrap().len();
        assert_eq!(w.base.flush_bytes, file_size);
    }
}
