use std::fmt;
use std::path::{Path, PathBuf};

/// The type of signal data stored in a parquet file.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum FileType {
    Metrics,
    Logs,
    TraceStats,
}

impl fmt::Display for FileType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            FileType::Metrics => write!(f, "metrics"),
            FileType::Logs => write!(f, "logs"),
            FileType::TraceStats => write!(f, "trace_stats"),
        }
    }
}

/// A signal file on disk with parsed metadata.
pub struct SignalEntry {
    pub path: PathBuf,
    pub timestamp_ms: u64,
    pub size: u64,
    pub file_type: FileType,
}

/// Parse a signal filename into its file type and timestamp.
///
/// Recognizes: `metrics-{ts}.parquet`, `logs-{ts}.parquet`,
///             `trace_stats-{ts}.parquet`
/// Also accepts the legacy `flush-` prefix for backward compatibility.
pub fn parse_signal_file(filename: &str) -> Option<(FileType, u64)> {
    let stem = filename.strip_suffix(".parquet")?;
    let (prefix, ts_str) = stem.rsplit_once('-')?;
    let ts: u64 = ts_str.parse().ok()?;
    let file_type = match prefix {
        "metrics" | "flush-metrics" => FileType::Metrics,
        "logs" | "flush-logs" => FileType::Logs,
        "trace_stats" | "flush-trace_stats" => FileType::TraceStats,
        _ => return None,
    };
    Some((file_type, ts))
}

/// Scan a directory for all `.parquet` signal files with parseable names.
pub async fn scan_signal_files(dir: &Path) -> anyhow::Result<Vec<SignalEntry>> {
    let mut entries = Vec::new();
    let mut dir_entries = tokio::fs::read_dir(dir).await?;
    while let Some(entry) = dir_entries.next_entry().await? {
        let name = match entry.file_name().into_string() {
            Ok(n) => n,
            Err(_) => continue,
        };
        let (file_type, timestamp_ms) = match parse_signal_file(&name) {
            Some(parsed) => parsed,
            None => continue,
        };
        let size = match entry.metadata().await {
            Ok(m) => m.len(),
            Err(_) => continue,
        };
        entries.push(SignalEntry {
            path: entry.path(),
            timestamp_ms,
            size,
            file_type,
        });
    }
    Ok(entries)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_signal_file() {
        // New naming (rotated files, no flush- prefix).
        assert_eq!(
            parse_signal_file("metrics-1710938400123.parquet"),
            Some((FileType::Metrics, 1710938400123))
        );
        assert_eq!(
            parse_signal_file("logs-9999999999999.parquet"),
            Some((FileType::Logs, 9999999999999))
        );
        assert_eq!(
            parse_signal_file("trace_stats-1234567890.parquet"),
            Some((FileType::TraceStats, 1234567890))
        );
        // Legacy flush- prefix still accepted.
        assert_eq!(
            parse_signal_file("flush-metrics-123.parquet"),
            Some((FileType::Metrics, 123))
        );
        // Old vortex files are not recognized.
        assert_eq!(parse_signal_file("flush-metrics-123.vortex"), None);
        assert_eq!(parse_signal_file("data.csv"), None);
        assert_eq!(parse_signal_file("unknown-123.parquet"), None);
        assert_eq!(parse_signal_file("metrics.parquet"), None);
    }

    #[tokio::test]
    async fn test_scan_signal_files() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("metrics-100.parquet"), "data").unwrap();
        std::fs::write(dir.path().join("logs-200.parquet"), "data").unwrap();
        std::fs::write(dir.path().join("trace_stats-300.parquet"), "stats").unwrap();
        // Non-signal files should be ignored
        std::fs::write(dir.path().join("contexts.bin"), "ctx").unwrap();
        std::fs::write(dir.path().join("readme.txt"), "keep").unwrap();
        // Old vortex files should be ignored
        std::fs::write(dir.path().join("flush-metrics-50.vortex"), "old").unwrap();

        let mut entries = scan_signal_files(dir.path()).await.unwrap();
        entries.sort_by_key(|e| e.timestamp_ms);

        assert_eq!(entries.len(), 3);
        assert_eq!(entries[0].file_type, FileType::Metrics);
        assert_eq!(entries[0].timestamp_ms, 100);
        assert_eq!(entries[1].file_type, FileType::Logs);
        assert_eq!(entries[1].timestamp_ms, 200);
        assert_eq!(entries[2].file_type, FileType::TraceStats);
        assert_eq!(entries[2].timestamp_ms, 300);
    }
}
