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

use super::data::{ContainerInfo, MetricInfo, TimeRange, TimeseriesPoint};
use crate::sidecar::{self, ContainerSidecar};

/// Data file time range information (computed from parquet file timestamps).
#[derive(Debug, Clone)]
pub struct DataRange {
    /// Earliest data file timestamp
    pub earliest: Option<DateTime<Utc>>,
    /// Latest data file timestamp
    pub latest: DateTime<Utc>,
    /// Rotation interval in seconds (for computing file paths)
    pub rotation_interval_sec: u64,
}

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

/// Discover parquet files using time-range based path computation.
/// REQ-MV-012: Avoids expensive glob operations by computing paths from timestamps.
/// REQ-MV-037: Supports configurable time ranges (1h, 1d, 1w, all).
///
/// Instead of globbing `**/*.parquet`, this function:
/// 1. Determines which date directories to scan based on time range
/// 2. Lists identifier subdirectories in each date directory
/// 3. Lists parquet files in each identifier directory
/// 4. Filters to files within the requested time range based on filename timestamps
fn discover_files_by_time_range(
    data_dir: &Path,
    data_range: &DataRange,
    time_range: TimeRange,
) -> Vec<PathBuf> {
    let start_time = std::time::Instant::now();

    let now = Utc::now();

    // REQ-MV-037: Convert TimeRange to search bounds
    let (search_start, search_end) = match time_range.to_duration() {
        Some(lookback) => {
            let earliest_wanted = now - lookback;
            let search_end = data_range.latest.min(now);

            // Fix: When data is stale (data_range.latest < earliest_wanted),
            // use a lookback window from the latest available data instead of from now
            let search_start = if data_range.latest < earliest_wanted {
                // Data is stale - go back from latest available data
                let stale_lookback = search_end - lookback;
                match data_range.earliest {
                    Some(earliest) => earliest.max(stale_lookback),
                    None => stale_lookback,
                }
            } else {
                // Data is current - use standard lookback from now
                match data_range.earliest {
                    Some(earliest) => earliest.max(earliest_wanted),
                    None => earliest_wanted,
                }
            };

            (search_start, search_end)
        }
        None => {
            // TimeRange::All - use full data range
            let start = data_range.earliest.unwrap_or(now - Duration::days(365));
            (start, data_range.latest.min(now))
        }
    };

    tracing::debug!(
        "[PERF] Time-range discovery: {} to {} (range={})",
        search_start.format("%Y-%m-%dT%H:%M:%S"),
        search_end.format("%Y-%m-%dT%H:%M:%S"),
        time_range
    );

    // Determine which date directories to scan (pre-size for typical week span)
    let days_span = (search_end - search_start).num_days().max(1) as usize;
    let mut dates_to_scan = Vec::with_capacity(days_span + 1);
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
    /// Metadata index (loaded at startup, refreshed periodically).
    index: RwLock<MetadataIndex>,
    /// Cached timeseries data: metric -> container -> points.
    timeseries_cache: RwLock<HashMap<String, HashMap<String, Vec<TimeseriesPoint>>>>,
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
    /// Data time range from the index.
    pub data_range: DataRange,
}

impl LazyDataStore {
    /// Create a new lazy data store from a data directory.
    ///
    /// This is the **primary constructor** - discovers parquet files in the directory
    /// and loads metadata using the fastest available method.
    ///
    /// ## Data Flow
    ///
    /// Two loading strategies, chosen automatically:
    ///
    /// 1. **Sidecar fast path** (preferred, ~20ms):
    ///    - Read `.containers` sidecar files written by collector on rotation
    ///    - Read ONE parquet schema for metric names
    ///    - No row group decompression required
    ///
    /// 2. **Parquet scan fallback** (~700ms per file):
    ///    - Sample row groups to discover containers from label columns
    ///    - Used when sidecars don't exist (older data, first run)
    ///
    /// ## Example
    ///
    /// ```ignore
    /// let store = LazyDataStore::new("/var/lib/fine-grained-monitor/data".into())?;
    /// ```
    pub fn new(data_dir: PathBuf) -> Result<Self> {
        tracing::info!("Building metadata from data directory...");
        let start = std::time::Instant::now();

        // Discover all parquet files
        let pattern = format!("{}/**/*.parquet", data_dir.display());
        let mut parquet_files: Vec<PathBuf> = glob::glob(&pattern)
            .map_err(|e| anyhow::anyhow!("Invalid glob pattern: {}", e))?
            .filter_map(Result::ok)
            .collect();

        if parquet_files.is_empty() {
            tracing::warn!("No parquet files found in {:?}", data_dir);
            return Ok(Self::empty_with_dir(data_dir));
        }

        // Sort by modification time (newest first)
        parquet_files.sort_by(|a, b| {
            let a_time = a.metadata().and_then(|m| m.modified()).ok();
            let b_time = b.metadata().and_then(|m| m.modified()).ok();
            b_time.cmp(&a_time)
        });

        // Compute data_range from file timestamps
        let data_range = compute_data_range(&parquet_files);
        let total_files = parquet_files.len();

        tracing::info!(
            "Found {} parquet files, time range: {:?} to {}",
            total_files,
            data_range.earliest.map(|t| t.format("%Y-%m-%dT%H:%M:%S").to_string()),
            data_range.latest.format("%Y-%m-%dT%H:%M:%S")
        );

        // Try sidecar fast path first
        let index = match load_from_sidecars(&parquet_files, data_range.clone()) {
            Ok(idx) => {
                tracing::info!(
                    "Sidecar fast path: {} containers from {} sidecars in {:.3}s",
                    idx.containers.len(),
                    idx.file_containers.len(),
                    start.elapsed().as_secs_f64()
                );
                idx
            }
            Err(e) => {
                // Fall back to parquet scanning
                tracing::info!(
                    "Sidecar fast path unavailable ({}), falling back to parquet scan...",
                    e
                );
                let mut idx = scan_metadata_limited(&parquet_files, 30)?;
                idx.data_range = data_range;
                tracing::info!(
                    "Parquet scan complete: {} metrics, {} containers in {:.2}s",
                    idx.metrics.len(),
                    idx.containers.len(),
                    start.elapsed().as_secs_f64()
                );
                idx
            }
        };

        Ok(Self {
            paths: RwLock::new(parquet_files),
            data_dir: Some(data_dir),
            index: RwLock::new(index),
            timeseries_cache: RwLock::new(HashMap::new()),
            last_refresh: RwLock::new(Some(std::time::Instant::now())),
        })
    }

