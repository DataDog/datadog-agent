#!/usr/bin/env -S cargo +nightly -Zscript

---
[dependencies]
parquet = { version = "54", features = ["arrow"] }
arrow = "54"
clap = { version = "4", features = ["derive"] }
anyhow = "1"

[profile.dev]
opt-level = 3
---

//! Detect CPU oscillation patterns in container metrics using autocorrelation.
//!
//! Rust port of oscillation_detector.py for faster processing of large datasets.
//!
//! Usage:
//!     ./oscillation_detector.rs metrics.parquet
//!     ./oscillation_detector.rs metrics.parquet --threshold 0.3 --min-amplitude 5.0

use anyhow::{Context, Result};
use arrow::array::{Array, Float64Array, MapArray, StringArray, StructArray, UInt64Array};
use arrow::datatypes::DataType;
use clap::Parser;
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
use std::collections::HashMap;
use std::fs::File;
use std::path::PathBuf;

#[derive(Parser, Debug)]
#[command(name = "oscillation_detector")]
#[command(about = "Detect CPU oscillation patterns using autocorrelation")]
struct Args {
    /// Input parquet file
    input: PathBuf,

    /// Minimum periodicity score (autocorrelation threshold)
    #[arg(short = 't', long, default_value = "0.6")]
    threshold: f64,

    /// Minimum amplitude (peak-to-trough %)
    #[arg(short = 'a', long, default_value = "10.0")]
    min_amplitude: f64,

    /// Analysis window size in samples
    #[arg(short = 'w', long, default_value = "60")]
    window: usize,

    /// Minimum period to detect in seconds
    #[arg(long, default_value = "2")]
    min_period: usize,

    /// Maximum period to detect in seconds
    #[arg(long, default_value = "30")]
    max_period: usize,

    /// Analyze only this container (prefix match)
    #[arg(short = 'c', long)]
    container: Option<String>,

    /// Show top N containers
    #[arg(short = 'n', long, default_value = "20")]
    top: usize,
}

#[derive(Debug, Clone)]
struct OscillationConfig {
    window_size: usize,
    min_periodicity_score: f64,
    min_amplitude: f64,
    min_period: usize,
    max_period: usize,
}

#[derive(Debug, Clone, Default)]
struct OscillationResult {
    detected: bool,
    periodicity_score: f64,
    period: f64,
    frequency: f64,
    amplitude: f64,
    stddev: f64,
}

#[derive(Debug)]
struct ContainerTimeseries {
    container_id: String,
    container_short: String,
    #[allow(dead_code)]
    timestamps: Vec<i64>,
    cpu_percent: Vec<f64>,
    qos_class: Option<String>,
}

/// Compute normalized autocorrelation at a given lag
fn autocorrelation(samples: &[f64], mean: f64, variance: f64, lag: usize) -> f64 {
    if variance == 0.0 || lag >= samples.len() {
        return 0.0;
    }

    let n = samples.len();
    let count = n - lag;
    if count == 0 {
        return 0.0;
    }

    let sum: f64 = (0..count)
        .map(|i| (samples[i] - mean) * (samples[i + lag] - mean))
        .sum();

    sum / (count as f64 * variance)
}

/// Analyze samples for oscillation patterns
fn analyze_oscillation(samples: &[f64], config: &OscillationConfig) -> OscillationResult {
    let mut result = OscillationResult::default();

    if samples.len() < config.window_size {
        if samples.len() > 1 {
            let min = samples.iter().cloned().fold(f64::INFINITY, f64::min);
            let max = samples.iter().cloned().fold(f64::NEG_INFINITY, f64::max);
            result.amplitude = max - min;

            let mean: f64 = samples.iter().sum::<f64>() / samples.len() as f64;
            let variance: f64 =
                samples.iter().map(|x| (x - mean).powi(2)).sum::<f64>() / samples.len() as f64;
            result.stddev = variance.sqrt();
        }
        return result;
    }

    // Use the most recent window_size samples
    let window = &samples[samples.len() - config.window_size..];

    // Compute statistics
    let mean: f64 = window.iter().sum::<f64>() / window.len() as f64;
    let variance: f64 =
        window.iter().map(|x| (x - mean).powi(2)).sum::<f64>() / window.len() as f64;
    result.stddev = variance.sqrt();

    // Calculate amplitude
    let min = window.iter().cloned().fold(f64::INFINITY, f64::min);
    let max = window.iter().cloned().fold(f64::NEG_INFINITY, f64::max);
    result.amplitude = max - min;

    // Early exit if amplitude below threshold
    if config.min_amplitude > 0.0 && result.amplitude < config.min_amplitude {
        return result;
    }

    // Find best autocorrelation lag
    let mut best_lag = 0;
    let mut best_corr = 0.0;

    for lag in config.min_period..=config.max_period {
        let corr = autocorrelation(window, mean, variance, lag);
        if corr > best_corr {
            best_corr = corr;
            best_lag = lag;
        }
    }

    result.periodicity_score = best_corr;

    if best_lag > 0 {
        result.period = best_lag as f64;
        result.frequency = 1.0 / result.period;
    }

    // Detection criteria
    let meets_periodicity = best_corr >= config.min_periodicity_score;
    let meets_amplitude = config.min_amplitude == 0.0 || result.amplitude >= config.min_amplitude;

    if meets_periodicity && meets_amplitude {
        result.detected = true;
    }

    result
}

