use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, BinaryArray, Int64Array, UInt32Array, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;

use super::apply_permutation;
use super::intern::StringInterner;
use super::parquet_helpers::{dict_utf8_type, interner_to_dict_array, BaseWriter};
use crate::generated::signals_generated::signals::TraceStatsBatch;

/// Schema for the trace stats Parquet file (18 columns).
fn trace_stats_schema() -> Arc<Schema> {
    let dt = dict_utf8_type();
    Arc::new(Schema::new(vec![
        Field::new("service", dt.clone(), false),
        Field::new("name", dt.clone(), false),
        Field::new("resource", dt.clone(), false),
        Field::new("type", dt.clone(), false),
        Field::new("span_kind", dt.clone(), false),
        Field::new("hostname", dt.clone(), false),
        Field::new("env", dt.clone(), false),
        Field::new("version", dt, false),
        Field::new("http_status_code", DataType::UInt32, false),
        Field::new("hits", DataType::UInt64, false),
        Field::new("errors", DataType::UInt64, false),
        Field::new("duration_ns", DataType::UInt64, false),
        Field::new("top_level_hits", DataType::UInt64, false),
        Field::new("ok_summary", DataType::Binary, false),
        Field::new("error_summary", DataType::Binary, false),
        Field::new("bucket_start_ns", DataType::Int64, false),
        Field::new("bucket_duration_ns", DataType::Int64, false),
        Field::new("timestamp_ns", DataType::Int64, false),
    ]))
}

/// Columnar accumulator for trace stats entries.
///
/// Trace stats are already aggregated (low volume: ~10-100 entries per 10s),
/// so no context-key deduplication or split ring buffers are needed.
pub struct TraceStatsWriter {
    pub base: BaseWriter,

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

}

impl TraceStatsWriter {
    pub fn new(
        output_dir: impl AsRef<Path>,
        flush_rows: usize,
        flush_interval: Duration,
    ) -> Self {
        let output_dir = output_dir.as_ref();
        Self {
            base: BaseWriter::new(output_dir, flush_rows, flush_interval),

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
                    e.error_summary()
                        .map(|v| v.bytes().to_vec())
                        .unwrap_or_default(),
                );
            }
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

        // Reorder binary columns in-place (avoids cloning every Vec<u8>).
        let sorted_ok = apply_permutation(ok_summaries, &order);
        let sorted_err = apply_permutation(error_summaries, &order);

        let columns: Vec<ArrayRef> = vec![
            Arc::new(interner_to_dict_array(service_vals, service_codes, &order)),
            Arc::new(interner_to_dict_array(name_vals, name_codes, &order)),
            Arc::new(interner_to_dict_array(
                resource_vals,
                resource_codes,
                &order,
            )),
            Arc::new(interner_to_dict_array(type_vals, type_codes, &order)),
            Arc::new(interner_to_dict_array(
                span_kind_vals,
                span_kind_codes,
                &order,
            )),
            Arc::new(interner_to_dict_array(
                hostname_vals,
                hostname_codes,
                &order,
            )),
            Arc::new(interner_to_dict_array(env_vals, env_codes, &order)),
            Arc::new(interner_to_dict_array(version_vals, version_codes, &order)),
            Arc::new(UInt32Array::from_iter_values(
                order.iter().map(|&i| http_status_codes[i]),
            )),
            Arc::new(UInt64Array::from_iter_values(
                order.iter().map(|&i| hits[i]),
            )),
            Arc::new(UInt64Array::from_iter_values(
                order.iter().map(|&i| errors[i]),
            )),
            Arc::new(UInt64Array::from_iter_values(
                order.iter().map(|&i| durations[i]),
            )),
            Arc::new(UInt64Array::from_iter_values(
                order.iter().map(|&i| top_level_hits[i]),
            )),
            Arc::new(BinaryArray::from_iter_values(
                sorted_ok.iter().map(|v| v.as_slice()),
            )),
            Arc::new(BinaryArray::from_iter_values(
                sorted_err.iter().map(|v| v.as_slice()),
            )),
            Arc::new(Int64Array::from_iter_values(
                order.iter().map(|&i| bucket_starts[i]),
            )),
            Arc::new(Int64Array::from_iter_values(
                order.iter().map(|&i| bucket_durations[i]),
            )),
            Arc::new(Int64Array::from_iter_values(
                order.iter().map(|&i| timestamps[i]),
            )),
        ];