    /// Create from explicit file paths.
    ///
    /// Tries sidecar fast path first (looks for `.containers` files next to each parquet),
    /// then falls back to parquet scanning if sidecars don't exist.
    ///
    /// **Prefer `new(data_dir)` when possible** - it enables automatic file refresh
    /// and is the standard production path.
    ///
    /// This method is useful for:
    /// - Testing with specific file subsets
    /// - Processing files from non-standard locations
    pub fn from_files<P: AsRef<Path>>(paths: &[P]) -> Result<Self> {
        let paths: Vec<PathBuf> = paths.iter().map(|p| p.as_ref().to_path_buf()).collect();

        if paths.is_empty() {
            return Ok(Self {
                paths: RwLock::new(vec![]),
                data_dir: None,
                index: RwLock::new(MetadataIndex {
                    metrics: vec![],
                    qos_classes: vec![],
                    namespaces: vec![],
                    containers: HashMap::new(),
                    metric_containers: HashMap::new(),
                    file_containers: HashMap::new(),
                    data_range: DataRange {
                        earliest: None,
                        latest: Utc::now(),
                        rotation_interval_sec: 90,
                    },
                }),
                timeseries_cache: RwLock::new(HashMap::new()),
                last_refresh: RwLock::new(None),
            });
        }

        let start = std::time::Instant::now();
        let data_range = compute_data_range(&paths);

        // Try sidecar fast path first
        let index = match load_from_sidecars(&paths, data_range.clone()) {
            Ok(idx) => {
                tracing::info!(
                    "Sidecar fast path: {} containers from {} sidecars in {:.3}s",
                    idx.containers.len(),
                    idx.file_containers.len(),
                    start.elapsed().as_secs_f64()
                );
                idx
            }
            Err(e) => {
                // Fall back to parquet scanning
                tracing::debug!(
                    "Sidecar fast path unavailable ({}), falling back to parquet scan...",
                    e
                );
                let mut idx = scan_metadata(&paths)?;
                idx.data_range = data_range;
                idx
            }
        };

        Ok(Self {
            paths: RwLock::new(paths),
            data_dir: None, // Static file list, no refresh
            index: RwLock::new(index),
            timeseries_cache: RwLock::new(HashMap::new()),
            last_refresh: RwLock::new(None),
        })
    }

    /// Create an empty store with just a data directory (no files yet).
    fn empty_with_dir(data_dir: PathBuf) -> Self {
        Self {
            paths: RwLock::new(vec![]),
            data_dir: Some(data_dir),
            index: RwLock::new(MetadataIndex {
                metrics: vec![],
                qos_classes: vec![],
                namespaces: vec![],
                containers: HashMap::new(),
                metric_containers: HashMap::new(),
                file_containers: HashMap::new(),
                data_range: DataRange {
                    earliest: None,
                    latest: Utc::now(),
                    rotation_interval_sec: 90,
                },
            }),
            timeseries_cache: RwLock::new(HashMap::new()),
            last_refresh: RwLock::new(Some(std::time::Instant::now())),
        }
    }

