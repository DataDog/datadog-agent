//! Benchmark comparing fast vs compact compression strategies,
//! and measuring the merge/compaction path.
//!
//! Run:  cargo bench --bench compression_bench
//!
//! This bench writes realistic metric & log data with both strategies and
//! prints a comparison table showing file sizes, compression ratios, write
//! throughput, and RSS overhead.

use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use std::sync::Arc;
use std::time::Instant;
use tokio::runtime::Runtime;
use vortex::array::arrays::{PrimitiveArray, StructArray, VarBinArray};
use vortex::array::dtype::FieldNames;
use vortex::array::validity::Validity;
use vortex::array::IntoArray;
use vortex::file::VortexWriteOptions;
use vortex::layout::LayoutStrategy;
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

// Bring in the strategies from the crate under test.
use flightrecorder::writers::strategy::{compact_strategy, fast_flush_strategy, low_memory_strategy};
use flightrecorder::writers::metrics::METRIC_FIELD_NAMES;
use flightrecorder::writers::logs::LOG_FIELD_NAMES;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn rss_kb() -> u64 {
    std::fs::read_to_string("/proc/self/status")
        .unwrap_or_default()
        .lines()
        .find(|l| l.starts_with("VmRSS:"))
        .and_then(|l| l.split_whitespace().nth(1).and_then(|v| v.parse().ok()))
        .unwrap_or(0)
}

/// Build a metric-shaped StructArray with decomposed tag columns (13 columns).
fn make_metrics(n_rows: usize) -> StructArray {
    let names: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("system.cpu.user.{}", i % 200).into_bytes())
        .collect();
    let tag_host: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("web{}.prod.us-east-1", i % 32).into_bytes())
        .collect();
    let tag_device: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| if i % 10 == 0 { b"sda1".to_vec() } else { b"".to_vec() })
        .collect();
    let tag_source: Vec<Vec<u8>> = vec![b"".to_vec(); n_rows];
    let tag_service: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("api-gateway-{}", i % 4).into_bytes())
        .collect();
    let tag_env: Vec<Vec<u8>> = vec![b"production".to_vec(); n_rows];
    let tag_version: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("v1.{}", i % 3).into_bytes())
        .collect();
    let tag_team: Vec<Vec<u8>> = vec![b"platform".to_vec(); n_rows];
    let tags_overflow: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("region:us-east-1|az:us-east-1{}", (b'a' + (i % 3) as u8) as char).into_bytes())
        .collect();
    let values: Vec<f64> = (0..n_rows).map(|i| i as f64 * 0.001).collect();
    let timestamps: Vec<i64> = (0..n_rows)
        .map(|i| 1_700_000_000_000_000_000 + i as i64)
        .collect();
    let sample_rates: Vec<f64> = vec![1.0f64; n_rows];
    let sources: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("agent-{}", i % 4).into_bytes())
        .collect();

    StructArray::try_new(
        FieldNames::from(METRIC_FIELD_NAMES),
        vec![
            VarBinArray::from(names).into_array(),
            VarBinArray::from(tag_host).into_array(),
            VarBinArray::from(tag_device).into_array(),
            VarBinArray::from(tag_source).into_array(),
            VarBinArray::from(tag_service).into_array(),
            VarBinArray::from(tag_env).into_array(),
            VarBinArray::from(tag_version).into_array(),
            VarBinArray::from(tag_team).into_array(),
            VarBinArray::from(tags_overflow).into_array(),
            values
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            timestamps
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            sample_rates
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
            VarBinArray::from(sources).into_array(),
        ],
        n_rows,
        Validity::NonNullable,
    )
    .expect("build metrics StructArray")
}

/// Build a log-shaped StructArray with decomposed tag columns (10 columns).
fn make_logs(n_rows: usize) -> StructArray {
    let hostnames: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("host-{}", i % 10).into_bytes())
        .collect();
    let sources: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("app-{}", i % 5).into_bytes())
        .collect();
    let statuses: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| {
            match i % 3 {
                0 => "info",
                1 => "warn",
                _ => "error",
            }
            .as_bytes()
            .to_vec()
        })
        .collect();
    let tag_service: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("svc-{}", i % 4).into_bytes())
        .collect();
    let tag_env: Vec<Vec<u8>> = vec![b"prod".to_vec(); n_rows];
    let tag_version: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("v{}", i % 3).into_bytes())
        .collect();
    let tag_team: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("platform-{}", i % 8).into_bytes())
        .collect();
    let tags_overflow: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| format!("custom_tag:val{}", i % 20).into_bytes())
        .collect();
    let contents: Vec<Vec<u8>> = (0..n_rows)
        .map(|i| {
            format!(
                "2024-03-20T12:00:{:02}.000Z INFO app.handler request_id={} status=200 latency={}ms user_agent=\"Mozilla/5.0\"",
                i % 60, i, i % 500
            ).into_bytes()
        })
        .collect();
    let timestamps: Vec<i64> = (0..n_rows)
        .map(|i| 1_700_000_000_000_000_000 + i as i64 * 1_000_000)
        .collect();

    StructArray::try_new(
        FieldNames::from(LOG_FIELD_NAMES),
        vec![
            VarBinArray::from(hostnames).into_array(),
            VarBinArray::from(sources).into_array(),
            VarBinArray::from(statuses).into_array(),
            VarBinArray::from(tag_service).into_array(),
            VarBinArray::from(tag_env).into_array(),
            VarBinArray::from(tag_version).into_array(),
            VarBinArray::from(tag_team).into_array(),
            VarBinArray::from(tags_overflow).into_array(),
            VarBinArray::from(contents).into_array(),
            timestamps
                .into_iter()
                .collect::<PrimitiveArray>()
                .into_array(),
        ],
        n_rows,
        Validity::NonNullable,
    )
    .expect("build logs StructArray")
}

