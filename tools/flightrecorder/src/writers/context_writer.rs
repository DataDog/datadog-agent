//! Dedicated context writer thread with lock-free queues.
//!
//! Owns the shared `ContextStore` (bloom filter + `contexts.bin`). Both the
//! metrics and logs writer threads forward context records here via separate
//! rtrb SPSC rings. The thread drains both rings and appends to `contexts.bin`.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use tracing::warn;

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
    ) -> (Self, ContextProducer, ContextProducer) {
        let (prod_m, cons_m) = rtrb::RingBuffer::new(capacity);
        let (prod_l, cons_l) = rtrb::RingBuffer::new(capacity);
        let shutdown = Arc::new(AtomicBool::new(false));

        let s = shutdown.clone();
        let join = std::thread::Builder::new()
            .name("fr-contexts".into())
            .spawn(move || context_thread_loop(store, cons_m, cons_l, s))
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

fn context_thread_loop(
    mut store: ContextStore,
    mut cons_m: rtrb::Consumer<ContextRecord>,
    mut cons_l: rtrb::Consumer<ContextRecord>,
    shutdown: Arc<AtomicBool>,
) {
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

        if shutdown.load(Ordering::Acquire) {
            // Final drain after shutdown signal.
            while let Ok(rec) = cons_m.pop() {
                let _ = store.try_record(rec.key, &rec.name, &rec.tags_joined);
            }
            while let Ok(rec) = cons_l.pop() {
                let _ = store.try_record(rec.key, &rec.name, &rec.tags_joined);
            }
            let _ = store.flush();
            break;
        }

        std::thread::park();
    }
}