    /// Refresh the file list by re-discovering parquet files.
    /// This allows the viewer to see newly written files.
    /// REQ-MV-037: Discovers files for the specified time range.
    /// Fix #9: Skips refresh if called within REFRESH_STALENESS_MS of last refresh.
    fn refresh_files(&self, time_range: TimeRange) {
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

            // Use the index's data_range for discovery instead of creating a new one
            // This ensures we use the actual data timespan, not a synthetic "now"
            let data_range = self.index.read().unwrap().data_range.clone();
            let new_files = discover_files_by_time_range(data_dir, &data_range, time_range);
            let new_count = new_files.len();

            let mut paths = self.paths.write().unwrap();
            let old_count = paths.len();
            *paths = new_files;

            // Update last refresh time
            *self.last_refresh.write().unwrap() = Some(std::time::Instant::now());

            if new_count != old_count {
                tracing::debug!(
                    "[PERF] refresh_files({}): {} -> {} files in {:.1}ms",
                    time_range,
                    old_count,
                    new_count,
                    start.elapsed().as_secs_f64() * 1000.0
                );
            }
        }
    }

    /// Get timeseries data for specific containers.
    /// REQ-MV-037: Loads data from files within the specified time range and filters points.
    /// Loads from parquet on first request, then caches.
    /// Automatically discovers new parquet files before loading.
    pub fn get_timeseries(
        &self,
        metric: &str,
        container_ids: &[&str],
        time_range: TimeRange,
    ) -> Result<Vec<(String, Vec<TimeseriesPoint>)>> {
        // Refresh file list to pick up newly written files for this time range
        self.refresh_files(time_range);

        // Check what's already cached (pre-size based on requested containers)
        let mut result = Vec::with_capacity(container_ids.len());
        let mut missing: Vec<&str> = Vec::with_capacity(container_ids.len());

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
            let index = self.index.read().unwrap();

            // File-level pruning: only process files that contain requested containers
            let missing_set: HashSet<&str> = missing.iter().copied().collect();
            let paths: Vec<PathBuf> = if index.file_containers.is_empty() {
                // No file_containers index available, use all files
                all_paths.clone()
            } else {
                all_paths
                    .iter()
                    .filter(|path| {
                        index
                            .file_containers
                            .get(*path)
                            .map(|containers| containers.iter().any(|c| missing_set.contains(c.as_str())))
                            .unwrap_or(true) // Include files not in index (safety)
                    })
                    .cloned()
                    .collect()
            };
            drop(index); // Release lock before expensive I/O

            tracing::debug!(
                "[PERF] File-level pruning: {} -> {} files for {} containers",
                all_paths.len(),
                paths.len(),
                missing.len()
            );

            let loaded = load_metric_data(&paths, metric, &missing)?;

            // Cache the loaded data (cache full data for reuse across time ranges)
            {
                let mut cache = self.timeseries_cache.write().unwrap();
                let metric_cache = cache.entry(metric.to_string()).or_default();
                for (id, points) in &loaded {
                    metric_cache.insert(id.clone(), points.clone());
                }
            }

            result.extend(loaded);
        }

        // REQ-MV-037: Filter results by time range cutoff
        if let Some(cutoff_ms) = time_range.cutoff_ms() {
            result = result
                .into_iter()
                .map(|(id, points)| {
                    let filtered: Vec<TimeseriesPoint> = points
                        .into_iter()
                        .filter(|p| p.time_ms >= cutoff_ms)
                        .collect();
                    (id, filtered)
                })
                .filter(|(_, points)| !points.is_empty())
                .collect();
        }

        Ok(result)
    }

    /// Get containers sorted by last_seen (most recent first).
    /// Filters to containers with data in the specified time range.
    /// This is instant as it only reads from the index, avoiding expensive parquet reads.
    pub fn get_containers_by_recency(&self, time_range: TimeRange) -> Vec<ContainerInfo> {
        let start = std::time::Instant::now();

        // Calculate cutoff time for filtering
        let cutoff_ms = time_range.to_duration().map(|d| {
            let now = Utc::now();
            (now - d).timestamp_millis()
        });

        let index = self.index.read().unwrap();
        // Pre-allocate based on index size (filter will reduce but avoids reallocation)
        let mut containers: Vec<ContainerInfo> = Vec::with_capacity(index.containers.len());
        containers.extend(index.containers.values().filter(|c| {
            // Filter by time range - container must have last_seen within range
            match (cutoff_ms, c.last_seen_ms) {
                (Some(cutoff), Some(last_seen)) => last_seen >= cutoff,
                // Include containers with unknown last_seen (parquet fallback path)
                // since we can't determine their relevance - data queries will filter
                (Some(_), None) => true,
                (None, _) => true, // TimeRange::All, include all
            }
        }).cloned());
        drop(index);

        // Sort by last_seen descending (most recent first)
        containers.sort_by(|a, b| {
            let a_time = a.last_seen_ms.unwrap_or(0);
            let b_time = b.last_seen_ms.unwrap_or(0);
            b_time.cmp(&a_time)
        });

        tracing::debug!(
            "[PERF] get_containers_by_recency({}): {} containers in {:.1}ms",
            time_range,
            containers.len(),
            start.elapsed().as_secs_f64() * 1000.0
        );

        containers
    }

    /// Get a clone of the metrics list.
    pub fn get_metrics(&self) -> Vec<MetricInfo> {
        self.index.read().unwrap().metrics.clone()
    }

    /// Get a clone of the QoS classes list.
    pub fn get_qos_classes(&self) -> Vec<String> {
        self.index.read().unwrap().qos_classes.clone()
    }

    /// Get a clone of the namespaces list.
    pub fn get_namespaces(&self) -> Vec<String> {
        self.index.read().unwrap().namespaces.clone()
    }

    /// Refresh container metadata from sidecar files (incremental).
    ///
    /// This is called periodically by a background task to pick up new containers
    /// without requiring a viewer restart. Only works in directory mode.
    ///
    /// Only reads NEW sidecar files - sidecars are immutable once written, so we
    /// skip files we've already processed (tracked via file_containers keys).
    ///
    /// Returns the number of containers after refresh, or None if not in directory mode.
    pub fn refresh_containers_from_sidecars(&self) -> Option<usize> {
        let data_dir = self.data_dir.as_ref()?;
        let start = std::time::Instant::now();

        // Discover all parquet files
        let pattern = format!("{}/**/*.parquet", data_dir.display());
        let parquet_files: Vec<PathBuf> = glob::glob(&pattern)
            .ok()?
            .filter_map(Result::ok)
            .collect();

        if parquet_files.is_empty() {
            tracing::debug!("No parquet files found during sidecar refresh");
            return Some(0);
        }

        // Find NEW parquet files (not yet in file_containers)
        let new_files: Vec<PathBuf> = {
            let index = self.index.read().unwrap();
            parquet_files
                .iter()
                .filter(|p| !index.file_containers.contains_key(*p))
                .cloned()
                .collect()
        };

        if new_files.is_empty() {
            // No new files - just return current count
            let count = self.index.read().unwrap().containers.len();
            tracing::trace!("Sidecar refresh: no new files, {} containers", count);
            return Some(count);
        }

        // Read only the NEW sidecars (pre-size based on number of new files)
        let mut new_containers: HashMap<String, ContainerInfo> = HashMap::with_capacity(new_files.len() * 4);
        let mut new_file_containers: HashMap<PathBuf, HashSet<String>> = HashMap::with_capacity(new_files.len());
        let mut new_qos: HashSet<String> = HashSet::with_capacity(4);
        let mut new_namespaces: HashSet<String> = HashSet::with_capacity(16);
        let mut new_metrics: HashSet<String> = HashSet::with_capacity(192);
        let mut container_timestamps: HashMap<String, (i64, i64)> = HashMap::with_capacity(new_files.len() * 4);
        let mut sidecars_read = 0usize;

        for parquet_path in &new_files {
            let sidecar_path = sidecar::sidecar_path_for_parquet(parquet_path);

            let sidecar = match sidecar::ContainerSidecar::read(&sidecar_path) {
                Ok(s) => s,
                Err(_) => continue,
            };

            // Also discover metrics from this new file (cheap: reads only first row group)
            if let Ok(metrics) = get_metrics_from_schema(parquet_path) {
                for metric in metrics {
                    new_metrics.insert(metric.name);
                }
            }

            // Parse file timestamp for container time bounds
            let file_ts_ms = parquet_path
                .file_name()
                .and_then(|n| n.to_str())
                .and_then(parse_file_timestamp)
                .map(|dt| dt.timestamp_millis());

            sidecars_read += 1;
            let mut file_container_ids: HashSet<String> = HashSet::with_capacity(sidecar.containers.len());

            for sc in sidecar.containers {
                file_container_ids.insert(sc.container_id.clone());
                new_qos.insert(sc.qos_class.clone());
                if let Some(ref ns) = sc.namespace {
                    new_namespaces.insert(ns.clone());
                }

                // Update container time bounds
                if let Some(ts_ms) = file_ts_ms {
                    container_timestamps
                        .entry(sc.container_id.clone())
                        .and_modify(|(min, max)| {
                            if ts_ms < *min { *min = ts_ms; }
                            if ts_ms > *max { *max = ts_ms; }
                        })
                        .or_insert((ts_ms, ts_ms));
                }

                new_containers.entry(sc.container_id.clone()).or_insert_with(|| {
                    ContainerInfo {
                        id: sc.container_id.clone(),
                        short_id: sc.container_id.clone(),
                        qos_class: Some(sc.qos_class.clone()),
                        namespace: sc.namespace.clone(),
                        pod_name: sc.pod_name.clone(),
                        container_name: sc.container_name.clone(),
                        first_seen_ms: None,
                        last_seen_ms: None,
                        labels: sc.labels.clone(),
                    }
                });
            }

            new_file_containers.insert(parquet_path.clone(), file_container_ids);
        }

        // Apply timestamps to new containers
        for (container_id, (first_seen, last_seen)) in container_timestamps {
            if let Some(info) = new_containers.get_mut(&container_id) {
                info.first_seen_ms = Some(first_seen);
                info.last_seen_ms = Some(last_seen);
            }
        }

        // Merge into existing index
        let (old_count, new_count) = {
            let mut index = self.index.write().unwrap();
            let old_count = index.containers.len();

            // Merge containers (update timestamps for existing, add new)
            for (id, new_info) in new_containers {
                index.containers
                    .entry(id)
                    .and_modify(|existing| {
                        // Update time bounds
                        if let Some(new_first) = new_info.first_seen_ms {
                            match existing.first_seen_ms {
                                Some(old) if new_first < old => existing.first_seen_ms = Some(new_first),
                                None => existing.first_seen_ms = Some(new_first),
                                _ => {}
                            }
                        }
                        if let Some(new_last) = new_info.last_seen_ms {
                            match existing.last_seen_ms {
                                Some(old) if new_last > old => existing.last_seen_ms = Some(new_last),
                                None => existing.last_seen_ms = Some(new_last),
                                _ => {}
                            }
                        }
                    })
                    .or_insert(new_info);
            }

            // Merge file_containers
            index.file_containers.extend(new_file_containers);

            // Merge qos_classes and namespaces
            for qos in new_qos {
                if !index.qos_classes.contains(&qos) {
                    index.qos_classes.push(qos);
                }
            }
            for ns in new_namespaces {
                if !index.namespaces.contains(&ns) {
                    index.namespaces.push(ns);
                }
            }

            // Merge metrics (add newly discovered metrics)
            let old_metrics_count = index.metrics.len();
            let existing_metrics: HashSet<String> = index.metrics.iter().map(|m| m.name.clone()).collect();
            for metric_name in new_metrics {
                if !existing_metrics.contains(&metric_name) {
                    index.metrics.push(MetricInfo { name: metric_name });
                }
            }
            if index.metrics.len() > old_metrics_count {
                index.metrics.sort_by(|a, b| a.name.cmp(&b.name));
            }

            // Update data_range
            index.data_range = compute_data_range(&parquet_files);

            (old_count, index.containers.len())
        };

        // Update paths
        *self.paths.write().unwrap() = parquet_files;

        if new_count != old_count {
            tracing::info!(
                "Sidecar refresh: {} -> {} containers ({} new sidecars) in {:.1}ms",
                old_count,
                new_count,
                sidecars_read,
                start.elapsed().as_secs_f64() * 1000.0
            );
        } else if sidecars_read > 0 {
            tracing::debug!(
                "Sidecar refresh: {} containers, {} new sidecars (no new containers) in {:.1}ms",
                new_count,
                sidecars_read,
                start.elapsed().as_secs_f64() * 1000.0
            );
        } else {
            tracing::trace!(
                "Sidecar refresh: {} containers (no changes)",
                new_count
            );
        }

        Some(new_count)
    }

    /// Clear all caches (useful for testing or memory pressure).
    #[allow(dead_code)]
    pub fn clear_cache(&self) {
        self.timeseries_cache.write().unwrap().clear();
    }
}

