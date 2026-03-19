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

/// Accumulates and writes log context definitions (context_key → hostname, source, status, tags)
/// to separate `log-contexts-*.vortex` files. Uses a bloom filter to skip already-persisted
/// context keys.
pub struct LogContextsWriter {
    output_dir: PathBuf,
    flush_rows: usize,
    flush_interval: Duration,
    last_flush: Instant,

    context_keys: Vec<u64>,
    hostnames: StringInterner,
    sources: StringInterner,
    statuses: StringInterner,
    tags: StringInterner,

    bloom: Bloom<u64>,

    write_buf: Vec<u8>,
}

impl LogContextsWriter {
    pub fn new(output_dir: impl AsRef<Path>, flush_rows: usize, flush_interval: Duration) -> Self {
        Self {
            output_dir: output_dir.as_ref().to_path_buf(),
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),

            context_keys: Vec::with_capacity(flush_rows),
            hostnames: StringInterner::with_capacity(flush_rows),
            sources: StringInterner::with_capacity(flush_rows),
            statuses: StringInterner::with_capacity(flush_rows),
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

    /// Record a log context definition if we haven't seen this key before.
    /// Returns true if the context was new and recorded, false if it was a bloom hit.
    #[inline]
    pub fn try_record(
        &mut self,
        context_key: u64,
        hostname: &str,
        source: &str,
        status: &str,
        tags_joined: &str,
    ) -> bool {
        if self.bloom.check(&context_key) {
            return false;
        }
        self.bloom.set(&context_key);
        self.context_keys.push(context_key);
        self.hostnames.intern(hostname);
        self.sources.intern(source);
        self.statuses.intern(status);
        self.tags.intern(tags_joined);
        true
    }

    /// Whether the buffered rows have reached the flush threshold.
    #[inline]
    pub fn should_flush(&self) -> bool {
        self.len() > 0
            && (self.len() >= self.flush_rows || self.last_flush.elapsed() >= self.flush_interval)
    }

    /// Flush buffered log context definitions to a `log-contexts-{ts_ms}.vortex` file.
    pub async fn flush(&mut self) -> Result<PathBuf> {
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no log context rows to flush");
        }

        let ts_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let path = self
            .output_dir
            .join(format!("log-contexts-{}.vortex", ts_ms));

        let context_keys = std::mem::take(&mut self.context_keys);
        let (hostname_vals, hostname_codes) = self.hostnames.take();
        let (source_vals, source_codes) = self.sources.take();
        let (status_vals, status_codes) = self.statuses.take();
        let (tag_vals, tag_codes) = self.tags.take();

        let hostname_array = DictArray::try_new(
            hostname_codes
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(hostname_vals).into_array(),
        )
        .context("building log context hostname DictArray")?;

        let source_array = DictArray::try_new(
            source_codes
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(source_vals).into_array(),
        )
        .context("building log context source DictArray")?;

        let status_array = DictArray::try_new(
            status_codes
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(status_vals).into_array(),
        )
        .context("building log context status DictArray")?;

        let tag_array = DictArray::try_new(
            tag_codes
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(tag_vals).into_array(),
        )
        .context("building log context tags DictArray")?;

        let st = StructArray::try_new(
            FieldNames::from(["context_key", "hostname", "source", "status", "tags"]),
            vec![
                context_keys
                    .into_iter()
                    .collect::<PrimitiveArray>()
                    .into_array(),
                hostname_array.into_array(),
                source_array.into_array(),
                status_array.into_array(),
                tag_array.into_array(),
            ],
            row_count,
            Validity::NonNullable,
        )
        .context("building log contexts StructArray")?;

        let strategy = super::strategy::low_memory_strategy();

        // Fresh session per flush to prevent registry accumulation in VortexSession.
        let session = VortexSession::default();

        self.write_buf.clear();
        VortexWriteOptions::new(session)
            .with_strategy(strategy)
            .write(&mut self.write_buf, st.into_array().to_array_stream())
            .await
            .context("writing log contexts vortex file")?;

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
    /// connection arrives and will re-send all log entries.
    pub fn reset(&mut self) {
        self.bloom.clear();
        self.context_keys.clear();
        self.hostnames.clear();
        self.sources.clear();
        self.statuses.clear();
        self.tags.clear();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;
    use vortex::array::stream::ArrayStreamExt;
    use vortex::array::Canonical;
    use vortex::file::OpenOptionsSessionExt;

    fn make_writer(dir: &Path) -> LogContextsWriter {
        LogContextsWriter::new(dir, 1000, Duration::from_secs(60))
    }

    /// Read back a vortex file and return the struct's canonical form.
    async fn read_back(path: &Path) -> vortex::array::arrays::StructArray {
        let session = VortexSession::default();
        let array = session
            .open_options()
            .open_path(path.to_path_buf())
            .await
            .unwrap()
            .scan()
            .unwrap()
            .into_array_stream()
            .unwrap()
            .read_all()
            .await
            .unwrap();
        let canonical = array.to_canonical().unwrap();
        canonical.into_struct()
    }

    fn read_u64_column(st: &vortex::array::arrays::StructArray, name: &str) -> Vec<u64> {
        let arr = st.unmasked_field_by_name(name).unwrap();
        let canonical = arr.to_canonical().unwrap();
        match canonical {
            Canonical::Primitive(prim) => prim.as_slice::<u64>().to_vec(),
            other => panic!("expected Primitive u64 for {name}, got {:?}", other.dtype()),
        }
    }

    fn read_string_column(st: &vortex::array::arrays::StructArray, name: &str) -> Vec<String> {
        let arr = st.unmasked_field_by_name(name).unwrap();
        let canonical = arr.to_canonical().unwrap();
        match canonical {
            Canonical::VarBinView(vbv) => (0..st.len())
                .map(|i| String::from_utf8_lossy(vbv.bytes_at(i).as_slice()).into_owned())
                .collect(),
            other => panic!("expected VarBinView for {name}, got {:?}", other.dtype()),
        }
    }

    #[test]
    fn test_try_record_new_key() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.try_record(1, "host1", "app", "info", "env:prod|team:ops"));
        assert_eq!(w.len(), 1);
    }

