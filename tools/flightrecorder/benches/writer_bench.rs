//! Benchmarks for the writer pipeline: frame decode → push → flush.
//!
//! Measures the writer pipeline (sync push on dedicated thread, rtrb channel)
//! for comparison against previous mutex-based implementation.

use std::sync::Arc;
use std::time::{Duration, Instant};

use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use tempfile::TempDir;

use flightrecorder::generated::signals_generated::signals;
use flightrecorder::writers::context_store::ContextStore;
use flightrecorder::writers::logs::LogsWriter;
use flightrecorder::writers::metrics::MetricsWriter;

// ---------------------------------------------------------------------------
// Frame builders
// ---------------------------------------------------------------------------

/// Build a MetricBatch frame with `n` context-key samples.
fn build_metric_frame(n: usize) -> Vec<u8> {
    let mut fbb = flatbuffers::FlatBufferBuilder::with_capacity(4096);

    let mut offsets = Vec::with_capacity(n);
    for i in (0..n).rev() {
        let name = fbb.create_string(&format!("cpu.user.{}", i % 100));
        let source = fbb.create_string("dogstatsd");
        let t1 = fbb.create_string(&format!("host:web-{}", i % 50));
        let t2 = fbb.create_string("env:staging");
        let tags = fbb.create_vector(&[t1, t2]);
        let sample = signals::MetricSample::create(
            &mut fbb,
            &signals::MetricSampleArgs {
                name: Some(name),
                value: i as f64 * 1.1,
                tags: Some(tags),
                timestamp_ns: 1_700_000_000_000_000_000 + i as i64 * 1_000_000,
                sample_rate: 1.0,
                source: Some(source),
                context_key: (i as u64) % 5000 + 1,
            },
        );
        offsets.push(sample);
    }
    offsets.reverse();
    let vec = fbb.create_vector(&offsets);
    let batch = signals::MetricBatch::create(
        &mut fbb,
        &signals::MetricBatchArgs {
            samples: Some(vec),
        },
    );
    let env = signals::SignalEnvelope::create(
        &mut fbb,
        &signals::SignalEnvelopeArgs {
            payload_type: signals::SignalPayload::MetricBatch,
            payload: Some(batch.as_union_value()),
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
        let status = fbb.create_string("info");
        let hostname = fbb.create_string(&format!("web-{}", i % 50));
        let source = fbb.create_string("docker");
        let t1 = fbb.create_string("service:api");
        let t2 = fbb.create_string("env:staging");
        let tags = fbb.create_vector(&[t1, t2]);
        let entry = signals::LogEntry::create(
            &mut fbb,
            &signals::LogEntryArgs {
                content: Some(content),
                status: Some(status),
                tags: Some(tags),
                hostname: Some(hostname),
                timestamp_ns: 1_700_000_000_000_000_000 + i as i64 * 1_000_000,
                source: Some(source),
            },
        );
        offsets.push(entry);
    }
    offsets.reverse();
    let vec = fbb.create_vector(&offsets);
    let batch = signals::LogBatch::create(
        &mut fbb,
        &signals::LogBatchArgs {
            entries: Some(vec),
        },
    );
    let env = signals::SignalEnvelope::create(
        &mut fbb,
        &signals::SignalEnvelopeArgs {
            payload_type: signals::SignalPayload::LogBatch,
            payload: Some(batch.as_union_value()),
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
                let store = ContextStore::new(dir.path()).unwrap();
                let mut writer = MetricsWriter::new(
                    dir.path(),
                    100_000, // high threshold to avoid flushing during push
                    Duration::from_secs(3600),
                    store,
                );

                let env = flatbuffers::root::<signals::SignalEnvelope>(frame).unwrap();
                let batch = env.payload_as_metric_batch().unwrap();

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
                let mut writer = LogsWriter::new(
                    dir.path(),
                    100_000,
                    Duration::from_secs(3600),
                );

                let env = flatbuffers::root::<signals::SignalEnvelope>(frame).unwrap();
                let batch = env.payload_as_log_batch().unwrap();

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
                let store = ContextStore::new(dir.path()).unwrap();
                // flush_rows = n so every push triggers a flush.
                let mut writer = MetricsWriter::new(
                    dir.path(),
                    n,
                    Duration::from_secs(3600),
                    store,
                );

                let env = flatbuffers::root::<signals::SignalEnvelope>(frame).unwrap();
                let batch = env.payload_as_metric_batch().unwrap();

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
                let mut writer = LogsWriter::new(
                    dir.path(),
                    n,
                    Duration::from_secs(3600),
                );

                let env = flatbuffers::root::<signals::SignalEnvelope>(frame).unwrap();
                let batch = env.payload_as_log_batch().unwrap();

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
                let store = ContextStore::new(dir.path()).unwrap();
                let writer = MetricsWriter::new(
                    dir.path(),
                    5000,
                    Duration::from_secs(15),
                    store,
                );
                let mut handle = WriterHandle::spawn(writer, 512, "bench");

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

criterion_group!(
    benches,
    bench_metrics_push,
    bench_logs_push,
    bench_metrics_push_flush,
    bench_logs_push_flush,
    bench_rtrb_e2e,
);
criterion_main!(benches);