/// Compute data_range from parquet file timestamps.
fn compute_data_range(parquet_files: &[PathBuf]) -> DataRange {
    let mut earliest: Option<DateTime<Utc>> = None;
    let mut latest: Option<DateTime<Utc>> = None;

    for path in parquet_files {
        if let Some(filename) = path.file_name().and_then(|n| n.to_str()) {
            if let Some(ts) = parse_file_timestamp(filename) {
                match earliest {
                    None => earliest = Some(ts),
                    Some(e) if ts < e => earliest = Some(ts),
                    _ => {}
                }
                match latest {
                    None => latest = Some(ts),
                    Some(l) if ts > l => latest = Some(ts),
                    _ => {}
                }
            }
        }
    }

    DataRange {
        earliest,
        latest: latest.unwrap_or_else(Utc::now),
        rotation_interval_sec: 90,
    }
}

/// Estimated number of containers in a typical workload.
/// Based on stress scenario: ~50 containers, realistic: ~20.
const ESTIMATED_CONTAINERS: usize = 64;

/// Estimated number of unique metrics.
const ESTIMATED_METRICS: usize = 64;

/// Load metadata from sidecar files (fast path).
///
/// Sidecars are small binary files (`.containers`) written by the collector alongside
/// each parquet file. They contain container metadata serialized with bincode, enabling
/// ~1000x faster startup than scanning parquet row groups.
///
/// Returns error if no sidecars found (caller should fall back to parquet scan).
fn load_from_sidecars(parquet_files: &[PathBuf], data_range: DataRange) -> Result<MetadataIndex> {
    // Pre-size HashMaps to avoid rehashing during population
    let mut all_containers: HashMap<String, ContainerInfo> = HashMap::with_capacity(ESTIMATED_CONTAINERS);
    let mut file_containers: HashMap<PathBuf, HashSet<String>> = HashMap::with_capacity(parquet_files.len());
    let mut qos_set: HashSet<String> = HashSet::with_capacity(4); // Typically: Guaranteed, Burstable, BestEffort
    let mut namespace_set: HashSet<String> = HashSet::with_capacity(16);
    let mut sidecars_read = 0usize;

    // Track min/max timestamps per container (from file timestamps)
    let mut container_timestamps: HashMap<String, (i64, i64)> = HashMap::with_capacity(ESTIMATED_CONTAINERS);

    for parquet_path in parquet_files {
        let sidecar_path = sidecar::sidecar_path_for_parquet(parquet_path);

        // Try to read sidecar
        let sidecar = match ContainerSidecar::read(&sidecar_path) {
            Ok(s) => s,
            Err(e) => {
                tracing::trace!(
                    path = %sidecar_path.display(),
                    error = %e,
                    "Sidecar not found or unreadable"
                );
                continue;
            }
        };

        // Parse file timestamp for container time bounds
        let file_ts_ms = parquet_path
            .file_name()
            .and_then(|n| n.to_str())
            .and_then(parse_file_timestamp)
            .map(|dt| dt.timestamp_millis());

        sidecars_read += 1;
        let mut file_container_ids: HashSet<String> = HashSet::with_capacity(sidecar.containers.len());

        for sc in sidecar.containers {
            file_container_ids.insert(sc.container_id.clone());

            // Track QoS and namespace
            qos_set.insert(sc.qos_class.clone());
            if let Some(ref ns) = sc.namespace {
                namespace_set.insert(ns.clone());
            }

            // Update container time bounds from file timestamp
            if let Some(ts_ms) = file_ts_ms {
                container_timestamps
                    .entry(sc.container_id.clone())
                    .and_modify(|(min, max)| {
                        if ts_ms < *min {
                            *min = ts_ms;
                        }
                        if ts_ms > *max {
                            *max = ts_ms;
                        }
                    })
                    .or_insert((ts_ms, ts_ms));
            }

            // Add/update container info (timestamps added after loop)
            all_containers
                .entry(sc.container_id.clone())
                .or_insert_with(|| ContainerInfo {
                    // Sidecar has short_id as container_id, construct full id placeholder
                    id: sc.container_id.clone(),
                    short_id: sc.container_id.clone(),
                    qos_class: Some(sc.qos_class.clone()),
                    namespace: sc.namespace.clone(),
                    pod_name: sc.pod_name.clone(),
                    container_name: sc.container_name.clone(),
                    first_seen_ms: None,
                    last_seen_ms: None,
                    labels: sc.labels.clone(),
                });
        }

        file_containers.insert(parquet_path.clone(), file_container_ids);
    }

    // Apply computed timestamps to containers
    for (container_id, (first_seen, last_seen)) in container_timestamps {
        if let Some(info) = all_containers.get_mut(&container_id) {
            info.first_seen_ms = Some(first_seen);
            info.last_seen_ms = Some(last_seen);
        }
    }

    // Require at least some sidecars to use this path
    if sidecars_read == 0 {
        anyhow::bail!("no sidecars found");
    }

    // Get metric names from parquet schema - try multiple files
    // (first file may be incomplete/being-written since list is sorted newest-first)
    let mut metrics = Vec::new();
    for file in parquet_files.iter() {
        match get_metrics_from_schema(file) {
            Ok(m) if !m.is_empty() => {
                metrics = m;
                tracing::debug!(
                    "Read {} metrics from schema of {:?}",
                    metrics.len(),
                    file.file_name().unwrap_or_default()
                );
                break;
            }
            Ok(_) => {
                tracing::trace!("Skipping {:?} (0 metrics)", file.file_name().unwrap_or_default());
                continue;
            }
            Err(e) => {
                tracing::trace!("Skipping {:?}: {}", file.file_name().unwrap_or_default(), e);
                continue;
            }
        }
    }

    let qos_classes: Vec<String> = qos_set.into_iter().collect();
    let namespaces: Vec<String> = namespace_set.into_iter().collect();

    // Build metric_containers map: all containers for all metrics
    // (We don't know which containers have which metrics without scanning parquet,
    // so we assume all containers might have any metric - queries will filter)
    let all_container_ids: HashSet<String> = all_containers.keys().cloned().collect();
    let mut metric_containers: HashMap<String, HashSet<String>> = HashMap::with_capacity(metrics.len());
    for m in &metrics {
        metric_containers.insert(m.name.clone(), all_container_ids.clone());
    }

    tracing::debug!(
        "Loaded {} containers from {} sidecars, {} metrics from schema",
        all_containers.len(),
        sidecars_read,
        metrics.len()
    );

    Ok(MetadataIndex {
        metrics,
        qos_classes,
        namespaces,
        containers: all_containers,
        metric_containers,
        file_containers,
        data_range,
    })
}

