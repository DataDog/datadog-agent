//! Lazy-loading parquet data store for metrics viewer.
//!
//! Two-phase loading approach:
//! - Phase 1 (startup): Fast metadata scan - metric names, containers, counts
//! - Phase 2 (on-demand): Load actual timeseries data when requested
//!
//! REQ-MV-012: Supports index-based fast startup for in-cluster viewer.
//!
//! This dramatically reduces startup time for large parquet files.
//!
//! ## Label Schema
//!
//! Uses flattened `l_<key>` columns (e.g., `l_container_id`, `l_namespace`):
//! - Enables predicate pushdown for efficient filtering
//! - Each label is a nullable Utf8 column

use anyhow::{Context, Result};
use arrow::array::{Array, BooleanArray, Float64Array, StringArray, UInt64Array};
use arrow::datatypes::DataType;
use chrono::{DateTime, Duration, Utc};
use parquet::arrow::arrow_reader::{ArrowPredicateFn, ParquetRecordBatchReaderBuilder, RowFilter};
use parquet::file::properties::ReaderProperties;
use parquet::file::reader::{FileReader, SerializedFileReader};
use parquet::file::serialized_reader::ReadOptionsBuilder;
use rayon::prelude::*;
use std::collections::{HashMap, HashSet};
use std::fs::{self, File};
use std::path::{Path, PathBuf};
use std::sync::{Arc, RwLock};

use super::data::{ContainerInfo, ContainerStats, MetricInfo, TimeseriesPoint};
use crate::index::{ContainerIndex, DataRange};

/// Metrics that represent cumulative counters (need rate conversion).
const CUMULATIVE_METRICS: &[&str] = &[
    "cgroup.v2.cpu.stat.usage_usec",
    "cgroup.v2.cpu.stat.user_usec",
    "cgroup.v2.cpu.stat.system_usec",
    "cgroup.v2.io.stat.rbytes",
    "cgroup.v2.io.stat.wbytes",
    "cgroup.v2.io.stat.rios",
    "cgroup.v2.io.stat.wios",
];

fn is_cumulative(metric_name: &str) -> bool {
    CUMULATIVE_METRICS.iter().any(|m| metric_name.contains(m))
}

/// Default time window for data loading (1 hour).
/// Limits initial data load to recent files for faster startup.
const DEFAULT_LOOKBACK_HOURS: i64 = 1;

/// Discover parquet files using time-range based path computation.
/// REQ-MV-012: Avoids expensive glob operations by computing paths from timestamps.
///
/// Instead of globbing `**/*.parquet`, this function:
/// 1. Determines which date directories to scan based on time range
/// 2. Lists identifier subdirectories in each date directory
/// 3. Lists parquet files in each identifier directory
/// 4. Filters to files within the requested time range based on filename timestamps
fn discover_files_by_time_range(
    data_dir: &Path,
    data_range: &DataRange,
    lookback_hours: Option<i64>,
) -> Vec<PathBuf> {
    let start_time = std::time::Instant::now();

    // Determine time range to load
    let lookback = Duration::hours(lookback_hours.unwrap_or(DEFAULT_LOOKBACK_HOURS));
    let now = Utc::now();
    let earliest_wanted = now - lookback;

    // Use data_range to bound our search, but don't go earlier than lookback window
    let search_start = match data_range.earliest {
        Some(earliest) => earliest.max(earliest_wanted),
        None => earliest_wanted,
    };
    let search_end = data_range.latest.min(now);

    tracing::debug!(
        "[PERF] Time-range discovery: {} to {} ({} hours lookback)",
        search_start.format("%Y-%m-%dT%H:%M:%S"),
        search_end.format("%Y-%m-%dT%H:%M:%S"),
        lookback.num_hours()
    );

    // Determine which date directories to scan
    let mut dates_to_scan = Vec::new();
    let mut current_date = search_start.date_naive();
    let end_date = search_end.date_naive();
    while current_date <= end_date {
        dates_to_scan.push(current_date);
        current_date = current_date.succ_opt().unwrap_or(current_date);
    }

    let mut parquet_files = Vec::new();
    let mut dirs_scanned = 0;
    let mut files_found = 0;
    let mut files_filtered = 0;

    for date in &dates_to_scan {
        let date_dir = data_dir.join(format!("dt={}", date.format("%Y-%m-%d")));
        if !date_dir.exists() {
            continue;
        }

        // List identifier subdirectories
        let identifier_dirs = match fs::read_dir(&date_dir) {
            Ok(entries) => entries,
            Err(_) => continue,
        };

        for entry in identifier_dirs.filter_map(Result::ok) {
            let id_path = entry.path();
            if !id_path.is_dir() {
                continue;
            }

            // Check if it's an identifier directory
            if let Some(name) = id_path.file_name().and_then(|n| n.to_str()) {
                if !name.starts_with("identifier=") {
                    continue;
                }
            }

            dirs_scanned += 1;

            // List parquet files in this identifier directory
            let files = match fs::read_dir(&id_path) {
                Ok(entries) => entries,
                Err(_) => continue,
            };

            for file_entry in files.filter_map(Result::ok) {
                let file_path = file_entry.path();
                if !file_path.is_file() {
                    continue;
                }

                let file_name = match file_path.file_name().and_then(|n| n.to_str()) {
                    Some(name) => name,
                    None => continue,
                };

                // Only consider parquet files
                if !file_name.ends_with(".parquet") {
                    continue;
                }

                files_found += 1;

                // Parse timestamp from filename to filter by time range
                if let Some(file_time) = parse_file_timestamp(file_name) {
                    // For consolidated files, use the end timestamp for filtering
                    if file_time >= search_start && file_time <= search_end {
                        parquet_files.push(file_path);
                    } else {
                        files_filtered += 1;
                    }
                } else {
                    // If we can't parse timestamp, include the file to be safe
                    parquet_files.push(file_path);
                }
            }
        }
    }

    // Fix #4: Extract mtime once per file, then sort (avoids O(n log n) syscalls)
    // Sort by modification time (newest first) for better cache behavior
    let mut files_with_mtime: Vec<_> = parquet_files
        .into_iter()
        .map(|p| {
            let mtime = p.metadata().and_then(|m| m.modified()).ok();
            (p, mtime)
        })
        .collect();
    files_with_mtime.sort_unstable_by(|(_, a_time), (_, b_time)| b_time.cmp(a_time));
    let parquet_files: Vec<PathBuf> = files_with_mtime.into_iter().map(|(p, _)| p).collect();

    let elapsed = start_time.elapsed();
    tracing::debug!(
        "[PERF] Time-range discovery complete: {} dirs, {} files found, {} filtered out, {} selected in {:.1}ms",
        dirs_scanned,
        files_found,
        files_filtered,
        parquet_files.len(),
        elapsed.as_secs_f64() * 1000.0
    );

    parquet_files
}

