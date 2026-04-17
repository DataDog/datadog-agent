use std::path::{Path, PathBuf};
use std::sync::{Arc, LazyLock};
use std::time::Duration;

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, BinaryArray, Int64Array, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;

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

/// Byte offset into an arena frame buffer.
struct ContentRef {
    frame_idx: u32,
    offset: u32,
    len: u32,
}

/// Columnar accumulator for log entries.
///
/// Uses an arena-based zero-copy design: incoming FlatBuffers frame buffers are
/// retained in `arena` and log content is referenced by offset, avoiding
/// per-entry heap allocations. At flush time, content slices are read directly
/// from the arena buffers to build the Arrow BinaryArray.
pub struct LogsWriter {
    pub base: BaseWriter,

    // Context records are forwarded to the shared context writer thread.
    ctx_producer: ContextProducer,

    // Buffer pool for recycling frame buffers after flush.
    buffer_pool: crate::BufferPool,

    // Arena: retained frame buffers. Returned to pool on flush.
    arena: Vec<Vec<u8>>,

    // Per-entry columnar data.
    context_keys: Vec<u64>,
    content_refs: Vec<ContentRef>,
    timestamps: Vec<i64>,
}

impl LogsWriter {
    pub fn new(
        output_dir: impl AsRef<Path>,
        flush_rows: usize,
        flush_interval: Duration,
        rotation_interval: Duration,
        ctx_producer: ContextProducer,
        disk_tracker: Arc<DiskTracker>,
        buffer_pool: crate::BufferPool,
    ) -> Self {
        Self {
            base: BaseWriter::new(output_dir.as_ref(), flush_rows, flush_interval, rotation_interval, disk_tracker),
            ctx_producer,
            buffer_pool,
            arena: Vec::new(),
            context_keys: Vec::with_capacity(flush_rows),
            content_refs: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
        }
    }

