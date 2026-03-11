use std::path::{Path, PathBuf};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use vortex::array::arrays::{PrimitiveArray, StructArray, VarBinArray};
use vortex::array::dtype::FieldNames;
use vortex::array::validity::Validity;
use vortex::array::IntoArray;
use vortex::file::{VortexWriteOptions, WriteStrategyBuilder};
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

use crate::generated::signals_generated::signals::LogBatch;

/// A row accumulated before writing to disk.
pub struct LogRow {
    pub content: Vec<u8>,
    pub status: String,
    pub tags: String, // joined with '|'
    pub hostname: String,
    pub timestamp_ns: i64,
    pub source: String,
}

/// Accumulates log entries and flushes them to Vortex files.
pub struct LogsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub rows: Vec<LogRow>,
    pub last_flush: Instant,
}

impl LogsWriter {
    pub fn new(output_dir: impl AsRef<Path>, flush_rows: usize, flush_interval: Duration) -> Self {
        Self {
            output_dir: output_dir.as_ref().to_path_buf(),
            flush_rows,
            flush_interval,
            rows: Vec::new(),
            last_flush: Instant::now(),
        }
    }

    /// Ingest a LogBatch. Flushes automatically when thresholds are reached.
    pub async fn push(&mut self, batch: &LogBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(entries) = batch.entries() {
            for i in 0..entries.len() {
                let e = entries.get(i);
                let tags: String = e
                    .tags()
                    .map(|tl| {
                        (0..tl.len())
                            .map(|j| tl.get(j).to_string())
                            .collect::<Vec<_>>()
                            .join("|")
                    })
                    .unwrap_or_default();
                let status = e.status().unwrap_or("").to_string();
                let hostname = e.hostname().unwrap_or("").to_string();
                let source = e.source().unwrap_or("").to_string();
                let content = e.content().map(|c| c.bytes().to_vec()).unwrap_or_default();

                self.rows.push(LogRow {
                    content,
                    status,
                    tags,
                    hostname,
                    timestamp_ns: e.timestamp_ns(),
                    source,
                });
            }
        }

        if self.rows.len() >= self.flush_rows
            || self.last_flush.elapsed() >= self.flush_interval
        {
            return self.flush().await.map(Some);
        }
        Ok(None)
    }

    /// Flush accumulated rows to a new Vortex file. Returns the file path.
    pub async fn flush(&mut self) -> Result<PathBuf> {
        if self.rows.is_empty() {
            anyhow::bail!("no rows to flush");
        }

        let ts_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let path = self.output_dir.join(format!("logs-{}.vortex", ts_ms));

        write_logs_file(&path, &self.rows).await?;

        self.rows.clear();
        self.last_flush = Instant::now();
        Ok(path)
    }

    /// Flush if any rows are buffered. Used on shutdown.
    pub async fn flush_if_any(&mut self) -> Result<Option<PathBuf>> {
        if self.rows.is_empty() {
            Ok(None)
        } else {
            self.flush().await.map(Some)
        }
    }
}

async fn write_logs_file(path: &Path, rows: &[LogRow]) -> Result<()> {
    let contents: Vec<Vec<u8>> = rows.iter().map(|r| r.content.clone()).collect();
    let statuses: Vec<String> = rows.iter().map(|r| r.status.clone()).collect();
    let tags: Vec<String> = rows.iter().map(|r| r.tags.clone()).collect();
    let hostnames: Vec<String> = rows.iter().map(|r| r.hostname.clone()).collect();
    let timestamps: Vec<i64> = rows.iter().map(|r| r.timestamp_ns).collect();
    let sources: Vec<String> = rows.iter().map(|r| r.source.clone()).collect();

    let len = rows.len();
    let st = StructArray::try_new(
        FieldNames::from(["content", "status", "tags", "hostname", "timestamp_ns", "source"]),
        vec![
            VarBinArray::from(contents).into_array(),
            VarBinArray::from(statuses).into_array(),
            VarBinArray::from(tags).into_array(),
            VarBinArray::from(hostnames).into_array(),
            timestamps.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(sources).into_array(),
        ],
        len,
        Validity::NonNullable,
    )
    .context("building logs StructArray")?;

    let session = VortexSession::default();
    let strategy = WriteStrategyBuilder::default()
        .with_compact_encodings()
        .build();
    let mut buffer: Vec<u8> = Vec::new();
    VortexWriteOptions::new(session)
        .with_strategy(strategy)
        .write(&mut buffer, st.into_array().to_array_stream())
        .await
        .context("writing logs vortex file")?;

    tokio::fs::write(path, &buffer)
        .await
        .with_context(|| format!("writing {}", path.display()))?;

    Ok(())
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
            w.rows.push(LogRow {
                content: format!("log line {}", i).into_bytes(),
                status: "info".to_string(),
                tags: "env:test".to_string(),
                hostname: "host1".to_string(),
                timestamp_ns: i as i64 * 1000,
                source: "app".to_string(),
            });
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
        w.rows.push(LogRow {
            content: vec![0u8, 1, 2, 3, 255, 0],
            status: "info".to_string(),
            tags: String::new(),
            hostname: "h".to_string(),
            timestamp_ns: 0,
            source: String::new(),
        });
        let path = w.flush().await.unwrap();
        assert!(path.exists());
        assert!(path.metadata().unwrap().len() > 0);
    }

    #[tokio::test]
    async fn test_readback_fields() {
        let dir = tempdir().unwrap();
        let mut w = make_writer(dir.path());
        w.rows.push(LogRow {
            content: b"hello world".to_vec(),
            status: "warn".to_string(),
            tags: "team:ops".to_string(),
            hostname: "server1".to_string(),
            timestamp_ns: 12345,
            source: "syslog".to_string(),
        });
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
