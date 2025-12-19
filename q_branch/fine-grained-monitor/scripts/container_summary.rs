#!/usr/bin/env -S cargo +nightly -Zscript

---
[package]
edition = "2024"

[dependencies]
parquet = { version = "54", features = ["arrow"] }
arrow = "54"
anyhow = "1"
clap = { version = "4", features = ["derive"] }
serde = { version = "1", features = ["derive"] }
serde_json = "1"

[profile.dev]
opt-level = 3
---

//! Generate summary statistics for each container from fine-grained-monitor data.
//!
//! Provides per-container aggregates for memory, CPU, throttling, and PSI.
//!
//! Usage:
//!     ./container_summary.rs metrics.parquet
//!     ./container_summary.rs metrics.parquet --format csv
//!     ./container_summary.rs metrics.parquet --sort-by cpu_avg_pct --top 10

use anyhow::{Context, Result};
use arrow::array::{Array, Float64Array, MapArray, StringArray, StructArray, UInt64Array};
use arrow::datatypes::DataType;
use clap::{Parser, ValueEnum};
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
use serde::Serialize;
use std::collections::HashMap;
use std::fs::File;
use std::path::PathBuf;
use std::time::Instant;

#[derive(Parser, Debug)]
#[command(name = "container_summary")]
#[command(about = "Generate per-container summary statistics")]
struct Args {
    /// Input parquet file
    input: PathBuf,

    /// Output format
    #[arg(short, long, value_enum, default_value = "table")]
    format: OutputFormat,

    /// Sort by column
    #[arg(short, long)]
    sort_by: Option<String>,

    /// Show only top N containers
    #[arg(short = 'n', long)]
    top: Option<usize>,
}

#[derive(Debug, Clone, ValueEnum)]
enum OutputFormat {
    Table,
    Csv,
    Json,
}

#[derive(Debug, Clone, Default, Serialize)]
struct ContainerStats {
    container: String,
    qos_class: String,
    node: String,
    duration_s: f64,
    samples: usize,
    memory_min_mib: Option<f64>,
    memory_avg_mib: Option<f64>,
    memory_max_mib: Option<f64>,
    memory_p95_mib: Option<f64>,
    cpu_avg_pct: Option<f64>,
    throttled_s: Option<f64>,
    throttle_events: Option<u64>,
    cpu_pressure_avg: Option<f64>,
    mem_pressure_avg: Option<f64>,
}

#[derive(Default)]
struct ContainerData {
    qos_class: Option<String>,
    node: Option<String>,
    time_min: Option<i64>,
    time_max: Option<i64>,
    fetch_indices: std::collections::HashSet<u64>,

    // Memory current values
    memory_current: Vec<f64>,

    // CPU usage (counter values with times for rate calc)
    cpu_usage: Vec<(i64, f64)>, // (time_ms, usage_usec)

    // Throttling
    throttled_usec: Vec<(i64, f64)>,
    nr_throttled: Vec<(i64, f64)>,

    // PSI pressure
    cpu_pressure: Vec<f64>,
    mem_pressure: Vec<f64>,
}

fn extract_label(labels: &[(String, String)], key: &str) -> Option<String> {
    labels.iter().find(|(k, _)| k == key).map(|(_, v)| v.clone())
}

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
        .context("Missing key column")?;

    let vals = struct_array
        .column(1)
        .as_any()
        .downcast_ref::<StringArray>()
        .context("Missing value column")?;

    let mut result = Vec::with_capacity(end - start);
    for i in start..end {
        if !keys.is_null(i) && !vals.is_null(i) {
            result.push((keys.value(i).to_string(), vals.value(i).to_string()));
        }
    }

    Ok(result)
}

fn percentile(values: &mut Vec<f64>, p: f64) -> Option<f64> {
    if values.is_empty() {
        return None;
    }
    values.sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));
    let idx = ((values.len() - 1) as f64 * p) as usize;
    Some(values[idx])
}

