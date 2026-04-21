//! Periodic disk cleanup using the in-memory DiskTracker.
//!
//! Runs every second. Calls `tracker.enforce_cap()` which deletes the
//! oldest signal files until disk usage is within the configured cap.
//! No filesystem scan needed — the tracker maintains an in-memory accounting
//! of all signal files.

use std::sync::Arc;
use std::time::Duration;

use tokio_util::sync::CancellationToken;
use tracing::info;

use crate::disk_tracker::DiskTracker;

/// Periodically enforces the disk cap by deleting the oldest signal files.
pub struct Janitor {
    tracker: Arc<DiskTracker>,
    interval: Duration,
}

impl Janitor {
    pub fn new(tracker: Arc<DiskTracker>) -> Self {
        Self {
            tracker,
            interval: Duration::from_secs(1),
        }
    }

    pub async fn run(self, cancel: CancellationToken) {
        info!(
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
                    self.tracker.enforce_cap();
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_janitor_enforces_cap() {
        let dir = tempdir().unwrap();
        for i in 1..=5 {
            let name = format!("metrics-{}.parquet", i * 1000);
            std::fs::write(dir.path().join(&name), vec![0u8; 100]).unwrap();
        }

        let tracker = Arc::new(DiskTracker::new(dir.path(), 300).unwrap());
        assert_eq!(tracker.current_bytes(), 500);

        let janitor = Janitor::new(tracker.clone());
        let cancel = CancellationToken::new();
        let cancel2 = cancel.clone();

        let handle = tokio::spawn(async move {
            janitor.run(cancel2).await;
        });

        // Wait for one cleanup cycle.
        tokio::time::sleep(Duration::from_millis(100)).await;
        cancel.cancel();
        handle.await.unwrap();

        // Should have deleted 2 oldest files (500 → 300).
        assert_eq!(tracker.current_bytes(), 300);
        assert_eq!(tracker.file_count(), 3);
    }
}
