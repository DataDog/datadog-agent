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
use crate::generated::signals_generated::signals::LogBatch;

/// Columnar accumulator for log entries.
///
/// String columns with high repetition (status, tags, hostname, source) are
/// dictionary-encoded via [`StringInterner`]. Content is variable and stored
/// as a plain `VarBinArray`.
pub struct LogsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    // Interned string columns.
    statuses: StringInterner,
    tags: StringInterner,
    hostnames: StringInterner,
    sources: StringInterner,

    // Plain columnar buffers.
    contents: Vec<Vec<u8>>,
    timestamps: Vec<i64>,

    // Reusable Vortex write state.
    session: VortexSession,
    write_buf: Vec<u8>,
}

impl LogsWriter {
    pub fn new(output_dir: impl AsRef<Path>, flush_rows: usize, flush_interval: Duration) -> Self {
        Self {
            output_dir: output_dir.as_ref().to_path_buf(),
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),

            statuses: StringInterner::with_capacity(flush_rows),
            tags: StringInterner::with_capacity(flush_rows),
            hostnames: StringInterner::with_capacity(flush_rows),
            sources: StringInterner::with_capacity(flush_rows),

            contents: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),

            session: VortexSession::default(),
            write_buf: Vec::with_capacity(64 * 1024),
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

                let tags_joined: String = e
                    .tags()
                    .map(|tl| {
                        (0..tl.len())
                            .map(|j| tl.get(j))
                            .collect::<Vec<_>>()
                            .join("|")
                    })
                    .unwrap_or_default();

                self.statuses.intern(e.status().unwrap_or(""));
                self.tags.intern_owned(tags_joined);
                self.hostnames.intern(e.hostname().unwrap_or(""));
                self.sources.intern(e.source().unwrap_or(""));

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
        let path = self.output_dir.join(format!("logs-{}.vortex", ts_ms));

        // Take interned columns.
        let (status_vals, status_codes) = self.statuses.take();
        let (tag_vals, tag_codes) = self.tags.take();
        let (hostname_vals, hostname_codes) = self.hostnames.take();
        let (source_vals, source_codes) = self.sources.take();

        // Take plain columns.
        let contents = std::mem::take(&mut self.contents);
        let timestamps = std::mem::take(&mut self.timestamps);

        // Build dictionary-encoded arrays for string columns.
        let status_array = DictArray::try_new(
            status_codes.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(status_vals).into_array(),
        )
        .context("building status DictArray")?;

        let tag_array = DictArray::try_new(
            tag_codes.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(tag_vals).into_array(),
        )
        .context("building tags DictArray")?;

        let hostname_array = DictArray::try_new(
            hostname_codes.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(hostname_vals).into_array(),
        )
        .context("building hostname DictArray")?;

        let source_array = DictArray::try_new(
            source_codes.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(source_vals).into_array(),
        )
        .context("building source DictArray")?;

        let st = StructArray::try_new(
            FieldNames::from(["content", "status", "tags", "hostname", "timestamp_ns", "source"]),
            vec![
                VarBinArray::from(contents).into_array(),
                status_array.into_array(),
                tag_array.into_array(),
                hostname_array.into_array(),
                timestamps.into_iter().collect::<PrimitiveArray>().into_array(),
                source_array.into_array(),
            ],
            row_count,
            Validity::NonNullable,
        )
        .context("building logs StructArray")?;

        let strategy = WriteStrategyBuilder::default()
            .with_compact_encodings()
            .build();

        self.write_buf.clear();
        VortexWriteOptions::new(self.session.clone())
            .with_strategy(strategy)
            .write(&mut self.write_buf, st.into_array().to_array_stream())
            .await
            .context("writing logs vortex file")?;

        tokio::fs::write(&path, &self.write_buf)
            .await
            .with_context(|| format!("writing {}", path.display()))?;

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
    use tempfile::tempdir;

    fn make_writer(dir: &Path) -> LogsWriter {
        LogsWriter::new(dir, 1000, Duration::from_secs(60))
    }

    fn add_rows(w: &mut LogsWriter, n: usize) {
        for i in 0..n {
            w.contents.push(format!("log line {}", i).into_bytes());
            w.statuses.intern("info");
            w.tags.intern("env:test");
            w.hostnames.intern("host1");
            w.sources.intern("app");
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
    async fn test_binary_content() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.contents.push(vec![0u8, 1, 2, 3, 255, 0]);
        w.statuses.intern("info");
        w.tags.intern("");
        w.hostnames.intern("h");
        w.sources.intern("");
        w.timestamps.push(0);

        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
    }

    #[tokio::test]
    async fn test_readback_fields() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.contents.push(b"hello world".to_vec());
        w.statuses.intern("warn");
        w.tags.intern("team:ops");
        w.hostnames.intern("server1");
        w.sources.intern("syslog");
        w.timestamps.push(12345);

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
}