/// Get metric names from parquet schema (reads footer only, no decompression).
fn get_metrics_from_schema(path: &PathBuf) -> Result<Vec<MetricInfo>> {
    let file = File::open(path)?;
    let reader = SerializedFileReader::new(file)?;
    let metadata = reader.metadata();

    // The schema has columns like: run_id, time, metric_name, value_int, l_container_id, etc.
    // We need to read some rows to get actual metric names, but we can at least
    // verify the schema is valid. For now, return empty and let queries discover metrics.

    // Actually, let's sample ONE row group to get metric names quickly
    if metadata.num_row_groups() == 0 {
        return Ok(vec![]);
    }

    let builder = ParquetRecordBatchReaderBuilder::try_new(File::open(path)?)?;
    let reader = builder
        .with_row_groups(vec![0]) // Just first row group
        .with_batch_size(10000) // Small batch
        .build()?;

    let mut metric_set: HashSet<String> = HashSet::with_capacity(ESTIMATED_METRICS);

    for batch_result in reader {
        let batch = batch_result?;
        if let Some(col) = batch.column_by_name("metric_name") {
            if let Some(arr) = col.as_any().downcast_ref::<StringArray>() {
                for i in 0..arr.len() {
                    if !arr.is_null(i) {
                        metric_set.insert(arr.value(i).to_string());
                    }
                }
            }
        }
    }

    let mut metrics: Vec<MetricInfo> = metric_set
        .into_iter()
        .map(|name| MetricInfo { name })
        .collect();
    metrics.sort_by(|a, b| a.name.cmp(&b.name));

    Ok(metrics)
}