/// Parse timestamp from parquet filename.
/// Handles both formats:
/// - metrics-20251230T120000Z.parquet -> single timestamp
/// - consolidated-20251230T120000Z-20251230T130000Z.parquet -> uses end timestamp
fn parse_file_timestamp(filename: &str) -> Option<DateTime<Utc>> {
    if filename.starts_with("metrics-") {
        // Format: metrics-YYYYMMDDTHHMMSSZ.parquet
        let ts_part = filename
            .strip_prefix("metrics-")?
            .strip_suffix(".parquet")?;
        parse_iso_compact(ts_part)
    } else if filename.starts_with("consolidated-") {
        // Format: consolidated-START-END.parquet, use END timestamp
        let rest = filename
            .strip_prefix("consolidated-")?
            .strip_suffix(".parquet")?;
        // Find the second timestamp (after the hyphen between timestamps)
        // Format: YYYYMMDDTHHMMSSZ-YYYYMMDDTHHMMSSZ
        if rest.len() >= 31 {
            // 15 chars for first timestamp + 1 hyphen + 15 chars for second
            let end_ts = &rest[16..]; // Skip first timestamp and hyphen
            parse_iso_compact(end_ts)
        } else {
            None
        }
    } else {
        None
    }
}

/// Parse compact ISO 8601 timestamp: YYYYMMDDTHHMMSSZ
fn parse_iso_compact(s: &str) -> Option<DateTime<Utc>> {
    if s.len() < 15 {
        return None;
    }

    let year: i32 = s[0..4].parse().ok()?;
    let month: u32 = s[4..6].parse().ok()?;
    let day: u32 = s[6..8].parse().ok()?;
    // Skip 'T' at position 8
    let hour: u32 = s[9..11].parse().ok()?;
    let min: u32 = s[11..13].parse().ok()?;
    let sec: u32 = s[13..15].parse().ok()?;

    chrono::NaiveDate::from_ymd_opt(year, month, day)?
        .and_hms_opt(hour, min, sec)?
        .and_utc()
        .into()
}

/// Minimum interval between file refresh operations (in milliseconds).
const REFRESH_STALENESS_MS: u64 = 5000;

/// Lazy-loading data store with on-demand parquet reads.
pub struct LazyDataStore {
    /// Paths to parquet files (refreshed on data load).
    paths: RwLock<Vec<PathBuf>>,
    /// Data directory for file discovery (None for static file list).
    data_dir: Option<PathBuf>,
    /// Metadata index (loaded at startup).
    pub index: MetadataIndex,
    /// Cached timeseries data: metric -> container -> points.
    timeseries_cache: RwLock<HashMap<String, HashMap<String, Vec<TimeseriesPoint>>>>,
    /// Fix #10: Cached stats wrapped in Arc to avoid deep clones on cache hit.
    stats_cache: RwLock<HashMap<String, Arc<HashMap<String, ContainerStats>>>>,
    /// Fix #9: Last refresh timestamp to avoid redundant file discovery.
    last_refresh: RwLock<Option<std::time::Instant>>,
}

/// Metadata index built during fast startup scan.
pub struct MetadataIndex {
    /// Available metrics with sample counts.
    pub metrics: Vec<MetricInfo>,
    /// Unique QoS classes found.
    pub qos_classes: Vec<String>,
    /// Unique namespaces found.
    pub namespaces: Vec<String>,
    /// Container info by short_id.
    pub containers: HashMap<String, ContainerInfo>,
    /// Which containers have data for which metrics: metric -> set of short_ids.
    pub metric_containers: HashMap<String, HashSet<String>>,
    /// Which containers exist in each file (for file-level pruning).
    /// Maps file path -> set of container short_ids found in that file.
    pub file_containers: HashMap<PathBuf, HashSet<String>>,
}

impl LazyDataStore {
    /// Create a new lazy data store by scanning metadata only.
    /// This is much faster than loading all data upfront.
    pub fn new<P: AsRef<Path>>(paths: &[P]) -> Result<Self> {
        let paths: Vec<PathBuf> = paths.iter().map(|p| p.as_ref().to_path_buf()).collect();
        let index = scan_metadata(&paths)?;

        Ok(Self {
            paths: RwLock::new(paths),
            data_dir: None, // Static file list, no refresh
            index,
            timeseries_cache: RwLock::new(HashMap::new()),
            stats_cache: RwLock::new(HashMap::new()),
            last_refresh: RwLock::new(None),
        })
    }

    /// REQ-MV-012: Create from container index for instant startup.
    /// Uses index for container metadata, reads metrics from parquet schema.
    pub fn from_index(container_index: ContainerIndex, data_dir: PathBuf) -> Result<Self> {
        tracing::debug!("Building metadata from index...");

        // REQ-MV-012: Use time-range based discovery instead of expensive glob
        // This dramatically reduces startup time by only scanning relevant date directories
        let parquet_files = discover_files_by_time_range(&data_dir, &container_index.data_range, None);
        tracing::debug!("Using {} parquet files from time-range discovery", parquet_files.len());

        // Get metric names from parquet schema - try multiple files
        let mut metrics = Vec::new();
        for file in &parquet_files {
            match read_metrics_from_schema(file) {
                Ok(m) => {
                    metrics = m;
                    tracing::debug!("Read {} metrics from schema of {:?}", metrics.len(), file);
                    break;
                }
                Err(e) => {
                    tracing::debug!("Skipping {:?}: {}", file, e);
                    continue;
                }
            }
        }
        if metrics.is_empty() && !parquet_files.is_empty() {
            tracing::debug!("Warning: Could not read metrics from any parquet file");
        }

        // Build container info from index
        // REQ-MV-016: Use pod_name and namespace from enriched index
        let mut containers = HashMap::new();
        let mut qos_classes = HashSet::new();
        let mut namespace_set = HashSet::new();

        for (short_id, entry) in &container_index.containers {
            containers.insert(
                short_id.clone(),
                ContainerInfo {
                    id: entry.full_id.clone(),
                    short_id: short_id.clone(),
                    qos_class: Some(entry.qos_class.clone()),
                    namespace: entry.namespace.clone(),
                    pod_name: entry.pod_name.clone(),
                    container_name: entry.container_name.clone(),
                    // REQ-MV-035: Store first_seen for time range computation
                    first_seen_ms: Some(entry.first_seen.timestamp_millis()),
                    // REQ-MV-019: Store last_seen for sorting
                    last_seen_ms: Some(entry.last_seen.timestamp_millis()),
                    // REQ-MV-032: Pod labels from Kubernetes API
                    labels: entry.labels.clone(),
                },
            );
            qos_classes.insert(entry.qos_class.clone());
            if let Some(ref ns) = entry.namespace {
                namespace_set.insert(ns.clone());
            }
        }

        // Build metric_containers map (all containers for all metrics initially)
        let all_container_ids: HashSet<String> = containers.keys().cloned().collect();
        let metric_containers: HashMap<String, HashSet<String>> = metrics
            .iter()
            .map(|m| (m.name.clone(), all_container_ids.clone()))
            .collect();

        // REQ-MV-016: Include namespaces from enriched index
        let mut namespaces: Vec<String> = namespace_set.into_iter().collect();
        namespaces.sort();

        let index = MetadataIndex {
            metrics,
            qos_classes: qos_classes.into_iter().collect(),
            namespaces,
            containers,
            metric_containers,
            // Note: file_containers is empty for index-based startup
            // File-level pruning will be disabled until files are scanned
            file_containers: HashMap::new(),
        };

        tracing::debug!(
            "Index-based startup complete: {} metrics, {} containers",
            index.metrics.len(),
            index.containers.len()
        );

        Ok(Self {
            paths: RwLock::new(parquet_files), // Start with discovered files
            data_dir: Some(data_dir),          // Store for refresh
            index,
            timeseries_cache: RwLock::new(HashMap::new()),
            stats_cache: RwLock::new(HashMap::new()),
            last_refresh: RwLock::new(Some(std::time::Instant::now())), // Just refreshed
        })
    }

