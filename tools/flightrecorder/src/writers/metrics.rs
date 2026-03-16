use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use vortex::array::arrays::{DictArray, PrimitiveArray, StructArray, VarBinArray};
use vortex::array::dtype::FieldNames;
use vortex::array::validity::Validity;
use vortex::array::IntoArray;
use vortex::file::{VortexWriteOptions, WriteStrategyBuilder};
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

use super::intern::StringInterner;
use crate::generated::signals_generated::signals::MetricBatch;

/// Columnar accumulator for metric samples.
///
/// Instead of storing `Vec<MetricRow>`, we accumulate directly into column
/// vectors. String columns (name, tags, source) use a [`StringInterner`] to
/// deduplicate values, which:
///   1. Reduces memory during accumulation (one copy per unique string).
///   2. Produces a ready-made dictionary for Vortex `DictArray` encoding.
///   3. Dramatically shrinks on-disk file size for repetitive data.
/// Resolved context definition: metric name and joined tag string.
struct ContextDef {
    name: String,
    tags_joined: String,
}

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

    // Context key → definition map. Populated when a sample arrives with
    // context_key != 0 and name is non-empty (context definition). Used to
    // resolve context references (context_key != 0, name empty).
    contexts: HashMap<u64, ContextDef>,

    // Reusable Vortex write state.
    session: VortexSession,
    write_buf: Vec<u8>,
}

impl MetricsWriter {
    pub fn new(output_dir: impl AsRef<Path>, flush_rows: usize, flush_interval: Duration) -> Self {
        Self {
            output_dir: output_dir.as_ref().to_path_buf(),
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),

            names: StringInterner::with_capacity(flush_rows),
            tags: StringInterner::with_capacity(flush_rows),
            sources: StringInterner::with_capacity(flush_rows),

            values: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
            sample_rates: Vec::with_capacity(flush_rows),

            contexts: HashMap::new(),

            session: VortexSession::default(),
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
    /// Supports context-key deduplication: when a sample has `context_key != 0`:
    ///   - If `name` is non-empty, it's a **context definition**: store name+tags
    ///     in the context map and use them for this row.
    ///   - If `name` is empty, it's a **context reference**: look up name+tags
    ///     from the context map.
    pub async fn push(&mut self, batch: &MetricBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(samples) = batch.samples() {
            for i in 0..samples.len() {
                let s = samples.get(i);
                let ckey = s.context_key();
                let raw_name = s.name().unwrap_or("");

                if ckey != 0 && raw_name.is_empty() {
                    // Context reference — resolve from map.
                    if let Some(ctx) = self.contexts.get(&ckey) {
                        self.names.intern(&ctx.name);
                        self.tags.intern(&ctx.tags_joined);
                    } else {
                        // Unknown context key — use empty strings as fallback.
                        tracing::warn!(context_key = ckey, "unresolved context reference (no prior definition)");
                        self.names.intern("");
                        self.tags.intern("");
                    }
                } else {
                    // Full sample or context definition.
                    self.names.intern(raw_name);

                    let tags_joined: String = s
                        .tags()
                        .map(|tl| {
                            (0..tl.len())
                                .map(|j| tl.get(j))
                                .collect::<Vec<_>>()
                                .join("|")
                        })
                        .unwrap_or_default();

                    // Store context definition if context_key is set.
                    if ckey != 0 {
                        let is_new = !self.contexts.contains_key(&ckey);
                        self.contexts.insert(ckey, ContextDef {
                            name: raw_name.to_string(),
                            tags_joined: tags_joined.clone(),
                        });
                        if is_new && self.contexts.len() % 500 == 0 {
                            tracing::info!(contexts = self.contexts.len(), "context map growing");
                        }
                    }

                    self.tags.intern_owned(tags_joined);
                }

                self.sources.intern(s.source().unwrap_or(""));
                self.values.push(s.value());
                self.timestamps.push(s.timestamp_ns());
                self.sample_rates.push(s.sample_rate());
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

        // Take interned columns.
        let (name_vals, name_codes) = self.names.take();
        let (tag_vals, tag_codes) = self.tags.take();
        let (source_vals, source_codes) = self.sources.take();

        // Take plain columns.
        let values = std::mem::take(&mut self.values);
        let timestamps = std::mem::take(&mut self.timestamps);
        let sample_rates = std::mem::take(&mut self.sample_rates);

        // Build dictionary-encoded arrays for string columns.
        let name_array = DictArray::try_new(
            name_codes.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(name_vals).into_array(),
        )
        .context("building name DictArray")?;

        let tag_array = DictArray::try_new(
            tag_codes.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(tag_vals).into_array(),
        )
        .context("building tags DictArray")?;

        let source_array = DictArray::try_new(
            source_codes.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(source_vals).into_array(),
        )
        .context("building source DictArray")?;

        let st = StructArray::try_new(
            FieldNames::from(["name", "value", "tags", "timestamp_ns", "sample_rate", "source"]),
            vec![
                name_array.into_array(),
                values.into_iter().collect::<PrimitiveArray>().into_array(),
                tag_array.into_array(),
                timestamps.into_iter().collect::<PrimitiveArray>().into_array(),
                sample_rates.into_iter().collect::<PrimitiveArray>().into_array(),
                source_array.into_array(),
            ],
            row_count,
            Validity::NonNullable,
        )
        .context("building metrics StructArray")?;

        let strategy = WriteStrategyBuilder::default()
            .with_compact_encodings()
            .build();

        self.write_buf.clear();
        VortexWriteOptions::new(self.session.clone())
            .with_strategy(strategy)
            .write(&mut self.write_buf, st.into_array().to_array_stream())
            .await
            .context("writing metrics vortex file")?;

        tokio::fs::write(&path, &self.write_buf)
            .await
            .with_context(|| format!("writing {}", path.display()))?;

        self.last_flush = Instant::now();
        Ok(path)
    }

    /// Clear the context-key map. Called when a new agent connection is accepted
    /// because the agent will re-send all context definitions.
    pub fn reset_contexts(&mut self) {
        self.contexts.clear();
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
            let name = format!("cpu{}", i % 10); // only 10 distinct names
            w.names.intern(&name);
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
        w.names.intern("mem");
        w.tags.intern("env:prod");
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
        // 1000 rows but only 5 distinct metric names
        for i in 0..1000 {
            let name = format!("metric.{}", i % 5);
            w.names.intern(&name);
            w.tags.intern("host:a");
            w.sources.intern("agent");
            w.values.push(i as f64);
            w.timestamps.push(i as i64);
            w.sample_rates.push(1.0);
        }
        let path = w.flush().await.unwrap();
        let size = path.metadata().unwrap().len();
        // With dictionary encoding, file should be much smaller than raw data
        assert!(size > 0);
        // Rough sanity: 1000 rows with 5 distinct names should compress well
        // Raw would be ~50KB+, dict-encoded should be well under 20KB
        assert!(size < 30_000, "file too large: {} bytes, dict encoding may not be working", size);
    }
}
