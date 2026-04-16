//! Benchmarks for the writer pipeline: frame decode → push → flush.
//!
//! Measures the writer pipeline (sync push on dedicated thread, rtrb channel)
//! for comparison against previous mutex-based implementation.

use std::sync::Arc;
use std::time::{Duration, Instant};

use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use tempfile::TempDir;

use flightrecorder::disk_tracker::DiskTracker;
use flightrecorder::generated::signals_generated::signals;
use flightrecorder::writers::context_store::ContextStore;
use flightrecorder::writers::context_writer::{ContextProducer, ContextWriterHandle};
use flightrecorder::writers::logs::LogsWriter;
use flightrecorder::writers::metrics::MetricsWriter;
use flightrecorder::writers::thread::SignalWriter;

fn make_ctx_producer() -> (ContextProducer, ContextProducer) {
    let dir = TempDir::new().unwrap();
    let store = ContextStore::new(dir.path()).unwrap();
    let (_handle, prod_m, prod_l) = ContextWriterHandle::spawn(store, 64);
    std::mem::forget(_handle);
    std::mem::forget(dir);
    (prod_m, prod_l)
}

// ---------------------------------------------------------------------------
// Frame builders
// ---------------------------------------------------------------------------

/// Build a MetricBatch frame with `n` context entries and `n` point entries.
fn build_metric_frame(n: usize) -> Vec<u8> {
    let mut fbb = flatbuffers::FlatBufferBuilder::with_capacity(4096);

    // Build context entries.
    let mut ctx_offsets = Vec::with_capacity(n);
    for i in (0..n).rev() {
        let name = fbb.create_string(&format!("cpu.user.{}", i % 100));
        let source = fbb.create_string("dogstatsd");
        let t1 = fbb.create_string(&format!("host:web-{}", i % 50));
        let t2 = fbb.create_string("env:staging");
        let tags = fbb.create_vector(&[t1, t2]);
        let ctx = signals::ContextEntry::create(
            &mut fbb,
            &signals::ContextEntryArgs {
                context_key: (i as u64) % 5000 + 1,
                name: Some(name),
                tags: Some(tags),
                source: Some(source),
            },
        );
        ctx_offsets.push(ctx);
    }
    ctx_offsets.reverse();
    let ctx_vec = fbb.create_vector(&ctx_offsets);

    // Build point entries.
    let mut pt_offsets = Vec::with_capacity(n);
    for i in (0..n).rev() {
        let pt = signals::PointEntry::create(
            &mut fbb,
            &signals::PointEntryArgs {
                context_key: (i as u64) % 5000 + 1,
                value: i as f64 * 1.1,
                timestamp_ns: 1_700_000_000_000_000_000 + i as i64 * 1_000_000,
                sample_rate: 1.0,
            },
        );
        pt_offsets.push(pt);
    }
    pt_offsets.reverse();
    let pt_vec = fbb.create_vector(&pt_offsets);

    let batch = signals::MetricBatch::create(
        &mut fbb,
        &signals::MetricBatchArgs {
            contexts: Some(ctx_vec),
            points: Some(pt_vec),
        },
    );
    let env = signals::SignalEnvelope::create(
        &mut fbb,
        &signals::SignalEnvelopeArgs {
            metric_batch: Some(batch),
            ..Default::default()
        },
    );
    fbb.finish(env, None);
    fbb.finished_data().to_vec()
}

