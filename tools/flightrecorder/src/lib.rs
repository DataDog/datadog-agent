pub mod context_parquet;
pub mod disk_tracker;
pub mod framing;
pub mod generated;
pub mod heap_prof;
pub mod signal_files;
pub mod telemetry;
pub mod writers;

use std::sync::{Arc, Mutex};

/// Shared pool of reusable frame buffers. Eliminates per-frame heap allocation
/// on the hot path by recycling Vec<u8> buffers between the connection handler
/// (which fills them) and writer threads (which drain and return them).
#[derive(Clone)]
pub struct BufferPool(Arc<Mutex<Vec<Vec<u8>>>>);

impl BufferPool {
    pub fn new() -> Self {
        Self(Arc::new(Mutex::new(Vec::new())))
    }

    /// Take a buffer from the pool, or return an empty Vec if the pool is empty.
    pub fn take(&self) -> Vec<u8> {
        self.0.lock().unwrap().pop().unwrap_or_default()
    }

    /// Return a buffer to the pool for reuse. Clears content but keeps capacity.
    pub fn put(&self, mut buf: Vec<u8>) {
        buf.clear();
        self.0.lock().unwrap().push(buf);
    }
}
