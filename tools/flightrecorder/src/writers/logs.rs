use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, BinaryArray, Int64Array, MapBuilder, StringBuilder};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;

use super::apply_permutation;
use super::intern::StringInterner;
use super::parquet_helpers::{dict_utf8_type, interner_to_dict_array, BaseWriter};
use super::thread::{SignalWriter, WriterStats};
use crate::generated::signals_generated::signals;

/// Reserved tag keys extracted into their own dictionary-encoded columns.
const RESERVED_KEYS: &[&str] = &["service", "env", "version", "team"];

/// Schema for the logs Parquet file.
///
/// Reserved tags (service, env, version, team) get dedicated dictionary-encoded
/// columns. All other tags go into a `tags` MAP<string, string> column for
/// direct key-value queries (e.g. `WHERE tags['custom_key'] = 'value'`).
fn logs_schema() -> Arc<Schema> {
    let dt = dict_utf8_type();
    let map_field = Field::new(
        "tags",
        DataType::Map(
            Arc::new(Field::new(
                "entries",
                DataType::Struct(
                    vec![
                        Field::new("keys", DataType::Utf8, false),
                        Field::new("values", DataType::Utf8, true),
                    ]
                    .into(),
                ),
                false,
            )),
            false,
        ),
        true,
    );
    Arc::new(Schema::new(vec![
        Field::new("hostname", dt.clone(), false),
        Field::new("source", dt.clone(), false),
        Field::new("status", dt.clone(), false),
        Field::new("service", dt.clone(), false),
        Field::new("env", dt.clone(), false),
        Field::new("version", dt.clone(), false),
        Field::new("team", dt, false),
        map_field,
        Field::new("content", DataType::Binary, false),
        Field::new("timestamp_ns", DataType::Int64, false),
    ]))
}

/// Overflow tag key-value pair.
struct TagKV {
    key: String,
    value: String,
}

/// Columnar accumulator for log entries.
pub struct LogsWriter {
    pub base: BaseWriter,

    // Interned string columns (dictionary-encoded on flush).
    hostnames: StringInterner,
    sources: StringInterner,
    statuses: StringInterner,
    service: StringInterner,
    env: StringInterner,
    version: StringInterner,
    team: StringInterner,

    // Overflow tags as key-value pairs per row.
    overflow_tags: Vec<Vec<TagKV>>,

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
            service: StringInterner::with_capacity(flush_rows),
            env: StringInterner::with_capacity(flush_rows),
            version: StringInterner::with_capacity(flush_rows),
            team: StringInterner::with_capacity(flush_rows),

            overflow_tags: Vec::with_capacity(flush_rows),

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

                // Decompose tags into reserved columns + overflow MAP.
                let mut reserved_found = [false; 4];
                let mut overflow = Vec::new();

                if let Some(tl) = e.tags() {
                    for j in 0..tl.len() {
                        let tag = tl.get(j);
                        if let Some(colon) = tag.find(':') {
                            let key = &tag[..colon];
                            let value = &tag[colon + 1..];
                            if let Some(idx) = RESERVED_KEYS.iter().position(|&k| k == key) {
                                [
                                    &mut self.service,
                                    &mut self.env,
                                    &mut self.version,
                                    &mut self.team,
                                ][idx]
                                    .intern(value);
                                reserved_found[idx] = true;
                                continue;
                            }
                            overflow.push(TagKV {
                                key: key.to_string(),
                                value: value.to_string(),
                            });
                        } else {
                            overflow.push(TagKV {
                                key: tag.to_string(),
                                value: String::new(),
                            });
                        }
                    }
                }

                // Intern "" for reserved keys not found in this row.
                let interners = [
                    &mut self.service,
                    &mut self.env,
                    &mut self.version,
                    &mut self.team,
                ];
                for (idx, found) in reserved_found.iter().enumerate() {
                    if !found {
                        interners[idx].intern("");
                    }
                }

                self.overflow_tags.push(overflow);

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
        let (service_vals, service_codes) = self.service.take();
        let (env_vals, env_codes) = self.env.take();
        let (version_vals, version_codes) = self.version.take();
        let (team_vals, team_codes) = self.team.take();
        let overflow_tags = std::mem::take(&mut self.overflow_tags);
        let contents = std::mem::take(&mut self.contents);
        let timestamps = std::mem::take(&mut self.timestamps);

        // Sort index by timestamp for better compression.
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        let sorted_contents = apply_permutation(contents, &order);
        let content_array: ArrayRef = Arc::new(BinaryArray::from_iter_values(
            sorted_contents.iter().map(|v| v.as_slice()),
        ));

        // Build the overflow tags MAP column in sorted order.
        let sorted_overflow = apply_permutation(overflow_tags, &order);
        let mut map_builder = MapBuilder::new(None, StringBuilder::new(), StringBuilder::new());
        for row_tags in &sorted_overflow {
            for kv in row_tags {
                map_builder.keys().append_value(&kv.key);
                map_builder.values().append_value(&kv.value);
            }
            map_builder.append(true).unwrap();
        }

        let columns: Vec<ArrayRef> = vec![
            Arc::new(interner_to_dict_array(hostname_vals, hostname_codes, &order)),
            Arc::new(interner_to_dict_array(source_vals, source_codes, &order)),
            Arc::new(interner_to_dict_array(status_vals, status_codes, &order)),
            Arc::new(interner_to_dict_array(service_vals, service_codes, &order)),
            Arc::new(interner_to_dict_array(env_vals, env_codes, &order)),
            Arc::new(interner_to_dict_array(version_vals, version_codes, &order)),
            Arc::new(interner_to_dict_array(team_vals, team_codes, &order)),
            Arc::new(map_builder.finish()) as ArrayRef,
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
            w.service.intern("api");
            w.env.intern("test");
            w.version.intern("");
            w.team.intern("");
            w.overflow_tags.push(vec![
                TagKV { key: "custom".to_string(), value: format!("val{i}") },
            ]);
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
        assert_eq!(
            col_names,
            vec![
                "hostname", "source", "status", "service", "env",
                "version", "team", "tags", "content", "timestamp_ns",
            ]
        );
    }

    #[test]
    fn test_overflow_tags_map() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        w.hostnames.intern("h1");
        w.sources.intern("src");
        w.statuses.intern("info");
        w.service.intern("api");
        w.env.intern("prod");
        w.version.intern("1.0");
        w.team.intern("platform");
        w.overflow_tags.push(vec![
            TagKV { key: "region".to_string(), value: "us-east-1".to_string() },
            TagKV { key: "cluster".to_string(), value: "main".to_string() },
        ]);
        w.contents.push(b"test log".to_vec());
        w.timestamps.push(1000);

        let path = w.flush().unwrap();
        w.base.close().unwrap();
        let batch = read_parquet(&path);

        // Verify the tags column is a MAP
        let tags_col = batch.column_by_name("tags").unwrap();
        let map = tags_col.as_map();
        assert_eq!(map.len(), 1); // 1 row

        // Verify the service column has no tag_ prefix and contains the value
        let service_col = batch.column_by_name("service").unwrap();
        let dict = service_col.as_dictionary::<UInt32Type>();
        let vals = dict.values().as_string::<i32>();
        assert_eq!(vals.value(dict.keys().value(0) as usize), "api");
    }

    #[test]
    fn test_binary_content_roundtrip() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        let binary_data = vec![0u8, 1, 2, 3, 255, 0];
        w.hostnames.intern("h");
        w.sources.intern("");
        w.statuses.intern("info");
        w.service.intern("");
        w.env.intern("");
        w.version.intern("");
        w.team.intern("");
        w.overflow_tags.push(vec![]);
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