/// Build a LogBatch frame with `n` log entries.
fn build_log_frame(n: usize) -> Vec<u8> {
    let mut fbb = flatbuffers::FlatBufferBuilder::with_capacity(4096);

    let mut offsets = Vec::with_capacity(n);
    for i in (0..n).rev() {
        let content = fbb.create_vector(format!("Log line {} with some realistic content for benchmarking the writer pipeline", i).as_bytes());
        let entry = signals::LogEntry::create(
            &mut fbb,
            &signals::LogEntryArgs {
                context_key: (i as u64) % 100 + 1,
                content: Some(content),
                timestamp_ns: 1_700_000_000_000_000_000 + i as i64 * 1_000_000,
            },
        );
        offsets.push(entry);
    }
    offsets.reverse();
    let vec = fbb.create_vector(&offsets);
    let batch = signals::LogBatch::create(
        &mut fbb,
        &signals::LogBatchArgs {
            contexts: None,
            entries: Some(vec),
        },
    );
    let env = signals::SignalEnvelope::create(
        &mut fbb,
        &signals::SignalEnvelopeArgs {
            log_batch: Some(batch),
            ..Default::default()
        },
    );
    fbb.finish(env, None);
    fbb.finished_data().to_vec()
}

// ---------------------------------------------------------------------------
// Push-only benchmarks (accumulation without flush)
// ---------------------------------------------------------------------------

fn bench_metrics_push(c: &mut Criterion) {
    let cases: &[usize] = &[100, 500, 2000];
    let mut group = c.benchmark_group("metrics_push");

    for &n in cases {
        let frame = build_metric_frame(n);
        group.throughput(Throughput::Elements(n as u64));

        group.bench_with_input(BenchmarkId::from_parameter(n), &frame, |b, frame| {
            b.iter_custom(|iters| {
                let dir = TempDir::new().unwrap();
                let (prod_m, _prod_l) = make_ctx_producer();
                let mut writer = MetricsWriter::new(
                    dir.path(),
                    100_000, // high threshold to avoid flushing during push
                    Duration::from_secs(3600),
                    Duration::from_secs(60),
                    prod_m,
                    Arc::new(DiskTracker::noop()),
                );

                let env = flatbuffers::root::<signals::SignalEnvelope>(frame).unwrap();
                let batch = env.metric_batch().unwrap();

                let start = Instant::now();
                for _ in 0..iters {
                    writer.push(&batch).unwrap();
                }
                start.elapsed()
            });
        });
    }
    group.finish();
}

fn bench_logs_push(c: &mut Criterion) {
    let cases: &[usize] = &[100, 500, 2000];
    let mut group = c.benchmark_group("logs_push");

    for &n in cases {
        let frame = build_log_frame(n);
        group.throughput(Throughput::Elements(n as u64));

        group.bench_with_input(BenchmarkId::from_parameter(n), &frame, |b, frame| {
            b.iter_custom(|iters| {
                let dir = TempDir::new().unwrap();
                let (_prod_m, prod_l) = make_ctx_producer();
                let mut writer = LogsWriter::new(
                    dir.path(),
                    100_000,
                    Duration::from_secs(3600),
                    Duration::from_secs(60),
                    prod_l,
                    Arc::new(DiskTracker::noop()),
                    flightrecorder::BufferPool::new(),
                );

                let start = Instant::now();
                for _ in 0..iters {
                    writer.process_frame(frame.clone()).unwrap();
                }
                start.elapsed()
            });
        });
    }
    group.finish();
}

// ---------------------------------------------------------------------------
// Push + flush benchmarks (full pipeline including Parquet I/O)
// ---------------------------------------------------------------------------

fn bench_metrics_push_flush(c: &mut Criterion) {
    let cases: &[usize] = &[500, 2000, 5000];
    let mut group = c.benchmark_group("metrics_push_flush");

    for &n in cases {
        let frame = build_metric_frame(n);
        group.throughput(Throughput::Elements(n as u64));

        group.bench_with_input(BenchmarkId::from_parameter(n), &frame, |b, frame| {
            b.iter_custom(|iters| {
                let dir = TempDir::new().unwrap();
                let (prod_m, _prod_l) = make_ctx_producer();
                // flush_rows = n so every push triggers a flush.
                let mut writer = MetricsWriter::new(
                    dir.path(),
                    n,
                    Duration::from_secs(3600),
                    Duration::from_secs(60),
                    prod_m,
                    Arc::new(DiskTracker::noop()),
                );

                let env = flatbuffers::root::<signals::SignalEnvelope>(frame).unwrap();
                let batch = env.metric_batch().unwrap();

                let start = Instant::now();
                for _ in 0..iters {
                    writer.push(&batch).unwrap();
                }
                start.elapsed()
            });
        });
    }
    group.finish();
}

