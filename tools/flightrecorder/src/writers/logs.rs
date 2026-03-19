use std::hash::{Hash, Hasher};
use std::path::{Path, PathBuf};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use vortex::array::arrays::{PrimitiveArray, StructArray, VarBinArray};
use vortex::array::dtype::FieldNames;
use vortex::array::validity::Validity;
use vortex::array::IntoArray;
use vortex::file::VortexWriteOptions;
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

use super::log_contexts::LogContextsWriter;
use crate::generated::signals_generated::signals::LogBatch;

/// Columnar accumulator for log entries.
///
/// Repetitive fields (hostname, source, status, tags) are extracted into
/// separate `log-contexts-*.vortex` files via an embedded [`LogContextsWriter`],
/// referenced by a `context_key` (u64). Log files contain only 3 columns:
/// `context_key`, `content`, `timestamp_ns`.
pub struct LogsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub last_flush: Instant,

    // Plain columnar buffers.
    context_keys: Vec<u64>,
    contents: Vec<Vec<u8>>,
    timestamps: Vec<i64>,

    // Writes log context definitions to separate vortex files.
    log_contexts: LogContextsWriter,

    write_buf: Vec<u8>,
}

impl LogsWriter {
    pub fn new(output_dir: impl AsRef<Path>, flush_rows: usize, flush_interval: Duration) -> Self {
        let output_dir = output_dir.as_ref().to_path_buf();
        Self {
            // Context rows are larger than log rows (hostname/tags strings),
            // so flush contexts in smaller batches to limit transient Vortex pipeline memory.
            log_contexts: LogContextsWriter::new(&output_dir, flush_rows.min(2000), flush_interval),

            output_dir,
            flush_rows,
            flush_interval,
            last_flush: Instant::now(),

            context_keys: Vec::with_capacity(flush_rows),
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

                let hostname = e.hostname().unwrap_or("");
                let source = e.source().unwrap_or("");
                let status = e.status().unwrap_or("");
                let tags_joined: String = e
                    .tags()
                    .map(|tl| {
                        (0..tl.len())
                            .map(|j| tl.get(j))
                            .collect::<Vec<_>>()
                            .join("|")
                    })
                    .unwrap_or_default();

                // Compute context key by hashing the repetitive fields.
                let mut hasher = std::hash::DefaultHasher::new();
                hostname.hash(&mut hasher);
                source.hash(&mut hasher);
                status.hash(&mut hasher);
                tags_joined.hash(&mut hasher);
                let context_key = hasher.finish();

                self.log_contexts
                    .try_record(context_key, hostname, source, status, &tags_joined);

                self.context_keys.push(context_key);
                self.contents.push(
                    e.content().map(|c| c.bytes().to_vec()).unwrap_or_default(),
                );
                self.timestamps.push(e.timestamp_ns());
            }
        }