    /// Refresh the file list by re-discovering parquet files.
    /// This allows the viewer to see newly written files.
    /// Fix #9: Skips refresh if called within REFRESH_STALENESS_MS of last refresh.
    fn refresh_files(&self) {
        if let Some(ref data_dir) = self.data_dir {
            // Check staleness - skip if refreshed recently
            {
                let last = self.last_refresh.read().unwrap();
                if let Some(last_time) = *last {
                    if last_time.elapsed().as_millis() < REFRESH_STALENESS_MS as u128 {
                        return; // Still fresh, skip
                    }
                }
            }

            let start = std::time::Instant::now();

            // Create a default DataRange for discovery (uses lookback window)
            let data_range = DataRange {
                earliest: None,
                latest: Utc::now(),
                rotation_interval_sec: 90, // Default flush interval
            };

            let new_files = discover_files_by_time_range(data_dir, &data_range, None);
            let new_count = new_files.len();

            let mut paths = self.paths.write().unwrap();
            let old_count = paths.len();
            *paths = new_files;

            // Update last refresh time
            *self.last_refresh.write().unwrap() = Some(std::time::Instant::now());

            if new_count != old_count {
                tracing::debug!(
                    "[PERF] refresh_files: {} -> {} files in {:.1}ms",
                    old_count,
                    new_count,
                    start.elapsed().as_secs_f64() * 1000.0
                );
            }
        }
    }

    /// Get timeseries data for specific containers.
    /// Loads from parquet on first request, then caches.
    /// Automatically discovers new parquet files before loading.
    pub fn get_timeseries(
        &self,
        metric: &str,
        container_ids: &[&str],
    ) -> Result<Vec<(String, Vec<TimeseriesPoint>)>> {
        // Refresh file list to pick up newly written files
        self.refresh_files();

        // Check what's already cached
        let mut result = Vec::new();
        let mut missing: Vec<&str> = Vec::new();

        {
            let cache = self.timeseries_cache.read().unwrap();
            if let Some(metric_cache) = cache.get(metric) {
                for &id in container_ids {
                    if let Some(points) = metric_cache.get(id) {
                        result.push((id.to_string(), points.clone()));
                    } else {
                        missing.push(id);
                    }
                }
            } else {
                missing.extend(container_ids);
            }
        }

        // Load missing data
        if !missing.is_empty() {
            let all_paths = self.paths.read().unwrap();

            // File-level pruning: only process files that contain requested containers
            let missing_set: HashSet<&str> = missing.iter().copied().collect();
            let paths: Vec<PathBuf> = if self.index.file_containers.is_empty() {
                // No file_containers index available, use all files
                all_paths.clone()
            } else {
                all_paths
                    .iter()
                    .filter(|path| {
                        self.index
                            .file_containers
                            .get(*path)
                            .map(|containers| containers.iter().any(|c| missing_set.contains(c.as_str())))
                            .unwrap_or(true) // Include files not in index (safety)
                    })
                    .cloned()
                    .collect()
            };

            tracing::debug!(
                "[PERF] File-level pruning: {} -> {} files for {} containers",
                all_paths.len(),
                paths.len(),
                missing.len()
            );

            let loaded = load_metric_data(&paths, metric, &missing)?;

            // Cache the loaded data
            {
                let mut cache = self.timeseries_cache.write().unwrap();
                let metric_cache = cache.entry(metric.to_string()).or_default();
                for (id, points) in &loaded {
                    metric_cache.insert(id.clone(), points.clone());
                }
            }

            result.extend(loaded);
        }

        Ok(result)
    }

    /// Get container stats for a metric.
    /// Computes stats directly from parquet without materializing full timeseries.
    /// Returns Arc to avoid deep clones on cache hit.
    pub fn get_container_stats(&self, metric: &str) -> Result<Arc<HashMap<String, ContainerStats>>> {
        let total_start = std::time::Instant::now();

        // Check cache first - Arc::clone is cheap (just refcount increment)
        {
            let cache = self.stats_cache.read().unwrap();
            if let Some(stats) = cache.get(metric) {
                tracing::debug!(
                    "[PERF] get_container_stats('{}') cache HIT in {:.1}ms",
                    metric,
                    total_start.elapsed().as_secs_f64() * 1000.0
                );
                return Ok(Arc::clone(stats));
            }
        }

        tracing::debug!("[PERF] get_container_stats('{}') cache MISS - loading...", metric);

        // Refresh file list to pick up newly written files
        self.refresh_files();

        // Get all containers for this metric
        let container_ids: Vec<&str> = self
            .index
            .metric_containers
            .get(metric)
            .map(|set| set.iter().map(|s| s.as_str()).collect())
            .unwrap_or_default();

        tracing::debug!("[PERF]   {} containers to load", container_ids.len());

        if container_ids.is_empty() {
            return Ok(Arc::new(HashMap::new()));
        }

        // Load stats directly (no timeseries materialization)
        let paths = self.paths.read().unwrap();
        let raw_stats = load_metric_stats(&paths, metric, &container_ids)?;

        // Convert to ContainerStats with info from index
        let mut stats = HashMap::new();
        for (id, (sample_count, avg, max)) in raw_stats {
            if let Some(info) = self.index.containers.get(&id) {
                stats.insert(
                    id,
                    ContainerStats {
                        info: info.clone(),
                        avg,
                        max,
                    },
                );
            }
        }

        // Wrap in Arc for cache storage and return
        let stats = Arc::new(stats);

        // Cache the stats (cheap Arc::clone)
        {
            let mut cache = self.stats_cache.write().unwrap();
            cache.insert(metric.to_string(), Arc::clone(&stats));
        }

        tracing::debug!(
            "[PERF] get_container_stats('{}') TOTAL: {:.1}ms",
            metric,
            total_start.elapsed().as_secs_f64() * 1000.0
        );

        Ok(stats)
    }