fn bench_logs_push_flush(c: &mut Criterion) {
    let cases: &[usize] = &[500, 2000, 5000];
    let mut group = c.benchmark_group("logs_push_flush");

    for &n in cases {
        let frame = build_log_frame(n);
        group.throughput(Throughput::Elements(n as u64));

        group.bench_with_input(BenchmarkId::from_parameter(n), &frame, |b, frame| {
            b.iter_custom(|iters| {
                let dir = TempDir::new().unwrap();
                let (_prod_m, prod_l) = make_ctx_producer();
                let mut writer = LogsWriter::new(
                    dir.path(),
                    n,
                    Duration::from_secs(3600),
                    Duration::from_secs(60),
                    prod_l,
                    Arc::new(DiskTracker::noop()),
                    flightrecorder::BufferPool::new(),
                );

                let start = Instant::now();
                for _ in 0..iters {
                    writer.process_frame(frame.clone()).unwrap();
                }
                start.elapsed()
            });
        });
    }
    group.finish();
}

// ---------------------------------------------------------------------------
// End-to-end: rtrb channel + writer thread
// ---------------------------------------------------------------------------

fn bench_rtrb_e2e(c: &mut Criterion) {
    use flightrecorder::writers::thread::WriterHandle;

    let mut group = c.benchmark_group("rtrb_e2e");

    let n_samples = 500;
    let n_frames = 1000;
    let frame = build_metric_frame(n_samples);

    group.throughput(Throughput::Elements((n_frames * n_samples) as u64));
    group.sample_size(10);

    group.bench_function("metrics_500x1000", |b| {
        b.iter_custom(|iters| {
            let frame = frame.clone();
            let mut total = Duration::ZERO;
            for _ in 0..iters {
                let dir = TempDir::new().unwrap();
                let (prod_m, _prod_l) = make_ctx_producer();
                let writer = MetricsWriter::new(
                    dir.path(),
                    5000,
                    Duration::from_secs(15),
                    Duration::from_secs(60),
                    prod_m,
                    Arc::new(DiskTracker::noop()),
                );
                let mut handle = WriterHandle::spawn(writer, 512, "bench", flightrecorder::BufferPool::new());

                let start = Instant::now();
                for _ in 0..n_frames {
                    handle.send_frame(frame.clone());
                }
                handle.shutdown();
                total += start.elapsed();
            }
            total
        });
    });

    group.finish();
}

// ---------------------------------------------------------------------------
// Log throughput saturation benchmark
//
// Measures the maximum sustained frame rate the logs writer can handle.
// Simulates the production hot path: rtrb push → writer thread decode →
// arena accumulate → Parquet flush. Reports frames/sec and MB/sec.
// ---------------------------------------------------------------------------

fn build_log_frame_with_content(n_entries: usize, content_len: usize) -> Vec<u8> {
    let mut fbb = flatbuffers::FlatBufferBuilder::with_capacity(n_entries * (content_len + 64));
    let content_bytes = vec![b'A'; content_len];
    let mut offsets = Vec::with_capacity(n_entries);
    for i in (0..n_entries).rev() {
        let content = fbb.create_vector(&content_bytes);
        let entry = signals::LogEntry::create(
            &mut fbb,
            &signals::LogEntryArgs {
                context_key: (i as u64) % 1000 + 1,
                content: Some(content),
                timestamp_ns: 1_700_000_000_000_000_000 + i as i64 * 1_000_000,
            },
        );
        offsets.push(entry);
    }
    offsets.reverse();
    let vec = fbb.create_vector(&offsets);
    let batch = signals::LogBatch::create(
        &mut fbb,
        &signals::LogBatchArgs {
            contexts: None,
            entries: Some(vec),
        },
    );
    let env = signals::SignalEnvelope::create(
        &mut fbb,
        &signals::SignalEnvelopeArgs {
            log_batch: Some(batch),
            ..Default::default()
        },
    );
    fbb.finish(env, None);
    fbb.finished_data().to_vec()
}

