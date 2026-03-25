use std::fmt;
use std::path::{Path, PathBuf};

/// The type of signal data stored in a vortex file.
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

/// A vortex file on disk with parsed metadata.
pub struct VortexEntry {
    pub path: PathBuf,
    pub timestamp_ms: u64,
    pub size: u64,
    pub file_type: FileType,
    /// True for uncompressed flush files (`flush-metrics-*`, `flush-logs-*`),
    /// false for compressed merged files (`metrics-*`, `logs-*`).
    pub is_flush: bool,
}

/// Parse a vortex filename into its file type, timestamp, and flush flag.
///
/// Recognizes:
///   - `flush-metrics-{ts}.vortex` / `flush-logs-{ts}.vortex` / `flush-trace_stats-{ts}.vortex` (flush files)
///   - `metrics-{ts}.vortex` / `logs-{ts}.vortex` / `trace_stats-{ts}.vortex` (merged files)
pub fn parse_vortex_file(filename: &str) -> Option<(FileType, u64, bool)> {
    let stem = filename.strip_suffix(".vortex")?;
    let (prefix, ts_str) = stem.rsplit_once('-')?;
    let ts: u64 = ts_str.parse().ok()?;
    let (file_type, is_flush) = match prefix {
        "flush-metrics" => (FileType::Metrics, true),
        "flush-logs" => (FileType::Logs, true),
        "flush-trace_stats" => (FileType::TraceStats, true),
        "metrics" => (FileType::Metrics, false),
        "logs" => (FileType::Logs, false),
        "trace_stats" => (FileType::TraceStats, false),
        _ => return None,
    };
    Some((file_type, ts, is_flush))
}

/// Scan a directory for all `.vortex` files with parseable names.
pub async fn scan_vortex_files(dir: &Path) -> anyhow::Result<Vec<VortexEntry>> {
    let mut entries = Vec::new();
    let mut dir_entries = tokio::fs::read_dir(dir).await?;
    while let Some(entry) = dir_entries.next_entry().await? {
        let name = match entry.file_name().into_string() {
            Ok(n) => n,
            Err(_) => continue,
        };
        let (file_type, timestamp_ms, is_flush) = match parse_vortex_file(&name) {
            Some(parsed) => parsed,
            None => continue,
        };
        let size = match entry.metadata().await {
            Ok(m) => m.len(),
            Err(_) => continue,
        };
        entries.push(VortexEntry {
            path: entry.path(),
            timestamp_ms,
            size,
            file_type,
            is_flush,
        });
    }
    Ok(entries)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_vortex_file() {
        // Flush files
        assert_eq!(
            parse_vortex_file("flush-metrics-1710938400123.vortex"),
            Some((FileType::Metrics, 1710938400123, true))
        );
        assert_eq!(
            parse_vortex_file("flush-logs-9999999999999.vortex"),
            Some((FileType::Logs, 9999999999999, true))
        );
        assert_eq!(
            parse_vortex_file("flush-trace_stats-1234567890.vortex"),
            Some((FileType::TraceStats, 1234567890, true))
        );
        // Merged files
        assert_eq!(
            parse_vortex_file("metrics-1710938400123.vortex"),
            Some((FileType::Metrics, 1710938400123, false))
        );
        assert_eq!(
            parse_vortex_file("logs-9999999999999.vortex"),
            Some((FileType::Logs, 9999999999999, false))
        );
        assert_eq!(
            parse_vortex_file("trace_stats-42.vortex"),
            Some((FileType::TraceStats, 42, false))
        );
        // Legacy context file types are no longer recognized
        assert_eq!(parse_vortex_file("contexts-1000.vortex"), None);
        assert_eq!(parse_vortex_file("log-contexts-42.vortex"), None);
        // Not a vortex file
        assert_eq!(parse_vortex_file("data.parquet"), None);
        // Unknown prefix
        assert_eq!(parse_vortex_file("unknown-123.vortex"), None);
        // No timestamp
        assert_eq!(parse_vortex_file("metrics.vortex"), None);
        // Tmp file
        assert_eq!(parse_vortex_file("metrics-123.vortex.tmp"), None);
    }

    #[tokio::test]
    async fn test_scan_vortex_files() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("flush-metrics-100.vortex"), "data").unwrap();
        std::fs::write(dir.path().join("metrics-50.vortex"), "merged").unwrap();
        std::fs::write(dir.path().join("flush-logs-200.vortex"), "data").unwrap();
        std::fs::write(dir.path().join("flush-trace_stats-300.vortex"), "stats").unwrap();
        // Legacy context files should be ignored
        std::fs::write(dir.path().join("contexts-400.vortex"), "data").unwrap();
        std::fs::write(dir.path().join("log-contexts-500.vortex"), "data").unwrap();
        std::fs::write(dir.path().join("readme.txt"), "keep").unwrap();

        let mut entries = scan_vortex_files(dir.path()).await.unwrap();
        entries.sort_by_key(|e| e.timestamp_ms);

        assert_eq!(entries.len(), 4);
        assert_eq!(entries[0].file_type, FileType::Metrics);
        assert_eq!(entries[0].timestamp_ms, 50);
        assert!(!entries[0].is_flush);
        assert_eq!(entries[1].file_type, FileType::Metrics);
        assert_eq!(entries[1].timestamp_ms, 100);
        assert!(entries[1].is_flush);
        assert_eq!(entries[2].file_type, FileType::Logs);
        assert_eq!(entries[2].timestamp_ms, 200);
        assert!(entries[2].is_flush);
        assert_eq!(entries[3].file_type, FileType::TraceStats);
        assert_eq!(entries[3].timestamp_ms, 300);
        assert!(entries[3].is_flush);
    }
}
