//! In-memory disk usage tracker.
//!
//! Tracks the total bytes of signal files on disk without periodic filesystem
//! scans. The writer threads call [`file_closed`] when rotating Parquet files,
//! and the janitor calls [`delete_oldest`] when the cap is exceeded.
//!
//! On startup, the directory is scanned once to seed the tracker.

use std::collections::VecDeque;
use std::path::{Path, PathBuf};
use std::sync::Mutex;

use anyhow::{Context, Result};
use tracing::{info, warn};

/// Tracks disk usage of signal files with in-memory accounting.
///
/// Thread-safe: writer threads call `file_closed` concurrently, the janitor
/// calls `delete_oldest` periodically.
pub struct DiskTracker {
    max_bytes: u64,
    inner: Mutex<Inner>,
}

struct Inner {
    current_bytes: u64,
    /// Sorted oldest-first by timestamp (extracted from filename).
    files: VecDeque<TrackedFile>,
}

struct TrackedFile {
    path: PathBuf,
    size: u64,
}

impl DiskTracker {
    /// Create a new DiskTracker with the given disk cap.
    ///
    /// Scans `output_dir` once to seed the tracker with existing files.
    /// Logs current disk usage at startup.
    pub fn new(output_dir: &Path, max_bytes: u64) -> Result<Self> {
        let mut files: Vec<(u64, PathBuf, u64)> = Vec::new();
        let mut total_bytes: u64 = 0;
        let mut contexts_bytes: u64 = 0;

        if output_dir.exists() {
            for entry in std::fs::read_dir(output_dir)
                .with_context(|| format!("scanning {}", output_dir.display()))?
            {
                let entry = entry?;
                let path = entry.path();
                let name = path
                    .file_name()
                    .unwrap_or_default()
                    .to_string_lossy()
                    .to_string();

                let size = entry.metadata().map(|m| m.len()).unwrap_or(0);

                if name == "contexts.bin" {
                    contexts_bytes = size;
                    continue; // Don't track contexts.bin — it's not rotated.
                }

                if !name.ends_with(".parquet") {
                    continue;
                }

                // Extract timestamp from filename: prefix-{timestamp_ms}.parquet
                let ts = name
                    .rsplit('-')
                    .next()
                    .and_then(|s| s.strip_suffix(".parquet"))
                    .and_then(|s| s.parse::<u64>().ok())
                    .unwrap_or(0);

                total_bytes += size;
                files.push((ts, path, size));
            }
        }

        // Sort oldest first.
        files.sort_by_key(|&(ts, _, _)| ts);

        let file_count = files.len();
        let deque: VecDeque<TrackedFile> = files
            .into_iter()
            .map(|(_, path, size)| TrackedFile { path, size })
            .collect();

        info!(
            current_mb = total_bytes / (1024 * 1024),
            max_mb = max_bytes / (1024 * 1024),
            contexts_kb = contexts_bytes / 1024,
            parquet_files = file_count,
            "disk tracker initialized: {} MB / {} MB (contexts.bin: {} KB, {} parquet files)",
            total_bytes / (1024 * 1024),
            max_bytes / (1024 * 1024),
            contexts_bytes / 1024,
            file_count,
        );

        Ok(Self {
            max_bytes,
            inner: Mutex::new(Inner {
                current_bytes: total_bytes,
                files: deque,
            }),
        })
    }

    /// Create a no-op DiskTracker (unlimited cap, no files).
    /// Used in tests where disk tracking is not relevant.
    pub fn noop() -> Self {
        Self {
            max_bytes: u64::MAX,
            inner: Mutex::new(Inner {
                current_bytes: 0,
                files: VecDeque::new(),
            }),
        }
    }

    /// Record a newly closed Parquet file. Called by writer threads after
    /// file rotation.
    pub fn file_closed(&self, path: PathBuf, size: u64) {
        let mut inner = self.inner.lock().unwrap();
        inner.current_bytes += size;
        inner.files.push_back(TrackedFile { path, size });
    }

    /// Return current disk usage in bytes.
    pub fn current_bytes(&self) -> u64 {
        self.inner.lock().unwrap().current_bytes
    }