    /// REQ-MV-019: Get containers sorted by last_seen (most recent first).
    /// This is instant as it only reads from the index, avoiding expensive parquet reads.
    pub fn get_containers_by_recency(&self) -> Vec<ContainerInfo> {
        let start = std::time::Instant::now();

        let mut containers: Vec<ContainerInfo> = self.index.containers.values().cloned().collect();

        // Sort by last_seen descending (most recent first)
        containers.sort_by(|a, b| {
            let a_time = a.last_seen_ms.unwrap_or(0);
            let b_time = b.last_seen_ms.unwrap_or(0);
            b_time.cmp(&a_time)
        });

        tracing::debug!(
            "[PERF] get_containers_by_recency: {} containers in {:.1}ms",
            containers.len(),
            start.elapsed().as_secs_f64() * 1000.0
        );

        containers
    }

    /// Clear all caches (useful for testing or memory pressure).
    #[allow(dead_code)]
    pub fn clear_cache(&self) {
        self.timeseries_cache.write().unwrap().clear();
        self.stats_cache.write().unwrap().clear();
    }
}

/// REQ-MV-012: Read metric names from a parquet file's metric_name column.
/// Samples the first row group to get unique metric names efficiently.
fn read_metrics_from_schema(path: &PathBuf) -> Result<Vec<MetricInfo>> {
    let file = File::open(path).context("Failed to open parquet file")?;

    // Check file size - skip files that are too small or being written
    if let Ok(metadata) = file.metadata() {
        if metadata.len() < 8 {
            anyhow::bail!("Parquet file too small (likely being written)");
        }
    }

    let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;
    let schema = builder.schema();
    let parquet_schema = builder.parquet_schema();

    // Project only the metric_name column for efficient reading
    let metric_name_idx = schema
        .index_of("metric_name")
        .context("Missing metric_name column in parquet schema")?;

    let projection_mask =
        parquet::arrow::ProjectionMask::roots(parquet_schema, vec![metric_name_idx]);

    // Only read first few row groups to get metric names (they're repeated)
    let num_row_groups = builder.metadata().num_row_groups();
    let row_groups_to_sample: Vec<usize> = if num_row_groups > 3 {
        vec![0, num_row_groups / 2, num_row_groups - 1]
    } else {
        (0..num_row_groups).collect()
    };

    let reader = builder
        .with_projection(projection_mask)
        .with_row_groups(row_groups_to_sample)
        .with_batch_size(65536)
        .build()?;

    let mut metric_set: HashSet<String> = HashSet::new();

    for batch_result in reader {
        let batch = batch_result?;

        let metric_names = batch
            .column_by_name("metric_name")
            .and_then(|c| c.as_any().downcast_ref::<StringArray>())
            .context("metric_name column is not a StringArray")?;

        for i in 0..metric_names.len() {
            if !metric_names.is_null(i) {
                metric_set.insert(metric_names.value(i).to_string());
            }
        }
    }

    // Convert to MetricInfo sorted by name
    let mut metrics: Vec<MetricInfo> = metric_set
        .into_iter()
        .map(|name| MetricInfo { name })
        .collect();

    metrics.sort_by(|a, b| a.name.cmp(&b.name));

    Ok(metrics)
}

