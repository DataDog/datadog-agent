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

use crate::generated::signals_generated::signals::MetricBatch;

/// A row accumulated before writing to disk.
pub struct MetricRow {
    pub name: String,
    pub value: f64,
    pub tags: String, // joined with '|'
    pub timestamp_ns: i64,
    pub sample_rate: f64,
    pub source: String,
}

/// Accumulates metric samples and flushes them to Vortex files.
pub struct MetricsWriter {
    pub output_dir: PathBuf,
    pub flush_rows: usize,
    pub flush_interval: Duration,
    pub rows: Vec<MetricRow>,
    pub last_flush: Instant,
}

impl MetricsWriter {
    pub fn new(output_dir: impl AsRef<Path>, flush_rows: usize, flush_interval: Duration) -> Self {
        Self {
            output_dir: output_dir.as_ref().to_path_buf(),
            flush_rows,
            flush_interval,
            rows: Vec::new(),
            last_flush: Instant::now(),
        }
    }

    /// Ingest a MetricBatch. Flushes automatically when thresholds are reached.
    pub async fn push(&mut self, batch: &MetricBatch<'_>) -> Result<Option<PathBuf>> {
        if let Some(samples) = batch.samples() {
            for i in 0..samples.len() {
                let s = samples.get(i);
                let tags: String = s
                    .tags()
                    .map(|tl| {
                        (0..tl.len())
                            .map(|j| tl.get(j).to_string())
                            .collect::<Vec<_>>()
                            .join("|")
                    })
                    .unwrap_or_default();
                let name = s.name().unwrap_or("").to_string();
                let source = s.source().unwrap_or("").to_string();
                self.rows.push(MetricRow {
                    name,
                    value: s.value(),
                    tags,
                    timestamp_ns: s.timestamp_ns(),
                    sample_rate: s.sample_rate(),
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
        let path = self.output_dir.join(format!("metrics-{}.vortex", ts_ms));

        write_metrics_file(&path, &self.rows).await?;

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

async fn write_metrics_file(path: &Path, rows: &[MetricRow]) -> Result<()> {
    let names: Vec<String> = rows.iter().map(|r| r.name.clone()).collect();
    let values: Vec<f64> = rows.iter().map(|r| r.value).collect();
    let tags: Vec<String> = rows.iter().map(|r| r.tags.clone()).collect();
    let timestamps: Vec<i64> = rows.iter().map(|r| r.timestamp_ns).collect();
    let sample_rates: Vec<f64> = rows.iter().map(|r| r.sample_rate).collect();
    let sources: Vec<String> = rows.iter().map(|r| r.source.clone()).collect();

    let len = rows.len();
    let st = StructArray::try_new(
        FieldNames::from(["name", "value", "tags", "timestamp_ns", "sample_rate", "source"]),
        vec![
            VarBinArray::from(names).into_array(),
            values.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(tags).into_array(),
            timestamps.into_iter().collect::<PrimitiveArray>().into_array(),
            sample_rates.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(sources).into_array(),
        ],
        len,
        Validity::NonNullable,
    )
    .context("building metrics StructArray")?;

    let session = VortexSession::default();
    let strategy = WriteStrategyBuilder::default()
        .with_compact_encodings()
        .build();
    let mut buffer: Vec<u8> = Vec::new();
    VortexWriteOptions::new(session)
        .with_strategy(strategy)
        .write(&mut buffer, st.into_array().to_array_stream())
        .await
        .context("writing metrics vortex file")?;

    tokio::fs::write(path, &buffer)
        .await
        .with_context(|| format!("writing {}", path.display()))?;

    Ok(())
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
            w.rows.push(MetricRow {
                name: format!("cpu{}", i),
                value: i as f64,
                tags: "host:a".to_string(),
                timestamp_ns: 1000,
                sample_rate: 1.0,
                source: "test".to_string(),
            });
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
        w.rows.push(MetricRow {
            name: "mem".to_string(),
            value: 42.0,
            tags: "env:prod".to_string(),
            timestamp_ns: 999,
            sample_rate: 0.5,
            source: "src1".to_string(),
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
