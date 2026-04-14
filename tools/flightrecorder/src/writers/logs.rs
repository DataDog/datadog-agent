use std::path::{Path, PathBuf};
use std::sync::{Arc, LazyLock};
use std::time::Duration;

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, BinaryArray, Int64Array, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;

use super::apply_permutation;
use super::context_writer::{ContextProducer, ContextRecord};
use super::parquet_helpers::{BaseWriter, DiskTracker};
use super::thread::{SignalWriter, WriterStats};
use crate::generated::signals_generated::signals;

/// Schema for the logs Parquet file (3 columns).
///
/// Log metadata (hostname, status, tags) is stored once in the shared
/// contexts.bin via ContextEntry. Each log row stores only a context_key
/// reference, the raw content, and a timestamp.
static LOGS_SCHEMA: LazyLock<Arc<Schema>> = LazyLock::new(|| {
    Arc::new(Schema::new(vec![
        Field::new("context_key", DataType::UInt64, false),
        Field::new("content", DataType::Binary, false),
        Field::new("timestamp_ns", DataType::Int64, false),
    ]))
});

/// Columnar accumulator for log entries.
pub struct LogsWriter {
    pub base: BaseWriter,

    // Context records are forwarded to the shared context writer thread.
    ctx_producer: ContextProducer,

    // Plain columnar buffers (3 columns).
    context_keys: Vec<u64>,
    contents: Vec<Vec<u8>>,
    timestamps: Vec<i64>,
}

impl LogsWriter {
    pub fn new(
        output_dir: impl AsRef<Path>,
        flush_rows: usize,
        flush_interval: Duration,
        ctx_producer: ContextProducer,
        disk_tracker: Arc<DiskTracker>,
    ) -> Self {
        Self {
            base: BaseWriter::new(output_dir.as_ref(), flush_rows, flush_interval, disk_tracker),
            ctx_producer,
            context_keys: Vec::with_capacity(flush_rows),
            contents: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
        }
    }

    /// Number of rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.timestamps.len()
    }

    /// Ingest a LogBatch from FlatBuffers. Flushes automatically when thresholds are reached.
    pub fn push(&mut self, batch: &signals::LogBatch<'_>) -> Result<Option<PathBuf>> {
        // Forward context definitions to the shared context writer thread.
        if let Some(contexts) = batch.contexts() {
            for i in 0..contexts.len() {
                let ctx = contexts.get(i);
                let ckey = ctx.context_key();
                if ckey != 0 {
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
                        name: ctx.name().unwrap_or("").to_string(),
                        tags_joined,
                    });
                }
            }
        }

        // Process log entries → accumulate for Parquet.
        if let Some(entries) = batch.entries() {
            for i in 0..entries.len() {
                let e = entries.get(i);
                self.context_keys.push(e.context_key());
                self.contents.push(
                    e.content().map(|c| c.bytes().to_vec()).unwrap_or_default(),
                );
                self.timestamps.push(e.timestamp_ns());
            }
        }

        if self.base.should_flush(self.len()) {
            return self.flush().map(Some);
        }
        Ok(None)
    }

    /// Flush accumulated columns to a new Parquet row group.
    pub fn flush(&mut self) -> Result<PathBuf> {
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no rows to flush");
        }

        let keys = std::mem::take(&mut self.context_keys);
        let contents = std::mem::take(&mut self.contents);
        let timestamps = std::mem::take(&mut self.timestamps);

        // Sort index by timestamp for better compression.
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        let sorted_contents = apply_permutation(contents, &order);
        let content_array: ArrayRef = Arc::new(BinaryArray::from_iter_values(
            sorted_contents.iter().map(|v| v.as_slice()),
        ));

        let columns: Vec<ArrayRef> = vec![
            Arc::new(UInt64Array::from_iter_values(
                order.iter().map(|&i| keys[i]),
            )),
            content_array,
            Arc::new(Int64Array::from_iter_values(
                order.iter().map(|&i| timestamps[i]),
            )),
        ];

        let schema = LOGS_SCHEMA.clone();
        let batch = RecordBatch::try_new(schema.clone(), columns)
            .context("building logs RecordBatch")?;

        self.base.write_batch("logs", schema, batch)
    }
}

impl SignalWriter for LogsWriter {
    fn process_frame(&mut self, buf: &[u8]) -> Result<()> {
        let env = flatbuffers::root::<signals::SignalEnvelope>(buf)
            .map_err(|e| anyhow::anyhow!("decode error: {e}"))?;
        if let Some(batch) = env.log_batch() {
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
    use super::*;
    use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
    use std::fs::File;
    use tempfile::tempdir;

    fn make_ctx_producer() -> ContextProducer {
        use super::super::context_store::ContextStore;
        use super::super::context_writer::ContextWriterHandle;
        // Spawn a real context writer with a noop store for tests.
        let dir = tempdir().unwrap();
        let store = ContextStore::new(dir.path()).unwrap();
        let (_handle, _prod_m, prod_l) = ContextWriterHandle::spawn(store, 64);
        // Leak the handle — test cleanup is handled by tempdir drop.
        std::mem::forget(_handle);
        std::mem::forget(dir);
        prod_l
    }

    fn make_writer(dir: &Path) -> LogsWriter {
        LogsWriter::new(
            dir,
            1000,
            Duration::from_secs(60),
            make_ctx_producer(),
            Arc::new(DiskTracker::noop()),
        )
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

    fn add_rows(w: &mut LogsWriter, n: usize) {
        for i in 0..n {
            w.context_keys.push(i as u64 % 100 + 1);
            w.contents.push(format!("log line {i}").into_bytes());
            w.timestamps.push(i as i64 * 1000);
        }
    }

    #[test]
    fn test_push_and_flush() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 50);

        let path = w.flush().unwrap();
        assert!(path.exists());
        w.base.close().unwrap();
        let batch = read_parquet(&path);
        assert_eq!(batch.num_rows(), 50);
        assert_eq!(batch.num_columns(), 3);
    }

    #[test]
    fn test_column_names() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 1);

        let path = w.flush().unwrap();
        w.base.close().unwrap();
        let batch = read_parquet(&path);

        let schema = batch.schema();
        let col_names: Vec<&str> = schema
            .fields()
            .iter()
            .map(|f| f.name().as_str())
            .collect();
        assert_eq!(col_names, vec!["context_key", "content", "timestamp_ns"]);
    }

    #[test]
    fn test_binary_content_roundtrip() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        let binary_data = vec![0u8, 1, 2, 3, 255, 0];
        w.context_keys.push(42);
        w.contents.push(binary_data.clone());
        w.timestamps.push(42);

        let path = w.flush().unwrap();
        w.base.close().unwrap();
        let batch = read_parquet(&path);
        assert_eq!(batch.num_rows(), 1);

        let content_col = batch
            .column_by_name("content")
            .unwrap()
            .as_any()
            .downcast_ref::<arrow::array::BinaryArray>()
            .unwrap();
        assert_eq!(content_col.value(0), binary_data.as_slice());
    }

    #[test]
    fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().is_err());
    }

    #[test]
    fn test_telemetry_counters() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 10);

        assert_eq!(w.base.flush_count, 0);
        let path = w.flush().unwrap();
        assert_eq!(w.base.flush_count, 1);
        assert_eq!(w.base.rows_written, 10);
        w.base.close().unwrap();
        assert!(w.base.flush_bytes > 0);
        assert_eq!(w.base.flush_bytes, std::fs::metadata(&path).unwrap().len());
    }
}
