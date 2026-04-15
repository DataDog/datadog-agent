//! Dedicated writer thread harness using an rtrb SPSC ring buffer.
//!
//! Each signal type (metrics, logs, trace stats) gets its own OS thread.
//! Async connection handlers push raw FlatBuffers frames into the ring
//! (~5-10ns per push), and the writer thread decodes + accumulates + flushes
//! to Parquet without involving the Tokio runtime.

use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;

use anyhow::Result;
use tracing::warn;

/// Telemetry counters shared between the writer thread and the telemetry
/// reporter. All fields are atomics — no locking required.
pub struct WriterTelemetry {
    pub buffered_rows: AtomicU64,
    pub flush_count: AtomicU64,
    pub flush_bytes: AtomicU64,
    pub rows_written: AtomicU64,
    pub last_flush_duration_ns: AtomicU64,
}

impl WriterTelemetry {
    pub fn new() -> Self {
        Self {
            buffered_rows: AtomicU64::new(0),
            flush_count: AtomicU64::new(0),
            flush_bytes: AtomicU64::new(0),
            rows_written: AtomicU64::new(0),
            last_flush_duration_ns: AtomicU64::new(0),
        }
    }

    /// Snapshot counters from a [`WriterStats`] source.
    pub fn update(&self, stats: &WriterStats) {
        self.buffered_rows.store(stats.buffered_rows, Ordering::Relaxed);
        self.flush_count.store(stats.flush_count, Ordering::Relaxed);
        self.flush_bytes.store(stats.flush_bytes, Ordering::Relaxed);
        self.rows_written.store(stats.rows_written, Ordering::Relaxed);
        self.last_flush_duration_ns
            .store(stats.last_flush_duration_ns, Ordering::Relaxed);
    }
}

/// Plain counter snapshot produced by a [`SignalWriter`].
pub struct WriterStats {
    pub buffered_rows: u64,
    pub flush_count: u64,
    pub flush_bytes: u64,
    pub rows_written: u64,
    pub last_flush_duration_ns: u64,
}

/// Trait implemented by each signal writer (metrics, logs, trace stats).
///
/// All methods are synchronous — the writer lives on a dedicated OS thread
/// and never touches the Tokio runtime.
pub trait SignalWriter: Send + 'static {
    /// Decode a raw FlatBuffers frame and accumulate rows. May trigger a
    /// Parquet flush if thresholds are reached.
    ///
    /// Takes ownership of the frame buffer so implementations can retain it
    /// (e.g. arena-based zero-copy accumulation in the logs writer).
    fn process_frame(&mut self, buf: Vec<u8>) -> Result<()>;

    /// Flush any buffered rows and close the active Parquet file.
    fn flush_and_close(&mut self) -> Result<()>;

    /// Current counter snapshot for telemetry.
    fn stats(&self) -> WriterStats;
}

/// Handle to a running writer thread.
///
/// Owns the producer side of the rtrb ring buffer. Frames are pushed with
/// [`send_frame`] (~5-10ns) and the writer thread is woken via
/// [`std::thread::Thread::unpark`].
pub struct WriterHandle {
    producer: rtrb::Producer<Vec<u8>>,
    thread_handle: std::thread::Thread,
    join: Option<std::thread::JoinHandle<()>>,
    shutdown: Arc<AtomicBool>,
    /// Shared telemetry counters (lock-free reads from the telemetry reporter).
    pub telemetry: Arc<WriterTelemetry>,
}

impl WriterHandle {
    /// Spawn a new writer thread with the given ring capacity.
    pub fn spawn<W: SignalWriter>(writer: W, capacity: usize, name: &str) -> Self {
        let (producer, consumer) = rtrb::RingBuffer::new(capacity);
        let telemetry = Arc::new(WriterTelemetry::new());
        let shutdown = Arc::new(AtomicBool::new(false));

        let t = telemetry.clone();
        let s = shutdown.clone();
        let join = std::thread::Builder::new()
            .name(format!("fr-{name}"))
            .spawn(move || writer_thread_loop(writer, consumer, t, s))
            .expect("failed to spawn writer thread");
        let thread_handle = join.thread().clone();

        Self {
            producer,
            thread_handle,
            join: Some(join),
            shutdown,
            telemetry,
        }
    }

