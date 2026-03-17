use std::path::{Path, PathBuf};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use bloomfilter::Bloom;
use vortex::array::arrays::{DictArray, PrimitiveArray, StructArray, VarBinArray};
use vortex::array::dtype::FieldNames;
use vortex::array::validity::Validity;
use vortex::array::IntoArray;
use vortex::file::VortexWriteOptions;
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

use super::intern::StringInterner;

/// Accumulates and writes context definitions (context_key → name, tags) to
/// separate `contexts-*.vortex` files. Uses a bloom filter to skip already-persisted
/// context keys, keeping memory at ~120 KB instead of ~50 MB for a full HashMap.
pub struct ContextsWriter {
    output_dir: PathBuf,
    flush_rows: usize,
    flush_interval: Duration,
    last_flush: Instant,

    context_keys: Vec<u64>,
    names: StringInterner,
    tags: StringInterner,

    bloom: Bloom<u64>,

    write_buf: Vec<u8>,
}

impl ContextsWriter {
    pub fn new(output_dir: impl AsRef<Path>, flush_rows: usize, flush_interval: Duration) -> Self {
        Self {
            output_dir: output_dir.as_ref().to_path_buf(),
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),

            context_keys: Vec::with_capacity(flush_rows),
            names: StringInterner::with_capacity(flush_rows),
            tags: StringInterner::with_capacity(flush_rows),

            bloom: Bloom::new_for_fp_rate(500_000, 0.001),

            write_buf: Vec::with_capacity(64 * 1024),
        }
    }

    /// Number of context rows currently buffered.
    #[inline]
    pub fn len(&self) -> usize {
        self.context_keys.len()
    }

    /// Record a context definition if we haven't seen this key before.
    /// Returns true if the context was new and recorded, false if it was a bloom hit.
    #[inline]
    pub fn try_record(&mut self, context_key: u64, name: &str, tags_joined: &str) -> bool {
        if self.bloom.check(&context_key) {
            return false;
        }
        self.bloom.set(&context_key);
        self.context_keys.push(context_key);
        self.names.intern(name);
        self.tags.intern(tags_joined);
        true
    }

    /// Whether the buffered rows have reached the flush threshold.
    #[inline]
    pub fn should_flush(&self) -> bool {
        self.len() > 0
            && (self.len() >= self.flush_rows || self.last_flush.elapsed() >= self.flush_interval)
    }

    /// Flush buffered context definitions to a `contexts-{ts_ms}.vortex` file.
    pub async fn flush(&mut self) -> Result<PathBuf> {
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no context rows to flush");
        }

        let ts_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let path = self.output_dir.join(format!("contexts-{}.vortex", ts_ms));

        let context_keys = std::mem::take(&mut self.context_keys);
        let (name_vals, name_codes) = self.names.take();
        let (tag_vals, tag_codes) = self.tags.take();

        let name_array = DictArray::try_new(
            name_codes
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(name_vals).into_array(),
        )
        .context("building context name DictArray")?;

        let tag_array = DictArray::try_new(
            tag_codes
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(tag_vals).into_array(),
        )
        .context("building context tags DictArray")?;

        let st = StructArray::try_new(
            FieldNames::from(["context_key", "name", "tags"]),
            vec![
                context_keys
                    .into_iter()
                    .collect::<PrimitiveArray>()
                    .into_array(),
                name_array.into_array(),
                tag_array.into_array(),
            ],
            row_count,
            Validity::NonNullable,
        )
        .context("building contexts StructArray")?;

        let strategy = super::strategy::low_memory_strategy();

        // Fresh session per flush to prevent registry accumulation in VortexSession.
        let session = VortexSession::default();

        self.write_buf.clear();
        VortexWriteOptions::new(session)
            .with_strategy(strategy)
            .write(&mut self.write_buf, st.into_array().to_array_stream())
            .await
            .context("writing contexts vortex file")?;

        tokio::fs::write(&path, &self.write_buf)
            .await
            .with_context(|| format!("writing {}", path.display()))?;

        // Shrink write_buf to release memory back to the allocator.
        self.write_buf = Vec::with_capacity(64 * 1024);

        self.last_flush = Instant::now();
        Ok(path)
    }

    /// Flush if any rows are buffered.
    pub async fn flush_if_any(&mut self) -> Result<Option<PathBuf>> {
        if self.len() == 0 {
            Ok(None)
        } else {
            self.flush().await.map(Some)
        }
    }

    /// Clear the bloom filter and any buffered rows. Called when a new agent
    /// connection arrives and will re-send all context definitions.
    pub fn reset(&mut self) {
        self.bloom.clear();
        self.context_keys.clear();
        self.names.clear();
        self.tags.clear();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    fn make_writer(dir: &Path) -> ContextsWriter {
        ContextsWriter::new(dir, 1000, Duration::from_secs(60))
    }

    #[test]
    fn test_try_record_new_key() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.try_record(1, "cpu.usage", "host:a|env:prod"));
        assert_eq!(w.len(), 1);
    }

    #[test]
    fn test_bloom_dedup() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.try_record(42, "mem.used", "host:b"));
        assert!(!w.try_record(42, "mem.used", "host:b"));
        assert_eq!(w.len(), 1);
    }

    #[test]
    fn test_reset_clears_bloom() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.try_record(42, "mem.used", "host:b"));
        w.reset();
        assert_eq!(w.len(), 0);
        // After reset, same key should be accepted again
        assert!(w.try_record(42, "mem.used", "host:b"));
        assert_eq!(w.len(), 1);
    }

    #[tokio::test]
    async fn test_flush_writes_file() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.try_record(1, "cpu.user", "host:a");
        w.try_record(2, "cpu.system", "host:a|env:prod");

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
        assert!(path.file_name().unwrap().to_str().unwrap().starts_with("contexts-"));
    }

    #[tokio::test]
    async fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().await.is_err());
    }

    #[tokio::test]
    async fn test_flush_if_any_empty() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush_if_any().await.unwrap().is_none());
    }
}
