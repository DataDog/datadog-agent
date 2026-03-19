use std::path::PathBuf;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use tokio_util::sync::CancellationToken;
use tracing::{debug, info, warn};

/// Periodically cleans up old `.vortex` files based on time-based retention and
/// an optional disk usage cap.
pub struct Janitor {
    output_dir: PathBuf,
    retention: Duration,
    max_disk_bytes: u64, // 0 = unlimited
    interval: Duration,
}

impl Janitor {
    pub fn new(output_dir: &str, retention: Duration, max_disk_bytes: u64) -> Self {
        Self {
            output_dir: PathBuf::from(output_dir),
            retention,
            max_disk_bytes,
            interval: Duration::from_secs(30),
        }
    }

    pub async fn run(self, cancel: CancellationToken) {
        info!(
            output_dir = %self.output_dir.display(),
            retention_secs = self.retention.as_secs(),
            max_disk_bytes = self.max_disk_bytes,
            interval_secs = self.interval.as_secs(),
            "janitor started"
        );
        loop {
            tokio::select! {
                _ = cancel.cancelled() => {
                    info!("janitor shutting down");
                    return;
                }
                _ = tokio::time::sleep(self.interval) => {
                    if let Err(e) = self.cleanup().await {
                        warn!("janitor cleanup error: {}", e);
                    }
                }
            }
        }
    }

    async fn cleanup(&self) -> anyhow::Result<()> {
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        let retention_ms = self.retention.as_millis() as u64;

        // Collect all .vortex files with their parsed timestamps and sizes.
        let mut entries: Vec<VortexEntry> = Vec::new();
        let mut dir = tokio::fs::read_dir(&self.output_dir).await?;
        while let Some(entry) = dir.next_entry().await? {
            let name = match entry.file_name().into_string() {
                Ok(n) => n,
                Err(_) => continue,
            };
            let ts = match parse_vortex_timestamp(&name) {
                Some(t) => t,
                None => continue,
            };
            let size = match entry.metadata().await {
                Ok(m) => m.len(),
                Err(_) => continue,
            };
            entries.push(VortexEntry {
                path: entry.path(),
                timestamp_ms: ts,
                size,
            });
        }

        // Sort oldest first.
        entries.sort_by_key(|e| e.timestamp_ms);

        // Pass 1: time-based retention.
        let mut deleted_time = 0u64;
        let mut remaining: Vec<VortexEntry> = Vec::with_capacity(entries.len());
        for entry in entries {
            if now_ms.saturating_sub(entry.timestamp_ms) > retention_ms {
                if let Err(e) = tokio::fs::remove_file(&entry.path).await {
                    warn!(path = %entry.path.display(), "failed to delete expired file: {}", e);
                } else {
                    info!(path = %entry.path.display(), age_hours = (now_ms - entry.timestamp_ms) / 3_600_000, "deleted expired vortex file");
                    deleted_time += 1;
                }
            } else {
                remaining.push(entry);
            }
        }

        // Pass 2: disk cap.
        let mut deleted_cap = 0u64;
        if self.max_disk_bytes > 0 {
            let mut total_size: u64 = remaining.iter().map(|e| e.size).sum();
            // remaining is already sorted oldest-first
            while total_size > self.max_disk_bytes {
                if let Some(oldest) = remaining.first() {
                    let path = oldest.path.clone();
                    let size = oldest.size;
                    if let Err(e) = tokio::fs::remove_file(&path).await {
                        warn!(path = %path.display(), "failed to delete file for disk cap: {}", e);
                        remaining.remove(0);
                    } else {
                        info!(path = %path.display(), size_mb = size / (1024 * 1024), "deleted vortex file (disk cap)");
                        total_size -= size;
                        deleted_cap += 1;
                        remaining.remove(0);
                    }
                } else {
                    break;
                }
            }
        }

        if deleted_time > 0 || deleted_cap > 0 {
            debug!(
                deleted_time,
                deleted_cap,
                remaining = remaining.len(),
                "janitor cleanup complete"
            );
        }

        Ok(())
    }
}

struct VortexEntry {
    path: PathBuf,
    timestamp_ms: u64,
    size: u64,
}

/// Extracts epoch-ms timestamp from filenames like "metrics-1710938400123.vortex".
fn parse_vortex_timestamp(filename: &str) -> Option<u64> {
    let stem = filename.strip_suffix(".vortex")?;
    let ts_str = stem.rsplit('-').next()?;
    ts_str.parse().ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_vortex_timestamp() {
        assert_eq!(
            parse_vortex_timestamp("metrics-1710938400123.vortex"),
            Some(1710938400123)
        );
        assert_eq!(
            parse_vortex_timestamp("logs-9999999999999.vortex"),
            Some(9999999999999)
        );
        assert_eq!(
            parse_vortex_timestamp("contexts-1000.vortex"),
            Some(1000)
        );
        // Not a vortex file
        assert_eq!(parse_vortex_timestamp("data.parquet"), None);
        // No timestamp portion
        assert_eq!(parse_vortex_timestamp("metrics.vortex"), None);
    }

    #[tokio::test]
    async fn test_cleanup_time_retention() {
        let dir = tempfile::tempdir().unwrap();
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        // Create an "old" file (2 hours ago) and a "new" file (now).
        let old_name = format!("metrics-{}.vortex", now_ms - 7_200_000);
        let new_name = format!("metrics-{}.vortex", now_ms);
        std::fs::write(dir.path().join(&old_name), "old").unwrap();
        std::fs::write(dir.path().join(&new_name), "new").unwrap();

        let janitor = Janitor::new(
            dir.path().to_str().unwrap(),
            Duration::from_secs(3600), // 1 hour retention
            0,
        );
        janitor.cleanup().await.unwrap();

        // Old file should be gone, new file should remain.
        assert!(!dir.path().join(&old_name).exists());
        assert!(dir.path().join(&new_name).exists());
    }

    #[tokio::test]
    async fn test_cleanup_disk_cap() {
        let dir = tempfile::tempdir().unwrap();
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        // Create 3 files, each ~100 bytes, with a 200-byte cap.
        let data = vec![0u8; 100];
        for i in 0..3 {
            let name = format!("metrics-{}.vortex", now_ms - (2 - i) * 1000);
            std::fs::write(dir.path().join(&name), &data).unwrap();
        }

        let janitor = Janitor::new(
            dir.path().to_str().unwrap(),
            Duration::from_secs(86400), // long retention so only cap kicks in
            200,                         // cap at 200 bytes
        );
        janitor.cleanup().await.unwrap();

        // Should have deleted the oldest file, keeping 2 (200 bytes).
        let remaining: Vec<_> = std::fs::read_dir(dir.path())
            .unwrap()
            .filter_map(|e| e.ok())
            .filter(|e| e.file_name().to_str().unwrap().ends_with(".vortex"))
            .collect();
        assert_eq!(remaining.len(), 2);
    }

    #[tokio::test]
    async fn test_cleanup_ignores_non_vortex() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("readme.txt"), "keep me").unwrap();

        let janitor = Janitor::new(
            dir.path().to_str().unwrap(),
            Duration::from_secs(0), // delete everything by time
            0,
        );
        janitor.cleanup().await.unwrap();

        // Non-vortex file should still exist.
        assert!(dir.path().join("readme.txt").exists());
    }
}
