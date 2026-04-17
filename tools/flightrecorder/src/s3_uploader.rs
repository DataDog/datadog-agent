//! Async S3 upload task for Parquet signal files.
//!
//! Uses the `object_store` crate for lightweight S3 uploads without the full
//! AWS SDK. Receives upload requests via a bounded mpsc channel and uploads
//! files with retry. After successful upload, the local file is deleted and
//! the DiskTracker is notified.

#![cfg(feature = "s3")]

use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use anyhow::Result;
use object_store::aws::AmazonS3Builder;
use object_store::path::Path as ObjectPath;
use object_store::{ObjectStore, ObjectStoreExt};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tracing::{error, info, warn};

use crate::disk_tracker::DiskTracker;

const MAX_RETRIES: u32 = 3;
const BASE_RETRY_DELAY: Duration = Duration::from_secs(1);
const CHANNEL_CAPACITY: usize = 64;

/// Request to upload a file to S3.
pub struct UploadRequest {
    pub path: PathBuf,
    pub size: u64,
}

/// Cloneable handle for sending upload requests from any thread.
#[derive(Clone)]
pub struct S3UploadHandle {
    tx: mpsc::Sender<UploadRequest>,
}

impl S3UploadHandle {
    /// Try to enqueue an upload request. Non-blocking: returns immediately
    /// if the channel is full (file stays on disk, janitor handles cleanup).
    pub fn try_send(&self, request: UploadRequest) {
        match self.tx.try_send(request) {
            Ok(()) => {}
            Err(mpsc::error::TrySendError::Full(req)) => {
                warn!(path = %req.path.display(), "S3 upload queue full, skipping");
            }
            Err(mpsc::error::TrySendError::Closed(req)) => {
                warn!(path = %req.path.display(), "S3 uploader shut down, skipping");
            }
        }
    }
}

/// Async S3 upload task.
pub struct S3Uploader {
    rx: mpsc::Receiver<UploadRequest>,
    store: Arc<dyn ObjectStore>,
    key_prefix: String,
    tracker: Arc<DiskTracker>,
}

/// Create a new S3 uploader and its send handle.
///
/// Uses the `object_store` AmazonS3Builder which reads credentials from the
/// standard AWS credential chain (env vars, instance profile, shared credentials).
pub fn new_s3_uploader(
    bucket: String,
    region: String,
    key_prefix: String,
    tracker: Arc<DiskTracker>,
) -> Result<(S3Uploader, S3UploadHandle)> {
    let store = AmazonS3Builder::from_env()
        .with_bucket_name(&bucket)
        .with_region(&region)
        .build()
        .map_err(|e| anyhow::anyhow!("failed to create S3 client: {e}"))?;

    let (tx, rx) = mpsc::channel(CHANNEL_CAPACITY);

    let uploader = S3Uploader {
        rx,
        store: Arc::new(store),
        key_prefix,
        tracker,
    };
    let handle = S3UploadHandle { tx };

    Ok((uploader, handle))
}

impl S3Uploader {
    /// Run the upload loop until cancelled.
    pub async fn run(mut self, cancel: CancellationToken) {
        info!(key_prefix = %self.key_prefix, "S3 uploader started");

        loop {
            let request = tokio::select! {
                _ = cancel.cancelled() => {
                    while let Ok(req) = self.rx.try_recv() {
                        self.upload_file(req).await;
                    }
                    info!("S3 uploader shutting down");
                    return;
                }
                req = self.rx.recv() => {
                    match req {
                        Some(r) => r,
                        None => {
                            info!("S3 upload channel closed");
                            return;
                        }
                    }
                }
            };

            self.upload_file(request).await;
        }
    }

    async fn upload_file(&self, request: UploadRequest) {
        let filename = match request.path.file_name() {
            Some(f) => f.to_string_lossy().into_owned(),
            None => {
                warn!(path = %request.path.display(), "upload request has no filename");
                return;
            }
        };

        let key = format!("{}{}", self.key_prefix, filename);
        let object_path = ObjectPath::from(key.as_str());

        for attempt in 0..MAX_RETRIES {
            match self.try_upload(&request.path, &object_path).await {
                Ok(()) => {
                    info!(
                        path = %request.path.display(),
                        key = %key,
                        size_mb = request.size / (1024 * 1024),
                        "S3 upload succeeded"
                    );
                    if let Err(e) = std::fs::remove_file(&request.path) {
                        warn!(
                            path = %request.path.display(),
                            "failed to delete local file after S3 upload: {e}"
                        );
                    }
                    self.tracker.file_uploaded(&request.path, request.size);
                    return;
                }
                Err(e) => {
                    let delay = BASE_RETRY_DELAY * 2u32.pow(attempt);
                    warn!(
                        path = %request.path.display(),
                        key = %key,
                        attempt = attempt + 1,
                        max_retries = MAX_RETRIES,
                        "S3 upload failed (retry in {delay:?}): {e}",
                    );
                    tokio::time::sleep(delay).await;
                }
            }
        }

        error!(
            path = %request.path.display(),
            key = %key,
            "S3 upload failed after {MAX_RETRIES} attempts, file stays on disk"
        );
    }

    async fn try_upload(&self, local_path: &std::path::Path, key: &ObjectPath) -> Result<()> {
        let data = tokio::fs::read(local_path)
            .await
            .map_err(|e| anyhow::anyhow!("reading file for S3 upload: {e}"))?;

        self.store
            .put(key, data.into())
            .await
            .map_err(|e| anyhow::anyhow!("S3 put: {e}"))?;

        Ok(())
    }
}
