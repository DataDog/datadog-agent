use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, Float64Array, Int64Array, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;

use std::sync::LazyLock;

use super::context_writer::{ContextProducer, ContextRecord};
use super::intern::StringInterner;
use super::parquet_helpers::{dict_utf8_type, interner_to_dict_array, BaseWriter, DiskTracker};
use super::thread::{SignalWriter, WriterStats};
use crate::generated::signals_generated::signals;

/// Schema for context-key mode (5 columns).
static CONTEXTKEY_SCHEMA: LazyLock<Arc<Schema>> = LazyLock::new(|| {
    Arc::new(Schema::new(vec![
        Field::new("context_key", DataType::UInt64, false),
        Field::new("value", DataType::Float64, false),
        Field::new("timestamp_ns", DataType::Int64, false),
        Field::new("sample_rate", DataType::Float64, false),
        Field::new("source", dict_utf8_type(), false),
    ]))
});

/// Columnar accumulator for metric samples.
///
/// Handles both ContextEntry (forwarded to context writer thread) and
/// PointEntry (written to Parquet). A single MetricBatch frame may contain
/// either or both vectors.
pub struct MetricsWriter {
    pub base: BaseWriter,

    // Accumulation columns for points.
    sources: StringInterner,
    values: Vec<f64>,
    timestamps: Vec<i64>,
    sample_rates: Vec<f64>,
    context_keys: Vec<u64>,

    // Context records are forwarded to the shared context writer thread.
    ctx_producer: ContextProducer,
}

impl MetricsWriter {
    pub fn new(
        output_dir: impl AsRef<Path>,
        flush_rows: usize,
        flush_interval: Duration,
        ctx_producer: ContextProducer,
        disk_tracker: Arc<DiskTracker>,
    ) -> Self {
        Self {
            base: BaseWriter::new(output_dir.as_ref(), flush_rows, flush_interval, disk_tracker),

            sources: StringInterner::with_capacity(flush_rows),
            values: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
            sample_rates: Vec::with_capacity(flush_rows),
            context_keys: Vec::with_capacity(flush_rows),

            ctx_producer,
        }
    }

    /// Number of point rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.values.len()
    }

    /// Process a MetricBatch: handle context definitions and data points.
    pub fn push(&mut self, batch: &signals::MetricBatch<'_>) -> Result<Option<PathBuf>> {
        // Forward context definitions to the shared context writer thread.
        if let Some(contexts) = batch.contexts() {
            for i in 0..contexts.len() {
                let ctx = contexts.get(i);
                let ckey = ctx.context_key();
                let name = ctx.name().unwrap_or("");
                if ckey != 0 && !name.is_empty() {
                    let tags_joined: String = ctx
                        .tags()
                        .map(|tl| {
                            (0..tl.len())
                                .map(|j| tl.get(j))
                                .collect::<Vec<_>>()
                                .join("|")
                        })
                        .unwrap_or_default();
                    self.ctx_producer.try_send(ContextRecord {
                        key: ckey,
                        name: name.to_string(),
                        tags_joined,
                    });
                }
            }
        }

        // Process data points → accumulate for Parquet.
        if let Some(points) = batch.points() {
            for i in 0..points.len() {
                let p = points.get(i);
                self.context_keys.push(p.context_key());
                self.sources.intern(""); // source is on context, not point
                self.values.push(p.value());
                self.timestamps.push(p.timestamp_ns());
                self.sample_rates.push(p.sample_rate());
            }
        }

        if self.base.should_flush(self.len()) {
            return self.flush().map(Some);
        }
        Ok(None)
    }

    /// Flush accumulated point columns to a Parquet row group.
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

        let schema = CONTEXTKEY_SCHEMA.clone();
        let batch = RecordBatch::try_new(schema.clone(), columns)
            .context("building metrics RecordBatch")?;

        self.base.write_batch("metrics", schema, batch)
    }
}

impl SignalWriter for MetricsWriter {
    fn process_frame(&mut self, buf: &[u8]) -> Result<()> {
        let env = flatbuffers::root::<signals::SignalEnvelope>(buf)
            .map_err(|e| anyhow::anyhow!("decode error: {e}"))?;
        if let Some(batch) = env.metric_batch() {
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
    use super::super::context_store::{read_contexts_bin, ContextStore};
    use super::super::context_writer::ContextWriterHandle;
    use super::*;
    use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
    use std::fs::File;
    use tempfile::tempdir;

    /// Spawn a context writer thread for testing. Returns the producer and
    /// the output dir (kept alive by the returned TempDir).
    fn make_ctx_producer(dir: &Path) -> ContextProducer {
        let store = ContextStore::new(dir).unwrap();
        let (_handle, prod_m, _prod_l) = ContextWriterHandle::spawn(store, 64);
        std::mem::forget(_handle);
        prod_m
    }

    fn make_writer(dir: &Path) -> MetricsWriter {
        let prod = make_ctx_producer(dir);
        MetricsWriter::new(dir, 1000, Duration::from_secs(60), prod, Arc::new(DiskTracker::noop()))
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

        w.context_keys.extend_from_slice(&[100, 200, 100]);
        w.sources.intern("");
        w.sources.intern("");
        w.sources.intern("");
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

        // Write contexts through the context writer thread.
        let store = ContextStore::new(dir.path()).unwrap();
        let (mut ctx_handle, mut prod_m, _prod_l) = ContextWriterHandle::spawn(store, 64);
        prod_m.try_send(ContextRecord { key: 100, name: "cpu.user".into(), tags_joined: "host:a|env:prod".into() });
        prod_m.try_send(ContextRecord { key: 200, name: "mem.usage".into(), tags_joined: "host:b".into() });
        // Give the context writer thread time to process.
        std::thread::sleep(std::time::Duration::from_millis(50));
        ctx_handle.shutdown();

        let mut w = MetricsWriter::new(dir.path(), 1000, Duration::from_secs(60), make_ctx_producer(dir.path()), Arc::new(DiskTracker::noop()));

        w.context_keys.extend_from_slice(&[100, 200]);
        w.sources.intern("");
        w.sources.intern("");
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

        for i in 0..10 {
            w.context_keys.push(1);
            w.sources.intern("");
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