/// Fast metadata-only scan of parquet files.
/// Uses sampling to quickly discover metrics and containers without reading all rows.
/// REQ-MV-012: Returns empty index when no paths provided.
fn scan_metadata(paths: &[PathBuf]) -> Result<MetadataIndex> {
    // REQ-MV-012: Handle empty file list gracefully
    if paths.is_empty() {
        tracing::debug!("No parquet files to scan - returning empty index");
        return Ok(MetadataIndex {
            metrics: vec![],
            qos_classes: vec![],
            namespaces: vec![],
            containers: HashMap::new(),
            metric_containers: HashMap::new(),
            file_containers: HashMap::new(),
        });
    }

    let start = std::time::Instant::now();

    let mut metric_set: HashSet<String> = HashSet::new();
    let mut metric_containers: HashMap<String, HashSet<String>> = HashMap::new();
    let mut all_containers: HashMap<String, ContainerInfo> = HashMap::new();
    let mut qos_set: HashSet<String> = HashSet::new();
    let mut namespace_set: HashSet<String> = HashSet::new();
    let mut file_containers: HashMap<PathBuf, HashSet<String>> = HashMap::new();

    let mut rows_sampled = 0u64;

    for path in paths {
        // Track containers found in this specific file
        let mut this_file_containers: HashSet<String> = HashSet::new();
        tracing::debug!("Scanning {:?}", path);

        // REQ-MV-012: Skip invalid/incomplete parquet files gracefully
        // This handles files being actively written by the collector
        let file = match File::open(path) {
            Ok(f) => f,
            Err(e) => {
                tracing::debug!("  Skipping (cannot open): {}", e);
                continue;
            }
        };

        // Check file size - parquet files need at least 8 bytes for magic number
        if let Ok(metadata) = file.metadata() {
            if metadata.len() < 8 {
                tracing::debug!("  Skipping (file too small: {} bytes)", metadata.len());
                continue;
            }
        }

        let builder = match ParquetRecordBatchReaderBuilder::try_new(file) {
            Ok(b) => b,
            Err(e) => {
                tracing::debug!("  Skipping (invalid parquet): {}", e);
                continue;
            }
        };

        let file_metadata = builder.metadata();
        let total_rows = file_metadata.file_metadata().num_rows() as usize;
        let num_row_groups = file_metadata.num_row_groups();

        let schema = builder.schema().clone();
        let parquet_schema = builder.parquet_schema();

        // Only project metric_name and l_* label columns - skip values for speed
        let mut projection: Vec<usize> = vec![];
        if let Ok(idx) = schema.index_of("metric_name") {
            projection.push(idx);
        }

        // Project individual l_* columns for known labels
        for label_key in &["container_id", "qos_class", "namespace", "pod_name", "container_name"] {
            let col_name = format!("l_{}", label_key);
            if let Ok(idx) = schema.index_of(&col_name) {
                projection.push(idx);
            }
        }

        let projection_mask = parquet::arrow::ProjectionMask::roots(parquet_schema, projection);

        // Sample strategy: read first, middle, and last row groups to catch all containers
        // For 62M rows across ~600 row groups, sampling 3-5 groups should find all containers
        let mut row_groups_to_read: Vec<usize> = vec![0]; // Always read first
        if num_row_groups > 1 {
            row_groups_to_read.push(num_row_groups / 2); // Middle
        }
        if num_row_groups > 2 {
            row_groups_to_read.push(num_row_groups - 1); // Last
        }
        // Add a few more spread across the file
        if num_row_groups > 10 {
            row_groups_to_read.push(num_row_groups / 4);
            row_groups_to_read.push(3 * num_row_groups / 4);
        }
        row_groups_to_read.sort();
        row_groups_to_read.dedup();

        tracing::debug!(
            "  {} rows in {} row groups, sampling {} groups",
            total_rows,
            num_row_groups,
            row_groups_to_read.len()
        );

        // Re-open file to create reader with specific row groups
        let file = File::open(path).context("Failed to reopen file")?;
        let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;

        let reader = builder
            .with_projection(projection_mask)
            .with_row_groups(row_groups_to_read.clone())
            .with_batch_size(65536)
            .build()?;

        for batch_result in reader {
            let batch = batch_result?;
            rows_sampled += batch.num_rows() as u64;

            let metric_names = batch
                .column_by_name("metric_name")
                .and_then(|c| c.as_any().downcast_ref::<StringArray>())
                .context("Missing metric_name column")?;

            // Get l_* label columns
            let l_container_id_col = batch
                .column_by_name("l_container_id")
                .and_then(|c| c.as_any().downcast_ref::<StringArray>())
                .context("Missing l_container_id column")?;
            let l_qos_class_col = batch
                .column_by_name("l_qos_class")
                .and_then(|c| c.as_any().downcast_ref::<StringArray>());
            let l_namespace_col = batch
                .column_by_name("l_namespace")
                .and_then(|c| c.as_any().downcast_ref::<StringArray>());
            let l_pod_name_col = batch
                .column_by_name("l_pod_name")
                .and_then(|c| c.as_any().downcast_ref::<StringArray>());
            let l_container_name_col = batch
                .column_by_name("l_container_name")
                .and_then(|c| c.as_any().downcast_ref::<StringArray>());

            for row in 0..batch.num_rows() {
                let metric = metric_names.value(row);
                metric_set.insert(metric.to_string());

                // Extract container_id from l_container_id column
                if l_container_id_col.is_null(row) {
                    continue;
                }
                let container_id = l_container_id_col.value(row).to_string();

                let short_id = if container_id.len() > 12 {
                    container_id[..12].to_string()
                } else {
                    container_id.clone()
                };

                // Track metric -> container relationship
                metric_containers
                    .entry(metric.to_string())
                    .or_default()
                    .insert(short_id.clone());

                // Track which containers are in this file (for file-level pruning)
                this_file_containers.insert(short_id.clone());

                // Only process container info once per container
                if !all_containers.contains_key(&short_id) {
                    // Extract other labels from l_* columns
                    let qos_class = l_qos_class_col
                        .filter(|c| !c.is_null(row))
                        .map(|c| c.value(row).to_string());
                    let namespace = l_namespace_col
                        .filter(|c| !c.is_null(row))
                        .map(|c| c.value(row).to_string());
                    let pod_name = l_pod_name_col
                        .filter(|c| !c.is_null(row))
                        .map(|c| c.value(row).to_string());
                    let container_name = l_container_name_col
                        .filter(|c| !c.is_null(row))
                        .map(|c| c.value(row).to_string());

                    if let Some(ref qos) = qos_class {
                        qos_set.insert(qos.clone());
                    }
                    if let Some(ref ns) = namespace {
                        namespace_set.insert(ns.clone());
                    }

                    all_containers.insert(
                        short_id.clone(),
                        ContainerInfo {
                            id: container_id,
                            short_id,
                            qos_class,
                            namespace,
                            pod_name,
                            container_name,
                            first_seen_ms: None, // Not available from parquet scan
                            last_seen_ms: None,  // Not available from parquet scan
                            labels: None,        // Not available from parquet scan
                        },
                    );
                }
            }
        }

        // Store containers found in this file (for file-level query pruning)
        if !this_file_containers.is_empty() {
            file_containers.insert(path.clone(), this_file_containers);
        }
    }

    // Build metric list (without exact counts since we sampled)
    let mut metrics: Vec<MetricInfo> = metric_set
        .into_iter()
        .map(|name| MetricInfo { name })
        .collect();
    metrics.sort_by(|a, b| a.name.cmp(&b.name));

    let mut qos_classes: Vec<String> = qos_set.into_iter().collect();
    qos_classes.sort();

    let mut namespaces: Vec<String> = namespace_set.into_iter().collect();
    namespaces.sort();

    tracing::debug!(
        "Sampled {} rows, found {} metrics, {} containers in {:.2}s",
        rows_sampled,
        metrics.len(),
        all_containers.len(),
        start.elapsed().as_secs_f64()
    );

    Ok(MetadataIndex {
        metrics,
        qos_classes,
        namespaces,
        containers: all_containers,
        metric_containers,
        file_containers,
    })
}

/// Load timeseries data for a specific metric and set of containers.
/// Uses predicate pushdown and parallel processing for speed.
fn load_metric_data(
    paths: &[PathBuf],
    metric: &str,
    container_ids: &[&str],
) -> Result<Vec<(String, Vec<TimeseriesPoint>)>> {
    let start = std::time::Instant::now();
    let container_set: Arc<HashSet<String>> = Arc::new(
        container_ids
            .iter()
            .map(|s| s.to_string())
            .collect(),
    );
    let is_cumulative_metric = is_cumulative(metric);
    let metric = metric.to_string();

    tracing::debug!(
        "[PERF]     load_metric_data: {} files, {} containers requested",
        paths.len(),
        container_ids.len()
    );

    // Process files in parallel, each file returns its own HashMap
    let parallel_start = std::time::Instant::now();
    let file_results: Vec<Result<HashMap<String, RawContainerData>>> = paths
        .par_iter()
        .map(|path| {
            load_metric_from_file(path, &metric, Arc::clone(&container_set), is_cumulative_metric)
        })
        .collect();
    let parallel_elapsed = parallel_start.elapsed();
    tracing::debug!(
        "[PERF]       parallel file reads: {:.1}ms",
        parallel_elapsed.as_secs_f64() * 1000.0
    );

    // Merge results from all files
    let merge_start = std::time::Instant::now();
    let mut raw_data: HashMap<String, RawContainerData> = HashMap::new();
    for result in file_results {
        let file_data = result?;
        for (id, data) in file_data {
            raw_data
                .entry(id)
                .or_insert_with(|| RawContainerData::with_capacity(is_cumulative_metric))
                .merge(data);
        }
    }
    tracing::debug!(
        "[PERF]       merge results: {:.1}ms",
        merge_start.elapsed().as_secs_f64() * 1000.0
    );

    // Convert to timeseries
    let convert_start = std::time::Instant::now();
    let result: Vec<(String, Vec<TimeseriesPoint>)> = raw_data
        .into_iter()
        .map(|(id, raw)| (id, raw.into_points()))
        .filter(|(_, points)| !points.is_empty())
        .collect();
    tracing::debug!(
        "[PERF]       convert to points: {:.1}ms",
        convert_start.elapsed().as_secs_f64() * 1000.0
    );

    tracing::debug!(
        "[PERF]     load_metric_data TOTAL: {:.1}ms ({} containers loaded)",
        start.elapsed().as_secs_f64() * 1000.0,
        result.len()
    );

    Ok(result)
}