fn main() -> Result<()> {
    let args = Args::parse();

    if !args.input.exists() {
        anyhow::bail!("File not found: {:?}", args.input);
    }

    let total_start = Instant::now();
    eprintln!("Loading {:?}", args.input);

    let file = File::open(&args.input).context("Failed to open file")?;
    let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;
    let reader = builder.with_batch_size(65536).build()?;

    // container_short -> ContainerData
    let mut containers: HashMap<String, ContainerData> = HashMap::new();
    let mut total_rows = 0u64;

    let read_start = Instant::now();
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

        let fetch_index = batch
            .column_by_name("fetch_index")
            .and_then(|c| c.as_any().downcast_ref::<UInt64Array>());

        let labels_col = batch.column_by_name("labels").context("Missing labels column")?;

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
            let time = time_values[row];

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

            let labels = extract_labels_from_column(labels_col, row)?;
            let container_id = match extract_label(&labels, "container_id") {
                Some(id) => id,
                None => continue,
            };

            let container_short = if container_id.len() > 12 {
                container_id[..12].to_string()
            } else {
                container_id.clone()
            };

            let entry = containers.entry(container_short).or_default();

            // Update metadata
            if entry.qos_class.is_none() {
                entry.qos_class = extract_label(&labels, "qos_class");
            }
            if entry.node.is_none() {
                entry.node = extract_label(&labels, "node_name");
            }

            entry.time_min = Some(entry.time_min.map_or(time, |m| m.min(time)));
            entry.time_max = Some(entry.time_max.map_or(time, |m| m.max(time)));

            if let Some(fi) = fetch_index {
                if !fi.is_null(row) {
                    entry.fetch_indices.insert(fi.value(row));
                }
            }

            // Collect metrics by type
            match metric {
                "cgroup.v2.memory.current" => entry.memory_current.push(value),
                "cgroup.v2.cpu.stat.usage_usec" => entry.cpu_usage.push((time, value)),
                "cgroup.v2.cpu.stat.throttled_usec" => entry.throttled_usec.push((time, value)),
                "cgroup.v2.cpu.stat.nr_throttled" => entry.nr_throttled.push((time, value)),
                "cgroup.v2.cpu.pressure.some.avg60" => entry.cpu_pressure.push(value),
                "cgroup.v2.memory.pressure.some.avg60" => entry.mem_pressure.push(value),
                _ => {}
            }
        }
    }

    eprintln!(
        "  Read {} rows, {} containers [{:.2}s]",
        format_number(total_rows),
        containers.len(),
        read_start.elapsed().as_secs_f64()
    );

    // Compute summaries
    let compute_start = Instant::now();
    let mut summaries: Vec<ContainerStats> = Vec::new();

    for (short_id, data) in containers {
        let mut stats = ContainerStats {
            container: short_id,
            qos_class: data.qos_class.unwrap_or_else(|| "?".to_string()),
            node: data.node.unwrap_or_else(|| "?".to_string()),
            samples: data.fetch_indices.len(),
            ..Default::default()
        };

        // Duration
        if let (Some(min), Some(max)) = (data.time_min, data.time_max) {
            stats.duration_s = (max - min) as f64 / 1000.0;
        }

        // Memory stats
        if !data.memory_current.is_empty() {
            let mut mem = data.memory_current.clone();
            let mib = 1024.0 * 1024.0;
            stats.memory_min_mib = mem.iter().cloned().reduce(f64::min).map(|v| v / mib);
            stats.memory_avg_mib = Some(mem.iter().sum::<f64>() / mem.len() as f64 / mib);
            stats.memory_max_mib = mem.iter().cloned().reduce(f64::max).map(|v| v / mib);
            stats.memory_p95_mib = percentile(&mut mem, 0.95).map(|v| v / mib);
        }

        // CPU rate (from counter)
        if data.cpu_usage.len() > 1 {
            let mut sorted: Vec<_> = data.cpu_usage.clone();
            sorted.sort_by_key(|(t, _)| *t);
            let first = sorted.first().unwrap();
            let last = sorted.last().unwrap();
            let time_span = (last.0 - first.0) as f64 / 1000.0; // seconds
            let usage_delta = last.1 - first.1; // microseconds
            if time_span > 0.0 {
                stats.cpu_avg_pct = Some(usage_delta / time_span / 10000.0);
            }
        }

        // Throttling
        if data.throttled_usec.len() > 1 {
            let mut sorted: Vec<_> = data.throttled_usec.clone();
            sorted.sort_by_key(|(t, _)| *t);
            let first = sorted.first().unwrap();
            let last = sorted.last().unwrap();
            let delta = last.1 - first.1;
            if delta > 0.0 {
                stats.throttled_s = Some(delta / 1_000_000.0);
            }
        }

        if data.nr_throttled.len() > 1 {
            let mut sorted: Vec<_> = data.nr_throttled.clone();
            sorted.sort_by_key(|(t, _)| *t);
            let first = sorted.first().unwrap();
            let last = sorted.last().unwrap();
            let delta = last.1 - first.1;
            if delta > 0.0 {
                stats.throttle_events = Some(delta as u64);
            }
        }

        // PSI pressure averages
        if !data.cpu_pressure.is_empty() {
            stats.cpu_pressure_avg = Some(data.cpu_pressure.iter().sum::<f64>() / data.cpu_pressure.len() as f64);
        }
        if !data.mem_pressure.is_empty() {
            stats.mem_pressure_avg = Some(data.mem_pressure.iter().sum::<f64>() / data.mem_pressure.len() as f64);
        }

        summaries.push(stats);
    }

    eprintln!(
        "  Computed {} summaries [{:.2}s]",
        summaries.len(),
        compute_start.elapsed().as_secs_f64()
    );

    // Sort
    if let Some(ref sort_col) = args.sort_by {
        summaries.sort_by(|a, b| {
            let av = get_sort_value(a, sort_col);
            let bv = get_sort_value(b, sort_col);
            bv.partial_cmp(&av).unwrap_or(std::cmp::Ordering::Equal)
        });
    } else {
        // Default sort by memory avg
        summaries.sort_by(|a, b| {
            let av = a.memory_avg_mib.unwrap_or(0.0);
            let bv = b.memory_avg_mib.unwrap_or(0.0);
            bv.partial_cmp(&av).unwrap_or(std::cmp::Ordering::Equal)
        });
    }

    // Limit
    if let Some(n) = args.top {
        summaries.truncate(n);
    }

    eprintln!("Ready in {:.2}s\n", total_start.elapsed().as_secs_f64());

    // Output
    match args.format {
        OutputFormat::Table => print_table(&summaries),
        OutputFormat::Csv => print_csv(&summaries),
        OutputFormat::Json => print_json(&summaries)?,
    }

    Ok(())
}

