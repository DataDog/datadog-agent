//! Lazy-loading parquet data store for metrics viewer.
//!
//! Two-phase loading approach:
//! - Phase 1 (startup): Fast metadata scan - metric names, containers, counts
//! - Phase 2 (on-demand): Load actual timeseries data when requested
//!
//! This dramatically reduces startup time for large parquet files.

use anyhow::{Context, Result};
use arrow::array::{
    Array, BooleanArray, Float64Array, MapArray, StringArray, StructArray, UInt64Array,
};
use arrow::datatypes::DataType;
use parquet::arrow::arrow_reader::{ArrowPredicateFn, ParquetRecordBatchReaderBuilder, RowFilter};
use rayon::prelude::*;
use std::collections::{HashMap, HashSet};
use std::fs::File;
use std::path::{Path, PathBuf};
use std::sync::{Arc, RwLock};

use super::data::{ContainerInfo, ContainerStats, MetricInfo, TimeseriesPoint};

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

/// Lazy-loading data store with on-demand parquet reads.
pub struct LazyDataStore {
    /// Paths to parquet files.
    paths: Vec<PathBuf>,
    /// Metadata index (loaded at startup).
    pub index: MetadataIndex,
    /// Cached timeseries data: metric -> container -> points.
    timeseries_cache: RwLock<HashMap<String, HashMap<String, Vec<TimeseriesPoint>>>>,
    /// Cached stats: metric -> container -> stats.
    stats_cache: RwLock<HashMap<String, HashMap<String, ContainerStats>>>,
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
}

impl LazyDataStore {
    /// Create a new lazy data store by scanning metadata only.
    /// This is much faster than loading all data upfront.
    pub fn new<P: AsRef<Path>>(paths: &[P]) -> Result<Self> {
        let paths: Vec<PathBuf> = paths.iter().map(|p| p.as_ref().to_path_buf()).collect();
        let index = scan_metadata(&paths)?;

        Ok(Self {
            paths,
            index,
            timeseries_cache: RwLock::new(HashMap::new()),
            stats_cache: RwLock::new(HashMap::new()),
        })
    }