/// Scan metadata from a limited number of files (wrapper for scan_metadata).
fn scan_metadata_limited(all_files: &[PathBuf], max_files: usize) -> Result<MetadataIndex> {
    let files_to_scan: Vec<PathBuf> = all_files.iter().take(max_files).cloned().collect();
    tracing::info!(
        "Scanning {} of {} files for container metadata...",
        files_to_scan.len(),
        all_files.len()
    );
    scan_metadata(&files_to_scan)
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
            data_range: DataRange {
                earliest: None,
                latest: Utc::now(),
                rotation_interval_sec: 90,
            },
        });
    }

    let start = std::time::Instant::now();

    // Pre-size HashMaps to avoid rehashing during population
    let mut metric_set: HashSet<String> = HashSet::with_capacity(ESTIMATED_METRICS);
    let mut metric_containers: HashMap<String, HashSet<String>> = HashMap::with_capacity(ESTIMATED_METRICS);
    let mut all_containers: HashMap<String, ContainerInfo> = HashMap::with_capacity(ESTIMATED_CONTAINERS);
    let mut qos_set: HashSet<String> = HashSet::with_capacity(4);
    let mut namespace_set: HashSet<String> = HashSet::with_capacity(16);
    let mut file_containers: HashMap<PathBuf, HashSet<String>> = HashMap::with_capacity(paths.len());
    // Track min/max timestamps per container (from file timestamps)
    let mut container_timestamps: HashMap<String, (i64, i64)> = HashMap::with_capacity(ESTIMATED_CONTAINERS);

    let mut rows_sampled = 0u64;

    for path in paths {
        // Track containers found in this specific file (pre-sized for typical container count per file)
        let mut this_file_containers: HashSet<String> = HashSet::with_capacity(ESTIMATED_CONTAINERS);
        tracing::debug!("Scanning {:?}", path);

        // Parse file timestamp for container time bounds
        let file_ts_ms = path
            .file_name()
            .and_then(|n| n.to_str())
            .and_then(parse_file_timestamp)
            .map(|dt| dt.timestamp_millis());

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

                // Update container time bounds from file timestamp
                if let Some(ts_ms) = file_ts_ms {
                    container_timestamps
                        .entry(short_id.clone())
                        .and_modify(|(min, max)| {
                            if ts_ms < *min {
                                *min = ts_ms;
                            }
                            if ts_ms > *max {
                                *max = ts_ms;
                            }
                        })
                        .or_insert((ts_ms, ts_ms));
                }

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
                            first_seen_ms: None, // Applied after loop
                            last_seen_ms: None,  // Applied after loop
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

    // Apply computed timestamps to containers
    for (container_id, (first_seen, last_seen)) in container_timestamps {
        if let Some(info) = all_containers.get_mut(&container_id) {
            info.first_seen_ms = Some(first_seen);
            info.last_seen_ms = Some(last_seen);
        }
    }

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
        data_range: DataRange {
            earliest: None,
            latest: Utc::now(),
            rotation_interval_sec: 90,
        },
    })
}