/// End-to-end log throughput: push N frames through rtrb → writer thread →
/// Parquet flush. Measures the ceiling of what the sidecar can sustain.
fn bench_logs_throughput_saturation(c: &mut Criterion) {
    use flightrecorder::writers::thread::WriterHandle;

    let mut group = c.benchmark_group("logs_throughput_saturation");
    group.sample_size(10);

    // Simulate production scenarios:
    // - p50 pod: ~100 logs/sec, ~200 bytes each
    // - p99 pod: ~7000 logs/sec, ~250 bytes each
    // - extreme pod: ~10000 logs/sec, ~500 bytes each
    let cases: &[(&str, usize, usize, usize)] = &[
        // (name, entries_per_frame, content_bytes, n_frames)
        ("p50_100entries_200B_x100frames", 100, 200, 100),
        ("p99_2000entries_250B_x100frames", 2000, 250, 100),
        ("extreme_2000entries_500B_x100frames", 2000, 500, 100),
        ("extreme_2000entries_1KB_x100frames", 2000, 1024, 100),
    ];

    for &(name, n_entries, content_len, n_frames) in cases {
        let frame = build_log_frame_with_content(n_entries, content_len);
        let frame_bytes = frame.len();
        let total_entries = n_entries * n_frames;
        let total_bytes = frame_bytes * n_frames;

        group.throughput(Throughput::Bytes(total_bytes as u64));

        group.bench_function(name, |b| {
            b.iter_custom(|iters| {
                let frame = frame.clone();
                let mut total = Duration::ZERO;
                for _ in 0..iters {
                    let dir = TempDir::new().unwrap();
                    let (_prod_m, prod_l) = make_ctx_producer();
                    let writer = LogsWriter::new(
                        dir.path(),
                        5000,
                        Duration::from_secs(15),
                        Duration::from_secs(60),
                        prod_l,
                        Arc::new(DiskTracker::noop()),
                        flightrecorder::BufferPool::new(),
                    );
                    let mut handle = WriterHandle::spawn(writer, 512, "bench-logs", flightrecorder::BufferPool::new());

                    let start = Instant::now();
                    for _ in 0..n_frames {
                        handle.send_frame(frame.clone());
                    }
                    handle.shutdown();
                    total += start.elapsed();
                }

                // Print throughput info on first iteration for visibility.
                if iters == 1 {
                    let secs = total.as_secs_f64();
                    eprintln!(
                        "  [{name}] {total_entries} entries in {n_frames} frames ({frame_bytes} bytes/frame), \
                         {:.0} frames/sec, {:.1} MB/sec, {:.0} entries/sec",
                        n_frames as f64 / secs,
                        total_bytes as f64 / secs / 1e6,
                        total_entries as f64 / secs,
                    );
                }
                total
            });
        });
    }

    group.finish();
}

// ---------------------------------------------------------------------------
// End-to-end UDS benchmark
//
// Full pipeline: UDS socket write → async read_frame → envelope peek →
// rtrb push → writer thread → Parquet flush. Simulates the real production
// path including kernel socket buffers and async I/O.
// ---------------------------------------------------------------------------

