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
use crate::generated::signals_generated::signals::LogBatch;

/// Columnar accumulator for log entries.
///
/// All fields (hostname, source, status, tags, content, timestamp_ns) are
/// written inline to self-contained log files. No separate context files needed.
pub struct LogsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    // Interned string columns (dictionary-encoded on flush).
    hostnames: StringInterner,
    sources: StringInterner,
    statuses: StringInterner,
    tags: StringInterner,

    // Plain columnar buffers.
    contents: Vec<Vec<u8>>,
    timestamps: Vec<i64>,

    write_buf: Vec<u8>,
}

impl LogsWriter {
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

            hostnames: StringInterner::with_capacity(flush_rows),
            sources: StringInterner::with_capacity(flush_rows),
            statuses: StringInterner::with_capacity(flush_rows),
            tags: StringInterner::with_capacity(flush_rows),

            contents: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),

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

                self.hostnames.intern(e.hostname().unwrap_or(""));
                self.sources.intern(e.source().unwrap_or(""));
                self.statuses.intern(e.status().unwrap_or(""));

                let tags_joined: String = e
                    .tags()
                    .map(|tl| {
                        (0..tl.len())
                            .map(|j| tl.get(j))
                            .collect::<Vec<_>>()
                            .join("|")
                    })
                    .unwrap_or_default();
                self.tags.intern_owned(tags_joined);

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

        // Take columns.
        let (hostname_vals, hostname_codes) = self.hostnames.take();
        let (source_vals, source_codes) = self.sources.take();
        let (status_vals, status_codes) = self.statuses.take();
        let (tag_vals, tag_codes) = self.tags.take();
        let contents = std::mem::take(&mut self.contents);
        let timestamps = std::mem::take(&mut self.timestamps);

        // Sort index by timestamp for better compression (delta encoding).
        // Only allocates one Vec<usize> (~80 KB for 10K rows) — all columns
        // are gathered in sorted order when building the Vortex arrays.
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        let hostname_array = DictArray::try_new(
            order
                .iter()
                .map(|&i| hostname_codes[i])
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(hostname_vals).into_array(),
        )
        .context("building hostname DictArray")?;

        let source_array = DictArray::try_new(
            order
                .iter()
                .map(|&i| source_codes[i])
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(source_vals).into_array(),
        )
        .context("building source DictArray")?;

        let status_array = DictArray::try_new(
            order
                .iter()
                .map(|&i| status_codes[i])
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(status_vals).into_array(),
        )
        .context("building status DictArray")?;

        let tag_array = DictArray::try_new(
            order
                .iter()
                .map(|&i| tag_codes[i])
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(tag_vals).into_array(),
        )
        .context("building tags DictArray")?;

        // Gather contents in sorted order. This is the only non-Copy column
        // so we must build a new Vec (contents are variable-size byte blobs).
        let sorted_contents: Vec<Vec<u8>> = order.iter().map(|&i| contents[i].clone()).collect();

        let st = StructArray::try_new(
            FieldNames::from([
                "hostname",
                "source",
                "status",
                "tags",
                "content",
                "timestamp_ns",
            ]),
            vec![
                hostname_array.into_array(),
                source_array.into_array(),
                status_array.into_array(),
                tag_array.into_array(),
                VarBinArray::from(sorted_contents).into_array(),
                order
                    .iter()
                    .map(|&i| timestamps[i])
                    .collect::<PrimitiveArray>()
                    .into_array(),
            ],
            row_count,
            Validity::NonNullable,
        )
        .context("building logs StructArray")?;

        let strategy = super::strategy::low_memory_strategy();

        // Fresh session per flush to prevent registry accumulation in VortexSession.
        let session = VortexSession::default();

        self.write_buf.clear();
        VortexWriteOptions::new(session)
            .with_strategy(strategy)
            .write(&mut self.write_buf, st.into_array().to_array_stream())
            .await
            .context("writing logs vortex file")?;

        tokio::fs::write(&path, &self.write_buf)
            .await
            .with_context(|| format!("writing {}", path.display()))?;

        // Shrink write_buf to release memory back to the allocator.
        self.write_buf = Vec::with_capacity(64 * 1024);

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
    use vortex::array::stream::ArrayStreamExt;
    use vortex::array::Canonical;
    use vortex::file::OpenOptionsSessionExt;

    fn make_writer(dir: &Path) -> LogsWriter {
        LogsWriter::new(dir, 1000, Duration::from_secs(60))
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

    fn read_i64_column(st: &vortex::array::arrays::StructArray, name: &str) -> Vec<i64> {
        let arr = st.unmasked_field_by_name(name).unwrap();
        let canonical = arr.to_canonical().unwrap();
        match canonical {
            Canonical::Primitive(prim) => prim.as_slice::<i64>().to_vec(),
            other => panic!("expected Primitive i64 for {name}, got {:?}", other.dtype()),
        }
    }

    fn read_bytes_column(st: &vortex::array::arrays::StructArray, name: &str) -> Vec<Vec<u8>> {
        let arr = st.unmasked_field_by_name(name).unwrap();
        let canonical = arr.to_canonical().unwrap();
        match canonical {
            Canonical::VarBinView(vbv) => (0..st.len())
                .map(|i| vbv.bytes_at(i).as_slice().to_vec())
                .collect(),
            other => panic!("expected VarBinView for {name}, got {:?}", other.dtype()),
        }
    }

    fn read_string_column(st: &vortex::array::arrays::StructArray, name: &str) -> Vec<String> {
        read_bytes_column(st, name)
            .into_iter()
            .map(|b| String::from_utf8_lossy(&b).into_owned())
            .collect()
    }

    fn add_rows(w: &mut LogsWriter, n: usize) {
        for i in 0..n {
            w.hostnames.intern("host1");
            w.sources.intern("app");
            w.statuses.intern("info");
            w.tags.intern("env:test");
            w.contents.push(format!("log line {}", i).into_bytes());
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
    async fn test_binary_content_roundtrip() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        let binary_data = vec![0u8, 1, 2, 3, 255, 0];
        w.hostnames.intern("h");
        w.sources.intern("");
        w.statuses.intern("info");
        w.tags.intern("");
        w.contents.push(binary_data.clone());
        w.timestamps.push(42);

        let path = w.flush().await.unwrap();
        let st = read_back(&path).await;

        assert_eq!(st.len(), 1);
        let contents = read_bytes_column(&st, "content");
        assert_eq!(contents[0], binary_data);
        let ts = read_i64_column(&st, "timestamp_ns");
        assert_eq!(ts[0], 42);
        let hostnames = read_string_column(&st, "hostname");
        assert_eq!(hostnames[0], "h");
    }

    #[tokio::test]
    async fn test_roundtrip_logs_inline() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Row 0: context 1
        w.hostnames.intern("server1");
        w.sources.intern("syslog");
        w.statuses.intern("warn");
        w.tags.intern("team:ops");
        w.contents.push(b"hello world".to_vec());
        w.timestamps.push(12345);

        // Row 1: context 2
        w.hostnames.intern("server2");
        w.sources.intern("app");
        w.statuses.intern("error");
        w.tags.intern("env:prod|team:sre");
        w.contents.push(b"something went wrong".to_vec());
        w.timestamps.push(67890);

        // Row 2: context 1 again
        w.hostnames.intern("server1");
        w.sources.intern("syslog");
        w.statuses.intern("warn");
        w.tags.intern("team:ops");
        w.contents.push(b"still going".to_vec());
        w.timestamps.push(99999);

        let logs_path = w.flush().await.unwrap();

        // --- Verify logs file ---
        let logs_st = read_back(&logs_path).await;
        assert_eq!(logs_st.len(), 3);

        // Check column names match new inline schema
        let col_names: Vec<String> = logs_st
            .names()
            .iter()
            .map(|s| s.as_ref().to_string())
            .collect();
        assert_eq!(
            col_names,
            vec!["hostname", "source", "status", "tags", "content", "timestamp_ns"]
        );

        let hostnames = read_string_column(&logs_st, "hostname");
        assert_eq!(hostnames, vec!["server1", "server2", "server1"]);

        let sources = read_string_column(&logs_st, "source");
        assert_eq!(sources, vec!["syslog", "app", "syslog"]);

        let statuses = read_string_column(&logs_st, "status");
        assert_eq!(statuses, vec!["warn", "error", "warn"]);

        let tags = read_string_column(&logs_st, "tags");
        assert_eq!(tags, vec!["team:ops", "env:prod|team:sre", "team:ops"]);

        let contents: Vec<String> = read_bytes_column(&logs_st, "content")
            .into_iter()
            .map(|b| String::from_utf8(b).unwrap())
            .collect();
        assert_eq!(
            contents,
            vec!["hello world", "something went wrong", "still going"]
        );

        let timestamps = read_i64_column(&logs_st, "timestamp_ns");
        assert_eq!(timestamps, vec![12345, 67890, 99999]);
    }

    #[tokio::test]
    async fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().await.is_err());
    }
}