    /// Get timeseries data for specific containers.
    /// Loads from parquet on first request, then caches.
    pub fn get_timeseries(
        &self,
        metric: &str,
        container_ids: &[&str],
    ) -> Result<Vec<(String, Vec<TimeseriesPoint>)>> {
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
            let loaded = load_metric_data(&self.paths, metric, &missing)?;

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
    /// Computes stats from timeseries data (loading if necessary).
    pub fn get_container_stats(&self, metric: &str) -> Result<HashMap<String, ContainerStats>> {
        // Check cache first
        {
            let cache = self.stats_cache.read().unwrap();
            if let Some(stats) = cache.get(metric) {
                return Ok(stats.clone());
            }
        }

        // Get all containers for this metric
        let container_ids: Vec<&str> = self
            .index
            .metric_containers
            .get(metric)
            .map(|set| set.iter().map(|s| s.as_str()).collect())
            .unwrap_or_default();

        if container_ids.is_empty() {
            return Ok(HashMap::new());
        }

        // Load timeseries data and compute stats
        let timeseries = self.get_timeseries(metric, &container_ids)?;

        let mut stats = HashMap::new();
        for (id, points) in timeseries {
            if points.is_empty() {
                continue;
            }

            let values: Vec<f64> = points.iter().map(|p| p.value).collect();
            let avg = values.iter().sum::<f64>() / values.len() as f64;
            let max = values.iter().cloned().fold(f64::NEG_INFINITY, f64::max);

            if let Some(info) = self.index.containers.get(&id) {
                stats.insert(
                    id.clone(),
                    ContainerStats {
                        info: info.clone(),
                        sample_count: points.len(),
                        avg,
                        max,
                    },
                );
            }
        }

        // Cache the stats
        {
            let mut cache = self.stats_cache.write().unwrap();
            cache.insert(metric.to_string(), stats.clone());
        }

        Ok(stats)
    }

    /// Clear all caches (useful for testing or memory pressure).
    #[allow(dead_code)]
    pub fn clear_cache(&self) {
        self.timeseries_cache.write().unwrap().clear();
        self.stats_cache.write().unwrap().clear();
    }
}

/// Fast metadata-only scan of parquet files.
/// Uses sampling to quickly discover metrics and containers without reading all rows.
fn scan_metadata(paths: &[PathBuf]) -> Result<MetadataIndex> {
    let start = std::time::Instant::now();

    let mut metric_set: HashSet<String> = HashSet::new();
    let mut metric_containers: HashMap<String, HashSet<String>> = HashMap::new();
    let mut all_containers: HashMap<String, ContainerInfo> = HashMap::new();
    let mut qos_set: HashSet<String> = HashSet::new();
    let mut namespace_set: HashSet<String> = HashSet::new();

    let mut rows_sampled = 0u64;

    for path in paths {
        eprintln!("Scanning {:?}", path);

        let file = File::open(path).context("Failed to open file")?;
        let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;

        let file_metadata = builder.metadata();
        let total_rows = file_metadata.file_metadata().num_rows() as usize;
        let num_row_groups = file_metadata.num_row_groups();

        let schema = builder.schema();
        let parquet_schema = builder.parquet_schema();

        // Only project metric_name and labels - skip values for speed
        let projection: Vec<usize> = ["metric_name", "labels"]
            .iter()
            .filter_map(|name| schema.index_of(name).ok())
            .collect();

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

        eprintln!(
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

            let labels_col = batch
                .column_by_name("labels")
                .context("Missing labels column")?;

            for row in 0..batch.num_rows() {
                let metric = metric_names.value(row);
                metric_set.insert(metric.to_string());

                let labels = extract_labels_from_column(labels_col.as_ref(), row)?;
                let container_id = match extract_label(&labels, "container_id") {
                    Some(id) => id,
                    None => continue,
                };

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

                // Only process container info once per container
                if !all_containers.contains_key(&short_id) {
                    let qos_class = extract_label(&labels, "qos_class");
                    let namespace = extract_label(&labels, "namespace");
                    let pod_name = extract_label(&labels, "pod_name");

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
                        },
                    );
                }
            }
        }
    }

    // Build metric list (without exact counts since we sampled)
    let mut metrics: Vec<MetricInfo> = metric_set
        .into_iter()
        .map(|name| {
            let container_count = metric_containers.get(&name).map(|s| s.len()).unwrap_or(0);
            MetricInfo {
                name,
                sample_count: container_count, // Use container count as proxy for importance
            }
        })
        .collect();
    metrics.sort_by(|a, b| b.sample_count.cmp(&a.sample_count));

    let mut qos_classes: Vec<String> = qos_set.into_iter().collect();
    qos_classes.sort();

    let mut namespaces: Vec<String> = namespace_set.into_iter().collect();
    namespaces.sort();

    eprintln!(
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

    // Process files in parallel, each file returns its own HashMap
    let file_results: Vec<Result<HashMap<String, RawContainerData>>> = paths
        .par_iter()
        .map(|path| {
            load_metric_from_file(path, &metric, &container_set, is_cumulative_metric)
        })
        .collect();

    // Merge results from all files
    let mut raw_data: HashMap<String, RawContainerData> = HashMap::new();
    for result in file_results {
        let file_data = result?;
        for (id, data) in file_data {
            raw_data
                .entry(id)
                .or_insert_with(|| RawContainerData {
                    is_cumulative: is_cumulative_metric,
                    initialized: true,
                    ..Default::default()
                })
                .merge(data);
        }
    }

    // Convert to timeseries
    let result: Vec<(String, Vec<TimeseriesPoint>)> = raw_data
        .into_iter()
        .map(|(id, raw)| (id, raw.into_points()))
        .filter(|(_, points)| !points.is_empty())
        .collect();

    eprintln!(
        "Loaded {} containers for metric '{}' in {:.2}s",
        result.len(),
        metric,
        start.elapsed().as_secs_f64()
    );

    Ok(result)
}

