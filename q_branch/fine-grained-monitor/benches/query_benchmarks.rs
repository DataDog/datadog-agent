//! Query benchmarks for fine-grained-monitor.
//!
//! Benchmarks cover:
//! - Startup: scan_metadata (parquet vs sidecar fast path)
//! - Index queries: list_metrics, list_containers (in-memory, instant)
//! - Timeseries queries: single container, all containers (parquet reads)
//! - Studies: periodicity, changepoint analysis algorithms
//! - MCP patterns: analyze_container, summarize_container (compound queries)
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
//! cargo bench -- scan_metadata        # startup benchmarks
//! cargo bench -- study_               # study algorithm benchmarks
//! cargo bench -- mcp_                 # MCP pattern benchmarks
//! cargo bench -- index_               # in-memory index benchmarks
//! ```
//!
//! Use BENCH_DATA env var to select scenario:
//! ```bash
//! BENCH_DATA=testdata/bench/realistic cargo bench
//! BENCH_DATA=testdata/bench/stress cargo bench
//! ```
//!
//! ## Determinism
//!
//! Benchmarks use deterministic selection of metrics and containers (sorted by name/ID)
//! to ensure consistent results across runs. This avoids variability from HashMap
//! iteration order or recency-based sorting.

use divan::Bencher;
use fine_grained_monitor::metrics_viewer::data::TimeRange;
use fine_grained_monitor::metrics_viewer::studies::{
    changepoint::ChangepointStudy, periodicity::PeriodicityStudy, Study,
};
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

/// Get all parquet files in the benchmark data directory (recursively).
/// Files are sorted by path for deterministic ordering.
fn get_parquet_files() -> Vec<PathBuf> {
    let dir = get_bench_data_dir();
    // Try recursive pattern first (for realistic scenarios with dt=/identifier= structure)
    let recursive_pattern = format!("{}/**/*.parquet", dir.display());
    let mut files: Vec<PathBuf> = glob::glob(&recursive_pattern)
        .expect("Failed to read glob pattern")
        .filter_map(Result::ok)
        .collect();

    if files.is_empty() {
        // Fall back to flat pattern for legacy scenarios
        let flat_pattern = format!("{}/*.parquet", dir.display());
        files = glob::glob(&flat_pattern)
            .expect("Failed to read glob pattern")
            .filter_map(Result::ok)
            .collect();
    }

    // Sort for deterministic ordering
    files.sort();
    files
}

/// Get the first metric name sorted alphabetically (deterministic selection).
fn get_first_metric_sorted(store: &LazyDataStore) -> Option<String> {
    let mut metrics = store.get_metrics();
    metrics.sort_by(|a, b| a.name.cmp(&b.name));
    metrics.first().map(|m| m.name.clone())
}

/// Get the first container ID sorted alphabetically (deterministic selection).
fn get_first_container_sorted(store: &LazyDataStore) -> Option<String> {
    let mut containers = store.get_containers_by_recency(TimeRange::All);
    containers.sort_by(|a, b| a.id.cmp(&b.id));
    containers.first().map(|c| c.id.clone())
}

/// Get all container IDs sorted alphabetically (deterministic ordering).
fn get_all_containers_sorted(store: &LazyDataStore) -> Vec<String> {
    let mut containers = store.get_containers_by_recency(TimeRange::All);
    containers.sort_by(|a, b| a.id.cmp(&b.id));
    containers.into_iter().map(|c| c.id).collect()
}

/// Get the first N metric names sorted alphabetically (deterministic selection).
fn get_first_n_metrics_sorted(store: &LazyDataStore, n: usize) -> Vec<String> {
    let mut metrics = store.get_metrics();
    metrics.sort_by(|a, b| a.name.cmp(&b.name));
    metrics.into_iter().take(n).map(|m| m.name).collect()
}

