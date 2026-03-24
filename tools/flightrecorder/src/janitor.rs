use std::path::PathBuf;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use tokio_util::sync::CancellationToken;
use tracing::{debug, info, warn};

use crate::merge;
use crate::vortex_files;

/// Periodically cleans up old `.vortex` files based on time-based retention and
/// an optional disk usage cap. Also runs periodic merge/compaction passes.
pub struct Janitor {
    output_dir: PathBuf,
    retention: Duration,
    max_disk_bytes: u64, // 0 = unlimited
    interval: Duration,
    merge_enabled: bool,
    merge_config: merge::MergeConfig,
    merge_interval: Duration,
}

impl Janitor {
    pub fn new(
        output_dir: &str,
        retention: Duration,
        max_disk_bytes: u64,
        merge_enabled: bool,
        merge_min_files: usize,
        merge_interval: Duration,
    ) -> Self {
        let output_dir = PathBuf::from(output_dir);
        Self {
            merge_config: merge::MergeConfig {
                output_dir: output_dir.clone(),
                min_files_to_trigger: merge_min_files,
            },
            output_dir,
            retention,
            max_disk_bytes,
            interval: Duration::from_secs(30),
            merge_enabled,
            merge_interval,
        }
    }

    pub async fn run(self, cancel: CancellationToken) {
        info!(
            output_dir = %self.output_dir.display(),
            retention_secs = self.retention.as_secs(),
            max_disk_bytes = self.max_disk_bytes,
            interval_secs = self.interval.as_secs(),
            merge_enabled = self.merge_enabled,
            merge_interval_secs = self.merge_interval.as_secs(),
            merge_min_files = self.merge_config.min_files_to_trigger,
            "janitor started"
        );
        let mut last_merge = Instant::now();
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
                    // Run merge pass if enabled and enough time has elapsed.
                    if self.merge_enabled && last_merge.elapsed() >= self.merge_interval {
                        crate::heap_prof::dump_heap_profile(&self.output_dir, "pre-merge");
                        match merge::merge_pass(&self.merge_config).await {
                            Ok(n) if n > 0 => {
                                info!(merged_files = n, "merge pass complete");
                            }
                            Ok(_) => {}
                            Err(e) => {
                                warn!("merge pass error: {}", e);
                            }
                        }
                        crate::heap_prof::dump_heap_profile(&self.output_dir, "post-merge");
                        last_merge = Instant::now();
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

        // Clean up leftover .tmp files from interrupted merges.
        self.cleanup_tmp_files().await;

        // Collect all .vortex files with their parsed timestamps and sizes.
        let mut entries = vortex_files::scan_vortex_files(&self.output_dir).await?;

        // Sort oldest first.
        entries.sort_by_key(|e| e.timestamp_ms);

        // Pass 1: time-based retention.
        let mut deleted_time = 0u64;
        let mut remaining: Vec<vortex_files::VortexEntry> = Vec::with_capacity(entries.len());
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

    /// Remove any `.vortex.tmp` files left over from interrupted merge passes.
    async fn cleanup_tmp_files(&self) {
        let mut dir = match tokio::fs::read_dir(&self.output_dir).await {
            Ok(d) => d,
            Err(_) => return,
        };
        while let Ok(Some(entry)) = dir.next_entry().await {
            let name = match entry.file_name().into_string() {
                Ok(n) => n,
                Err(_) => continue,
            };
            if name.ends_with(".vortex.tmp") {
                if let Err(e) = tokio::fs::remove_file(entry.path()).await {
                    warn!(path = %entry.path().display(), "failed to delete tmp file: {}", e);
                } else {
                    info!(path = %entry.path().display(), "deleted leftover tmp file");
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

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
            false,
            5,
            Duration::from_secs(300),
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
            false,
            5,
            Duration::from_secs(300),
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
            false,
            5,
            Duration::from_secs(300),
        );
        janitor.cleanup().await.unwrap();

        // Non-vortex file should still exist.
        assert!(dir.path().join("readme.txt").exists());
    }

    #[tokio::test]
    async fn test_cleanup_removes_tmp_files() {
        let dir = tempfile::tempdir().unwrap();
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        let recent_name = format!("metrics-{}.vortex", now_ms);
        std::fs::write(dir.path().join("metrics-123.vortex.tmp"), "leftover").unwrap();
        std::fs::write(dir.path().join(&recent_name), "keep").unwrap();

        let janitor = Janitor::new(
            dir.path().to_str().unwrap(),
            Duration::from_secs(86400),
            0,
            false,
            5,
            Duration::from_secs(300),
        );
        janitor.cleanup().await.unwrap();

        assert!(!dir.path().join("metrics-123.vortex.tmp").exists());
        assert!(dir.path().join(&recent_name).exists());
    }
}