    /// Return the configured max disk usage in bytes.
    pub fn max_bytes(&self) -> u64 {
        self.max_bytes
    }

    /// Return the number of tracked files.
    pub fn file_count(&self) -> usize {
        self.inner.lock().unwrap().files.len()
    }

    /// Delete the oldest files until disk usage is within the cap.
    /// Returns the number of files deleted.
    ///
    /// Logs a summary line with current usage after cleanup.
    pub fn enforce_cap(&self) -> usize {
        let mut inner = self.inner.lock().unwrap();
        let mut deleted = 0;

        while inner.current_bytes > self.max_bytes {
            let oldest = match inner.files.pop_front() {
                Some(f) => f,
                None => break,
            };

            match std::fs::remove_file(&oldest.path) {
                Ok(()) => {
                    inner.current_bytes = inner.current_bytes.saturating_sub(oldest.size);
                    deleted += 1;
                }
                Err(e) => {
                    // File may already be gone (manual delete, another process).
                    // Don't put it back — just adjust the counter.
                    warn!(
                        path = %oldest.path.display(),
                        "failed to delete signal file: {e}"
                    );
                    inner.current_bytes = inner.current_bytes.saturating_sub(oldest.size);
                }
            }
        }

        if deleted > 0 {
            info!(
                current_mb = inner.current_bytes / (1024 * 1024),
                max_mb = self.max_bytes / (1024 * 1024),
                files = inner.files.len(),
                deleted,
                "disk cleanup: {} MB / {} MB ({} files, {} deleted)",
                inner.current_bytes / (1024 * 1024),
                self.max_bytes / (1024 * 1024),
                inner.files.len(),
                deleted,
            );
        }

        deleted
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_startup_scan() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("metrics-1000.parquet"), vec![0u8; 100]).unwrap();
        std::fs::write(dir.path().join("metrics-2000.parquet"), vec![0u8; 200]).unwrap();
        std::fs::write(dir.path().join("contexts.bin"), vec![0u8; 50]).unwrap();

        let tracker = DiskTracker::new(dir.path(), 1024 * 1024).unwrap();
        assert_eq!(tracker.current_bytes(), 300); // 100 + 200, not 50 (contexts.bin excluded)
        assert_eq!(tracker.file_count(), 2);
    }

    #[test]
    fn test_file_closed() {
        let dir = tempdir().unwrap();
        let tracker = DiskTracker::new(dir.path(), 1024 * 1024).unwrap();
        assert_eq!(tracker.current_bytes(), 0);

        tracker.file_closed(dir.path().join("test-1.parquet"), 500);
        assert_eq!(tracker.current_bytes(), 500);

        tracker.file_closed(dir.path().join("test-2.parquet"), 300);
        assert_eq!(tracker.current_bytes(), 800);
    }

    #[test]
    fn test_enforce_cap() {
        let dir = tempdir().unwrap();
        // Create 3 files, 100 bytes each.
        for i in 1..=3 {
            let name = format!("metrics-{}.parquet", i * 1000);
            std::fs::write(dir.path().join(&name), vec![0u8; 100]).unwrap();
        }

        // Cap at 200 bytes — should delete the oldest file.
        let tracker = DiskTracker::new(dir.path(), 200).unwrap();
        assert_eq!(tracker.current_bytes(), 300);

        let deleted = tracker.enforce_cap();
        assert_eq!(deleted, 1);
        assert_eq!(tracker.current_bytes(), 200);
        assert_eq!(tracker.file_count(), 2);
        assert!(!dir.path().join("metrics-1000.parquet").exists());
        assert!(dir.path().join("metrics-2000.parquet").exists());
        assert!(dir.path().join("metrics-3000.parquet").exists());
    }

    #[test]
    fn test_empty_dir() {
        let dir = tempdir().unwrap();
        let tracker = DiskTracker::new(dir.path(), 1024).unwrap();
        assert_eq!(tracker.current_bytes(), 0);
        assert_eq!(tracker.file_count(), 0);
        assert_eq!(tracker.enforce_cap(), 0);
    }
}
