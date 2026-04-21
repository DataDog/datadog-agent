use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use tokio::runtime::Runtime;

extern crate flatbuffers;

#[path = "../src/generated/signals_generated.rs"]
#[allow(unused_imports, dead_code, clippy::all)]
mod signals_generated;

use signals_generated::signals;

// ---------------------------------------------------------------------------
// Frame builder helpers
// ---------------------------------------------------------------------------

/// Encode a metric batch of `n_samples` context entries into a size-prefixed FlatBuffers buffer.
fn encode_metric_frame(n_samples: usize) -> Vec<u8> {
    let mut builder = flatbuffers::FlatBufferBuilder::with_capacity(1024);

    let mut ctx_offsets = Vec::with_capacity(n_samples);
    for i in (0..n_samples).rev() {
        let name = builder.create_string(&format!("metric_{}", i));
        let source = builder.create_string("");
        let ctx = signals::ContextEntry::create(
            &mut builder,
            &signals::ContextEntryArgs {
                context_key: i as u64 + 1,
                name: Some(name),
                tags: None,
                source: Some(source),
            },
        );
        ctx_offsets.push(ctx);
    }
    ctx_offsets.reverse();
    let ctx_vec = builder.create_vector(&ctx_offsets);

    let batch = signals::MetricBatch::create(
        &mut builder,
        &signals::MetricBatchArgs {
            contexts: Some(ctx_vec),
            ..Default::default()
        },
    );

    let env = signals::SignalEnvelope::create(
        &mut builder,
        &signals::SignalEnvelopeArgs {
            metric_batch: Some(batch),
            ..Default::default()
        },
    );

    builder.finish(env, None);
    let data = builder.finished_data();

    // Size-prefix
    let mut buf = Vec::with_capacity(4 + data.len());
    buf.extend_from_slice(&(data.len() as u32).to_le_bytes());
    buf.extend_from_slice(data);
    buf
}

/// Encode N frames and concatenate into a single byte stream.
fn encode_n_concatenated_frames(n_samples_per_frame: usize, n_frames: usize) -> Vec<u8> {
    let frame = encode_metric_frame(n_samples_per_frame);
    frame.repeat(n_frames)
}

// ---------------------------------------------------------------------------
// Decode routines
// ---------------------------------------------------------------------------

async fn decode_single(payload: &[u8]) {
    // Skip 4-byte length prefix, then verify root.
    let data = &payload[4..];
    let _env = flatbuffers::root::<signals::SignalEnvelope>(data).expect("decode failed");
}

/// Decode `n_frames` from a concatenated stream (serial).
async fn decode_serial_from_stream(buf: &[u8], n_frames: usize) {
    let mut offset = 0;
    for _ in 0..n_frames {
        let len = u32::from_le_bytes(buf[offset..offset + 4].try_into().unwrap()) as usize;
        offset += 4;
        let _env =
            flatbuffers::root::<signals::SignalEnvelope>(&buf[offset..offset + len]).expect("decode failed");
        offset += len;
    }
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

fn bench_single_frame_latency(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();

    let small_payload = encode_metric_frame(1);
    let medium_payload = encode_metric_frame(10);
    let large_payload = encode_metric_frame(100);

    let cases: &[(&str, &Vec<u8>)] = &[
        ("1_sample", &small_payload),
        ("10_samples", &medium_payload),
        ("100_samples", &large_payload),
    ];

    let mut group = c.benchmark_group("single_frame_decode_latency");
    group.throughput(Throughput::Elements(1));

    for &(name, payload) in cases {
        group.bench_with_input(BenchmarkId::from_parameter(name), payload, |b, payload| {
            b.to_async(&rt).iter(|| decode_single(payload));
        });
    }

    group.finish();
}

fn bench_serial_stream_throughput(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();

    const N_FRAMES: usize = 200;
    let stream_1s = encode_n_concatenated_frames(1, N_FRAMES);
    let stream_10s = encode_n_concatenated_frames(10, N_FRAMES);
    let stream_100s = encode_n_concatenated_frames(100, N_FRAMES);

    let cases: &[(&str, &Vec<u8>)] = &[
        ("1_sample_frames", &stream_1s),
        ("10_sample_frames", &stream_10s),
        ("100_sample_frames", &stream_100s),
    ];

    let mut group = c.benchmark_group("serial_stream_decode");
    for &(name, buf) in cases {
        group.throughput(Throughput::Elements(N_FRAMES as u64));
        group.bench_with_input(BenchmarkId::from_parameter(name), buf, |b, buf| {
            b.to_async(&rt)
                .iter(|| decode_serial_from_stream(buf, N_FRAMES));
        });
    }

    group.finish();
}

criterion_group!(benches, bench_single_frame_latency, bench_serial_stream_throughput);
criterion_main!(benches);
