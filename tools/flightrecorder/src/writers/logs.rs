use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, BinaryArray, Int64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;

use super::apply_permutation;
use super::intern::StringInterner;
use super::parquet_helpers::{dict_utf8_type, interner_to_dict_array, BaseWriter};
use super::tags::{decompose_tags_into_interners, LOG_RESERVED_KEYS};
use super::thread::{SignalWriter, WriterStats};
use crate::generated::signals_generated::signals;

/// Schema for the logs Parquet file (10 columns).
fn logs_schema() -> Arc<Schema> {
    let dt = dict_utf8_type();
    Arc::new(Schema::new(vec![
        Field::new("hostname", dt.clone(), false),
        Field::new("source", dt.clone(), false),
        Field::new("status", dt.clone(), false),
        Field::new("tag_service", dt.clone(), false),
        Field::new("tag_env", dt.clone(), false),
        Field::new("tag_version", dt.clone(), false),
        Field::new("tag_team", dt.clone(), false),
        Field::new("tags_overflow", dt, false),
        Field::new("content", DataType::Binary, false),
        Field::new("timestamp_ns", DataType::Int64, false),
    ]))
}

/// Columnar accumulator for log entries.
pub struct LogsWriter {
    pub base: BaseWriter,

    // Interned string columns (dictionary-encoded on flush).
    hostnames: StringInterner,
    sources: StringInterner,
    statuses: StringInterner,
    tag_service: StringInterner,
    tag_env: StringInterner,
    tag_version: StringInterner,
    tag_team: StringInterner,
    tags_overflow: StringInterner,

    // Plain columnar buffers.
    contents: Vec<Vec<u8>>,
    timestamps: Vec<i64>,
}

