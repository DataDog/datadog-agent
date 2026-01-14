#!/usr/bin/env -S cargo +nightly -Zscript

---
[package]
edition = "2024"

[dependencies]
parquet = { version = "54", features = ["arrow"] }
arrow = "54"
anyhow = "1"
clap = { version = "4", features = ["derive"] }

[profile.dev]
opt-level = 3
---

//! Inspect fine-grained-monitor Parquet metrics files.
//!
//! Shows schema, row counts, unique metrics, time range, and sample data.
//!
//! Usage:
//!     ./inspect_metrics.rs metrics.parquet

use anyhow::{Context, Result};
use arrow::array::{Array, StringArray};
use arrow::datatypes::DataType;
use clap::Parser;
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
use std::collections::{HashMap, HashSet};
use std::fs::File;
use std::path::PathBuf;
use std::time::Instant;

#[derive(Parser, Debug)]
#[command(name = "inspect_metrics")]
#[command(about = "Inspect Parquet metrics files")]
struct Args {
    /// Input parquet file
    input: PathBuf,

    /// Show sample rows
    #[arg(short, long, default_value = "10")]
    sample: usize,

    /// Max unique metrics to display
    #[arg(short, long, default_value = "30")]
    max_metrics: usize,
}

fn main() -> Result<()> {
    let args = Args::parse();

    if !args.input.exists() {
        anyhow::bail!("File not found: {:?}", args.input);
    }

    let file_size = args.input.metadata()?.len();

    println!("Inspecting: {:?}", args.input);
    println!("{}", "=".repeat(60));

    let start = Instant::now();
    let file = File::open(&args.input).context("Failed to open file")?;
    let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;

    // Get metadata
    let parquet_meta = builder.metadata();
    let schema = builder.schema();
    let num_row_groups = parquet_meta.num_row_groups();
    let total_rows: i64 = (0..num_row_groups)
        .map(|i| parquet_meta.row_group(i).num_rows())
        .sum();

    println!("\n=== Schema ===");
    for field in schema.fields() {
        println!("  {}: {:?}", field.name(), field.data_type());
    }

    println!("\n=== File Info ===");
    println!("  Row groups: {}", num_row_groups);
    println!("  Total rows: {}", format_number(total_rows as u64));
    println!("  File size: {:.2} MB", file_size as f64 / 1024.0 / 1024.0);
    println!("  Metadata read: {:.2}s", start.elapsed().as_secs_f64());

    // Now read actual data to get statistics
    let scan_start = Instant::now();
    let file = File::open(&args.input)?;
    let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;
    let reader = builder.with_batch_size(65536).build()?;

    let mut unique_metrics: HashSet<String> = HashSet::new();
    let mut time_min: Option<i64> = None;
    let mut time_max: Option<i64> = None;
    let mut column_stats: HashMap<String, usize> = HashMap::new();
    let mut sample_rows: Vec<String> = Vec::new();
    let mut rows_scanned: u64 = 0;

    for batch_result in reader {
        let batch = batch_result?;
        rows_scanned += batch.num_rows() as u64;

        // Collect sample rows from first batch
        if sample_rows.len() < args.sample {
            let schema = batch.schema();
            for row in 0..batch.num_rows().min(args.sample - sample_rows.len()) {
                let mut row_str = String::new();
                for (i, col) in batch.columns().iter().enumerate() {
                    if i > 0 {
                        row_str.push_str(" | ");
                    }
                    let col_name = schema.field(i).name();
                    let val = format_value(col, row);
                    row_str.push_str(&format!("{}={}", col_name, val));
                }
                sample_rows.push(row_str);
            }
        }

        // Get metric names
        if let Some(col) = batch.column_by_name("metric_name") {
            if let Some(arr) = col.as_any().downcast_ref::<StringArray>() {
                for i in 0..arr.len() {
                    if !arr.is_null(i) {
                        unique_metrics.insert(arr.value(i).to_string());
                    }
                }
            }
        }

        // Get time range
        if let Some(col) = batch.column_by_name("time") {
            match col.data_type() {
                DataType::Timestamp(_, _) => {
                    if let Some(arr) = col.as_any().downcast_ref::<arrow::array::TimestampMillisecondArray>() {
                        for i in 0..arr.len() {
                            if !arr.is_null(i) {
                                let val = arr.value(i);
                                time_min = Some(time_min.map_or(val, |m| m.min(val)));
                                time_max = Some(time_max.map_or(val, |m| m.max(val)));
                            }
                        }
                    }
                }
                _ => {}
            }
        }

        // Count unique values per column (for string columns only)
        let batch_schema = batch.schema();
        for (i, col) in batch.columns().iter().enumerate() {
            let col_name = batch_schema.field(i).name().to_string();
            if let Some(arr) = col.as_any().downcast_ref::<StringArray>() {
                let count = column_stats.entry(col_name).or_insert(0);
                for j in 0..arr.len() {
                    if !arr.is_null(j) {
                        *count += 1;
                    }
                }
            }
        }
    }

    println!("  Data scan: {:.2}s", scan_start.elapsed().as_secs_f64());

    println!("\n=== Sample Data (first {} rows) ===", sample_rows.len());
    for (i, row) in sample_rows.iter().enumerate() {
        println!("  [{}] {}", i, row);
    }

    println!("\n=== Unique Metrics ({} total) ===", unique_metrics.len());
    let mut metrics: Vec<_> = unique_metrics.iter().collect();
    metrics.sort();
    for metric in metrics.iter().take(args.max_metrics) {
        println!("  - {}", metric);
    }
    if metrics.len() > args.max_metrics {
        println!("  ... and {} more", metrics.len() - args.max_metrics);
    }

    if let (Some(min), Some(max)) = (time_min, time_max) {
        println!("\n=== Time Range ===");
        println!("  Start: {}", format_timestamp(min));
        println!("  End:   {}", format_timestamp(max));
        let duration_s = (max - min) / 1000;
        println!("  Duration: {}s ({:.1} min)", duration_s, duration_s as f64 / 60.0);
    }

    println!("\n=== Summary ===");
    println!("  {} rows, {} unique metrics", format_number(rows_scanned), unique_metrics.len());
    println!("  Compression ratio: {:.1}x",
        (rows_scanned as f64 * 100.0) / file_size as f64); // rough estimate

    Ok(())
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

fn format_timestamp(ms: i64) -> String {
    // Convert milliseconds to a readable format
    let secs = ms / 1000;
    let datetime = chrono_lite(secs);
    format!("{} ({} ms)", datetime, ms)
}

fn chrono_lite(unix_secs: i64) -> String {
    // Simple timestamp formatting without chrono dependency
    // Just show as ISO-ish format
    let days_since_epoch = unix_secs / 86400;
    let time_of_day = unix_secs % 86400;
    let hours = time_of_day / 3600;
    let minutes = (time_of_day % 3600) / 60;
    let seconds = time_of_day % 60;

    // Approximate date (good enough for display)
    let year = 1970 + (days_since_epoch / 365);
    let day_of_year = days_since_epoch % 365;

    format!("~{}-day{:03} {:02}:{:02}:{:02}Z", year, day_of_year, hours, minutes, seconds)
}

fn format_value(col: &dyn Array, row: usize) -> String {
    if col.is_null(row) {
        return "null".to_string();
    }

    match col.data_type() {
        DataType::Utf8 => {
            if let Some(arr) = col.as_any().downcast_ref::<StringArray>() {
                let val = arr.value(row);
                if val.len() > 30 {
                    format!("{}...", &val[..27])
                } else {
                    val.to_string()
                }
            } else {
                "?".to_string()
            }
        }
        DataType::UInt64 => {
            if let Some(arr) = col.as_any().downcast_ref::<arrow::array::UInt64Array>() {
                format_number(arr.value(row))
            } else {
                "?".to_string()
            }
        }
        DataType::Float64 => {
            if let Some(arr) = col.as_any().downcast_ref::<arrow::array::Float64Array>() {
                format!("{:.2}", arr.value(row))
            } else {
                "?".to_string()
            }
        }
        DataType::Timestamp(_, _) => {
            if let Some(arr) = col.as_any().downcast_ref::<arrow::array::TimestampMillisecondArray>() {
                let ms = arr.value(row);
                format_timestamp(ms)
            } else {
                "?".to_string()
            }
        }
        DataType::Map(_, _) => {
            "[map]".to_string()
        }
        _ => format!("{:?}", col.data_type()),
    }
}
