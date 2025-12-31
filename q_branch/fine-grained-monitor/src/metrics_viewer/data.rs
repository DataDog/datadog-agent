//! Parquet loading and metric discovery for metrics viewer.
//!
//! REQ-MV-002: Discovers all available metric types from parquet files.
//! REQ-MV-003: Extracts container attributes (qos_class, namespace) for search/filtering.

use anyhow::{Context, Result};
use arrow::array::{Array, Float64Array, MapArray, StringArray, StructArray, UInt64Array};
use arrow::datatypes::DataType;
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
use std::collections::{HashMap, HashSet};
use std::fs::File;
use std::path::Path;

/// A single timeseries data point.
#[derive(Debug, Clone, serde::Serialize)]
pub struct TimeseriesPoint {
    pub time_ms: i64,
    pub value: f64,
}

/// Container metadata extracted from labels.
#[derive(Debug, Clone, serde::Serialize)]
pub struct ContainerInfo {
    pub id: String,
    pub short_id: String,
    pub qos_class: Option<String>,
    pub namespace: Option<String>,
    pub pod_name: Option<String>,
}

/// Summary statistics for a container's metric.
#[derive(Debug, Clone, serde::Serialize)]
pub struct ContainerStats {
    pub info: ContainerInfo,
    pub sample_count: usize,
    pub avg: f64,
    pub max: f64,
}

/// Metric metadata.
#[derive(Debug, Clone, serde::Serialize)]
pub struct MetricInfo {
    pub name: String,
    pub sample_count: usize,
}