    /// Number of rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.timestamps.len()
    }

    /// Ingest a LogBatch from an owned FlatBuffers frame buffer.
    ///
    /// The frame buffer is retained in the arena so that log content bytes
    /// can be referenced without copying. Flushes automatically when
    /// thresholds are reached.
    pub fn push_owned(&mut self, buf: Vec<u8>) -> Result<Option<PathBuf>> {
        let env = flatbuffers::root::<signals::SignalEnvelope>(&buf)
            .map_err(|e| anyhow::anyhow!("decode error: {e}"))?;

        let batch = match env.log_batch() {
            Some(b) => b,
            None => return Ok(None),
        };

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

        // Process log entries — record offsets into the frame buffer.
        if let Some(entries) = batch.entries() {
            if entries.len() > 0 {
                let frame_idx = self.arena.len() as u32;

                for i in 0..entries.len() {
                    let e = entries.get(i);
                    self.context_keys.push(e.context_key());
                    // Record the content byte range within the frame buffer.
                    // FlatBuffers content() returns a slice into buf.
                    match e.content() {
                        Some(c) => {
                            let bytes = c.bytes();
                            let offset = bytes.as_ptr() as usize - buf.as_ptr() as usize;
                            self.content_refs.push(ContentRef {
                                frame_idx,
                                offset: offset as u32,
                                len: bytes.len() as u32,
                            });
                        }
                        None => {
                            self.content_refs.push(ContentRef {
                                frame_idx,
                                offset: 0,
                                len: 0,
                            });
                        }
                    }
                    self.timestamps.push(e.timestamp_ns());
                }

                // Retain the frame buffer in the arena.
                self.arena.push(buf);
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
        let content_refs = std::mem::take(&mut self.content_refs);
        let arena = std::mem::take(&mut self.arena);
        let timestamps = std::mem::take(&mut self.timestamps);

        // Sort index by timestamp for better compression.
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        // Build the BinaryArray by slicing into arena buffers — zero copy.
        let content_array: ArrayRef = Arc::new(BinaryArray::from_iter_values(
            order.iter().map(|&i| {
                let r = &content_refs[i];
                if r.len == 0 {
                    &[] as &[u8]
                } else {
                    let frame = &arena[r.frame_idx as usize];
                    &frame[r.offset as usize..r.offset as usize + r.len as usize]
                }
            }),
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

        let result = self.base.write_batch("logs", schema, batch);

        // Return arena frame buffers to the pool for reuse.
        for buf in arena {
            self.buffer_pool.put(buf);
        }

        result
    }
}

impl SignalWriter for LogsWriter {
    fn process_frame(&mut self, buf: Vec<u8>) -> Result<Option<Vec<u8>>> {
        self.push_owned(buf)?;
        Ok(None) // buffer retained in arena
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
        let dir = tempdir().unwrap();
        let store = ContextStore::new(dir.path()).unwrap();
        let (_handle, _prod_m, prod_l) = ContextWriterHandle::spawn(store, 64, None);
        std::mem::forget(_handle);
        std::mem::forget(dir);
        prod_l
    }

    fn make_writer(dir: &Path) -> LogsWriter {
        LogsWriter::new(
            dir,
            1000,
            Duration::from_secs(60),
            Duration::from_secs(60),
            make_ctx_producer(),
            Arc::new(DiskTracker::noop()),
            crate::BufferPool::new(),
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

    /// Build a FlatBuffers LogBatch frame with `n` entries for testing.
    fn build_log_frame(entries: &[(u64, &[u8], i64)]) -> Vec<u8> {
        use crate::generated::signals_generated::signals as sig;
        let mut fbb = flatbuffers::FlatBufferBuilder::with_capacity(4096);
        let mut offsets = Vec::with_capacity(entries.len());
        for &(ckey, content, ts) in entries.iter().rev() {
            let content_off = fbb.create_vector(content);
            let entry = sig::LogEntry::create(
                &mut fbb,
                &sig::LogEntryArgs {
                    context_key: ckey,
                    content: Some(content_off),
                    timestamp_ns: ts,
                },
            );
            offsets.push(entry);
        }
        offsets.reverse();
        let vec = fbb.create_vector(&offsets);
        let batch = sig::LogBatch::create(
            &mut fbb,
            &sig::LogBatchArgs {
                contexts: None,
                entries: Some(vec),
            },
        );
        let env = sig::SignalEnvelope::create(
            &mut fbb,
            &sig::SignalEnvelopeArgs {
                log_batch: Some(batch),
                ..Default::default()
            },
        );
        fbb.finish(env, None);
        fbb.finished_data().to_vec()
    }

    #[test]
    fn test_push_and_flush() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        let entries: Vec<(u64, &[u8], i64)> = (0..50)
            .map(|i| (i as u64 % 10 + 1, format!("log line {i}").leak().as_bytes(), i as i64 * 1000))
            .collect();
        let frame = build_log_frame(&entries);
        w.process_frame(frame).unwrap();

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

        let frame = build_log_frame(&[(1, b"hello", 1000)]);
        w.process_frame(frame).unwrap();

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
        let binary_data: &[u8] = &[0u8, 1, 2, 3, 255, 0];

        let frame = build_log_frame(&[(42, binary_data, 42)]);
        w.process_frame(frame).unwrap();

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
        assert_eq!(content_col.value(0), binary_data);
    }

    #[test]
    fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().is_err());
    }

    #[test]
    fn test_multiple_frames_arena() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Push multiple frames — each retained in the arena.
        for i in 0..5 {
            let frame = build_log_frame(&[
                (i as u64, format!("frame {i} line 0").leak().as_bytes(), i as i64 * 1000),
                (i as u64, format!("frame {i} line 1").leak().as_bytes(), i as i64 * 1000 + 1),
            ]);
            w.process_frame(frame).unwrap();
        }

        assert_eq!(w.len(), 10);
        assert_eq!(w.arena.len(), 5); // 5 frames retained

        let path = w.flush().unwrap();
        w.base.close().unwrap();
        let batch = read_parquet(&path);
        assert_eq!(batch.num_rows(), 10);

        // Arena should be cleared after flush.
        assert_eq!(w.arena.len(), 0);
    }

    #[test]
    fn test_telemetry_counters() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        let entries: Vec<(u64, &[u8], i64)> = (0..10)
            .map(|i| (1u64, b"test".as_slice(), i as i64 * 1000))
            .collect();
        let frame = build_log_frame(&entries);
        w.process_frame(frame).unwrap();

        assert_eq!(w.base.flush_count, 0);
        let path = w.flush().unwrap();
        assert_eq!(w.base.flush_count, 1);
        assert_eq!(w.base.rows_written, 10);
        w.base.close().unwrap();
        assert!(w.base.flush_bytes > 0);
        assert_eq!(w.base.flush_bytes, std::fs::metadata(&path).unwrap().len());
    }
}