/// Create a store for benchmarking, with error message if data missing
fn create_store() -> Option<LazyDataStore> {
    let dir = get_bench_data_dir();
    match LazyDataStore::new(dir.clone()) {
        Ok(store) => Some(store),
        Err(_) => {
            eprintln!("No benchmark data found at {:?}", dir);
            eprintln!(
                "Run: cargo run --release --bin generate-bench-data -- --scenario realistic"
            );
            None
        }
    }
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
        .with_inputs(|| {
            let store =
                LazyDataStore::from_files(&files).expect("Failed to create LazyDataStore");
            let metric = get_first_metric_sorted(&store);
            let container = get_first_container_sorted(&store);
            (store, metric, container)
        })
        .bench_values(|(store, metric, container)| {
            if let (Some(metric_name), Some(container_id)) = (metric, container) {
                let _ =
                    store.get_timeseries(&metric_name, &[container_id.as_str()], TimeRange::All);
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
            let store =
                LazyDataStore::from_files(&files).expect("Failed to create LazyDataStore");
            let metric = get_first_metric_sorted(&store);
            let container_ids = get_all_containers_sorted(&store);
            (store, metric, container_ids)
        })
        .bench_values(|(store, metric, container_ids)| {
            if let Some(metric_name) = metric {
                let ids: Vec<&str> = container_ids.iter().map(|s| s.as_str()).collect();
                let _ = store.get_timeseries(&metric_name, &ids, TimeRange::All);
            }
        });
}

// =============================================================================
// INDEX BENCHMARKS - In-memory operations, should be <1ms
// =============================================================================

/// Benchmark: get_metrics() - Returns list of available metrics
///
/// MCP pattern: list_metrics tool. Should be instant (in-memory index read).
#[divan::bench]
fn index_list_metrics(bencher: Bencher) {
    let store = match create_store() {
        Some(s) => s,
        None => return,
    };

    bencher.bench(|| store.get_metrics());
}

/// Benchmark: get_containers_by_recency() - Returns containers sorted by last seen
///
/// MCP pattern: list_containers tool. Should be instant (in-memory index read + sort).
#[divan::bench]
fn index_list_containers(bencher: Bencher) {
    let store = match create_store() {
        Some(s) => s,
        None => return,
    };

    bencher.bench(|| store.get_containers_by_recency(TimeRange::All));
}

/// Benchmark: get_containers_by_recency with 1-hour time filter
///
/// Tests time-range filtering at the index level (no parquet reads).
#[divan::bench]
fn index_list_containers_1h(bencher: Bencher) {
    let store = match create_store() {
        Some(s) => s,
        None => return,
    };

    bencher.bench(|| store.get_containers_by_recency(TimeRange::Hour1));
}

// =============================================================================
// STUDY BENCHMARKS - Algorithm performance (isolates algorithm from I/O)
// =============================================================================

/// Benchmark: Periodicity detection algorithm
///
/// Analyzes timeseries for periodic patterns using autocorrelation.
/// Uses with_inputs to pre-load timeseries data.
/// Uses deterministic metric/container selection (sorted alphabetically).
#[divan::bench]
fn study_periodicity(bencher: Bencher) {
    let store = match create_store() {
        Some(s) => s,
        None => return,
    };

    let metric = get_first_metric_sorted(&store);
    let container = get_first_container_sorted(&store);

    let timeseries = if let (Some(metric_name), Some(container_id)) = (metric, container) {
        store
            .get_timeseries(&metric_name, &[container_id.as_str()], TimeRange::All)
            .ok()
            .and_then(|mut ts| ts.pop())
            .map(|(_, points)| points)
    } else {
        None
    };

    let timeseries = match timeseries {
        Some(ts) if !ts.is_empty() => ts,
        _ => {
            eprintln!("No timeseries data available for periodicity benchmark");
            return;
        }
    };

    bencher
        .with_inputs(|| timeseries.clone())
        .bench_values(|ts| {
            let study = PeriodicityStudy::default();
            study.analyze(&ts)
        });
}

/// Benchmark: Changepoint detection algorithm (PELT)
///
/// Uses PELT (Pruned Exact Linear Time) changepoint detection.
/// Uses with_inputs to pre-load timeseries data.
/// Uses deterministic metric/container selection (sorted alphabetically).
#[divan::bench]
fn study_changepoint(bencher: Bencher) {
    let store = match create_store() {
        Some(s) => s,
        None => return,
    };

    let metric = get_first_metric_sorted(&store);
    let container = get_first_container_sorted(&store);

    let timeseries = if let (Some(metric_name), Some(container_id)) = (metric, container) {
        store
            .get_timeseries(&metric_name, &[container_id.as_str()], TimeRange::All)
            .ok()
            .and_then(|mut ts| ts.pop())
            .map(|(_, points)| points)
    } else {
        None
    };

    let timeseries = match timeseries {
        Some(ts) if !ts.is_empty() => ts,
        _ => {
            eprintln!("No timeseries data available for changepoint benchmark");
            return;
        }
    };

    bencher
        .with_inputs(|| timeseries.clone())
        .bench_values(|ts| {
            let study = ChangepointStudy::default();
            study.analyze(&ts)
        });
}

// =============================================================================
// MCP PATTERN BENCHMARKS - Compound queries matching real MCP tool usage
// =============================================================================

/// Benchmark: analyze_container with single metric
///
/// MCP pattern: Get timeseries + run one study for a single container.
/// Uses deterministic metric/container selection (sorted alphabetically).
#[divan::bench]
fn mcp_analyze_single_metric(bencher: Bencher) {
    let store = match create_store() {
        Some(s) => s,
        None => return,
    };

    let metric_name = match get_first_metric_sorted(&store) {
        Some(m) => m,
        None => {
            eprintln!("No metrics for mcp_analyze_single_metric benchmark");
            return;
        }
    };

    let container_id = match get_first_container_sorted(&store) {
        Some(c) => c,
        None => {
            eprintln!("No containers for mcp_analyze_single_metric benchmark");
            return;
        }
    };

    let study = ChangepointStudy::default();

    bencher.bench(|| {
        let ts_result =
            store.get_timeseries(&metric_name, &[container_id.as_str()], TimeRange::All);
        if let Ok(mut ts) = ts_result {
            if let Some((_, points)) = ts.pop() {
                let _ = study.analyze(&points);
            }
        }
    });
}

/// Benchmark: analyze_container with metric_prefix (multi-metric)
///
/// MCP pattern: List metrics matching prefix, then analyze each.
/// Simulates metric_prefix="cgroup.v2.cpu" which matches multiple metrics.
/// Uses deterministic metric/container selection (sorted alphabetically).
#[divan::bench]
fn mcp_analyze_metric_prefix(bencher: Bencher) {
    let store = match create_store() {
        Some(s) => s,
        None => return,
    };

    let container_id = match get_first_container_sorted(&store) {
        Some(c) => c,
        None => {
            eprintln!("No containers for mcp_analyze_metric_prefix benchmark");
            return;
        }
    };

    // Get first 5 metrics (sorted) to simulate prefix match
    let matching_metrics = get_first_n_metrics_sorted(&store, 5);

    if matching_metrics.is_empty() {
        eprintln!("No metrics for mcp_analyze_metric_prefix benchmark");
        return;
    }

    let study = ChangepointStudy::default();

    bencher.bench(|| {
        for metric_name in &matching_metrics {
            let ts_result =
                store.get_timeseries(metric_name, &[container_id.as_str()], TimeRange::All);
            if let Ok(mut ts) = ts_result {
                if let Some((_, points)) = ts.pop() {
                    let _ = study.analyze(&points);
                }
            }
        }
    });
}

/// Key metrics for container health summary
const KEY_METRICS: &[&str] = &[
    "cgroup.v2.cpu.stat.usage_usec",
    "cgroup.v2.cpu.stat.user_usec",
    "cgroup.v2.cpu.stat.system_usec",
    "cgroup.v2.cpu.stat.throttled_usec",
    "cgroup.v2.memory.current",
    "cgroup.v2.memory.stat.anon",
    "cgroup.v2.memory.stat.file",
    "cgroup.v2.memory.swap.current",
    "cgroup.v2.io.stat.rbytes",
    "cgroup.v2.io.stat.wbytes",
    "cgroup.v2.pids.current",
    "cpu_percentage",
];

/// Benchmark: summarize_container (12 key metrics)
///
/// MCP pattern: Fetch 12 KEY_METRICS for quick container health summary.
/// No studies run - just timeseries fetching.
/// Uses deterministic container selection (sorted alphabetically).
#[divan::bench]
fn mcp_summarize_container(bencher: Bencher) {
    let store = match create_store() {
        Some(s) => s,
        None => return,
    };

    let container_id = match get_first_container_sorted(&store) {
        Some(c) => c,
        None => {
            eprintln!("No containers for mcp_summarize_container benchmark");
            return;
        }
    };

    // Find which key metrics actually exist in the data
    let available_metrics = store.get_metrics();
    let available_names: std::collections::HashSet<&str> =
        available_metrics.iter().map(|m| m.name.as_str()).collect();

    let metrics_to_fetch: Vec<&str> = KEY_METRICS
        .iter()
        .filter(|m| available_names.contains(*m))
        .copied()
        .collect();

    // If no key metrics match, fall back to first 12 available metrics (sorted)
    let metrics_to_fetch: Vec<String> = if metrics_to_fetch.is_empty() {
        get_first_n_metrics_sorted(&store, 12)
    } else {
        // Sort key metrics for determinism
        let mut sorted: Vec<String> = metrics_to_fetch.iter().map(|s| s.to_string()).collect();
        sorted.sort();
        sorted
    };

    if metrics_to_fetch.is_empty() {
        eprintln!("No metrics available for mcp_summarize_container benchmark");
        return;
    }

    bencher.bench(|| {
        for metric_name in &metrics_to_fetch {
            let _ = store.get_timeseries(metric_name, &[container_id.as_str()], TimeRange::All);
        }
    });
}

/// Benchmark: Full analyze_container flow with both studies
///
/// Most expensive MCP pattern: single metric analyzed with BOTH studies.
/// Uses deterministic metric/container selection (sorted alphabetically).
#[divan::bench]
fn mcp_analyze_all_studies(bencher: Bencher) {
    let store = match create_store() {
        Some(s) => s,
        None => return,
    };

    let metric_name = match get_first_metric_sorted(&store) {
        Some(m) => m,
        None => {
            eprintln!("No metrics for mcp_analyze_all_studies benchmark");
            return;
        }
    };

    let container_id = match get_first_container_sorted(&store) {
        Some(c) => c,
        None => {
            eprintln!("No containers for mcp_analyze_all_studies benchmark");
            return;
        }
    };

    let periodicity = PeriodicityStudy::default();
    let changepoint = ChangepointStudy::default();

    bencher.bench(|| {
        let ts_result =
            store.get_timeseries(&metric_name, &[container_id.as_str()], TimeRange::All);
        if let Ok(mut ts) = ts_result {
            if let Some((_, points)) = ts.pop() {
                let _ = periodicity.analyze(&points);
                let _ = changepoint.analyze(&points);
            }
        }
    });
}
