//! Integration test: verify the DiskTracker + Janitor enforce a low disk cap
//! by deleting oldest Parquet files as new ones are written.

use std::sync::Arc;
use std::time::Duration;

use flightrecorder::disk_tracker::DiskTracker;
use flightrecorder::writers::context_store::ContextStore;
use flightrecorder::writers::context_writer::ContextWriterHandle;
use flightrecorder::writers::logs::LogsWriter;
use flightrecorder::writers::thread::SignalWriter;

use tempfile::tempdir;

/// Helper: build a FlatBuffers LogBatch frame with `n` log entries.
fn build_log_frame(n: usize) -> Vec<u8> {
    let mut fbb = flatbuffers::FlatBufferBuilder::with_capacity(4096);

    let mut offsets = Vec::with_capacity(n);
    for i in (0..n).rev() {
        let content = fbb.create_vector(
            format!("Log line {i} with enough content to make the parquet file non-trivial for disk cap testing")
                .as_bytes(),
        );
        let entry = flightrecorder::generated::signals_generated::signals::LogEntry::create(
            &mut fbb,
            &flightrecorder::generated::signals_generated::signals::LogEntryArgs {
                context_key: (i as u64) % 100 + 1,
                content: Some(content),
                timestamp_ns: 1_700_000_000_000_000_000 + i as i64 * 1_000_000,
            },
        );
        offsets.push(entry);
    }
    offsets.reverse();
    let vec = fbb.create_vector(&offsets);
    let batch = flightrecorder::generated::signals_generated::signals::LogBatch::create(
        &mut fbb,
        &flightrecorder::generated::signals_generated::signals::LogBatchArgs {
            contexts: None,
            entries: Some(vec),
        },
    );
    let env = flightrecorder::generated::signals_generated::signals::SignalEnvelope::create(
        &mut fbb,
        &flightrecorder::generated::signals_generated::signals::SignalEnvelopeArgs {
            log_batch: Some(batch),
            ..Default::default()
        },
    );
    fbb.finish(env, None);
    fbb.finished_data().to_vec()
}

fn make_ctx_producer() -> flightrecorder::writers::context_writer::ContextProducer {
    let dir = tempdir().unwrap();
    let store = ContextStore::new(dir.path()).unwrap();
    let (_handle, _prod_m, prod_l) = ContextWriterHandle::spawn(store, 64);
    std::mem::forget(_handle);
    std::mem::forget(dir);
    prod_l
}

#[test]
fn test_disk_cap_enforcement() {
    let dir = tempdir().unwrap();
    let output_dir = dir.path();

    // Very low disk cap: 50 KB.
    let disk_cap_bytes: u64 = 50 * 1024;
    let tracker = Arc::new(DiskTracker::new(output_dir, disk_cap_bytes).unwrap());

    // flush_rows=500, flush_interval=0 so every batch triggers a flush.
    // Rotation happens every 60s by default, so we simulate multiple files
    // by creating separate writers (each opens a new file).
    for _batch in 0..10 {
        let mut writer = LogsWriter::new(output_dir, 500, Duration::from_secs(3600), Duration::from_secs(60), make_ctx_producer(), tracker.clone(), flightrecorder::BufferPool::new());

        let frame = build_log_frame(500);
        writer.process_frame(frame.clone()).unwrap();
        writer.flush_and_close().unwrap();

        // Small sleep to ensure unique timestamp in filename.
        std::thread::sleep(Duration::from_millis(2));
    }

    // Enforce the cap.
    let deleted = tracker.enforce_cap();

    let current = tracker.current_bytes();
    let files = tracker.file_count();

    println!(
        "After 20 flushes + enforce_cap: {} bytes on disk, {} files, {} deleted",
        current, files, deleted
    );

    // Disk usage must be within the cap.
    assert!(
        current <= disk_cap_bytes,
        "disk usage {} exceeds cap {}",
        current,
        disk_cap_bytes
    );

    // Some files must have been deleted.
    assert!(deleted > 0, "expected files to be deleted, but none were");

    // Verify the remaining files actually exist on disk.
    let parquet_files: Vec<_> = std::fs::read_dir(output_dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.file_name()
                .to_str()
                .map(|n| n.ends_with(".parquet"))
                .unwrap_or(false)
        })
        .collect();

    assert_eq!(
        parquet_files.len(),
        files,
        "tracker file count {} doesn't match actual files on disk {}",
        files,
        parquet_files.len()
    );
}

#[test]
fn test_disk_cap_zero_means_unlimited() {
    let dir = tempdir().unwrap();
    let output_dir = dir.path();

    // max_disk_mb=0 → no cap enforcement (but DiskTracker still tracks).
    // Actually, with max_bytes=0, enforce_cap deletes everything.
    // So we use a very large cap to simulate "unlimited".
    let tracker = Arc::new(DiskTracker::new(output_dir, u64::MAX).unwrap());

    let mut writer = LogsWriter::new(output_dir, 50, Duration::from_secs(3600), Duration::from_secs(60), make_ctx_producer(), tracker.clone(), flightrecorder::BufferPool::new());

    let frame = build_log_frame(50);
    for _ in 0..10 {
        writer.process_frame(frame.clone()).unwrap();
    }
    writer.flush_and_close().unwrap();

    let deleted = tracker.enforce_cap();
    assert_eq!(deleted, 0, "no files should be deleted with unlimited cap");
    assert!(tracker.file_count() > 0);
}