/// All data loaded from parquet files.
pub struct LoadedData {
    /// Available metrics with sample counts.
    pub metrics: Vec<MetricInfo>,
    /// Unique QoS classes found.
    pub qos_classes: Vec<String>,
    /// Unique namespaces found.
    pub namespaces: Vec<String>,
    /// Timeseries data: metric_name -> container_short_id -> points
    pub timeseries: HashMap<String, HashMap<String, Vec<TimeseriesPoint>>>,
    /// Container info by short_id.
    pub containers: HashMap<String, ContainerInfo>,
    /// Container stats per metric: metric_name -> container_short_id -> stats
    pub stats: HashMap<String, HashMap<String, ContainerStats>>,
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

/// Check if a metric is cumulative and needs rate conversion.
fn is_cumulative(metric_name: &str) -> bool {
    CUMULATIVE_METRICS.iter().any(|m| metric_name.contains(m))
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

/// Load data from one or more parquet files.
///
/// REQ-MV-002: Scans metric_name column to build unique metric list.
/// REQ-MV-003: Extracts qos_class, namespace for search/filtering.
pub fn load_parquet_files<P: AsRef<Path>>(paths: &[P]) -> Result<LoadedData> {
    let start = std::time::Instant::now();

    // Collect raw data: metric -> container_id -> (times, values, last_value, last_time, labels)
    let mut raw_data: HashMap<String, HashMap<String, RawContainerData>> = HashMap::new();
    let mut all_containers: HashMap<String, ContainerInfo> = HashMap::new();
    let mut metric_counts: HashMap<String, usize> = HashMap::new();
    let mut qos_set: HashSet<String> = HashSet::new();
    let mut namespace_set: HashSet<String> = HashSet::new();

    let mut total_rows = 0u64;

    for path in paths {
        let path = path.as_ref();
        eprintln!("Loading {:?}", path);

        let file = File::open(path).context("Failed to open file")?;
        let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;

        let schema = builder.schema();
        let parquet_schema = builder.parquet_schema();
        let projection: Vec<usize> =
            ["metric_name", "time", "value_int", "value_float", "labels"]
                .iter()
                .filter_map(|name| schema.index_of(name).ok())
                .collect();

        let projection_mask = parquet::arrow::ProjectionMask::roots(parquet_schema, projection);

        let reader = builder
            .with_projection(projection_mask)
            .with_batch_size(65536)
            .build()?;

        for batch_result in reader {
            let batch = batch_result?;
            total_rows += batch.num_rows() as u64;

            let metric_names = batch
                .column_by_name("metric_name")
                .and_then(|c| c.as_any().downcast_ref::<StringArray>())
                .context("Missing metric_name column")?;

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
                let metric = metric_names.value(row);

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

                let labels = extract_labels_from_column(labels_col.as_ref(), row)?;
                let container_id = match extract_label(&labels, "container_id") {
                    Some(id) => id,
                    None => continue,
                };

                let qos_class = extract_label(&labels, "qos_class");
                let namespace = extract_label(&labels, "namespace");
                let pod_name = extract_label(&labels, "pod_name");

                // Track unique filter values
                if let Some(ref qos) = qos_class {
                    qos_set.insert(qos.clone());
                }
                if let Some(ref ns) = namespace {
                    namespace_set.insert(ns.clone());
                }

                // Build container info
                let short_id = if container_id.len() > 12 {
                    container_id[..12].to_string()
                } else {
                    container_id.clone()
                };

                all_containers.entry(short_id.clone()).or_insert_with(|| {
                    ContainerInfo {
                        id: container_id.clone(),
                        short_id: short_id.clone(),
                        qos_class: qos_class.clone(),
                        namespace: namespace.clone(),
                        pod_name: pod_name.clone(),
                    }
                });

                // Collect raw data
                let metric_data = raw_data.entry(metric.to_string()).or_default();
                let container_data = metric_data.entry(short_id).or_default();

                container_data.add_point(time, value, is_cumulative(metric));

                *metric_counts.entry(metric.to_string()).or_insert(0) += 1;
            }
        }
    }

    eprintln!(
        "Loaded {} rows, {} metrics, {} containers",
        total_rows,
        metric_counts.len(),
        all_containers.len()
    );

    // Build final timeseries and stats
    let mut timeseries: HashMap<String, HashMap<String, Vec<TimeseriesPoint>>> = HashMap::new();
    let mut stats: HashMap<String, HashMap<String, ContainerStats>> = HashMap::new();

    for (metric_name, containers) in raw_data {
        let mut metric_ts: HashMap<String, Vec<TimeseriesPoint>> = HashMap::new();
        let mut metric_stats: HashMap<String, ContainerStats> = HashMap::new();

        for (short_id, raw) in containers {
            let points = raw.into_points();
            if points.is_empty() {
                continue;
            }

            let values: Vec<f64> = points.iter().map(|p| p.value).collect();
            let avg = values.iter().sum::<f64>() / values.len() as f64;
            let max = values.iter().cloned().fold(f64::NEG_INFINITY, f64::max);

            if let Some(info) = all_containers.get(&short_id) {
                metric_stats.insert(
                    short_id.clone(),
                    ContainerStats {
                        info: info.clone(),
                        sample_count: points.len(),
                        avg,
                        max,
                    },
                );
            }

            metric_ts.insert(short_id, points);
        }

        timeseries.insert(metric_name.clone(), metric_ts);
        stats.insert(metric_name, metric_stats);
    }

    // Build sorted metric list
    let mut metrics: Vec<MetricInfo> = metric_counts
        .into_iter()
        .map(|(name, sample_count)| MetricInfo { name, sample_count })
        .collect();
    metrics.sort_by(|a, b| b.sample_count.cmp(&a.sample_count));

    let mut qos_classes: Vec<String> = qos_set.into_iter().collect();
    qos_classes.sort();

    let mut namespaces: Vec<String> = namespace_set.into_iter().collect();
    namespaces.sort();

    eprintln!("Ready in {:.2}s", start.elapsed().as_secs_f64());

    Ok(LoadedData {
        metrics,
        qos_classes,
        namespaces,
        timeseries,
        containers: all_containers,
        stats,
    })
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
            // Compute rate for cumulative metrics
            if self.last_time > 0 && time > self.last_time {
                let value_delta = value - self.last_value;
                let time_delta_ms = time - self.last_time;

                if value_delta >= 0.0 && time_delta_ms > 0 {
                    // Convert to rate per second
                    let rate = if self.is_cumulative && value_delta > 0.0 {
                        // For CPU usec: convert to percentage
                        // usec delta / ms delta / 10 = CPU %
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
            // Gauge metric - use raw value
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
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_cumulative() {
        assert!(is_cumulative("cgroup.v2.cpu.stat.usage_usec"));
        assert!(is_cumulative("cgroup.v2.io.stat.rbytes"));
        assert!(!is_cumulative("cgroup.v2.memory.current"));
    }
}