fn get_sort_value(stats: &ContainerStats, col: &str) -> f64 {
    match col {
        "memory_avg_mib" | "memory_avg" | "memory" => stats.memory_avg_mib.unwrap_or(0.0),
        "memory_max_mib" | "memory_max" => stats.memory_max_mib.unwrap_or(0.0),
        "cpu_avg_pct" | "cpu_avg" | "cpu" => stats.cpu_avg_pct.unwrap_or(0.0),
        "throttled_s" | "throttled" => stats.throttled_s.unwrap_or(0.0),
        "throttle_events" => stats.throttle_events.unwrap_or(0) as f64,
        "duration_s" | "duration" => stats.duration_s,
        "samples" => stats.samples as f64,
        _ => 0.0,
    }
}

fn print_table(summaries: &[ContainerStats]) {
    // Header
    println!(
        "{:<12} {:<12} {:>10} {:>8} {:>10} {:>10} {:>10} {:>10} {:>8} {:>8}",
        "Container", "QoS", "Duration", "Samples", "Mem Avg", "Mem Max", "Mem P95", "CPU Avg", "Throttle", "CPU PSI"
    );
    println!("{}", "-".repeat(110));

    for s in summaries {
        println!(
            "{:<12} {:<12} {:>10.0} {:>8} {:>10} {:>10} {:>10} {:>10} {:>8} {:>8}",
            truncate(&s.container, 12),
            truncate(&s.qos_class, 12),
            s.duration_s,
            s.samples,
            format_opt_f64(s.memory_avg_mib, 1),
            format_opt_f64(s.memory_max_mib, 1),
            format_opt_f64(s.memory_p95_mib, 1),
            format_opt_f64(s.cpu_avg_pct, 1),
            format_opt_f64(s.throttled_s, 1),
            format_opt_f64(s.cpu_pressure_avg, 1),
        );
    }
}

fn print_csv(summaries: &[ContainerStats]) {
    println!("container,qos_class,node,duration_s,samples,memory_avg_mib,memory_max_mib,memory_p95_mib,cpu_avg_pct,throttled_s,throttle_events,cpu_pressure_avg,mem_pressure_avg");
    for s in summaries {
        println!(
            "{},{},{},{:.1},{},{},{},{},{},{},{},{},{}",
            s.container,
            s.qos_class,
            s.node,
            s.duration_s,
            s.samples,
            format_opt_csv(s.memory_avg_mib),
            format_opt_csv(s.memory_max_mib),
            format_opt_csv(s.memory_p95_mib),
            format_opt_csv(s.cpu_avg_pct),
            format_opt_csv(s.throttled_s),
            s.throttle_events.map_or("".to_string(), |v| v.to_string()),
            format_opt_csv(s.cpu_pressure_avg),
            format_opt_csv(s.mem_pressure_avg),
        );
    }
}

fn print_json(summaries: &[ContainerStats]) -> Result<()> {
    let json = serde_json::to_string_pretty(summaries)?;
    println!("{}", json);
    Ok(())
}

fn truncate(s: &str, max: usize) -> String {
    if s.len() > max {
        format!("{}...", &s[..max - 3])
    } else {
        s.to_string()
    }
}

fn format_opt_f64(v: Option<f64>, decimals: usize) -> String {
    match v {
        Some(val) => format!("{:.1$}", val, decimals),
        None => "-".to_string(),
    }
}

fn format_opt_csv(v: Option<f64>) -> String {
    match v {
        Some(val) => format!("{:.2}", val),
        None => "".to_string(),
    }
}

fn format_number(n: u64) -> String {
    let s = n.to_string();
    let mut result = String::new();
    for (i, c) in s.chars().rev().enumerate() {
        if i > 0 && i % 3 == 0 {
            result.insert(0, ',');
        }
        result.insert(0, c);
    }
    result
}
