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

/// Common state and I/O logic shared by all signal writers.
///
/// Keeps an open Parquet file and appends row groups on each `write_batch()`.
/// Rotates to a new file every `rotation_interval` (default 1 minute). This
/// avoids creating thousands of small files (which causes filesystem overhead)
/// and improves compression (dictionary sharing across row groups).
pub struct BaseWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    rotation_interval: Duration,

    // Currently open writer + path (None until first write).
    active_writer: Option<ActiveFile>,

    // Telemetry counters (read by the telemetry reporter).
    pub flush_count: u64,
    pub flush_bytes: u64,
    pub rows_written: u64,
    pub last_flush_duration_ns: u64,
}

struct ActiveFile {
    writer: ArrowWriter<File>,
    path: PathBuf,
    opened_at: Instant,
    row_groups: u64,
}

impl BaseWriter {
    pub fn new(output_dir: &Path, flush_rows: usize, flush_interval: Duration) -> Self {
        Self {
            output_dir: output_dir.to_path_buf(),
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),
            rotation_interval: Duration::from_secs(60),
            active_writer: None,
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

    /// Append a RecordBatch as a new row group to the current Parquet file.
    /// Rotates to a new file if the rotation interval has elapsed.
    ///
    /// This method performs synchronous file I/O. It is called from dedicated
    /// writer threads (not the Tokio async runtime).
    pub fn write_batch(
        &mut self,
        prefix: &str,
        schema: Arc<Schema>,
        batch: RecordBatch,
    ) -> Result<PathBuf> {
        let flush_start = Instant::now();
        let row_count = batch.num_rows();

        // Rotate if needed (interval elapsed or no active file).
        let needs_rotation = match &self.active_writer {
            None => true,
            Some(af) => af.opened_at.elapsed() >= self.rotation_interval,
        };
        if needs_rotation {
            self.rotate(prefix, &schema)?;
        }

        let af = self.active_writer.as_mut().unwrap();
        af.writer
            .write(&batch)
            .with_context(|| format!("writing {prefix} row group"))?;
        af.row_groups += 1;

        self.flush_count += 1;
        self.rows_written += row_count as u64;
        self.last_flush_duration_ns = flush_start.elapsed().as_nanos() as u64;
        self.last_flush = Instant::now();

        Ok(af.path.clone())
    }

    /// Close the current file (if any) and open a new one.
    fn rotate(&mut self, prefix: &str, schema: &Arc<Schema>) -> Result<()> {
        // Close previous file.
        if let Some(af) = self.active_writer.take() {
            af.writer
                .close()
                .with_context(|| format!("closing {prefix} Parquet file"))?;
            let bytes = std::fs::metadata(&af.path)
                .map(|m| m.len())
                .unwrap_or(0);
            self.flush_bytes += bytes;
        }

        let ts_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let path = self
            .output_dir
            .join(format!("{prefix}-{ts_ms}.parquet"));

        let file =
            File::create(&path).with_context(|| format!("creating {}", path.display()))?;
        let props = default_writer_props();
        let writer = ArrowWriter::try_new(file, schema.clone(), Some(props))
            .with_context(|| format!("creating Parquet writer for {prefix}"))?;

        self.active_writer = Some(ActiveFile {
            writer,
            path,
            opened_at: Instant::now(),
            row_groups: 0,
        });

        Ok(())
    }

    /// Close the active file if any. Called on shutdown or client disconnect.
    pub fn close(&mut self) -> Result<()> {
        if let Some(af) = self.active_writer.take() {
            af.writer.close().context("closing active Parquet file")?;
            let bytes = std::fs::metadata(&af.path)
                .map(|m| m.len())
                .unwrap_or(0);
            self.flush_bytes += bytes;
        }
        Ok(())
    }
}
