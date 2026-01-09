//! Query benchmarks for fine-grained-monitor parquet loading.
//!
//! Benchmarks the hot paths in lazy_data.rs:
//! - scan_metadata (startup)
//! - get_timeseries (query)
//!
//! ## Running benchmarks
//!
//! First generate test data:
//! ```bash
//! cargo run --release --bin generate-bench-data -- --scenario realistic --duration 1h
//! cargo run --release --bin generate-bench-data -- --scenario stress --duration 1h
//! ```
//!
//! Then run benchmarks:
//! ```bash
//! cargo bench
//! cargo bench -- scan_metadata  # run specific benchmark
//! ```
//!
//! Use BENCH_DATA env var to select scenario:
//! ```bash
//! BENCH_DATA=testdata/bench/realistic cargo bench
//! BENCH_DATA=testdata/bench/stress cargo bench
//! ```

use divan::Bencher;
use fine_grained_monitor::metrics_viewer::data::TimeRange;
use fine_grained_monitor::metrics_viewer::LazyDataStore;
use std::path::PathBuf;

fn main() {
    divan::main();
}

/// Get the benchmark data directory from env or default to realistic scenario
fn get_bench_data_dir() -> PathBuf {
    std::env::var("BENCH_DATA")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("testdata/bench/realistic"))
}

/// Get all parquet files in the benchmark data directory (recursively)
fn get_parquet_files() -> Vec<PathBuf> {
    let dir = get_bench_data_dir();
    // Try recursive pattern first (for realistic scenarios with dt=/identifier= structure)
    let recursive_pattern = format!("{}/**/*.parquet", dir.display());
    let files: Vec<PathBuf> = glob::glob(&recursive_pattern)
        .expect("Failed to read glob pattern")
        .filter_map(Result::ok)
        .collect();

    if !files.is_empty() {
        return files;
    }

    // Fall back to flat pattern for legacy scenarios
    let flat_pattern = format!("{}/*.parquet", dir.display());
    glob::glob(&flat_pattern)
        .expect("Failed to read glob pattern")
        .filter_map(Result::ok)
        .collect()
}

/// Benchmark: LazyDataStore::from_files() which tries sidecars first, then scans parquet
///
/// This measures the explicit file list path. With sidecars present, this should
/// be fast. Without sidecars, it falls back to parquet scanning (slow).
#[divan::bench]
fn scan_metadata_from_files(bencher: Bencher) {
    let files = get_parquet_files();
    if files.is_empty() {
        eprintln!("No benchmark data found at {:?}", get_bench_data_dir());
        eprintln!("Run: cargo run --release --bin generate-bench-data -- --scenario realistic");
        return;
    }

    bencher.bench(|| {
        LazyDataStore::from_files(&files).expect("Failed to create LazyDataStore")
    });
}

/// Benchmark: LazyDataStore::new() which is the primary constructor
///
/// This is the RECOMMENDED path - discovers parquet files in a directory and
/// uses sidecar fast path for metadata loading.
#[divan::bench]
fn scan_metadata_directory(bencher: Bencher) {
    let dir = get_bench_data_dir();
    if !dir.exists() {
        eprintln!("No benchmark data found at {:?}", dir);
        eprintln!("Run: cargo run --release --bin generate-bench-data -- --scenario realistic");
        return;
    }

    bencher.bench(|| {
        LazyDataStore::new(dir.clone()).expect("Failed to create LazyDataStore")
    });
}

/// Benchmark: get_timeseries for a single container (cold)
#[divan::bench]
fn get_timeseries_single_container(bencher: Bencher) {
    let files = get_parquet_files();
    if files.is_empty() {
        return;
    }

    bencher
        .with_inputs(|| LazyDataStore::from_files(&files).expect("Failed to create LazyDataStore"))
        .bench_values(|store| {
            let metrics = store.get_metrics();
            let containers = store.get_containers_by_recency(TimeRange::All);
            if let Some(metric) = metrics.first() {
                if let Some(container) = containers.first() {
                    let _ = store.get_timeseries(&metric.name, &[container.id.as_str()], TimeRange::All);
                }
            }
        });
}

/// Benchmark: get_timeseries for all containers (cold)
#[divan::bench]
fn get_timeseries_all_containers(bencher: Bencher) {
    let files = get_parquet_files();
    if files.is_empty() {
        return;
    }

    bencher
        .with_inputs(|| {
            let store = LazyDataStore::from_files(&files).expect("Failed to create LazyDataStore");
            let container_ids: Vec<String> = store
                .get_containers_by_recency(TimeRange::All)
                .into_iter()
                .map(|c| c.id)
                .collect();
            (store, container_ids)
        })
        .bench_values(|(store, container_ids)| {
            let metrics = store.get_metrics();
            if let Some(metric) = metrics.first() {
                let ids: Vec<&str> = container_ids.iter().map(|s| s.as_str()).collect();
                let _ = store.get_timeseries(&metric.name, &ids, TimeRange::All);
            }
        });
}
