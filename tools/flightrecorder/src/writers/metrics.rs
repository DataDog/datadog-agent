use std::path::{Path, PathBuf};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use vortex::array::arrays::{DictArray, PrimitiveArray, StructArray, VarBinArray};
use vortex::array::dtype::FieldNames;
use vortex::array::validity::Validity;
use vortex::array::IntoArray;
use vortex::file::VortexWriteOptions;
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

use super::contexts::ContextsWriter;
use super::intern::StringInterner;
use crate::generated::signals_generated::signals::MetricBatch;

/// Columnar accumulator for metric samples.
///
/// Metric data points reference contexts by `context_key` (u64) only.
/// Context definitions (name + tags) are written to separate context files
/// via an embedded [`ContextsWriter`].
pub struct MetricsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    // Interned string column (dictionary-encoded on flush).
    sources: StringInterner,

    // Plain columnar buffers.
    context_keys: Vec<u64>,
    values: Vec<f64>,
    timestamps: Vec<i64>,
    sample_rates: Vec<f64>,

    // Writes context definitions to separate vortex files.
    contexts_writer: ContextsWriter,

    write_buf: Vec<u8>,
}

impl MetricsWriter {
    pub fn new(output_dir: impl AsRef<Path>, flush_rows: usize, flush_interval: Duration) -> Self {
        let output_dir = output_dir.as_ref().to_path_buf();
        Self {
            // Context rows are much larger than metric rows (long name/tags strings),
            // so flush contexts in smaller batches to limit transient Vortex pipeline memory.
            contexts_writer: ContextsWriter::new(&output_dir, flush_rows.min(2000), flush_interval),

            output_dir,
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),

            sources: StringInterner::with_capacity(flush_rows),

            context_keys: Vec::with_capacity(flush_rows),
            values: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
            sample_rates: Vec::with_capacity(flush_rows),

            write_buf: Vec::with_capacity(64 * 1024),
        }
    }

    /// Number of rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.values.len()
    }

    /// Ingest a MetricBatch from FlatBuffers. Flushes automatically when thresholds are reached.
    ///
    /// Context definitions (context_key != 0, name non-empty) are forwarded to the
    /// embedded ContextsWriter. All samples store only the context_key in the metrics file.
    pub async fn push(&mut self, batch: &MetricBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(samples) = batch.samples() {
            for i in 0..samples.len() {
                let s = samples.get(i);
                let ckey = s.context_key();
                let raw_name = s.name().unwrap_or("");

                // If this is a context definition, record it in the contexts writer.
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

                    self.contexts_writer.try_record(ckey, raw_name, &tags_joined);
                }

                self.context_keys.push(ckey);
                self.sources.intern(s.source().unwrap_or(""));
                self.values.push(s.value());
                self.timestamps.push(s.timestamp_ns());
                self.sample_rates.push(s.sample_rate());
            }
        }

        // Flush contexts if their thresholds are reached.
        if self.contexts_writer.should_flush() {
            if let Err(e) = self.contexts_writer.flush().await {
                tracing::warn!("contexts flush error: {}", e);
            }
        }

        if self.len() >= self.flush_rows || self.last_flush.elapsed() >= self.flush_interval {
            return self.flush().await.map(Some);
        }
        Ok(None)
    }

    /// Flush accumulated columns to a new Vortex file. Returns the file path.
    pub async fn flush(&mut self) -> Result<PathBuf> {
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no rows to flush");
        }

        let ts_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let path = self.output_dir.join(format!("metrics-{}.vortex", ts_ms));

        // Take columns.
        let context_keys = std::mem::take(&mut self.context_keys);
        let (source_vals, source_codes) = self.sources.take();
        let values = std::mem::take(&mut self.values);
        let timestamps = std::mem::take(&mut self.timestamps);
        let sample_rates = std::mem::take(&mut self.sample_rates);

        let source_array = DictArray::try_new(
            source_codes
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(source_vals).into_array(),
        )
        .context("building source DictArray")?;

        let st = StructArray::try_new(
            FieldNames::from([
                "context_key",
                "value",
                "timestamp_ns",
                "sample_rate",
                "source",
            ]),
            vec![
                context_keys
                    .into_iter()
                    .collect::<PrimitiveArray>()
                    .into_array(),
                values
                    .into_iter()
                    .collect::<PrimitiveArray>()
                    .into_array(),
                timestamps
                    .into_iter()
                    .collect::<PrimitiveArray>()
                    .into_array(),
                sample_rates
                    .into_iter()
                    .collect::<PrimitiveArray>()
                    .into_array(),
                source_array.into_array(),
            ],
            row_count,
            Validity::NonNullable,
        )
        .context("building metrics StructArray")?;

        let strategy = super::strategy::low_memory_strategy();

        // Fresh session per flush to prevent registry accumulation in VortexSession.
        let session = VortexSession::default();

        self.write_buf.clear();
        VortexWriteOptions::new(session)
            .with_strategy(strategy)
            .write(&mut self.write_buf, st.into_array().to_array_stream())
            .await
            .context("writing metrics vortex file")?;

        tokio::fs::write(&path, &self.write_buf)
            .await
            .with_context(|| format!("writing {}", path.display()))?;

        // Shrink write_buf to release memory back to the allocator.
        self.write_buf = Vec::with_capacity(64 * 1024);

        self.last_flush = Instant::now();
        Ok(path)
    }

    /// Clear the context bloom filter and flush any pending context rows.
    /// Called when a new agent connection is accepted because the agent will
    /// re-send all context definitions.
    pub async fn reset_contexts(&mut self) -> Result<()> {
        if let Err(e) = self.contexts_writer.flush_if_any().await {
            tracing::warn!("contexts flush error during reset: {}", e);
        }
        self.contexts_writer.reset();
        Ok(())
    }

    /// Flush if any rows are buffered. Used on shutdown.
    pub async fn flush_if_any(&mut self) -> Result<Option<PathBuf>> {
        // Flush pending contexts first.
        if let Err(e) = self.contexts_writer.flush_if_any().await {
            tracing::warn!("contexts flush error during metrics flush_if_any: {}", e);
        }
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
    use tempfile::tempdir;

    fn make_writer(dir: &Path) -> MetricsWriter {
        MetricsWriter::new(dir, 1000, Duration::from_secs(60))
    }

    fn add_rows(w: &mut MetricsWriter, n: usize) {
        for i in 0..n {
            w.context_keys.push((i % 10) as u64);
            w.sources.intern("test");
            w.values.push(i as f64);
            w.timestamps.push(1000);
            w.sample_rates.push(1.0);
        }
    }

    #[tokio::test]
    async fn test_push_and_flush() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 100);

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
    }

    #[tokio::test]
    async fn test_readback_fields() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.context_keys.push(42);
        w.sources.intern("src1");
        w.values.push(42.0);
        w.timestamps.push(999);
        w.sample_rates.push(0.5);

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
    }

    #[tokio::test]
    async fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().await.is_err());
    }

    #[tokio::test]
    async fn test_interning_deduplicates() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        for i in 0..1000 {
            w.context_keys.push((i % 5) as u64);
            w.sources.intern("agent");
            w.values.push(i as f64);
            w.timestamps.push(i as i64);
            w.sample_rates.push(1.0);
        }
        let path = w.flush().await.unwrap();
        let size = path.metadata().unwrap().len();
        assert!(size > 0);
        assert!(
            size < 30_000,
            "file too large: {} bytes, dict encoding may not be working",
            size
        );
    }

    #[tokio::test]
    async fn test_context_definition_writes_to_contexts_file() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Manually record a context definition
        w.contexts_writer
            .try_record(100, "cpu.user", "host:a|env:prod");
        w.contexts_writer
            .try_record(200, "mem.used", "host:b");

        let path = w.contexts_writer.flush().await.unwrap();
        assert!(path.exists());
        assert!(
            path.file_name()
                .unwrap()
                .to_str()
                .unwrap()
                .starts_with("contexts-")
        );
    }

    #[tokio::test]
    async fn test_context_reference_does_not_write_context() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Record a definition first
        w.contexts_writer
            .try_record(100, "cpu.user", "host:a");

        // A reference (name empty) doesn't go through try_record
        // Just push a sample with context_key
        w.context_keys.push(100);
        w.sources.intern("agent");
        w.values.push(1.0);
        w.timestamps.push(1000);
        w.sample_rates.push(1.0);

        // Only 1 context row (the definition), not 2
        assert_eq!(w.contexts_writer.len(), 1);
    }

    #[tokio::test]
    async fn test_bloom_dedup_in_metrics() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Same context_key sent twice — only first should be recorded
        assert!(w
            .contexts_writer
            .try_record(42, "cpu.system", "host:x"));
        assert!(!w
            .contexts_writer
            .try_record(42, "cpu.system", "host:x"));
        assert_eq!(w.contexts_writer.len(), 1);
    }
}
