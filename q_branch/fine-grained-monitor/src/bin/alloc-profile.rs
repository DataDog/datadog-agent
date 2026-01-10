//! Allocation profiling binary - counts allocations during get_timeseries hot path.
//!
//! Uses a counting allocator to measure allocation behavior.

use anyhow::Result;
use fine_grained_monitor::metrics_viewer::data::TimeRange;
use fine_grained_monitor::metrics_viewer::LazyDataStore;
use std::alloc::{GlobalAlloc, Layout, System};
use std::path::PathBuf;
use std::sync::atomic::{AtomicUsize, Ordering};

/// Counting allocator that wraps the system allocator
struct CountingAlloc;

static ALLOC_COUNT: AtomicUsize = AtomicUsize::new(0);
static ALLOC_BYTES: AtomicUsize = AtomicUsize::new(0);
static DEALLOC_COUNT: AtomicUsize = AtomicUsize::new(0);
static DEALLOC_BYTES: AtomicUsize = AtomicUsize::new(0);

unsafe impl GlobalAlloc for CountingAlloc {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        ALLOC_COUNT.fetch_add(1, Ordering::Relaxed);
        ALLOC_BYTES.fetch_add(layout.size(), Ordering::Relaxed);
        unsafe { System.alloc(layout) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        DEALLOC_COUNT.fetch_add(1, Ordering::Relaxed);
        DEALLOC_BYTES.fetch_add(layout.size(), Ordering::Relaxed);
        unsafe { System.dealloc(ptr, layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        // realloc counts as dealloc + alloc
        DEALLOC_COUNT.fetch_add(1, Ordering::Relaxed);
        DEALLOC_BYTES.fetch_add(layout.size(), Ordering::Relaxed);
        ALLOC_COUNT.fetch_add(1, Ordering::Relaxed);
        ALLOC_BYTES.fetch_add(new_size, Ordering::Relaxed);
        unsafe { System.realloc(ptr, layout, new_size) }
    }
}

#[global_allocator]
static GLOBAL: CountingAlloc = CountingAlloc;

fn reset_counters() {
    ALLOC_COUNT.store(0, Ordering::Relaxed);
    ALLOC_BYTES.store(0, Ordering::Relaxed);
    DEALLOC_COUNT.store(0, Ordering::Relaxed);
    DEALLOC_BYTES.store(0, Ordering::Relaxed);
}

fn get_counters() -> (usize, usize, usize, usize) {
    (
        ALLOC_COUNT.load(Ordering::Relaxed),
        ALLOC_BYTES.load(Ordering::Relaxed),
        DEALLOC_COUNT.load(Ordering::Relaxed),
        DEALLOC_BYTES.load(Ordering::Relaxed),
    )
}

fn main() -> Result<()> {
    let dir = PathBuf::from("testdata/bench/realistic");

    eprintln!("Loading store from {:?}...", dir);
    let store = LazyDataStore::new(dir)?;

    let metrics = store.get_metrics();
    let containers = store.get_containers_by_recency(TimeRange::All);

    let metric_name = metrics.first().map(|m| m.name.clone()).unwrap_or_default();
    let container_ids: Vec<String> = containers.iter().take(10).map(|c| c.id.clone()).collect();
    let id_refs: Vec<&str> = container_ids.iter().map(|s| s.as_str()).collect();

    eprintln!("Metric: {}", metric_name);
    eprintln!("Containers: {}", container_ids.len());

    // Warm up - load data once
    eprintln!("\n=== WARMUP ===");
    store.clear_cache();
    let _ = store.get_timeseries(&metric_name, &id_refs, TimeRange::All);

    // Profile 5 iterations of the hot path
    eprintln!("\n=== PROFILING HOT PATH (5 iterations) ===");

    let mut total_allocs = 0usize;
    let mut total_bytes = 0usize;
    let mut total_deallocs = 0usize;
    let mut total_dealloc_bytes = 0usize;

    for i in 0..5 {
        store.clear_cache();
        reset_counters();

        let result = store.get_timeseries(&metric_name, &id_refs, TimeRange::All);

        let (allocs, bytes, deallocs, dealloc_bytes) = get_counters();
        total_allocs += allocs;
        total_bytes += bytes;
        total_deallocs += deallocs;
        total_dealloc_bytes += dealloc_bytes;

        if let Ok(data) = result {
            let total_points: usize = data.iter().map(|(_, pts)| pts.len()).sum();
            eprintln!(
                "Iter {}: {} allocs ({:.1} KB), {} deallocs ({:.1} KB) | {} containers, {} points",
                i + 1,
                allocs,
                bytes as f64 / 1024.0,
                deallocs,
                dealloc_bytes as f64 / 1024.0,
                data.len(),
                total_points
            );
        }
    }

    eprintln!("\n=== SUMMARY (5 iterations) ===");
    eprintln!(
        "Total allocations:   {} ({:.2} MB)",
        total_allocs,
        total_bytes as f64 / 1024.0 / 1024.0
    );
    eprintln!(
        "Total deallocations: {} ({:.2} MB)",
        total_deallocs,
        total_dealloc_bytes as f64 / 1024.0 / 1024.0
    );
    eprintln!("Average per iteration:");
    eprintln!(
        "  Allocations:   {} ({:.1} KB)",
        total_allocs / 5,
        total_bytes as f64 / 5.0 / 1024.0
    );
    eprintln!(
        "  Deallocations: {} ({:.1} KB)",
        total_deallocs / 5,
        total_dealloc_bytes as f64 / 5.0 / 1024.0
    );

    // Output machine-readable summary
    println!("ALLOC_COUNT={}", total_allocs / 5);
    println!("ALLOC_BYTES={}", total_bytes / 5);
    println!("DEALLOC_COUNT={}", total_deallocs / 5);
    println!("DEALLOC_BYTES={}", total_dealloc_bytes / 5);

    Ok(())
}