    #[test]
    fn test_bloom_dedup() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.try_record(42, "host1", "syslog", "warn", "env:test"));
        assert!(!w.try_record(42, "host1", "syslog", "warn", "env:test"));
        assert_eq!(w.len(), 1);
    }

    #[test]
    fn test_reset_clears_bloom() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.try_record(42, "host1", "syslog", "warn", "env:test"));
        w.reset();
        assert_eq!(w.len(), 0);
        // After reset, same key should be accepted again
        assert!(w.try_record(42, "host1", "syslog", "warn", "env:test"));
        assert_eq!(w.len(), 1);
    }

    #[tokio::test]
    async fn test_flush_writes_file() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.try_record(1, "host-a", "app", "info", "env:prod");
        w.try_record(2, "host-b", "syslog", "error", "env:staging|team:sre");

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
        assert!(path
            .file_name()
            .unwrap()
            .to_str()
            .unwrap()
            .starts_with("log-contexts-"));
    }

    #[tokio::test]
    async fn test_roundtrip_values() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.try_record(100, "host-a", "myapp", "info", "env:prod|team:ops");
        w.try_record(200, "host-b", "syslog", "error", "env:staging");
        w.try_record(300, "host-a", "myapp", "warn", "");

        let path = w.flush().await.unwrap();
        let st = read_back(&path).await;

        assert_eq!(st.len(), 3);

        let ckeys = read_u64_column(&st, "context_key");
        assert_eq!(ckeys, vec![100, 200, 300]);

        let hostnames = read_string_column(&st, "hostname");
        assert_eq!(hostnames, vec!["host-a", "host-b", "host-a"]);

        let sources = read_string_column(&st, "source");
        assert_eq!(sources, vec!["myapp", "syslog", "myapp"]);

        let statuses = read_string_column(&st, "status");
        assert_eq!(statuses, vec!["info", "error", "warn"]);

        let tags = read_string_column(&st, "tags");
        assert_eq!(tags, vec!["env:prod|team:ops", "env:staging", ""]);
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