/// Load stats for a specific metric and set of containers.
/// Computes aggregates (count, avg, max) directly without materializing full timeseries.
/// Returns HashMap<container_id, (sample_count, avg, max)>.
fn load_metric_stats(
    paths: &[PathBuf],
    metric: &str,
    container_ids: &[&str],
) -> Result<HashMap<String, (usize, f64, f64)>> {
    let start = std::time::Instant::now();
    let container_set: Arc<HashSet<String>> = Arc::new(
        container_ids
            .iter()
            .map(|s| s.to_string())
            .collect(),
    );
    let is_cumulative_metric = is_cumulative(metric);
    let metric = metric.to_string();

    tracing::debug!(
        "[PERF]     load_metric_stats: {} files, {} containers requested",
        paths.len(),
        container_ids.len()
    );

    // Process files in parallel
    let parallel_start = std::time::Instant::now();
    let file_results: Vec<Result<HashMap<String, RawContainerData>>> = paths
        .par_iter()
        .map(|path| {
            load_metric_from_file(path, &metric, Arc::clone(&container_set), is_cumulative_metric)
        })
        .collect();
    let parallel_elapsed = parallel_start.elapsed();
    tracing::debug!(
        "[PERF]       parallel file reads: {:.1}ms",
        parallel_elapsed.as_secs_f64() * 1000.0
    );

    // Merge results from all files
    let merge_start = std::time::Instant::now();
    let mut raw_data: HashMap<String, RawContainerData> = HashMap::new();
    for result in file_results {
        let file_data = result?;
        for (id, data) in file_data {
            raw_data
                .entry(id)
                .or_insert_with(|| RawContainerData::with_capacity(is_cumulative_metric))
                .merge(data);
        }
    }
    tracing::debug!(
        "[PERF]       merge results: {:.1}ms",
        merge_start.elapsed().as_secs_f64() * 1000.0
    );

    // Compute stats directly (no points allocation)
    let stats_start = std::time::Instant::now();
    let result: HashMap<String, (usize, f64, f64)> = raw_data
        .into_iter()
        .filter_map(|(id, raw)| raw.into_stats().map(|s| (id, s)))
        .collect();
    tracing::debug!(
        "[PERF]       compute stats: {:.1}ms",
        stats_start.elapsed().as_secs_f64() * 1000.0
    );

    tracing::debug!(
        "[PERF]     load_metric_stats TOTAL: {:.1}ms ({} containers)",
        start.elapsed().as_secs_f64() * 1000.0,
        result.len()
    );

    Ok(result)
}

/// Load metric data from a single parquet file.
/// Processes all row groups sequentially within the file.
/// Parallelism is at the file level (in load_metric_data), not within files,
/// to avoid thread pool contention from nested parallelism.
/// Uses row group statistics to skip row groups that can't contain the target metric.
fn load_metric_from_file(
    path: &PathBuf,
    metric: &str,
    container_set: Arc<HashSet<String>>,
    is_cumulative_metric: bool,
) -> Result<HashMap<String, RawContainerData>> {
    let file_start = std::time::Instant::now();

    // REQ-MV-012: Skip invalid/incomplete parquet files gracefully
    let file = match File::open(path) {
        Ok(f) => f,
        Err(_) => return Ok(HashMap::new()),
    };

    // Check file size
    if let Ok(metadata) = file.metadata() {
        if metadata.len() < 8 {
            return Ok(HashMap::new());
        }
    }

    let builder = match ParquetRecordBatchReaderBuilder::try_new(file) {
        Ok(b) => b,
        Err(_) => return Ok(HashMap::new()),
    };
    let file_metadata = builder.metadata();
    let num_row_groups = file_metadata.num_row_groups();

    // Row group statistics pruning: skip row groups where metric_name can't match
    // Find metric_name column index in parquet schema
    let parquet_schema = builder.parquet_schema();
    let metric_col_idx = (0..parquet_schema.num_columns())
        .find(|&i| parquet_schema.column(i).name() == "metric_name");

    let row_groups_to_read: Option<Vec<usize>> = metric_col_idx.map(|col_idx| {
        let metric_bytes = metric.as_bytes();
        (0..num_row_groups)
            .filter(|&rg_idx| {
                let rg_metadata = file_metadata.row_group(rg_idx);
                let col_chunk = rg_metadata.column(col_idx);

                // Check statistics if available
                if let Some(stats) = col_chunk.statistics() {
                    // For string columns, min/max are lexicographically ordered
                    // Skip row group if metric < min or metric > max
                    if let (Some(min_bytes), Some(max_bytes)) =
                        (stats.min_bytes_opt(), stats.max_bytes_opt())
                    {
                        // metric < min -> can't be in this row group
                        if metric_bytes < min_bytes {
                            return false;
                        }
                        // metric > max -> can't be in this row group
                        if metric_bytes > max_bytes {
                            return false;
                        }
                    }
                }
                // Include row group if stats unavailable or metric is within range
                true
            })
            .collect()
    });

    let rg_count_after_stats_pruning = row_groups_to_read
        .as_ref()
        .map(|v| v.len())
        .unwrap_or(num_row_groups);

    // Bloom filter pruning for l_container_id (when specific containers are requested)
    // Bloom filters have no false negatives - if it says "not present", it's definitely not there
    let (row_groups_to_read, bloom_pruned_count) = if !container_set.is_empty() {
        let container_col_idx = (0..parquet_schema.num_columns())
            .find(|&i| parquet_schema.column(i).name() == "l_container_id");

        if let Some(col_idx) = container_col_idx {
            // Get list of row groups to check (either from stats pruning or all)
            let candidate_rgs: Vec<usize> = row_groups_to_read
                .clone()
                .unwrap_or_else(|| (0..num_row_groups).collect());

            // Open file with bloom filter reading enabled
            // This is a lightweight operation - we're only reading bloom filter metadata
            if let Ok(bf_file) = File::open(path) {
                let bf_reader_result = SerializedFileReader::new_with_options(
                    bf_file,
                    ReadOptionsBuilder::new()
                        .with_reader_properties(
                            ReaderProperties::builder()
                                .set_read_bloom_filter(true)
                                .build(),
                        )
                        .build(),
                );

                if let Ok(bf_reader) = bf_reader_result {
                    let filtered: Vec<usize> = candidate_rgs
                        .into_iter()
                        .filter(|&rg_idx| {
                            // Get row group reader for bloom filter access
                            if let Ok(rg_reader) = bf_reader.get_row_group(rg_idx) {
                                if let Some(sbbf) = rg_reader.get_column_bloom_filter(col_idx) {
                                    // Keep row group if ANY container_id might be present
                                    // Bloom filter check: returns true if maybe present, false if definitely absent
                                    container_set.iter().any(|cid| sbbf.check(&cid.as_str()))
                                } else {
                                    // No bloom filter for this row group, must keep it
                                    true
                                }
                            } else {
                                // Couldn't get row group reader, keep it to be safe
                                true
                            }
                        })
                        .collect();

                    let pruned = rg_count_after_stats_pruning - filtered.len();
                    (Some(filtered), pruned)
                } else {
                    // Couldn't create bloom filter reader, use original list
                    (row_groups_to_read, 0)
                }
            } else {
                // Couldn't re-open file, use original list
                (row_groups_to_read, 0)
            }
        } else {
            // No l_container_id column, can't use bloom filter
            (row_groups_to_read, 0)
        }
    } else {
        // No container filter, bloom filter not applicable
        (row_groups_to_read, 0)
    };

    let rg_count_after_pruning = row_groups_to_read
        .as_ref()
        .map(|v| v.len())
        .unwrap_or(num_row_groups);

    // Process row groups (filtered by statistics and bloom filter)
    // Fix #2: Pass builder directly to avoid double file open
    let result = load_row_groups(
        builder,
        metric,
        container_set,
        is_cumulative_metric,
        row_groups_to_read,
    )?;

    let elapsed = file_start.elapsed();
    if elapsed.as_millis() > 100 || !result.is_empty() {
        let stats_pruned = num_row_groups - rg_count_after_stats_pruning;
        tracing::debug!(
            "[PERF-FILE] {:?}: {}ms, {}/{} row_groups (stats_pruned={}, bloom_pruned={}), {} containers matched",
            path.file_name().unwrap_or_default(),
            elapsed.as_millis(),
            rg_count_after_pruning,
            num_row_groups,
            stats_pruned,
            bloom_pruned_count,
            result.len()
        );
    }

    Ok(result)
}

