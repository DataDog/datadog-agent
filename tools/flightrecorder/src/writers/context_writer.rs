//! Dedicated context writer thread with lock-free queues.
//!
//! Owns the shared `ContextStore` (bloom filter + `contexts.bin`). Both the
//! metrics and logs writer threads forward context records here via separate
//! rtrb SPSC rings. The thread drains both rings and appends to `contexts.bin`.
//!
//! When S3 upload is enabled, periodically converts new contexts to Parquet
//! and sends them to the S3 uploader for incremental upload.

use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use tracing::{info, warn};

use super::context_store::ContextStore;

/// A context record to be persisted. Sent from signal writer threads.
pub struct ContextRecord {
    pub key: u64,
    pub name: String,
    pub tags_joined: String,
}

/// Producer handle for sending context records to the writer thread.
/// Each signal writer thread owns one of these.
pub struct ContextProducer {
    producer: rtrb::Producer<ContextRecord>,
    thread_handle: std::thread::Thread,
}

impl ContextProducer {
    /// Try to send a context record. Drops silently if the ring is full
    /// (harmless: the Go-side bloom filter will re-send on false-positive miss).
    pub fn try_send(&mut self, record: ContextRecord) {
        match self.producer.push(record) {
            Ok(()) => self.thread_handle.unpark(),
            Err(_) => {
                // Ring full — context already sent by Go side, will be retried.
            }
        }
    }
}

/// Optional S3 upload configuration for incremental context uploads.
pub struct ContextUploadConfig {
    #[cfg(feature = "s3")]
    pub upload_handle: crate::s3_uploader::S3UploadHandle,
    pub output_dir: PathBuf,
}

/// Handle for the context writer thread. Holds the join handle and shutdown flag.
pub struct ContextWriterHandle {
    join: Option<std::thread::JoinHandle<()>>,
    shutdown: Arc<AtomicBool>,
    thread_handle: std::thread::Thread,
}

impl ContextWriterHandle {
    /// Spawn the context writer thread with two SPSC rings (one per producer).
    /// Returns the handle and two producers (for metrics and logs writer threads).
    pub fn spawn(
        store: ContextStore,
        capacity: usize,
        upload_config: Option<ContextUploadConfig>,
    ) -> (Self, ContextProducer, ContextProducer) {
        let (prod_m, cons_m) = rtrb::RingBuffer::new(capacity);
        let (prod_l, cons_l) = rtrb::RingBuffer::new(capacity);
        let shutdown = Arc::new(AtomicBool::new(false));

        let s = shutdown.clone();
        let join = std::thread::Builder::new()
            .name("fr-contexts".into())
            .spawn(move || context_thread_loop(store, cons_m, cons_l, s, upload_config))
            .expect("failed to spawn context writer thread");
        let thread_handle = join.thread().clone();

        let handle = Self {
            join: Some(join),
            shutdown,
            thread_handle: thread_handle.clone(),
        };
        let prod_metrics = ContextProducer {
            producer: prod_m,
            thread_handle: thread_handle.clone(),
        };
        let prod_logs = ContextProducer {
            producer: prod_l,
            thread_handle,
        };

        (handle, prod_metrics, prod_logs)
    }

    /// Shut down the context writer thread. Call after signal writers are stopped
    /// to ensure all remaining context records are drained.
    pub fn shutdown(&mut self) {
        self.shutdown.store(true, Ordering::Release);
        self.thread_handle.unpark();
        if let Some(h) = self.join.take() {
            let _ = h.join();
        }
    }
}

/// Interval between incremental context uploads to S3.
const CONTEXT_UPLOAD_INTERVAL: Duration = Duration::from_secs(30);
/// Minimum new contexts before triggering an upload (avoid tiny files).
const MIN_NEW_CONTEXTS_FOR_UPLOAD: usize = 10;

fn context_thread_loop(
    mut store: ContextStore,
    mut cons_m: rtrb::Consumer<ContextRecord>,
    mut cons_l: rtrb::Consumer<ContextRecord>,
    shutdown: Arc<AtomicBool>,
    upload_config: Option<ContextUploadConfig>,
) {
    let has_upload = upload_config.is_some();
    let mut last_upload_offset: u64 = 0;
    let mut last_upload_time = Instant::now();

    loop {
        let mut processed = false;

        // Drain both rings.
        while let Ok(rec) = cons_m.pop() {
            let _ = store.try_record(rec.key, &rec.name, &rec.tags_joined);
            processed = true;
        }
        while let Ok(rec) = cons_l.pop() {
            let _ = store.try_record(rec.key, &rec.name, &rec.tags_joined);
            processed = true;
        }

        if processed {
            if let Err(e) = store.flush() {
                warn!("context store flush error: {e}");
            }
        }

        // Periodic incremental context upload to S3.
        if let Some(ref cfg) = upload_config {
            if last_upload_time.elapsed() >= CONTEXT_UPLOAD_INTERVAL {
                upload_new_contexts(&store, cfg, &mut last_upload_offset);
                last_upload_time = Instant::now();
            }
        }

        if shutdown.load(Ordering::Acquire) {
            // Final drain after shutdown signal.
            while let Ok(rec) = cons_m.pop() {
                let _ = store.try_record(rec.key, &rec.name, &rec.tags_joined);
            }
            while let Ok(rec) = cons_l.pop() {
                let _ = store.try_record(rec.key, &rec.name, &rec.tags_joined);
            }
            let _ = store.flush();

            // Final context upload.
            if let Some(ref cfg) = upload_config {
                upload_new_contexts(&store, cfg, &mut last_upload_offset);
            }
            break;
        }

        // Park with timeout when S3 is enabled (for periodic uploads),
        // otherwise park indefinitely (wake only on new records).
        if has_upload {
            std::thread::park_timeout(CONTEXT_UPLOAD_INTERVAL);
        } else {
            std::thread::park();
        }
    }
}

fn upload_new_contexts(
    store: &ContextStore,
    cfg: &ContextUploadConfig,
    last_offset: &mut u64,
) {
    let (new_contexts, new_offset) = match super::context_store::read_contexts_bin_from(
        store.path(),
        *last_offset,
    ) {
        Ok(r) => r,
        Err(e) => {
            warn!("failed to read new contexts for S3 upload: {e}");
            return;
        }
    };

    if new_contexts.len() < MIN_NEW_CONTEXTS_FOR_UPLOAD {
        return;
    }

    let ts_ms = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis();
    let parquet_path = cfg.output_dir.join(format!("contexts-{ts_ms}.parquet"));

    if let Err(e) = crate::context_parquet::write_contexts_parquet(&new_contexts, &parquet_path) {
        warn!("failed to write context Parquet for S3: {e}");
        return;
    }

    let size = std::fs::metadata(&parquet_path)
        .map(|m| m.len())
        .unwrap_or(0);

    #[cfg(feature = "s3")]
    cfg.upload_handle.try_send(crate::s3_uploader::UploadRequest {
        path: parquet_path.clone(),
        size,
    });

    // Without S3 feature, the context Parquet stays on disk (useful for local testing).
    #[cfg(not(feature = "s3"))]
    let _ = (&parquet_path, size);

    *last_offset = new_offset;
    info!(
        new_contexts = new_contexts.len(),
        offset = new_offset,
        "incremental context Parquet ready for upload"
    );
}