        let schema = trace_stats_schema();
        let batch = RecordBatch::try_new(schema.clone(), columns)
            .context("building trace_stats RecordBatch")?;

        self.base.write_batch("trace_stats", schema, batch)
    }

    /// Flush any buffered rows and close the active Parquet file. Used on shutdown.
    pub async fn flush_if_any(&mut self) -> Result<Option<PathBuf>> {
        let result = if self.len() == 0 {
            Ok(None)
        } else {
            self.flush().await.map(Some)
        };
        self.base.close()?;
        result
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use arrow::array::{Array, AsArray, BinaryArray, Int64Array, UInt32Array, UInt64Array};
    use arrow::datatypes::UInt32Type;
    use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
    use std::fs::File;
    use tempfile::tempdir;

    fn make_writer(dir: &Path) -> TraceStatsWriter {
        TraceStatsWriter::new(dir, 1000, Duration::from_secs(60))
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

    fn add_rows(w: &mut TraceStatsWriter, n: usize) {
        for i in 0..n {
            w.services.intern("web-service");
            w.names.intern("http.request");
            w.resources.intern("/api/v1/users");
            w.types.intern("web");
            w.span_kinds.intern("server");
            w.hostnames.intern("host1");
            w.envs.intern("prod");
            w.versions.intern("1.0.0");

            w.http_status_codes.push(200);
            w.hits.push(100 + i as u64);
            w.errors.push(i as u64);
            w.durations.push(1_000_000 * (i as u64 + 1));
            w.top_level_hits.push(50 + i as u64);
            w.bucket_starts.push(1_000_000_000 * i as i64);
            w.bucket_durations.push(10_000_000_000);
            w.timestamps.push(i as i64 * 1000);

            w.ok_summaries.push(vec![0x0a, 0x01, i as u8]);
            w.error_summaries.push(vec![0x0b, 0x02, i as u8]);
        }
    }

    #[tokio::test]
    async fn test_push_and_flush() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 50);

        let path = w.flush().await.unwrap();
        assert!(path.exists());

        w.base.close().unwrap();
        assert!(path.metadata().unwrap().len() > 0);
        let batch = read_parquet(&path);
        assert_eq!(batch.num_rows(), 50);
        assert_eq!(batch.num_columns(), 18);
    }

    #[tokio::test]
    async fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().await.is_err());
    }

    #[tokio::test]
    async fn test_roundtrip_trace_stats() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Row 0 (later timestamp).
        w.services.intern("svc-a");
        w.names.intern("op1");
        w.resources.intern("/foo");
        w.types.intern("web");
        w.span_kinds.intern("server");
        w.hostnames.intern("h1");
        w.envs.intern("staging");
        w.versions.intern("2.0");
        w.http_status_codes.push(200);
        w.hits.push(10);
        w.errors.push(1);
        w.durations.push(5000);
        w.top_level_hits.push(5);
        w.bucket_starts.push(1000);
        w.bucket_durations.push(10_000_000_000);
        w.timestamps.push(2000);
        w.ok_summaries.push(vec![1, 2, 3]);
        w.error_summaries.push(vec![4, 5]);

        // Row 1 (earlier timestamp — should come first after sort).
        w.services.intern("svc-b");
        w.names.intern("op2");
        w.resources.intern("/bar");
        w.types.intern("rpc");
        w.span_kinds.intern("client");
        w.hostnames.intern("h2");
        w.envs.intern("prod");
        w.versions.intern("3.0");
        w.http_status_codes.push(500);
        w.hits.push(20);
        w.errors.push(5);
        w.durations.push(10000);
        w.top_level_hits.push(15);
        w.bucket_starts.push(500);
        w.bucket_durations.push(10_000_000_000);
        w.timestamps.push(1000);
        w.ok_summaries.push(vec![10, 20]);
        w.error_summaries.push(vec![30]);

        let path = w.flush().await.unwrap();
        w.base.close().unwrap();
        let batch = read_parquet(&path);

        assert_eq!(batch.num_rows(), 2);

        // Verify column names.
        let schema = batch.schema();
        let col_names: Vec<&str> = schema
            .fields()
            .iter()
            .map(|f| f.name().as_str())
            .collect();
        assert_eq!(
            col_names,
            vec![
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
            ]
        );

        // Sorted by timestamp: row 1 (ts=1000) comes first, row 0 (ts=2000) second.
        let services = read_dict_string_column(&batch, "service");
        assert_eq!(services, vec!["svc-b", "svc-a"]);

        let names = read_dict_string_column(&batch, "name");
        assert_eq!(names, vec!["op2", "op1"]);

        let resources = read_dict_string_column(&batch, "resource");
        assert_eq!(resources, vec!["/bar", "/foo"]);

        let types = read_dict_string_column(&batch, "type");
        assert_eq!(types, vec!["rpc", "web"]);

        let span_kinds = read_dict_string_column(&batch, "span_kind");
        assert_eq!(span_kinds, vec!["client", "server"]);

        // Numeric columns (sorted by timestamp).
        let http_col = batch
            .column_by_name("http_status_code")
            .unwrap()
            .as_any()
            .downcast_ref::<UInt32Array>()
            .unwrap();
        assert_eq!(http_col.values().to_vec(), vec![500, 200]);

        let hits_col = batch
            .column_by_name("hits")
            .unwrap()
            .as_any()
            .downcast_ref::<UInt64Array>()
            .unwrap();
        assert_eq!(hits_col.values().to_vec(), vec![20, 10]);

        let errors_col = batch
            .column_by_name("errors")
            .unwrap()
            .as_any()
            .downcast_ref::<UInt64Array>()
            .unwrap();
        assert_eq!(errors_col.values().to_vec(), vec![5, 1]);

        let dur_col = batch
            .column_by_name("duration_ns")
            .unwrap()
            .as_any()
            .downcast_ref::<UInt64Array>()
            .unwrap();
        assert_eq!(dur_col.values().to_vec(), vec![10000, 5000]);

        let ts_col = batch
            .column_by_name("timestamp_ns")
            .unwrap()
            .as_any()
            .downcast_ref::<Int64Array>()
            .unwrap();
        assert_eq!(ts_col.values().to_vec(), vec![1000, 2000]);

        // Binary columns (sorted by timestamp).
        let ok_col = batch
            .column_by_name("ok_summary")
            .unwrap()
            .as_any()
            .downcast_ref::<BinaryArray>()
            .unwrap();
        assert_eq!(ok_col.value(0), &[10, 20]);
        assert_eq!(ok_col.value(1), &[1, 2, 3]);

        let err_col = batch
            .column_by_name("error_summary")
            .unwrap()
            .as_any()
            .downcast_ref::<BinaryArray>()
            .unwrap();
        assert_eq!(err_col.value(0), &[30]);
        assert_eq!(err_col.value(1), &[4, 5]);
    }

    #[tokio::test]
    async fn test_telemetry_counters() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 10);

        assert_eq!(w.base.flush_count, 0);
        assert_eq!(w.base.flush_bytes, 0);

        let path = w.flush().await.unwrap();
        assert_eq!(w.base.flush_count, 1);
        assert!(w.base.last_flush_duration_ns > 0);

        // flush_bytes is updated on file rotation/close.
        w.base.close().unwrap();
        assert!(w.base.flush_bytes > 0);
        let file_size = std::fs::metadata(&path).unwrap().len();
        assert_eq!(w.base.flush_bytes, file_size);
    }
}