/// Load specific row groups from a parquet file.
/// Uses flat `l_*` columns for labels with predicate pushdown for efficient filtering.
/// Fix #1: Takes Arc<HashSet> directly to avoid cloning in predicates.
/// Fix #2: Takes pre-opened builder to avoid double file open.
fn load_row_groups(
    builder: ParquetRecordBatchReaderBuilder<File>,
    metric: &str,
    container_set: Arc<HashSet<String>>,
    is_cumulative_metric: bool,
    row_groups: Option<Vec<usize>>,
) -> Result<HashMap<String, RawContainerData>> {
    let schema = builder.schema().clone();

    // Build projection: only columns we actually use in the data loop
    // NOTE: metric_name is NOT included - it's only needed for predicate filtering,
    // not for the output data. Including it would waste CPU decompressing ~60MB
    // of string data per file that we never use.
    let projection: Vec<usize> = ["time", "value_int", "value_float", "l_container_id"]
        .iter()
        .filter_map(|name| schema.index_of(name).ok())
        .collect();

    // Build predicates for filtering - push down BOTH metric_name AND l_container_id filters
    let metric_name_idx = schema.index_of("metric_name").ok();
    let l_container_id_idx = schema.index_of("l_container_id").ok();

    // Build all projection masks BEFORE consuming the builder (parquet_schema borrows builder)
    let parquet_schema = builder.parquet_schema();
    let projection_mask =
        parquet::arrow::ProjectionMask::roots(parquet_schema, projection.clone());
    let metric_pred_mask = metric_name_idx
        .map(|idx| parquet::arrow::ProjectionMask::roots(parquet_schema, vec![idx]));
    let container_pred_mask = l_container_id_idx
        .map(|idx| parquet::arrow::ProjectionMask::roots(parquet_schema, vec![idx]));

    // Now consume builder
    let mut reader_builder = builder.with_projection(projection_mask).with_batch_size(65536);

    // Apply row group filter if specified
    if let Some(rgs) = row_groups {
        reader_builder = reader_builder.with_row_groups(rgs);
    }

    // Build row filter with predicates
    let mut predicates: Vec<Box<dyn parquet::arrow::arrow_reader::ArrowPredicate>> = Vec::new();

    // Predicate 1: metric_name filter (works for both schemas)
    if let Some(pred_mask) = metric_pred_mask {
        let target_metric = Arc::new(metric.to_string());
        let predicate = ArrowPredicateFn::new(pred_mask, move |batch| {
            let metric_col = batch.column(0).as_any().downcast_ref::<StringArray>();
            match metric_col {
                Some(arr) => {
                    let matches: BooleanArray = arr
                        .iter()
                        .map(|opt| opt.map(|s| s == target_metric.as_str()))
                        .collect();
                    Ok(matches)
                }
                None => Ok(BooleanArray::from(vec![true; batch.num_rows()])),
            }
        });
        predicates.push(Box::new(predicate));
    }

    // Predicate 2: l_container_id filter
    // Key optimization - skip rows at parquet level based on container_id
    // Fix #1: Reuse the Arc directly instead of cloning all strings into new HashSet
    if let Some(pred_mask) = container_pred_mask {
        let container_filter = Arc::clone(&container_set);
        let predicate = ArrowPredicateFn::new(pred_mask, move |batch| {
            let container_col = batch.column(0).as_any().downcast_ref::<StringArray>();
            match container_col {
                Some(arr) => {
                    let matches: BooleanArray = arr
                        .iter()
                        .map(|opt| {
                            opt.map(|s| {
                                // Match either full ID or short (12-char) ID
                                let short_id = if s.len() > 12 { &s[..12] } else { s };
                                container_filter.contains(short_id)
                            })
                        })
                        .collect();
                    Ok(matches)
                }
                None => Ok(BooleanArray::from(vec![true; batch.num_rows()])),
            }
        });
        predicates.push(Box::new(predicate));
    }

    let reader = if !predicates.is_empty() {
        let row_filter = RowFilter::new(predicates);
        reader_builder.with_row_filter(row_filter).build()?
    } else {
        reader_builder.build()?
    };

    let mut raw_data: HashMap<String, RawContainerData> = HashMap::new();

    for batch_result in reader {
        let batch = batch_result?;

        let times = batch.column_by_name("time").context("Missing time column")?;

        let values_int = batch
            .column_by_name("value_int")
            .and_then(|c| c.as_any().downcast_ref::<UInt64Array>());

        let values_float = batch
            .column_by_name("value_float")
            .and_then(|c| c.as_any().downcast_ref::<Float64Array>());

        let time_values: Vec<i64> = match times.data_type() {
            DataType::Timestamp(_, _) => {
                let ts_array = times
                    .as_any()
                    .downcast_ref::<arrow::array::TimestampMillisecondArray>()
                    .context("Failed to cast timestamp")?;
                (0..ts_array.len()).map(|i| ts_array.value(i)).collect()
            }
            _ => anyhow::bail!("Unexpected time column type"),
        };

        // Extract container_id from l_container_id column
        let l_container_id_col = batch
            .column_by_name("l_container_id")
            .and_then(|c| c.as_any().downcast_ref::<StringArray>());

        for (row, &time) in time_values.iter().enumerate() {
            let container_id = match l_container_id_col {
                Some(arr) if !arr.is_null(row) => arr.value(row),
                _ => continue,
            };

            let short_id = if container_id.len() > 12 {
                &container_id[..12]
            } else {
                container_id
            };

            // Note: If we have predicate pushdown, this check is redundant
            // but we keep it for safety and for cases where pushdown isn't applied
            if !container_set.contains(short_id) {
                continue;
            }

            let value = extract_value(values_float, values_int, row);
            if let Some(v) = value {
                raw_data
                    .entry(short_id.to_string())
                    .or_insert_with(|| RawContainerData::with_capacity(is_cumulative_metric))
                    .add_point(time, v);
            }
        }
    }

    Ok(raw_data)
}