async fn write_with_strategy(
    st: StructArray,
    strategy: Arc<dyn LayoutStrategy>,
) -> Vec<u8> {
    let session = VortexSession::default();
    let mut buffer: Vec<u8> = Vec::new();
    VortexWriteOptions::new(session)
        .with_strategy(strategy)
        .write(&mut buffer, st.into_array().to_array_stream())
        .await
        .expect("vortex write");
    buffer
}

// ---------------------------------------------------------------------------
// Comparison table (runs once, prints size/ratio/RSS for both strategies)
// ---------------------------------------------------------------------------

fn print_comparison_table(rt: &Runtime) {
    println!("\n{}", "=".repeat(80));
    println!("  COMPRESSION STRATEGY COMPARISON: fast_flush vs compact");
    println!("{}\n", "=".repeat(80));

    struct Row {
        label: &'static str,
        n_rows: usize,
        fast_bytes: usize,
        compact_bytes: usize,
        fast_rss_delta_kb: u64,
        compact_rss_delta_kb: u64,
        fast_write_us: u128,
        compact_write_us: u128,
    }

    let cases: Vec<(&str, Box<dyn Fn() -> StructArray>, usize)> = vec![
        ("metrics_1K", Box::new(|| make_metrics(1_000)), 1_000),
        ("metrics_10K", Box::new(|| make_metrics(10_000)), 10_000),
        ("logs_1K", Box::new(|| make_logs(1_000)), 1_000),
        ("logs_10K", Box::new(|| make_logs(10_000)), 10_000),
    ];

    let mut rows = Vec::new();

    for (label, make_fn, n_rows) in &cases {

        // Fast flush strategy (no compression)
        let rss_before = rss_kb();
        let start = Instant::now();
        let fast_buf = rt.block_on(write_with_strategy(make_fn(), fast_flush_strategy()));
        let fast_write_us = start.elapsed().as_micros();
        let fast_rss = rss_kb().saturating_sub(rss_before);

        // Compact strategy (full compression)
        let rss_before = rss_kb();
        let start = Instant::now();
        let compact_buf = rt.block_on(write_with_strategy(make_fn(), compact_strategy()));
        let compact_write_us = start.elapsed().as_micros();
        let compact_rss = rss_kb().saturating_sub(rss_before);

        rows.push(Row {
            label,
            n_rows: *n_rows,
            fast_bytes: fast_buf.len(),
            compact_bytes: compact_buf.len(),
            fast_rss_delta_kb: fast_rss,
            compact_rss_delta_kb: compact_rss,
            fast_write_us,
            compact_write_us,
        });
    }

    // Print table
    println!(
        "{:<18} {:>6} {:>10} {:>10} {:>8} {:>10} {:>10} {:>10} {:>10}",
        "Workload", "Rows", "Fast(B)", "Compact(B)", "Savings", "Fast(ms)", "Compact(ms)", "Fast RSS", "Cmpct RSS"
    );
    println!("{}", "-".repeat(108));

    for r in &rows {
        let savings = if r.fast_bytes > 0 {
            100.0 * (1.0 - r.compact_bytes as f64 / r.fast_bytes as f64)
        } else {
            0.0
        };
        println!(
            "{:<18} {:>6} {:>10} {:>10} {:>7.1}% {:>9.1}ms {:>10.1}ms {:>8}KB {:>8}KB",
            r.label,
            r.n_rows,
            r.fast_bytes,
            r.compact_bytes,
            savings,
            r.fast_write_us as f64 / 1000.0,
            r.compact_write_us as f64 / 1000.0,
            r.fast_rss_delta_kb,
            r.compact_rss_delta_kb,
        );
    }
    println!();
}

// ---------------------------------------------------------------------------
// Merge benchmark: write N small files, merge them, compare sizes
// ---------------------------------------------------------------------------