/// Interned container ID for zero-copy sharing across parallel file reads.
/// Uses Arc<str> to avoid String cloning - incrementing ref count is much faster.
type InternedId = Arc<str>;

/// Container ID interner - maps string to its interned Arc<str> representation.
/// This eliminates thousands of String allocations per query by reusing the same
/// Arc<str> for each unique container ID.
type ContainerInterner = Arc<HashMap<Box<str>, InternedId>>;

/// Create an interner from container IDs.
/// The interner maps each container ID (as borrowed str) to its Arc<str> representation.
fn create_interner(container_ids: &[&str]) -> ContainerInterner {
    Arc::new(
        container_ids
            .iter()
            .map(|&s| {
                let boxed: Box<str> = s.into();
                let arc: Arc<str> = Arc::from(s);
                (boxed, arc)
            })
            .collect(),
    )
}

/// Load timeseries data for a specific metric and set of containers.
/// Uses predicate pushdown and parallel processing for speed.
/// Uses Arc<str> interning to eliminate String cloning in the hot path.
fn load_metric_data(
    paths: &[PathBuf],
    metric: &str,
    container_ids: &[&str],
) -> Result<Vec<(String, Vec<TimeseriesPoint>)>> {
    let start = std::time::Instant::now();

    // Create interner for zero-copy container ID sharing across parallel reads
    let interner = create_interner(container_ids);

    // Also keep a simple HashSet for fast contains() checks in predicates
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
    // Uses interner for zero-copy container ID sharing
    let parallel_start = std::time::Instant::now();
    let file_results: Vec<(PathBuf, Result<HashMap<InternedId, RawContainerData>>)> = paths
        .par_iter()
        .map(|path| {
            let result = load_metric_from_file(
                path,
                &metric,
                Arc::clone(&container_set),
                Arc::clone(&interner),
                is_cumulative_metric,
            );
            (path.clone(), result)
        })
        .collect();
    let parallel_elapsed = parallel_start.elapsed();
    tracing::debug!(
        "[PERF]       parallel file reads: {:.1}ms",
        parallel_elapsed.as_secs_f64() * 1000.0
    );

    // Merge results from all files (pre-size based on requested containers)
    // Uses InternedId (Arc<str>) for zero-copy key sharing
    let merge_start = std::time::Instant::now();
    let mut raw_data: HashMap<InternedId, RawContainerData> = HashMap::with_capacity(container_ids.len());
    for (path, result) in file_results {
        let file_data = match result {
            Ok(data) => data,
            Err(e) => {
                eprintln!("Error loading parquet file {:?}: {}", path, e);
                continue; // Skip bad files instead of failing entirely
            }
        };
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

    // Convert to timeseries (convert InternedId back to String for API)
    let convert_start = std::time::Instant::now();
    let result: Vec<(String, Vec<TimeseriesPoint>)> = raw_data
        .into_iter()
        .map(|(id, raw)| (id.to_string(), raw.into_points()))
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

/// Load metric data from a single parquet file.
/// Processes all row groups sequentially within the file.
/// Parallelism is at the file level (in load_metric_data), not within files,
/// to avoid thread pool contention from nested parallelism.
/// Uses row group statistics to skip row groups that can't contain the target metric.
/// Uses interner for zero-copy container ID sharing.
fn load_metric_from_file(
    path: &PathBuf,
    metric: &str,
    container_set: Arc<HashSet<String>>,
    interner: ContainerInterner,
    is_cumulative_metric: bool,
) -> Result<HashMap<InternedId, RawContainerData>> {
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
        interner,
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
/// Uses interner for zero-copy container ID sharing.
fn load_row_groups(
    builder: ParquetRecordBatchReaderBuilder<File>,
    metric: &str,
    container_set: Arc<HashSet<String>>,
    interner: ContainerInterner,
    is_cumulative_metric: bool,
    row_groups: Option<Vec<usize>>,
) -> Result<HashMap<InternedId, RawContainerData>> {
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

    // Pre-size based on number of containers in the filter set
    // Uses InternedId (Arc<str>) for zero-copy key sharing
    let mut raw_data: HashMap<InternedId, RawContainerData> = HashMap::with_capacity(container_set.len());

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

            // Look up the interned ID (Arc<str>) - this is O(1) and avoids String allocation
            // The interner was created with all requested container IDs, so this should always succeed
            let interned_id = match interner.get(short_id) {
                Some(id) => Arc::clone(id),
                None => continue, // Container not in interner (shouldn't happen with predicate pushdown)
            };

            let value = extract_value(values_float, values_int, row);
            if let Some(v) = value {
                raw_data
                    .entry(interned_id)
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
}
