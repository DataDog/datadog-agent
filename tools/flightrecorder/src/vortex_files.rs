use std::fmt;
use std::path::{Path, PathBuf};

/// The type of signal data stored in a vortex file.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum FileType {
    Metrics,
    Logs,
}

impl fmt::Display for FileType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            FileType::Metrics => write!(f, "metrics"),
            FileType::Logs => write!(f, "logs"),
        }
    }
}

/// A vortex file on disk with parsed metadata.
pub struct VortexEntry {
    pub path: PathBuf,
    pub timestamp_ms: u64,
    pub size: u64,
    pub file_type: FileType,
}

/// Parse a vortex filename into its file type and timestamp.
///
/// Recognizes: `metrics-{ts}.vortex`, `logs-{ts}.vortex`.
pub fn parse_vortex_file(filename: &str) -> Option<(FileType, u64)> {
    let stem = filename.strip_suffix(".vortex")?;
    let (prefix, ts_str) = stem.rsplit_once('-')?;
    let ts: u64 = ts_str.parse().ok()?;
    let file_type = match prefix {
        "metrics" => FileType::Metrics,
        "logs" => FileType::Logs,
        _ => return None,
    };
    Some((file_type, ts))
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
        let (file_type, timestamp_ms) = match parse_vortex_file(&name) {
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
        });
    }
    Ok(entries)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_vortex_file() {
        assert_eq!(
            parse_vortex_file("metrics-1710938400123.vortex"),
            Some((FileType::Metrics, 1710938400123))
        );
        assert_eq!(
            parse_vortex_file("logs-9999999999999.vortex"),
            Some((FileType::Logs, 9999999999999))
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
        std::fs::write(dir.path().join("metrics-100.vortex"), "data").unwrap();
        std::fs::write(dir.path().join("logs-200.vortex"), "data").unwrap();
        // Legacy context files should be ignored
        std::fs::write(dir.path().join("contexts-300.vortex"), "data").unwrap();
        std::fs::write(dir.path().join("log-contexts-400.vortex"), "data").unwrap();
        std::fs::write(dir.path().join("readme.txt"), "keep").unwrap();

        let mut entries = scan_vortex_files(dir.path()).await.unwrap();
        entries.sort_by_key(|e| e.timestamp_ms);

        assert_eq!(entries.len(), 2);
        assert_eq!(entries[0].file_type, FileType::Metrics);
        assert_eq!(entries[0].timestamp_ms, 100);
        assert_eq!(entries[1].file_type, FileType::Logs);
    }
}