fn bench_logs_uds_e2e(c: &mut Criterion) {
    use flightrecorder::writers::thread::WriterHandle;
    use std::io::Write;
    use std::os::unix::net::UnixStream as StdUnixStream;
    use tokio::net::UnixStream as TokioUnixStream;
    use tokio::io::BufReader;

    let mut group = c.benchmark_group("logs_uds_e2e");
    group.sample_size(10);

    let cases: &[(&str, usize, usize, usize)] = &[
        // (name, entries_per_frame, content_bytes, n_frames)
        ("p99_2000x250B_x100", 2000, 250, 100),
        ("extreme_2000x500B_x100", 2000, 500, 100),
    ];

    for &(name, n_entries, content_len, n_frames) in cases {
        let frame = build_log_frame_with_content(n_entries, content_len);
        let frame_len = frame.len();
        let total_entries = n_entries * n_frames;
        let total_bytes = frame_len * n_frames;

        // Pre-build the wire-format payload: [4-byte LE len][frame] repeated.
        let mut wire_payload = Vec::with_capacity((4 + frame_len) * n_frames);
        for _ in 0..n_frames {
            wire_payload.extend_from_slice(&(frame_len as u32).to_le_bytes());
            wire_payload.extend_from_slice(&frame);
        }

        group.throughput(Throughput::Bytes(total_bytes as u64));

        group.bench_function(name, |b| {
            b.iter_custom(|iters| {
                let wire_payload = wire_payload.clone();
                let mut total = Duration::ZERO;

                for _ in 0..iters {
                    let dir = TempDir::new().unwrap();
                    let (_prod_m, prod_l) = make_ctx_producer();
                    let writer = LogsWriter::new(
                        dir.path(),
                        5000,
                        Duration::from_secs(60),
                        Duration::from_secs(60),
                        prod_l,
                        Arc::new(DiskTracker::noop()),
                        flightrecorder::BufferPool::new(),
                    );
                    let mut handle = WriterHandle::spawn(writer, 512, "bench-uds", flightrecorder::BufferPool::new());

                    // Create a UDS socket pair.
                    let (sender_std, receiver_std) = StdUnixStream::pair().unwrap();
                    sender_std.set_nonblocking(false).unwrap();
                    receiver_std.set_nonblocking(true).unwrap();

                    let rt = tokio::runtime::Builder::new_current_thread()
                        .enable_io()
                        .build()
                        .unwrap();

                    let start = Instant::now();

                    // Sender thread: write all frames to the UDS socket, then close.
                    let wire = wire_payload.clone();
                    let sender_thread = std::thread::spawn(move || {
                        let mut s = sender_std;
                        s.write_all(&wire).unwrap();
                        drop(s); // Close → reader sees EOF.
                    });

                    // Reader: async loop reading frames and routing to writer.
                    rt.block_on(async {
                        let stream = TokioUnixStream::from_std(receiver_std).unwrap();
                        let mut reader = BufReader::new(stream);
                        let mut buf = Vec::new();
                        loop {
                            match flightrecorder::framing::read_frame(&mut reader, &mut buf).await {
                                Ok(Some(_)) => {
                                    let frame = std::mem::take(&mut buf);
                                    handle.send_frame(frame);
                                }
                                Ok(None) => break, // EOF
                                Err(e) => panic!("read error: {e}"),
                            }
                        }
                    });

                    sender_thread.join().unwrap();
                    handle.shutdown();
                    total += start.elapsed();

                    if iters == 1 {
                        let secs = total.as_secs_f64();
                        eprintln!(
                            "  [{name}] {total_entries} entries in {n_frames} frames via UDS, \
                             {:.0} frames/sec, {:.1} MB/sec, {:.0} entries/sec",
                            n_frames as f64 / secs,
                            total_bytes as f64 / secs / 1e6,
                            total_entries as f64 / secs,
                        );
                    }
                }
                total
            });
        });
    }

    group.finish();
}

criterion_group!(
    benches,
    bench_metrics_push,
    bench_logs_push,
    bench_metrics_push_flush,
    bench_logs_push_flush,
    bench_rtrb_e2e,
    bench_logs_throughput_saturation,
    bench_logs_uds_e2e,
);
criterion_main!(benches);
