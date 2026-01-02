//! Query benchmarks for fine-grained-monitor parquet loading.
//!
//! Benchmarks the hot paths in lazy_data.rs:
//! - scan_metadata (startup)
//! - get_timeseries (query)
//! - get_container_stats (aggregation)
//!
//! ## Running benchmarks
//!
//! First generate test data:
//! ```bash
//! cargo run --release --bin generate-bench-data -- --scenario small
//! cargo run --release --bin generate-bench-data -- --scenario medium
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
//! BENCH_DATA=testdata/bench/medium cargo bench
//! ```

use divan::Bencher;
use fine_grained_monitor::metrics_viewer::LazyDataStore;
use std::path::PathBuf;

fn main() {
    divan::main();
}

/// Get the benchmark data directory from env or default to small scenario
fn get_bench_data_dir() -> PathBuf {
    std::env::var("BENCH_DATA")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("testdata/bench/small"))
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

/// Benchmark: LazyDataStore::new() which calls scan_metadata
///
/// This is the startup path - measures how long it takes to scan parquet
/// files and build the metadata index.
#[divan::bench]
fn scan_metadata(bencher: Bencher) {
    let files = get_parquet_files();
    if files.is_empty() {
        eprintln!("No benchmark data found at {:?}", get_bench_data_dir());
        eprintln!("Run: cargo run --release --bin generate-bench-data -- --scenario small");
        return;
    }

    bencher.bench(|| {
        LazyDataStore::new(&files).expect("Failed to create LazyDataStore")
    });
}

/// Benchmark: get_container_stats for first metric (includes data loading)
///
/// This measures a cold query - loading data from parquet and computing stats.
#[divan::bench]
fn get_container_stats_cold(bencher: Bencher) {
    let files = get_parquet_files();
    if files.is_empty() {
        return;
    }

    bencher
        .with_inputs(|| {
            // Create fresh store for each iteration to measure cold path
            LazyDataStore::new(&files).expect("Failed to create LazyDataStore")
        })
        .bench_values(|store| {
            // Get first metric
            if let Some(metric) = store.index.metrics.first() {
                store
                    .get_container_stats(&metric.name)
                    .expect("Failed to get stats");
            }
        });
}

/// Benchmark: get_container_stats after warmup (cached)
///
/// This measures a warm query - data already loaded, just returning from cache.
#[divan::bench]
fn get_container_stats_warm(bencher: Bencher) {
    let files = get_parquet_files();
    if files.is_empty() {
        return;
    }

    let store = LazyDataStore::new(&files).expect("Failed to create LazyDataStore");
    let metric_name = store
        .index
        .metrics
        .first()
        .map(|m| m.name.clone())
        .unwrap_or_default();

    // Warm the cache
    if !metric_name.is_empty() {
        let _ = store.get_container_stats(&metric_name);
    }

    bencher.bench(|| {
        if !metric_name.is_empty() {
            store
                .get_container_stats(&metric_name)
                .expect("Failed to get stats");
        }
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
        .with_inputs(|| LazyDataStore::new(&files).expect("Failed to create LazyDataStore"))
        .bench_values(|store| {
            if let Some(metric) = store.index.metrics.first() {
                if let Some(container_id) = store.index.containers.keys().next() {
                    let _ = store.get_timeseries(&metric.name, &[container_id.as_str()]);
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
            let store = LazyDataStore::new(&files).expect("Failed to create LazyDataStore");
            let container_ids: Vec<String> = store.index.containers.keys().cloned().collect();
            (store, container_ids)
        })
        .bench_values(|(store, container_ids)| {
            if let Some(metric) = store.index.metrics.first() {
                let ids: Vec<&str> = container_ids.iter().map(|s| s.as_str()).collect();
                let _ = store.get_timeseries(&metric.name, &ids);
            }
        });
}

/// Benchmark: Loading all metrics sequentially (simulates full dashboard load)
#[divan::bench]
fn load_all_metrics(bencher: Bencher) {
    let files = get_parquet_files();
    if files.is_empty() {
        return;
    }

    bencher
        .with_inputs(|| LazyDataStore::new(&files).expect("Failed to create LazyDataStore"))
        .bench_values(|store| {
            for metric in &store.index.metrics {
                let _ = store.get_container_stats(&metric.name);
            }
        });
}