        // Flush log contexts if their thresholds are reached.
        if self.log_contexts.should_flush() {
            if let Err(e) = self.log_contexts.flush().await {
                tracing::warn!("log contexts flush error: {}", e);
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
        let context_keys = std::mem::take(&mut self.context_keys);
        let contents = std::mem::take(&mut self.contents);
        let timestamps = std::mem::take(&mut self.timestamps);

        let st = StructArray::try_new(
            FieldNames::from(["context_key", "content", "timestamp_ns"]),
            vec![
                context_keys
                    .into_iter()
                    .collect::<PrimitiveArray>()
                    .into_array(),
                VarBinArray::from(contents).into_array(),
                timestamps
                    .into_iter()
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

    /// Clear the log context bloom filter and flush any pending context rows.
    /// Called when a new agent connection is accepted because the agent will
    /// re-send all log entries.
    pub async fn reset_contexts(&mut self) -> Result<()> {
        if let Err(e) = self.log_contexts.flush_if_any().await {
            tracing::warn!("log contexts flush error during reset: {}", e);
        }
        self.log_contexts.reset();
        Ok(())
    }

    /// Flush if any rows are buffered. Used on shutdown.
    pub async fn flush_if_any(&mut self) -> Result<Option<PathBuf>> {
        // Flush pending log contexts first.
        if let Err(e) = self.log_contexts.flush_if_any().await {
            tracing::warn!("log contexts flush error during logs flush_if_any: {}", e);
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
    use std::collections::HashMap;
    use tempfile::tempdir;
    use vortex::array::stream::ArrayStreamExt;
    use vortex::array::Canonical;
    use vortex::file::OpenOptionsSessionExt;

    fn make_writer(dir: &Path) -> LogsWriter {
        LogsWriter::new(dir, 1000, Duration::from_secs(60))
    }

    /// Compute a context key the same way LogsWriter does.
    fn compute_context_key(hostname: &str, source: &str, status: &str, tags: &str) -> u64 {
        let mut hasher = std::hash::DefaultHasher::new();
        hostname.hash(&mut hasher);
        source.hash(&mut hasher);
        status.hash(&mut hasher);
        tags.hash(&mut hasher);
        hasher.finish()
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
            let context_key = compute_context_key("host1", "app", "info", "env:test");
            w.log_contexts
                .try_record(context_key, "host1", "app", "info", "env:test");
            w.context_keys.push(context_key);
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
        let context_key = compute_context_key("h", "", "info", "");
        w.log_contexts
            .try_record(context_key, "h", "", "info", "");
        w.context_keys.push(context_key);
        w.contents.push(binary_data.clone());
        w.timestamps.push(42);

        let path = w.flush().await.unwrap();
        let st = read_back(&path).await;

        assert_eq!(st.len(), 1);
        let contents = read_bytes_column(&st, "content");
        assert_eq!(contents[0], binary_data);
        let ts = read_i64_column(&st, "timestamp_ns");
        assert_eq!(ts[0], 42);
        let ckeys = read_u64_column(&st, "context_key");
        assert_eq!(ckeys[0], context_key);
    }

    #[tokio::test]
    async fn test_roundtrip_logs_and_contexts() {
        // Write multiple log rows with 2 distinct contexts, then read back both
        // log file and context file and verify values match what was sent.
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        let ctx1_key = compute_context_key("server1", "syslog", "warn", "team:ops");
        let ctx2_key = compute_context_key("server2", "app", "error", "env:prod|team:sre");
        assert_ne!(ctx1_key, ctx2_key, "distinct inputs must produce distinct keys");

        // Row 0: context 1
        w.log_contexts
            .try_record(ctx1_key, "server1", "syslog", "warn", "team:ops");
        w.context_keys.push(ctx1_key);
        w.contents.push(b"hello world".to_vec());
        w.timestamps.push(12345);

        // Row 1: context 2
        w.log_contexts
            .try_record(ctx2_key, "server2", "app", "error", "env:prod|team:sre");
        w.context_keys.push(ctx2_key);
        w.contents.push(b"something went wrong".to_vec());
        w.timestamps.push(67890);

        // Row 2: context 1 again (should be deduped in contexts file)
        w.log_contexts
            .try_record(ctx1_key, "server1", "syslog", "warn", "team:ops");
        w.context_keys.push(ctx1_key);
        w.contents.push(b"still going".to_vec());
        w.timestamps.push(99999);

        // Flush both logs and contexts
        let ctx_path = w.log_contexts.flush().await.unwrap();
        let logs_path = w.flush().await.unwrap();

        // --- Verify logs file ---
        let logs_st = read_back(&logs_path).await;
        assert_eq!(logs_st.len(), 3);

        // Check column names are exactly what we expect (no legacy inline columns)
        let col_names: Vec<String> = logs_st.names().iter().map(|s| s.as_ref().to_string()).collect();
        assert_eq!(col_names, vec!["context_key", "content", "timestamp_ns"]);

        let ckeys = read_u64_column(&logs_st, "context_key");
        assert_eq!(ckeys, vec![ctx1_key, ctx2_key, ctx1_key]);

        let contents: Vec<String> = read_bytes_column(&logs_st, "content")
            .into_iter()
            .map(|b| String::from_utf8(b).unwrap())
            .collect();
        assert_eq!(contents, vec!["hello world", "something went wrong", "still going"]);

        let timestamps = read_i64_column(&logs_st, "timestamp_ns");
        assert_eq!(timestamps, vec![12345, 67890, 99999]);

        // --- Verify contexts file ---
        let ctx_st = read_back(&ctx_path).await;
        // Only 2 unique contexts should have been written (bloom dedup)
        assert_eq!(ctx_st.len(), 2);

        let ctx_ckeys = read_u64_column(&ctx_st, "context_key");
        let ctx_hostnames = read_string_column(&ctx_st, "hostname");
        let ctx_sources = read_string_column(&ctx_st, "source");
        let ctx_statuses = read_string_column(&ctx_st, "status");
        let ctx_tags = read_string_column(&ctx_st, "tags");

        // Build a lookup to verify context resolution
        let mut ctx_map: HashMap<u64, (&str, &str, &str, &str)> = HashMap::new();
        for i in 0..2 {
            ctx_map.insert(
                ctx_ckeys[i],
                (&ctx_hostnames[i], &ctx_sources[i], &ctx_statuses[i], &ctx_tags[i]),
            );
        }

        let (h1, s1, st1, t1) = ctx_map[&ctx1_key];
        assert_eq!(h1, "server1");
        assert_eq!(s1, "syslog");
        assert_eq!(st1, "warn");
        assert_eq!(t1, "team:ops");

        let (h2, s2, st2, t2) = ctx_map[&ctx2_key];
        assert_eq!(h2, "server2");
        assert_eq!(s2, "app");
        assert_eq!(st2, "error");
        assert_eq!(t2, "env:prod|team:sre");

        // Verify we can resolve every log row's context_key back to the right context
        for ckey in &ckeys {
            assert!(ctx_map.contains_key(ckey), "log row has unresolvable context_key {ckey}");
        }
    }

    #[tokio::test]
    async fn test_empty_flush_errors() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        assert!(w.flush().await.is_err());
    }

    #[tokio::test]
    async fn test_context_dedup() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());

        // Add multiple rows with the same context — only 1 context should be recorded
        add_rows(&mut w, 10);
        assert_eq!(w.log_contexts.len(), 1); // bloom dedup

        // Flush contexts and verify file exists
        let ctx_path = w.log_contexts.flush().await.unwrap();
        assert!(ctx_path
            .file_name()
            .unwrap()
            .to_str()
            .unwrap()
            .starts_with("log-contexts-"));
    }

    #[tokio::test]
    async fn test_reset_contexts() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        add_rows(&mut w, 5);
        assert_eq!(w.log_contexts.len(), 1);

        w.reset_contexts().await.unwrap();
        assert_eq!(w.log_contexts.len(), 0);

        // After reset, same context should be accepted again
        let context_key = compute_context_key("host1", "app", "info", "env:test");
        assert!(w
            .log_contexts
            .try_record(context_key, "host1", "app", "info", "env:test"));
    }
}
