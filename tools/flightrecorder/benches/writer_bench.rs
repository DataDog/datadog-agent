use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use std::time::Instant;
use tokio::runtime::Runtime;
use vortex::array::arrays::{PrimitiveArray, StructArray, VarBinArray};
use vortex::array::dtype::FieldNames;
use vortex::array::validity::Validity;
use vortex::array::IntoArray;
use vortex::file::WriteOptionsSessionExt;
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

// ---------------------------------------------------------------------------
// System resource helpers
// ---------------------------------------------------------------------------

fn rss_kb() -> u64 {
    std::fs::read_to_string("/proc/self/status")
        .unwrap_or_default()
        .lines()
        .find(|l| l.starts_with("VmRSS:"))
        .and_then(|l| l.split_whitespace().nth(1).and_then(|v| v.parse().ok()))
        .unwrap_or(0)
}

// ---------------------------------------------------------------------------
// Array construction helpers
// ---------------------------------------------------------------------------

/// True uncompressed row size for a metric row with the given string lengths.
fn raw_row_bytes(name_len: usize) -> usize {
    // name(name_len) + value(8) + tags(~14) + timestamp_ns(8) + sample_rate(8) + source(9)
    name_len + 8 + 14 + 8 + 8 + 9
}

fn make_metric_struct(n_rows: usize, str_len: usize) -> StructArray {
    let pad: String = std::iter::repeat('x')
        .take(str_len.saturating_sub(6))
        .collect();
    let names: Vec<String> = (0..n_rows)
        .map(|i| format!("m{:04}{}", i % 9999, &pad))
        .collect();
    let values: Vec<f64> = (0..n_rows).map(|i| i as f64 * 0.001).collect();
    let tags: Vec<String> = (0..n_rows)
        .map(|i| format!("host:b{}", i % 16))
        .collect();
    let timestamps: Vec<i64> = (0..n_rows)
        .map(|i| 1_700_000_000_000_000_000 + i as i64)
        .collect();
    let sample_rates: Vec<f64> = vec![1.0f64; n_rows];
    let sources: Vec<String> = vec!["benchmark".to_string(); n_rows];

    StructArray::try_new(
        FieldNames::from(["name", "value", "tags", "timestamp_ns", "sample_rate", "source"]),
        vec![
            VarBinArray::from(names).into_array(),
            values.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(tags).into_array(),
            timestamps.into_iter().collect::<PrimitiveArray>().into_array(),
            sample_rates.into_iter().collect::<PrimitiveArray>().into_array(),
            VarBinArray::from(sources).into_array(),
        ],
        n_rows,
        Validity::NonNullable,
    )
    .expect("build StructArray")
}

async fn write_to_vec(n_rows: usize, str_len: usize) -> Vec<u8> {
    let st = make_metric_struct(n_rows, str_len);
    let session = VortexSession::default();
    let mut buffer: Vec<u8> = Vec::new();
    session
        .write_options()
        .write(&mut buffer, st.into_array().to_array_stream())
        .await
        .expect("vortex write");
    buffer
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

/// Bench 1: Write throughput at varying row counts and string sizes.
/// Output is to Vec<u8> (RAM only) to isolate from disk I/O.
fn bench_write_throughput(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();

    let row_counts = [100usize, 1_000, 10_000];
    let str_sizes: &[(&str, usize)] = &[("small_20B", 20), ("large_2KB", 2048)];

    let mut group = c.benchmark_group("vortex_write_throughput");

    for &n_rows in &row_counts {
        for &(str_label, str_len) in str_sizes {
            let label = format!("{}rows_{}", n_rows, str_label);
            group.throughput(Throughput::Elements(n_rows as u64));

            // ── Metadata probe (runs once before the benchmark) ───────────────
            // Criterion calls the bench closure multiple times (warmup + 100
            // samples), so we compute and print metadata *outside* the closure.
            let probe_buf = rt.block_on(write_to_vec(n_rows, str_len));
            let raw_bytes = n_rows * raw_row_bytes(str_len);
            let vortex_bytes = probe_buf.len();
            // ratio > 1.0 → good compression; < 1.0 → format overhead dominates
            let compression_ratio = raw_bytes as f64 / vortex_bytes as f64;
            let rss_before = rss_kb();
            drop(rt.block_on(write_to_vec(n_rows, str_len))); // warm RSS
            let rss_after = rss_kb();
            println!(
                "BENCH_META rows={n_rows} str_len={str_len} raw_row_bytes={raw_bytes} \
                 vortex_bytes={vortex_bytes} compression_ratio={compression_ratio:.3} \
                 rss_delta_kb={}",
                rss_after.saturating_sub(rss_before),
            );

            group.bench_with_input(
                BenchmarkId::from_parameter(&label),
                &(n_rows, str_len),
                |b, &(n_rows, str_len)| {
                    b.iter_custom(|iters| {
                        rt.block_on(async {
                            let start = Instant::now();
                            for _ in 0..iters {
                                let _ = write_to_vec(n_rows, str_len).await;
                            }
                            start.elapsed()
                        })
                    });
                },
            );
        }
    }

    group.finish();
}

/// Bench 2: Throughput at the flush batch sizes used by the recorder.
///
/// The recorder flushes at `RECORDER_FLUSH_ROWS` rows (default 1000). This
/// bench sweeps across realistic batch sizes so you can read off the
/// sustainable write rate at your configured flush threshold.
///
/// To get ns/row: compute `1e9 / thrpt_elem_per_s`.
///   rows_100  → ~15K rows/s → ~67µs/row
///   rows_500  → ~58K rows/s → ~17µs/row
///   rows_1000 → same group → ~9ms/flush
fn bench_write_flush_batch_sizes(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();

    // Small strings (20B name) — representative of real metric names.
    let flush_sizes: &[(&str, usize)] = &[
        ("rows_100", 100),
        ("rows_500", 500),
        ("rows_1000", 1000),
    ];

    let mut group = c.benchmark_group("vortex_write_flush_batch");

    for &(label, n_rows) in flush_sizes {
        group.throughput(Throughput::Elements(n_rows as u64));
        group.bench_function(label, |b| {
            b.iter_custom(|iters| {
                rt.block_on(async {
                    let start = Instant::now();
                    for _ in 0..iters {
                        let _ = write_to_vec(n_rows, 20).await;
                    }
                    start.elapsed()
                })
            });
        });
    }

    group.finish();
}

criterion_group!(benches, bench_write_throughput, bench_write_flush_batch_sizes);
criterion_main!(benches);