fn print_merge_comparison(rt: &Runtime) {
    use flightrecorder::merge::{merge_pass, MergeConfig};
    use flightrecorder::vortex_files;

    println!("\n{}", "=".repeat(80));
    println!("  MERGE COMPACTION BENCHMARK");
    println!("{}\n", "=".repeat(80));

    let dir = tempfile::tempdir().unwrap();
    let n_files = 10usize;
    let rows_per_file = 1_000usize;

    // Write N small flush metric files using fast_flush strategy (no compression)
    let mut total_input_bytes = 0u64;
    for i in 0..n_files {
        let st = make_metrics(rows_per_file);
        let buf = rt.block_on(write_with_strategy(st, fast_flush_strategy()));
        total_input_bytes += buf.len() as u64;
        let path = dir.path().join(format!("flush-metrics-{}.vortex", (i + 1) * 100));
        std::fs::write(&path, &buf).unwrap();
    }

    println!("Input: {} files x {} rows = {} total rows", n_files, rows_per_file, n_files * rows_per_file);
    println!("Total input size: {} bytes ({:.1} KB)", total_input_bytes, total_input_bytes as f64 / 1024.0);
    println!();

    // Run merge
    let rss_before = rss_kb();
    let start = Instant::now();
    let config = MergeConfig {
        output_dir: dir.path().to_path_buf(),
        min_files_to_trigger: 2,
    };
    let merged_count = rt.block_on(merge_pass(&config)).unwrap();
    let merge_time = start.elapsed();
    let merge_rss = rss_kb().saturating_sub(rss_before);

    // Count output files and sizes
    let entries = rt.block_on(vortex_files::scan_vortex_files(dir.path())).unwrap();
    let total_output_bytes: u64 = entries.iter().map(|e| e.size).sum();
    let output_files = entries.len();

    let savings = 100.0 * (1.0 - total_output_bytes as f64 / total_input_bytes as f64);

    println!("Merge results:");
    println!("  Files merged:    {}", merged_count);
    println!("  Output files:    {}", output_files);
    println!("  Output size:     {} bytes ({:.1} KB)", total_output_bytes, total_output_bytes as f64 / 1024.0);
    println!("  Size reduction:  {:.1}%", savings);
    println!("  Merge time:      {:.1}ms", merge_time.as_micros() as f64 / 1000.0);
    println!("  Merge RSS delta: {} KB", merge_rss);
    println!();
}

// ---------------------------------------------------------------------------
// Criterion benchmarks
// ---------------------------------------------------------------------------

fn bench_fast_vs_compact(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();

    // Print the one-shot comparison table first
    print_comparison_table(&rt);
    print_merge_comparison(&rt);

    // Now run timed benchmarks for criterion
    let mut group = c.benchmark_group("compression_strategy");

    for n_rows in [1_000usize, 10_000] {
        group.throughput(Throughput::Elements(n_rows as u64));

        group.bench_with_input(
            BenchmarkId::new("fast_flush_metrics", n_rows),
            &n_rows,
            |b, &n| {
                b.iter_custom(|iters| {
                    rt.block_on(async {
                        let start = Instant::now();
                        for _ in 0..iters {
                            let st = make_metrics(n);
                            let _ = write_with_strategy(st, fast_flush_strategy()).await;
                        }
                        start.elapsed()
                    })
                });
            },
        );

        group.bench_with_input(
            BenchmarkId::new("compact_metrics", n_rows),
            &n_rows,
            |b, &n| {
                b.iter_custom(|iters| {
                    rt.block_on(async {
                        let start = Instant::now();
                        for _ in 0..iters {
                            let st = make_metrics(n);
                            let _ = write_with_strategy(st, compact_strategy()).await;
                        }
                        start.elapsed()
                    })
                });
            },
        );

        group.bench_with_input(
            BenchmarkId::new("fast_flush_logs", n_rows),
            &n_rows,
            |b, &n| {
                b.iter_custom(|iters| {
                    rt.block_on(async {
                        let start = Instant::now();
                        for _ in 0..iters {
                            let st = make_logs(n);
                            let _ = write_with_strategy(st, fast_flush_strategy()).await;
                        }
                        start.elapsed()
                    })
                });
            },
        );

        group.bench_with_input(
            BenchmarkId::new("compact_logs", n_rows),
            &n_rows,
            |b, &n| {
                b.iter_custom(|iters| {
                    rt.block_on(async {
                        let start = Instant::now();
                        for _ in 0..iters {
                            let st = make_logs(n);
                            let _ = write_with_strategy(st, compact_strategy()).await;
                        }
                        start.elapsed()
                    })
                });
            },
        );
    }

    group.finish();
}

criterion_group!(benches, bench_fast_vs_compact);
criterion_main!(benches);
