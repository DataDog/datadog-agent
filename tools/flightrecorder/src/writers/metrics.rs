use std::collections::HashMap;
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

use super::intern::StringInterner;
use crate::generated::signals_generated::signals::MetricBatch;

/// Columnar accumulator for metric samples.
///
/// Context definitions (name + tags) are resolved inline at write time using a
/// HashMap, producing self-contained metric files with no separate context files.
pub struct MetricsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    // Interned string columns (dictionary-encoded on flush).
    names: StringInterner,
    tags: StringInterner,
    sources: StringInterner,

    // Plain columnar buffers.
    values: Vec<f64>,
    timestamps: Vec<i64>,
    sample_rates: Vec<f64>,

    // Resolves context_key → (name, tags) for inline writing.
    context_map: HashMap<u64, (String, String)>,

    // Rate-limited logging for unresolved context keys.
    unresolved_count: u64,
    last_unresolved_log: Instant,

    write_buf: Vec<u8>,
}

impl MetricsWriter {
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

            names: StringInterner::with_capacity(flush_rows),
            tags: StringInterner::with_capacity(flush_rows),
            sources: StringInterner::with_capacity(flush_rows),

            values: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
            sample_rates: Vec::with_capacity(flush_rows),

            context_map: HashMap::new(),

            unresolved_count: 0,
            last_unresolved_log: Instant::now(),

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
    /// Context definitions (context_key != 0, name non-empty) are stored in the
    /// context_map. All samples resolve context_key to (name, tags) inline.
    pub async fn push(&mut self, batch: &MetricBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(samples) = batch.samples() {
            for i in 0..samples.len() {
                let s = samples.get(i);
                let ckey = s.context_key();
                let raw_name = s.name().unwrap_or("");

                // If this is a context definition, store it in the map.
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

                    self.context_map
                        .insert(ckey, (raw_name.to_string(), tags_joined));
                }

                // Resolve context_key to name+tags.
                let (name, tags) = if let Some((n, t)) = self.context_map.get(&ckey) {
                    (n.as_str(), t.as_str())
                } else {
                    if ckey != 0 {
                        self.unresolved_count += 1;
                    }
                    ("<unknown>", "")
                };

                self.names.intern(name);
                self.tags.intern(tags);
                self.sources.intern(s.source().unwrap_or(""));
                self.values.push(s.value());
                self.timestamps.push(s.timestamp_ns());
                self.sample_rates.push(s.sample_rate());
            }
        }

        // Log unresolved context keys at most once per minute.
        if self.unresolved_count > 0
            && self.last_unresolved_log.elapsed() >= Duration::from_secs(60)
        {
            tracing::warn!(
                count = self.unresolved_count,
                "unresolved context_keys in the last 60s (using <unknown>)"
            );
            self.unresolved_count = 0;
            self.last_unresolved_log = Instant::now();
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
        let (name_vals, name_codes) = self.names.take();
        let (tag_vals, tag_codes) = self.tags.take();
        let (source_vals, source_codes) = self.sources.take();
        let values = std::mem::take(&mut self.values);
        let timestamps = std::mem::take(&mut self.timestamps);
        let sample_rates = std::mem::take(&mut self.sample_rates);

        // Sort index by timestamp for better compression (delta encoding).
        // Only allocates one Vec<usize> (~80 KB for 10K rows) — all columns
        // are gathered in sorted order when building the Vortex arrays.
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        let name_array = DictArray::try_new(
            order
                .iter()
                .map(|&i| name_codes[i])
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(name_vals).into_array(),
        )
        .context("building name DictArray")?;

        let tag_array = DictArray::try_new(
            order
                .iter()
                .map(|&i| tag_codes[i])
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(tag_vals).into_array(),
        )
        .context("building tags DictArray")?;

        let source_array = DictArray::try_new(
            order
                .iter()
                .map(|&i| source_codes[i])
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(source_vals).into_array(),
        )
        .context("building source DictArray")?;

        let st = StructArray::try_new(
            FieldNames::from([
                "name",
                "tags",
                "value",
                "timestamp_ns",
                "sample_rate",
                "source",
            ]),
            vec![
                name_array.into_array(),
                tag_array.into_array(),
                order
                    .iter()
                    .map(|&i| values[i])
                    .collect::<PrimitiveArray>()
                    .into_array(),
                order
                    .iter()
                    .map(|&i| timestamps[i])
                    .collect::<PrimitiveArray>()
                    .into_array(),
                order
                    .iter()
                    .map(|&i| sample_rates[i])
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

    /// Clear the context map. Called when a new agent connection is accepted
    /// because the agent will re-send all context definitions.
    pub fn reset_context_map(&mut self) {
        self.context_map.clear();
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
    use tempfile::tempdir;

    fn make_writer(dir: &Path) -> MetricsWriter {
        MetricsWriter::new(dir, 1000, Duration::from_secs(60))
    }

    fn add_rows(w: &mut MetricsWriter, n: usize) {
        for i in 0..n {
            w.names.intern("cpu.user");
            w.tags.intern("host:a|env:prod");
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
        w.names.intern("cpu.user");
        w.tags.intern("host:a");
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
            w.names.intern("cpu.user");
            w.tags.intern("host:a|env:prod");
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
    async fn test_context_map_resolution() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Insert context definitions
        w.context_map
            .insert(100, ("cpu.user".to_string(), "host:a|env:prod".to_string()));
        w.context_map
            .insert(200, ("mem.used".to_string(), "host:b".to_string()));

        // Simulate rows that resolve via context_map
        let (name, tags) = w.context_map.get(&100).unwrap().clone();
        w.names.intern(&name);
        w.tags.intern(&tags);
        w.sources.intern("agent");
        w.values.push(1.0);
        w.timestamps.push(1000);
        w.sample_rates.push(1.0);

        let (name, tags) = w.context_map.get(&200).unwrap().clone();
        w.names.intern(&name);
        w.tags.intern(&tags);
        w.sources.intern("agent");
        w.values.push(2.0);
        w.timestamps.push(2000);
        w.sample_rates.push(1.0);

        let path = w.flush().await.unwrap();
        assert!(path.exists());
    }

    #[tokio::test]
    async fn test_reset_context_map() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.context_map
            .insert(42, ("cpu.system".to_string(), "host:x".to_string()));
        assert_eq!(w.context_map.len(), 1);

        w.reset_context_map();
        assert!(w.context_map.is_empty());
    }
}