/// Load metric data from a single parquet file using parallel row group reading.
fn load_metric_from_file(
    path: &PathBuf,
    metric: &str,
    container_set: &HashSet<String>,
    is_cumulative_metric: bool,
) -> Result<HashMap<String, RawContainerData>> {
    // First, get metadata to determine row groups
    let file = File::open(path).context("Failed to open file")?;
    let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;
    let num_row_groups = builder.metadata().num_row_groups();

    // For small files, process sequentially
    if num_row_groups <= 4 {
        return load_row_groups(path, metric, container_set, is_cumulative_metric, None);
    }

    // Split row groups across threads for parallel processing
    let num_threads = rayon::current_num_threads().min(num_row_groups);
    let chunk_size = (num_row_groups + num_threads - 1) / num_threads;

    let row_group_chunks: Vec<Vec<usize>> = (0..num_row_groups)
        .collect::<Vec<_>>()
        .chunks(chunk_size)
        .map(|c| c.to_vec())
        .collect();

    // Process row group chunks in parallel
    let chunk_results: Vec<Result<HashMap<String, RawContainerData>>> = row_group_chunks
        .par_iter()
        .map(|row_groups| {
            load_row_groups(
                path,
                metric,
                container_set,
                is_cumulative_metric,
                Some(row_groups.clone()),
            )
        })
        .collect();

    // Merge results
    let mut raw_data: HashMap<String, RawContainerData> = HashMap::new();
    for result in chunk_results {
        let chunk_data = result?;
        for (id, data) in chunk_data {
            raw_data
                .entry(id)
                .or_insert_with(|| RawContainerData {
                    is_cumulative: is_cumulative_metric,
                    initialized: true,
                    ..Default::default()
                })
                .merge(data);
        }
    }

    Ok(raw_data)
}

/// Load specific row groups from a parquet file.
fn load_row_groups(
    path: &PathBuf,
    metric: &str,
    container_set: &HashSet<String>,
    is_cumulative_metric: bool,
    row_groups: Option<Vec<usize>>,
) -> Result<HashMap<String, RawContainerData>> {
    let file = File::open(path).context("Failed to open file")?;
    let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;

    let schema = builder.schema().clone();
    let parquet_schema = builder.parquet_schema();

    // Project needed columns for data reading
    let projection: Vec<usize> = ["metric_name", "time", "value_int", "value_float", "labels"]
        .iter()
        .filter_map(|name| schema.index_of(name).ok())
        .collect();

    let projection_mask = parquet::arrow::ProjectionMask::roots(parquet_schema, projection.clone());

    // Create predicate mask before consuming builder
    let metric_name_idx = schema.index_of("metric_name").ok();
    let predicate_mask = metric_name_idx
        .map(|idx| parquet::arrow::ProjectionMask::roots(parquet_schema, vec![idx]));

    let mut reader_builder = builder.with_projection(projection_mask).with_batch_size(65536);

    // Apply row group filter if specified
    if let Some(rgs) = row_groups {
        reader_builder = reader_builder.with_row_groups(rgs);
    }

    let reader = if let Some(pred_mask) = predicate_mask {
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

        let row_filter = RowFilter::new(vec![Box::new(predicate)]);
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

        let labels_col = batch
            .column_by_name("labels")
            .context("Missing labels column")?;

        // Hoist downcasts outside the row loop - do once per batch
        let map_array = labels_col
            .as_any()
            .downcast_ref::<MapArray>()
            .context("Labels column is not a MapArray")?;

        let entries = map_array.entries();
        let struct_array = entries
            .as_any()
            .downcast_ref::<StructArray>()
            .context("Map entries is not a StructArray")?;

        let label_keys = struct_array
            .column(0)
            .as_any()
            .downcast_ref::<StringArray>()
            .context("Missing key column in labels")?;

        let label_vals = struct_array
            .column(1)
            .as_any()
            .downcast_ref::<StringArray>()
            .context("Missing value column in labels")?;

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

        for row in 0..batch.num_rows() {
            // Direct container_id extraction - no intermediate Vec allocation
            let container_id =
                match extract_container_id_direct(map_array, label_keys, label_vals, row) {
                    Some(id) => id,
                    None => continue,
                };

            let short_id = if container_id.len() > 12 {
                &container_id[..12]
            } else {
                container_id
            };

            // Filter by container
            if !container_set.contains(short_id) {
                continue;
            }

            // Get value
            let value = if let Some(arr) = values_float {
                if !arr.is_null(row) {
                    arr.value(row)
                } else if let Some(int_arr) = values_int {
                    if !int_arr.is_null(row) {
                        int_arr.value(row) as f64
                    } else {
                        continue;
                    }
                } else {
                    continue;
                }
            } else if let Some(int_arr) = values_int {
                if !int_arr.is_null(row) {
                    int_arr.value(row) as f64
                } else {
                    continue;
                }
            } else {
                continue;
            };

            let time = time_values[row];

            raw_data
                .entry(short_id.to_string())
                .or_default()
                .add_point(time, value, is_cumulative_metric);
        }
    }

    Ok(raw_data)
}