/// Extract value from either float or int column.
#[inline]
fn extract_value(
    values_float: Option<&Float64Array>,
    values_int: Option<&UInt64Array>,
    row: usize,
) -> Option<f64> {
    if let Some(arr) = values_float {
        if !arr.is_null(row) {
            return Some(arr.value(row));
        }
    }
    if let Some(arr) = values_int {
        if !arr.is_null(row) {
            return Some(arr.value(row) as f64);
        }
    }
    None
}

/// Helper struct for accumulating raw container data.
/// Stores raw (time, value) pairs during parallel reads.
/// Rate computation for cumulative metrics happens after merge+sort in `into_points()`.
#[derive(Default)]
struct RawContainerData {
    times: Vec<i64>,
    values: Vec<f64>,
    is_cumulative: bool,
}

/// Estimated points per container for preallocation.
/// Based on 1-hour lookback with 1-second samples = 3600 points,
/// but data is spread across ~80 containers, so ~45 points each.
/// Use 64 for power-of-2 allocation efficiency.
const ESTIMATED_POINTS_PER_CONTAINER: usize = 64;

impl RawContainerData {
    /// Create with pre-allocated capacity to avoid reallocation churn.
    fn with_capacity(is_cumulative: bool) -> Self {
        Self {
            times: Vec::with_capacity(ESTIMATED_POINTS_PER_CONTAINER),
            values: Vec::with_capacity(ESTIMATED_POINTS_PER_CONTAINER),
            is_cumulative,
        }
    }

    /// Store raw (time, value) pair. Rate computation deferred to into_points().
    fn add_point(&mut self, time: i64, value: f64) {
        self.times.push(time);
        self.values.push(value);
    }

    /// Convert to TimeseriesPoints, computing rates for cumulative metrics.
    /// MUST be called after all parallel merges are complete.
    fn into_points(self) -> Vec<TimeseriesPoint> {
        if self.times.is_empty() {
            return vec![];
        }

        // Sort by time after merging data from parallel file reads
        let mut pairs: Vec<(i64, f64)> = self
            .times
            .into_iter()
            .zip(self.values)
            .collect();
        pairs.sort_unstable_by_key(|(time, _)| *time);

        if !self.is_cumulative {
            // Gauge metrics: just return sorted values
            return pairs
                .into_iter()
                .map(|(time_ms, value)| TimeseriesPoint { time_ms, value })
                .collect();
        }

        // Cumulative metrics: compute rates from consecutive points AFTER sorting
        // This fixes the parallelism bug where cross-chunk boundaries were lost
        let mut points = Vec::with_capacity(pairs.len().saturating_sub(1));
        for window in pairs.windows(2) {
            let (prev_time, prev_value) = window[0];
            let (curr_time, curr_value) = window[1];

            let time_delta_ms = curr_time - prev_time;
            let value_delta = curr_value - prev_value;

            // Skip invalid deltas (time going backwards, counter resets)
            if time_delta_ms > 0 && value_delta >= 0.0 {
                // Convert to rate per second (value_delta is in original units)
                // time_delta_ms is milliseconds, so multiply by 1000 to get per-second rate
                let rate = value_delta * 1000.0 / time_delta_ms as f64;
                points.push(TimeseriesPoint {
                    time_ms: curr_time,
                    value: rate,
                });
            }
        }
        points
    }

    /// Merge another RawContainerData into this one.
    /// Used when combining results from parallel file processing.
    fn merge(&mut self, other: RawContainerData) {
        self.times.extend(other.times);
        self.values.extend(other.values);
    }

    /// Compute stats (count, sum, max) directly without allocating full points vector.
    /// Returns (sample_count, avg, max) or None if no valid data.
    fn into_stats(self) -> Option<(usize, f64, f64)> {
        if self.times.is_empty() {
            return None;
        }

        // Sort by time after merging data from parallel file reads
        let mut pairs: Vec<(i64, f64)> = self
            .times
            .into_iter()
            .zip(self.values)
            .collect();
        pairs.sort_unstable_by_key(|(time, _)| *time);

        if !self.is_cumulative {
            // Gauge metrics: compute stats directly from values
            let count = pairs.len();
            if count == 0 {
                return None;
            }
            let sum: f64 = pairs.iter().map(|(_, v)| v).sum();
            let max = pairs.iter().map(|(_, v)| *v).fold(f64::NEG_INFINITY, f64::max);
            return Some((count, sum / count as f64, max));
        }

        // Cumulative metrics: compute stats from rates
        let mut count = 0usize;
        let mut sum = 0.0f64;
        let mut max = f64::NEG_INFINITY;

        for window in pairs.windows(2) {
            let (prev_time, prev_value) = window[0];
            let (curr_time, curr_value) = window[1];

            let time_delta_ms = curr_time - prev_time;
            let value_delta = curr_value - prev_value;

            if time_delta_ms > 0 && value_delta >= 0.0 {
                let rate = value_delta * 1000.0 / time_delta_ms as f64;
                count += 1;
                sum += rate;
                max = max.max(rate);
            }
        }

        if count == 0 {
            return None;
        }
        Some((count, sum / count as f64, max))
    }
}