/// Extract a label value from the labels map
fn extract_label(labels: &[(String, String)], key: &str) -> Option<String> {
    labels
        .iter()
        .find(|(k, _)| k == key)
        .map(|(_, v)| v.clone())
}

fn main() -> Result<()> {
    let args = Args::parse();

    if !args.input.exists() {
        anyhow::bail!("File not found: {:?}", args.input);
    }

    let config = OscillationConfig {
        window_size: args.window,
        min_periodicity_score: args.threshold,
        min_amplitude: args.min_amplitude,
        min_period: args.min_period,
        max_period: args.max_period,
    };

    eprintln!("Loading {:?}...", args.input);

    let file = File::open(&args.input).context("Failed to open file")?;
    let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;

    // Project only needed columns for better performance
    let schema = builder.schema();
    let projection_indices: Vec<usize> = ["metric_name", "time", "value_int", "value_float", "labels"]
        .iter()
        .filter_map(|name| schema.index_of(name).ok())
        .collect();

    // Build the projection mask while we still have access to parquet_schema
    let projection_mask = parquet::arrow::ProjectionMask::roots(
        builder.parquet_schema(),
        projection_indices,
    );

    let reader = builder
        .with_projection(projection_mask)
        .with_batch_size(65536)  // Larger batches for efficiency
        .build()?;

    // Collect CPU usage data per container
    // Key: container_id, Value: (timestamps, cpu_values, qos_class, last_value, last_time)
    let mut container_data: HashMap<String, (Vec<i64>, Vec<f64>, Option<String>, f64, i64)> =
        HashMap::new();

    let mut total_rows = 0u64;
    let cpu_metric = "cgroup.v2.cpu.stat.usage_usec";

    for batch_result in reader {
        let batch = batch_result?;
        total_rows += batch.num_rows() as u64;

        // Get columns
        let metric_names = batch
            .column_by_name("metric_name")
            .and_then(|c| c.as_any().downcast_ref::<StringArray>())
            .context("Missing metric_name column")?;

        let times = batch
            .column_by_name("time")
            .context("Missing time column")?;

        let values_int = batch
            .column_by_name("value_int")
            .and_then(|c| c.as_any().downcast_ref::<UInt64Array>());

        let values_float = batch
            .column_by_name("value_float")
            .and_then(|c| c.as_any().downcast_ref::<Float64Array>());

        let labels_col = batch
            .column_by_name("labels")
            .context("Missing labels column")?;

        // Extract timestamp values
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

        // Process each row
        for row in 0..batch.num_rows() {
            let metric = metric_names.value(row);
            if metric != cpu_metric {
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

            // Extract labels
            let labels = extract_labels_from_column(labels_col, row)?;
            let container_id = match extract_label(&labels, "container_id") {
                Some(id) => id,
                None => continue,
            };
            let qos_class = extract_label(&labels, "qos_class");

            // Update container data
            let entry = container_data
                .entry(container_id.clone())
                .or_insert_with(|| (Vec::new(), Vec::new(), qos_class.clone(), -1.0, 0));

            // Compute delta if we have a previous value
            if entry.3 >= 0.0 && time > entry.4 {
                let value_delta = value - entry.3;
                let time_delta_ms = time - entry.4;

                if value_delta >= 0.0 && time_delta_ms > 0 {
                    // Convert usec delta to CPU percentage
                    // usec/ms = usec / (ms * 1000) * 100 = usec / ms / 10
                    let cpu_percent = value_delta / (time_delta_ms as f64) / 10.0;
                    entry.0.push(time);
                    entry.1.push(cpu_percent);
                }
            }

            entry.3 = value;
            entry.4 = time;
            if entry.2.is_none() {
                entry.2 = qos_class;
            }
        }
    }

    eprintln!("Loaded {} rows", total_rows);

    // Build timeseries
    let mut timeseries: Vec<ContainerTimeseries> = container_data
        .into_iter()
        .map(|(id, (timestamps, cpu_percent, qos_class, _, _))| {
            let container_short = if id.len() > 12 {
                id[..12].to_string()
            } else {
                id.clone()
            };
            ContainerTimeseries {
                container_id: id,
                container_short,
                timestamps,
                cpu_percent,
                qos_class,
            }
        })
        .collect();

    eprintln!("Found {} containers", timeseries.len());

    // Filter by container if specified
    if let Some(ref prefix) = args.container {
        timeseries.retain(|ts| {
            ts.container_id.starts_with(prefix) || ts.container_short.starts_with(prefix)
        });
        eprintln!("Filtered to {} container(s)", timeseries.len());
    }

    // Analyze each container
    let mut results: Vec<(ContainerTimeseries, OscillationResult)> = timeseries
        .into_iter()
        .map(|ts| {
            let result = analyze_oscillation(&ts.cpu_percent, &config);
            (ts, result)
        })
        .collect();

    // Sort by periodicity score descending
    results.sort_by(|a, b| {
        b.1.periodicity_score
            .partial_cmp(&a.1.periodicity_score)
            .unwrap_or(std::cmp::Ordering::Equal)
    });

    // Print results
    println!();
    println!("{}", "=".repeat(80));
    println!("CPU OSCILLATION DETECTION RESULTS");
    println!("{}", "=".repeat(80));
    println!();
    println!("Configuration:");
    println!("  Window size:          {} samples", config.window_size);
    println!("  Min periodicity:      {}", config.min_periodicity_score);
    println!("  Min amplitude:        {}%", config.min_amplitude);
    println!(
        "  Period range:         {}-{} seconds",
        config.min_period, config.max_period
    );

    // Detected containers
    let detected: Vec<_> = results.iter().filter(|(_, r)| r.detected).collect();

    if !detected.is_empty() {
        println!();
        println!("{:=^80}", "OSCILLATION DETECTED");
        println!();
        println!(
            "{:<14} {:>8} {:>8} {:>8} {:>8} {:>8} {:<12}",
            "Container", "Period", "Freq", "Score", "Amp", "StdDev", "QoS"
        );
        println!("{}", "-".repeat(80));

        for (ts, result) in &detected {
            println!(
                "{:<14} {:>7.1}s {:>7.3}Hz {:>7.2} {:>7.1}% {:>7.2} {:<12}",
                ts.container_short,
                result.period,
                result.frequency,
                result.periodicity_score,
                result.amplitude,
                result.stddev,
                ts.qos_class.as_deref().unwrap_or("unknown")
            );
        }
    } else {
        println!();
        println!("No oscillation patterns detected above threshold.");
    }

    // All containers
    println!();
    println!("{:=^80}", "ALL CONTAINERS");
    println!();
    println!(
        "{:<14} {:>8} {:>8} {:>8} {:>8} {:>8} {:<8}",
        "Container", "Samples", "Score", "Period", "Amp", "StdDev", "Detected"
    );
    println!("{}", "-".repeat(80));

    for (ts, result) in results.iter().take(args.top) {
        let detected_str = if result.detected { "YES" } else { "-" };
        let period_str = if result.period > 0.0 {
            format!("{:.1}s", result.period)
        } else {
            "-".to_string()
        };

        println!(
            "{:<14} {:>8} {:>7.2} {:>8} {:>7.1}% {:>7.2} {:<8}",
            ts.container_short,
            ts.cpu_percent.len(),
            result.periodicity_score,
            period_str,
            result.amplitude,
            result.stddev,
            detected_str
        );
    }

    if results.len() > args.top {
        println!();
        println!("... and {} more containers", results.len() - args.top);
    }

    Ok(())
}

/// Extract labels from the labels column for a specific row
fn extract_labels_from_column(col: &dyn Array, row: usize) -> Result<Vec<(String, String)>> {
    // The labels column is a Map<String, String>
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