/// Extract container_id directly from MapArray without creating intermediate Vec.
/// Much faster than extract_labels_from_column + extract_label.
#[inline]
fn extract_container_id_direct<'a>(
    map_array: &MapArray,
    keys: &'a StringArray,
    vals: &'a StringArray,
    row: usize,
) -> Option<&'a str> {
    if map_array.is_null(row) {
        return None;
    }

    let start = map_array.value_offsets()[row] as usize;
    let end = map_array.value_offsets()[row + 1] as usize;

    for i in start..end {
        if !keys.is_null(i) && keys.value(i) == "container_id" {
            if !vals.is_null(i) {
                return Some(vals.value(i));
            }
        }
    }

    None
}

/// Extract a label value from a labels list.
fn extract_label(labels: &[(String, String)], key: &str) -> Option<String> {
    labels.iter().find(|(k, _)| k == key).map(|(_, v)| v.clone())
}

/// Extract labels from the labels column for a specific row.
fn extract_labels_from_column(col: &dyn Array, row: usize) -> Result<Vec<(String, String)>> {
    let map_array = col
        .as_any()
        .downcast_ref::<MapArray>()
        .context("Labels column is not a MapArray")?;

    if map_array.is_null(row) {
        return Ok(vec![]);
    }

    let start = map_array.value_offsets()[row] as usize;
    let end = map_array.value_offsets()[row + 1] as usize;

    let entries = map_array.entries();
    let struct_array = entries
        .as_any()
        .downcast_ref::<StructArray>()
        .context("Map entries is not a StructArray")?;

    let keys = struct_array
        .column(0)
        .as_any()
        .downcast_ref::<StringArray>()
        .context("Missing key column in labels")?;

    let vals = struct_array
        .column(1)
        .as_any()
        .downcast_ref::<StringArray>()
        .context("Missing value column in labels")?;

    let mut result = Vec::with_capacity(end - start);
    for i in start..end {
        if !keys.is_null(i) && !vals.is_null(i) {
            result.push((keys.value(i).to_string(), vals.value(i).to_string()));
        }
    }

    Ok(result)
}

/// Helper struct for accumulating raw container data.
#[derive(Default)]
struct RawContainerData {
    times: Vec<i64>,
    values: Vec<f64>,
    last_value: f64,
    last_time: i64,
    is_cumulative: bool,
    initialized: bool,
}

impl RawContainerData {
    fn add_point(&mut self, time: i64, value: f64, is_cumulative: bool) {
        if !self.initialized {
            self.is_cumulative = is_cumulative;
            self.initialized = true;
        }

        if self.is_cumulative {
            if self.last_time > 0 && time > self.last_time {
                let value_delta = value - self.last_value;
                let time_delta_ms = time - self.last_time;

                if value_delta >= 0.0 && time_delta_ms > 0 {
                    let rate = if self.is_cumulative && value_delta > 0.0 {
                        value_delta / (time_delta_ms as f64) / 10.0
                    } else {
                        value_delta / (time_delta_ms as f64) * 1000.0
                    };
                    self.times.push(time);
                    self.values.push(rate);
                }
            }
            self.last_value = value;
            self.last_time = time;
        } else {
            self.times.push(time);
            self.values.push(value);
        }
    }

    fn into_points(self) -> Vec<TimeseriesPoint> {
        self.times
            .into_iter()
            .zip(self.values)
            .map(|(time_ms, value)| TimeseriesPoint { time_ms, value })
            .collect()
    }

    /// Merge another RawContainerData into this one.
    /// Used when combining results from parallel file processing.
    fn merge(&mut self, other: RawContainerData) {
        self.times.extend(other.times);
        self.values.extend(other.values);
    }
}
