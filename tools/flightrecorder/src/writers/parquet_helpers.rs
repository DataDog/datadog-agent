use std::fs::File;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use arrow::array::{DictionaryArray, StringArray, UInt32Array};
use arrow::datatypes::{DataType, Schema, UInt32Type};
use arrow::record_batch::RecordBatch;
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;

/// Convert StringInterner output to an Arrow DictionaryArray, applying sort order.
pub fn interner_to_dict_array(
    vals: Vec<String>,
    codes: Vec<u32>,
    order: &[usize],
) -> DictionaryArray<UInt32Type> {
    let sorted_codes: UInt32Array = order.iter().map(|&i| codes[i]).collect();
    let values = Arc::new(StringArray::from(vals));
    DictionaryArray::try_new(sorted_codes, values).expect("DictionaryArray construction failed")
}

/// Standard dictionary-encoded UTF8 data type.
pub fn dict_utf8_type() -> DataType {
    DataType::Dictionary(Box::new(DataType::UInt32), Box::new(DataType::Utf8))
}

/// Default Parquet writer properties: Snappy compression, dictionary enabled.
pub fn default_writer_props() -> WriterProperties {
    WriterProperties::builder()
        .set_compression(Compression::SNAPPY)
        .set_dictionary_enabled(true)
        .build()
}

/// Common state and I/O logic shared by all signal writers (metrics, logs, trace_stats).
///
/// Each concrete writer embeds a `BaseWriter` and delegates the Parquet write +
/// telemetry bookkeeping to it. The concrete writer is responsible for column
/// accumulation, schema definition, and RecordBatch construction.
pub struct BaseWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    // Telemetry counters (read by the telemetry reporter).
    pub flush_count: u64,
    pub flush_bytes: u64,
    pub rows_written: u64,
    pub last_flush_duration_ns: u64,
}

impl BaseWriter {
    pub fn new(output_dir: &Path, flush_rows: usize, flush_interval: Duration) -> Self {
        Self {
            output_dir: output_dir.to_path_buf(),
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),
            flush_count: 0,
            flush_bytes: 0,
            rows_written: 0,
            last_flush_duration_ns: 0,
        }
    }

    /// Check if a flush should be triggered based on row count and time.
    #[inline]
    pub fn should_flush(&self, buffered_rows: usize) -> bool {
        buffered_rows >= self.flush_rows || self.last_flush.elapsed() >= self.flush_interval
    }

    /// Write a RecordBatch to a new Parquet file with Snappy compression.
    ///
    /// Generates a timestamped path (`flush-{prefix}-{ts_ms}.parquet`), writes
    /// the batch, and updates all telemetry counters. Returns the file path.
    pub fn write_parquet(
        &mut self,
        prefix: &str,
        schema: Arc<Schema>,
        batch: RecordBatch,
    ) -> Result<PathBuf> {
        let flush_start = Instant::now();
        let row_count = batch.num_rows();

        let ts_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let path = self.output_dir.join(format!("flush-{prefix}-{ts_ms}.parquet"));

        let file =
            File::create(&path).with_context(|| format!("creating {}", path.display()))?;
        let props = default_writer_props();
        let mut writer = ArrowWriter::try_new(file, schema, Some(props))
            .with_context(|| format!("creating Parquet writer for {prefix}"))?;
        writer
            .write(&batch)
            .with_context(|| format!("writing {prefix} batch"))?;
        writer
            .close()
            .with_context(|| format!("closing {prefix} Parquet writer"))?;

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
}