impl LogsWriter {
    pub fn new(
        output_dir: impl AsRef<Path>,
        flush_rows: usize,
        flush_interval: Duration,
    ) -> Self {
        Self {
            base: BaseWriter::new(output_dir.as_ref(), flush_rows, flush_interval),

            hostnames: StringInterner::with_capacity(flush_rows),
            sources: StringInterner::with_capacity(flush_rows),
            statuses: StringInterner::with_capacity(flush_rows),
            tag_service: StringInterner::with_capacity(flush_rows),
            tag_env: StringInterner::with_capacity(flush_rows),
            tag_version: StringInterner::with_capacity(flush_rows),
            tag_team: StringInterner::with_capacity(flush_rows),
            tags_overflow: StringInterner::with_capacity(flush_rows),

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
        if let Some(entries) = batch.entries() {
            for i in 0..entries.len() {
                let e = entries.get(i);

                self.hostnames.intern(e.hostname().unwrap_or(""));
                self.sources.intern(e.source().unwrap_or(""));
                self.statuses.intern(e.status().unwrap_or(""));

                decompose_tags_into_interners(
                    e.tags(),
                    LOG_RESERVED_KEYS,
                    &mut [
                        &mut self.tag_service,
                        &mut self.tag_env,
                        &mut self.tag_version,
                        &mut self.tag_team,
                    ],
                    &mut self.tags_overflow,
                );

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

        // Take columns.
        let (hostname_vals, hostname_codes) = self.hostnames.take();
        let (source_vals, source_codes) = self.sources.take();
        let (status_vals, status_codes) = self.statuses.take();
        let (service_vals, service_codes) = self.tag_service.take();
        let (env_vals, env_codes) = self.tag_env.take();
        let (version_vals, version_codes) = self.tag_version.take();
        let (team_vals, team_codes) = self.tag_team.take();
        let (overflow_vals, overflow_codes) = self.tags_overflow.take();
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
            Arc::new(interner_to_dict_array(hostname_vals, hostname_codes, &order)),
            Arc::new(interner_to_dict_array(source_vals, source_codes, &order)),
            Arc::new(interner_to_dict_array(status_vals, status_codes, &order)),
            Arc::new(interner_to_dict_array(service_vals, service_codes, &order)),
            Arc::new(interner_to_dict_array(env_vals, env_codes, &order)),
            Arc::new(interner_to_dict_array(version_vals, version_codes, &order)),
            Arc::new(interner_to_dict_array(team_vals, team_codes, &order)),
            Arc::new(interner_to_dict_array(overflow_vals, overflow_codes, &order)),
            content_array,
            Arc::new(Int64Array::from_iter_values(
                order.iter().map(|&i| timestamps[i]),
            )),
        ];

        let schema = logs_schema();
        let batch = RecordBatch::try_new(schema.clone(), columns)
            .context("building logs RecordBatch")?;

        self.base.write_batch("logs", schema, batch)
    }
}

impl SignalWriter for LogsWriter {
    fn process_frame(&mut self, buf: &[u8]) -> Result<()> {
        let env = flatbuffers::root::<signals::SignalEnvelope>(buf)
            .map_err(|e| anyhow::anyhow!("decode error: {e}"))?;
        if let Some(batch) = env.payload_as_log_batch() {
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
    use arrow::array::{Array, AsArray};
    use arrow::datatypes::UInt32Type;
    use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
    use std::fs::File;
    use tempfile::tempdir;

    fn make_writer(dir: &Path) -> LogsWriter {
        LogsWriter::new(dir, 1000, Duration::from_secs(60))
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
            w.hostnames.intern("host1");
            w.sources.intern("app");
            w.statuses.intern("info");
            w.tag_service.intern("");
            w.tag_env.intern("test");
            w.tag_version.intern("");
            w.tag_team.intern("");
            w.tags_overflow.intern("");
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
        assert_eq!(batch.num_columns(), 10);
    }

    #[test]
    fn test_binary_content_roundtrip() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        let binary_data = vec![0u8, 1, 2, 3, 255, 0];
        w.hostnames.intern("h");
        w.sources.intern("");
        w.statuses.intern("info");
        w.tag_service.intern("");
        w.tag_env.intern("");
        w.tag_version.intern("");
        w.tag_team.intern("");
        w.tags_overflow.intern("");
        w.contents.push(binary_data.clone());
        w.timestamps.push(42);

        let path = w.flush().unwrap();
        w.base.close().unwrap();
        let batch = read_parquet(&path);
        assert_eq!(batch.num_rows(), 1);

        // Verify binary content roundtrip.
        let content_col = batch
            .column_by_name("content")
            .unwrap()
            .as_any()
            .downcast_ref::<arrow::array::BinaryArray>()
            .unwrap();
        assert_eq!(content_col.value(0), binary_data.as_slice());
    }

    #[test]
    fn test_roundtrip_logs_inline() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        w.hostnames.intern("server1");
        w.sources.intern("syslog");
        w.statuses.intern("warn");
        w.tag_service.intern("");
        w.tag_env.intern("");
        w.tag_version.intern("");
        w.tag_team.intern("ops");
        w.tags_overflow.intern("");
        w.contents.push(b"hello world".to_vec());
        w.timestamps.push(12345);

        w.hostnames.intern("server2");
        w.sources.intern("app");
        w.statuses.intern("error");
        w.tag_service.intern("");
        w.tag_env.intern("prod");
        w.tag_version.intern("");
        w.tag_team.intern("sre");
        w.tags_overflow.intern("");
        w.contents.push(b"something went wrong".to_vec());
        w.timestamps.push(67890);

        let path = w.flush().unwrap();
        w.base.close().unwrap();
        let batch = read_parquet(&path);
        assert_eq!(batch.num_rows(), 2);

        let schema = batch.schema();
        let col_names: Vec<&str> = schema
            .fields()
            .iter()
            .map(|f| f.name().as_str())
            .collect();
        assert_eq!(
            col_names,
            vec![
                "hostname", "source", "status", "tag_service", "tag_env",
                "tag_version", "tag_team", "tags_overflow", "content", "timestamp_ns",
            ]
        );

        // Verify hostnames (sorted by timestamp: 12345, 67890).
        let hostname_col = batch.column_by_name("hostname").unwrap();
        let dict = hostname_col.as_dictionary::<UInt32Type>();
        let vals = dict.values().as_string::<i32>();
        let h0 = vals.value(dict.keys().value(0) as usize);
        let h1 = vals.value(dict.keys().value(1) as usize);
        assert_eq!(h0, "server1");
        assert_eq!(h1, "server2");
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
        // flush_bytes is updated on file rotation/close.
        w.base.close().unwrap();
        assert!(w.base.flush_bytes > 0);
        assert_eq!(w.base.flush_bytes, std::fs::metadata(&path).unwrap().len());
    }
}
