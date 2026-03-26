use std::fs::File;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, BinaryArray, Int64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;
use parquet::arrow::ArrowWriter;

use super::apply_permutation;
use super::intern::StringInterner;
use super::parquet_helpers::{default_writer_props, dict_utf8_type, interner_to_dict_array};
use super::tags::{decompose_tags_into_interners, LOG_RESERVED_KEYS};
use crate::generated::signals_generated::signals::LogBatch;

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
///
/// Tags are decomposed into per-key reserved columns (service, env, version,
/// team) plus an overflow column. Hostname, source, and status remain as
/// dedicated columns (they already existed in the old schema).
pub struct LogsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

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

    // Telemetry counters (read by the telemetry reporter).
    pub flush_count: u64,
    pub flush_bytes: u64,
    pub rows_written: u64,
    pub last_flush_duration_ns: u64,
}

impl LogsWriter {
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

            flush_count: 0,
            flush_bytes: 0,
            rows_written: 0,
            last_flush_duration_ns: 0,
        }
    }

    /// Number of rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.timestamps.len()
    }

    /// Ingest a LogBatch from FlatBuffers. Flushes automatically when thresholds are reached.
    pub async fn push(&mut self, batch: &LogBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(entries) = batch.entries() {
            for i in 0..entries.len() {
                let e = entries.get(i);

                self.hostnames.intern(e.hostname().unwrap_or(""));
                self.sources.intern(e.source().unwrap_or(""));
                self.statuses.intern(e.status().unwrap_or(""));

                // Decompose tags directly into interners — no intermediate
                // String allocations (was 5 Strings per row before).
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

        if self.len() >= self.flush_rows || self.last_flush.elapsed() >= self.flush_interval {
            return self.flush().await.map(Some);
        }
        Ok(None)
    }

    /// Flush accumulated columns to a new Parquet file. Returns the file path.
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
        let path = self.output_dir.join(format!("flush-logs-{}.parquet", ts_ms));

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

        // Sort index by timestamp for better compression (delta encoding).
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        // Reorder contents in-place using the sort permutation to avoid
        // cloning every Vec<u8> (was doubling peak content memory).
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
            Arc::new(interner_to_dict_array(
                overflow_vals,
                overflow_codes,
                &order,
            )),
            content_array,
            Arc::new(Int64Array::from_iter_values(
                order.iter().map(|&i| timestamps[i]),
            )),
        ];

        let schema = logs_schema();
        let batch = RecordBatch::try_new(schema.clone(), columns)
            .context("building logs RecordBatch")?;

        let file =
            File::create(&path).with_context(|| format!("creating {}", path.display()))?;
        let props = default_writer_props();
        let mut writer = ArrowWriter::try_new(file, schema, Some(props))
            .context("creating Parquet writer for logs")?;
        writer.write(&batch).context("writing logs batch")?;
        writer.close().context("closing logs Parquet writer")?;

        let bytes_written = std::fs::metadata(&path)
            .with_context(|| format!("reading metadata for {}", path.display()))?
            .len();

        self.flush_count += 1;
        self.flush_bytes += bytes_written;
        self.rows_written += row_count as u64;
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

#[cfg(test)]
mod tests {
    use super::*;
    use arrow::array::{Array, AsArray, BinaryArray, Int64Array};
    use arrow::datatypes::UInt32Type;
    use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
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
        assert_eq!(batches.len(), 1, "expected exactly one row group");
        batches.into_iter().next().unwrap()
    }

    fn read_dict_string_column(batch: &RecordBatch, name: &str) -> Vec<String> {
        let col = batch
            .column_by_name(name)
            .unwrap_or_else(|| panic!("column {name} not found"));
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

    fn read_i64_column(batch: &RecordBatch, name: &str) -> Vec<i64> {
        let col = batch
            .column_by_name(name)
            .unwrap_or_else(|| panic!("column {name} not found"));
        col.as_any()
            .downcast_ref::<Int64Array>()
            .unwrap()
            .values()
            .iter()
            .copied()
            .collect()
    }

    fn read_bytes_column(batch: &RecordBatch, name: &str) -> Vec<Vec<u8>> {
        let col = batch
            .column_by_name(name)
            .unwrap_or_else(|| panic!("column {name} not found"));
        let bin = col.as_any().downcast_ref::<BinaryArray>().unwrap();
        (0..bin.len()).map(|i| bin.value(i).to_vec()).collect()
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
            w.contents.push(format!("log line {}", i).into_bytes());
            w.timestamps.push(i as i64 * 1000);
        }
    }

    #[tokio::test]
    async fn test_push_and_flush() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 50);

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
    }

    #[tokio::test]
    async fn test_binary_content_roundtrip() {
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

        let path = w.flush().await.unwrap();
        let batch = read_parquet(&path);

        assert_eq!(batch.num_rows(), 1);
        let contents = read_bytes_column(&batch, "content");
        assert_eq!(contents[0], binary_data);
        let ts = read_i64_column(&batch, "timestamp_ns");
        assert_eq!(ts[0], 42);
        let hostnames = read_dict_string_column(&batch, "hostname");
        assert_eq!(hostnames[0], "h");
    }

    #[tokio::test]
    async fn test_roundtrip_logs_inline() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Row 0
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

        // Row 1
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

        // Row 2 (same as row 0)
        w.hostnames.intern("server1");
        w.sources.intern("syslog");
        w.statuses.intern("warn");
        w.tag_service.intern("");
        w.tag_env.intern("");
        w.tag_version.intern("");
        w.tag_team.intern("ops");
        w.tags_overflow.intern("");
        w.contents.push(b"still going".to_vec());
        w.timestamps.push(99999);

        let logs_path = w.flush().await.unwrap();

        // --- Verify logs file ---
        let batch = read_parquet(&logs_path);
        assert_eq!(batch.num_rows(), 3);

        // Check column names match new schema.
        let schema = batch.schema();
        let col_names: Vec<&str> = schema
            .fields()
            .iter()
            .map(|f| f.name().as_str())
            .collect();
        assert_eq!(
            col_names,
            vec![
                "hostname",
                "source",
                "status",
                "tag_service",
                "tag_env",
                "tag_version",
                "tag_team",
                "tags_overflow",
                "content",
                "timestamp_ns",
            ]
        );

        let hostnames = read_dict_string_column(&batch, "hostname");
        assert_eq!(hostnames, vec!["server1", "server2", "server1"]);

        let sources = read_dict_string_column(&batch, "source");
        assert_eq!(sources, vec!["syslog", "app", "syslog"]);

        let statuses = read_dict_string_column(&batch, "status");
        assert_eq!(statuses, vec!["warn", "error", "warn"]);

        let tag_team = read_dict_string_column(&batch, "tag_team");
        assert_eq!(tag_team, vec!["ops", "sre", "ops"]);

        let tag_env = read_dict_string_column(&batch, "tag_env");
        assert_eq!(tag_env, vec!["", "prod", ""]);

        let contents: Vec<String> = read_bytes_column(&batch, "content")
            .into_iter()
            .map(|b| String::from_utf8(b).unwrap())
            .collect();
        assert_eq!(
            contents,
            vec!["hello world", "something went wrong", "still going"]
        );

        let timestamps = read_i64_column(&batch, "timestamp_ns");
        assert_eq!(timestamps, vec![12345, 67890, 99999]);
    }

    #[tokio::test]
    async fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().await.is_err());
    }

    #[tokio::test]
    async fn test_telemetry_counters() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 10);

        assert_eq!(w.flush_count, 0);
        assert_eq!(w.flush_bytes, 0);

        let path = w.flush().await.unwrap();
        assert_eq!(w.flush_count, 1);
        assert!(w.flush_bytes > 0);
        assert!(w.last_flush_duration_ns > 0);

        // flush_bytes should match the file size on disk.
        let file_size = std::fs::metadata(&path).unwrap().len();
        assert_eq!(w.flush_bytes, file_size);
    }
}