    /// Push a frame to the writer thread. Typically completes in ~5-10ns.
    ///
    /// If the ring is full (extremely unlikely with a 512-slot ring and
    /// sub-millisecond consumer), falls back to a spin loop wrapped in
    /// `block_in_place` so the Tokio runtime can schedule other tasks.
    pub fn send_frame(&mut self, buf: Vec<u8>) {
        match self.producer.push(buf) {
            Ok(()) => {
                self.thread_handle.unpark();
            }
            Err(rtrb::PushError::Full(mut item)) => {
                // Ring full — spin until space. This path should be extremely
                // rare; log once so we can tune the ring capacity.
                tokio::task::block_in_place(|| loop {
                    match self.producer.push(item) {
                        Ok(()) => {
                            self.thread_handle.unpark();
                            return;
                        }
                        Err(rtrb::PushError::Full(returned)) => {
                            item = returned;
                            std::hint::spin_loop();
                        }
                    }
                });
            }
        }
    }

    /// Signal the writer thread to shut down and wait for it to finish.
    ///
    /// The writer thread drains any remaining frames in the ring, flushes
    /// buffered rows to Parquet, and closes the active file.
    pub fn shutdown(&mut self) {
        self.shutdown.store(true, Ordering::Release);
        self.thread_handle.unpark();
        if let Some(h) = self.join.take() {
            let _ = h.join();
        }
    }
}

impl Drop for WriterHandle {
    fn drop(&mut self) {
        if self.join.is_some() {
            self.shutdown();
        }
    }
}

fn writer_thread_loop<W: SignalWriter>(
    mut writer: W,
    mut consumer: rtrb::Consumer<Vec<u8>>,
    telemetry: Arc<WriterTelemetry>,
    shutdown: Arc<AtomicBool>,
) {
    loop {
        // Drain all available frames without blocking.
        let mut processed = false;
        while let Ok(buf) = consumer.pop() {
            if let Err(e) = writer.process_frame(buf) {
                warn!("writer error: {e}");
            }
            processed = true;
        }

        if processed {
            telemetry.update(&writer.stats());
        }

        // Check shutdown AFTER draining — ensures all in-flight frames are
        // processed before the final flush.
        if shutdown.load(Ordering::Acquire) {
            break;
        }

        // Ring empty — park until the producer pushes a frame and calls unpark.
        std::thread::park();
    }

    // Final flush.
    telemetry.update(&writer.stats());
    if let Err(e) = writer.flush_and_close() {
        warn!("final flush error: {e}");
    }
    telemetry.update(&writer.stats());
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::AtomicUsize;

    struct MockWriter {
        frames_processed: Arc<AtomicUsize>,
        buffered: usize,
    }

    impl SignalWriter for MockWriter {
        fn process_frame(&mut self, _buf: Vec<u8>) -> Result<()> {
            self.frames_processed.fetch_add(1, Ordering::Relaxed);
            self.buffered += 1;
            Ok(())
        }

        fn flush_and_close(&mut self) -> Result<()> {
            self.buffered = 0;
            Ok(())
        }

        fn stats(&self) -> WriterStats {
            WriterStats {
                buffered_rows: self.buffered as u64,
                flush_count: 0,
                flush_bytes: 0,
                rows_written: self.frames_processed.load(Ordering::Relaxed) as u64,
                last_flush_duration_ns: 0,
            }
        }
    }

    #[test]
    fn test_send_and_shutdown() {
        let counter = Arc::new(AtomicUsize::new(0));
        let writer = MockWriter {
            frames_processed: counter.clone(),
            buffered: 0,
        };
        let mut handle = WriterHandle::spawn(writer, 64, "test");

        for i in 0..100 {
            handle.send_frame(vec![i as u8; 10]);
        }

        handle.shutdown();
        assert_eq!(counter.load(Ordering::Relaxed), 100);
    }

    #[test]
    fn test_telemetry_updated() {
        let counter = Arc::new(AtomicUsize::new(0));
        let writer = MockWriter {
            frames_processed: counter.clone(),
            buffered: 0,
        };
        let mut handle = WriterHandle::spawn(writer, 64, "test-telem");

        for _ in 0..10 {
            handle.send_frame(vec![0; 4]);
        }

        handle.shutdown();
        assert_eq!(
            handle.telemetry.rows_written.load(Ordering::Relaxed),
            10
        );
    }

    #[test]
    fn test_drop_triggers_shutdown() {
        let counter = Arc::new(AtomicUsize::new(0));
        let writer = MockWriter {
            frames_processed: counter.clone(),
            buffered: 0,
        };
        let mut handle = WriterHandle::spawn(writer, 64, "test-drop");
        handle.send_frame(vec![1, 2, 3]);
        drop(handle);
        assert_eq!(counter.load(Ordering::Relaxed), 1);
    }
}
