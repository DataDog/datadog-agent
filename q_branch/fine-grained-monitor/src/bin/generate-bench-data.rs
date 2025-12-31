//! Synthetic parquet data generator for benchmarks.
//!
//! Generates parquet files matching the fine-grained-monitor schema
//! for reproducible performance testing.
//!
//! Usage:
//!   cargo run --release --bin generate-bench-data -- --scenario small
//!   cargo run --release --bin generate-bench-data -- --scenario medium
//!   cargo run --release --bin generate-bench-data -- --help

use anyhow::Result;
use arrow::array::{
    ArrayRef, Float64Builder, MapBuilder, StringBuilder, TimestampMillisecondBuilder, UInt64Builder,
};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;
use clap::{Parser, ValueEnum};
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;
use std::fs::{self, File};
use std::path::PathBuf;
use std::sync::Arc;

/// Predefined benchmark scenarios
#[derive(Debug, Clone, Copy, ValueEnum)]
enum Scenario {
    /// Small: 2 files, 10 containers, 5 metrics, 10K rows/file
    Small,
    /// Medium: 50 files, 50 containers, 30 metrics, 50K rows/file
    Medium,
    /// Large: 200 files, 100 containers, 30 metrics, 100K rows/file
    Large,
    /// Production: 500 files, 100 containers, 30 metrics, 100K rows/file
    Production,
}

#[derive(Parser)]
#[command(name = "generate-bench-data")]
#[command(about = "Generate synthetic parquet data for benchmarks")]
struct Args {
    /// Predefined scenario to generate
    #[arg(short, long, value_enum)]
    scenario: Scenario,

    /// Output directory (default: testdata/bench/<scenario>)
    #[arg(short, long)]
    output: Option<PathBuf>,
}

/// Scenario configuration
struct ScenarioConfig {
    num_files: usize,
    num_containers: usize,
    num_metrics: usize,
    rows_per_file: usize,
    duration_secs: u64,
}

impl From<Scenario> for ScenarioConfig {
    fn from(s: Scenario) -> Self {
        match s {
            Scenario::Small => ScenarioConfig {
                num_files: 2,
                num_containers: 10,
                num_metrics: 5,
                rows_per_file: 10_000,
                duration_secs: 3600, // 1 hour
            },
            Scenario::Medium => ScenarioConfig {
                num_files: 50,
                num_containers: 50,
                num_metrics: 30,
                rows_per_file: 50_000,
                duration_secs: 3600 * 24, // 1 day
            },
            Scenario::Large => ScenarioConfig {
                num_files: 200,
                num_containers: 100,
                num_metrics: 30,
                rows_per_file: 100_000,
                duration_secs: 3600 * 24 * 5, // 5 days
            },
            Scenario::Production => ScenarioConfig {
                num_files: 500,
                num_containers: 100,
                num_metrics: 30,
                rows_per_file: 100_000,
                duration_secs: 3600 * 24 * 7, // 7 days
            },
        }
    }
}

/// Metrics used in real fine-grained-monitor (subset for benchmarks)
const METRIC_NAMES: &[&str] = &[
    "cpu_percentage",
    "total_cpu_usage_millicores",
    "cpu_limit_millicores",
    "user_cpu_percentage",
    "kernel_cpu_percentage",
    "cgroup.v2.cpu.stat.usage_usec",
    "cgroup.v2.cpu.stat.user_usec",
    "cgroup.v2.cpu.stat.system_usec",
    "cgroup.v2.cpu.stat.throttled_usec",
    "cgroup.v2.cpu.stat.nr_throttled",
    "cgroup.v2.cpu.pressure.some.avg10",
    "cgroup.v2.memory.current",
    "cgroup.v2.memory.max",
    "cgroup.v2.memory.peak",
    "cgroup.v2.memory.stat.anon",
    "cgroup.v2.memory.stat.file",
    "cgroup.v2.memory.stat.pgmajfault",
    "cgroup.v2.memory.events.oom_kill",
    "cgroup.v2.memory.pressure.some.avg10",
    "cgroup.v2.memory.swap.current",
    "smaps_rollup.pss",
    "cgroup.v2.io.stat.rbytes",
    "cgroup.v2.io.stat.wbytes",
    "cgroup.v2.io.stat.rios",
    "cgroup.v2.io.stat.wios",
    "cgroup.v2.io.pressure.some.avg10",
    "container.pid_count",
    "cgroup.v2.pids.current",
    "cgroup.v2.cgroup.threads",
    "system_cpu_percentage",
];

const QOS_CLASSES: &[&str] = &["Guaranteed", "Burstable", "BestEffort"];
const NAMESPACES: &[&str] = &[
    "default",
    "kube-system",
    "monitoring",
    "app-production",
    "app-staging",
];

fn main() -> Result<()> {
    let args = Args::parse();
    let config = ScenarioConfig::from(args.scenario);

    let output_dir = args.output.unwrap_or_else(|| {
        PathBuf::from("testdata/bench").join(format!("{:?}", args.scenario).to_lowercase())
    });

    println!("Generating {:?} scenario:", args.scenario);
    println!("  Files: {}", config.num_files);
    println!("  Containers: {}", config.num_containers);
    println!("  Metrics: {}", config.num_metrics);
    println!("  Rows/file: {}", config.rows_per_file);
    println!("  Output: {}", output_dir.display());

    // Create output directory
    fs::create_dir_all(&output_dir)?;

    // Generate container metadata
    // Use UUIDs that are distinguishable in first 12 chars (short_id used by viewer)
    let containers: Vec<Container> = (0..config.num_containers)
        .map(|i| {
            // Generate a pseudo-random but deterministic container ID
            // Make first 12 chars unique per container so short_id extraction works
            let short_id_part = format!("{:012x}", (i as u64 + 1) * 0x111111111111u64);
            let id = format!("{}{:052x}", short_id_part, i);
            Container {
                id,
                qos_class: QOS_CLASSES[i % QOS_CLASSES.len()].to_string(),
                namespace: NAMESPACES[i % NAMESPACES.len()].to_string(),
                pod_name: format!("pod-{}-{}", NAMESPACES[i % NAMESPACES.len()], i),
            }
        })
        .collect();

    // Select metrics to use
    let metrics: Vec<&str> = METRIC_NAMES.iter().take(config.num_metrics).copied().collect();

    // Calculate time distribution
    let base_time_ms = 1704067200000i64; // 2024-01-01 00:00:00 UTC
    let duration_ms = config.duration_secs * 1000;
    let time_per_file = duration_ms / config.num_files as u64;

    // Generate files
    for file_idx in 0..config.num_files {
        let file_start_ms = base_time_ms + (file_idx as i64 * time_per_file as i64);
        let file_path = output_dir.join(format!("bench-{:04}.parquet", file_idx));

        print!("\r  Generating file {}/{}", file_idx + 1, config.num_files);

        generate_parquet_file(
            &file_path,
            &containers,
            &metrics,
            config.rows_per_file,
            file_start_ms,
            time_per_file,
        )?;
    }

    println!("\n  Done!");

    // Print summary
    let total_rows = config.num_files * config.rows_per_file;
    let total_size: u64 = fs::read_dir(&output_dir)?
        .filter_map(|e| e.ok())
        .filter_map(|e| e.metadata().ok())
        .map(|m| m.len())
        .sum();

    println!("\nSummary:");
    println!("  Total rows: {}", total_rows);
    println!("  Total size: {:.1} MB", total_size as f64 / 1_000_000.0);
    println!(
        "  Avg file size: {:.1} KB",
        total_size as f64 / config.num_files as f64 / 1_000.0
    );

    Ok(())
}

struct Container {
    id: String,
    qos_class: String,
    namespace: String,
    pod_name: String,
}

fn generate_parquet_file(
    path: &PathBuf,
    containers: &[Container],
    metrics: &[&str],
    num_rows: usize,
    base_time_ms: i64,
    time_range_ms: u64,
) -> Result<()> {
    // Schema matching lazy_data.rs expectations
    // Note: MapBuilder uses "keys" and "values" as default field names
    let schema = Arc::new(Schema::new(vec![
        Field::new("metric_name", DataType::Utf8, false),
        Field::new(
            "time",
            DataType::Timestamp(arrow::datatypes::TimeUnit::Millisecond, None),
            false,
        ),
        Field::new("value_int", DataType::UInt64, true),
        Field::new("value_float", DataType::Float64, true),
        Field::new(
            "labels",
            DataType::Map(
                Arc::new(Field::new(
                    "entries",
                    DataType::Struct(
                        vec![
                            Field::new("keys", DataType::Utf8, false),
                            Field::new("values", DataType::Utf8, true),
                        ]
                        .into(),
                    ),
                    false,
                )),
                false,
            ),
            true,
        ),
    ]));

    let file = File::create(path)?;
    let props = WriterProperties::builder()
        .set_compression(Compression::SNAPPY)
        .build();
    let mut writer = ArrowWriter::try_new(file, schema.clone(), Some(props))?;

    // Build data in batches
    let batch_size = 10_000;
    let mut rows_written = 0;
    let time_step = time_range_ms as i64 / num_rows as i64;

    while rows_written < num_rows {
        let this_batch = (num_rows - rows_written).min(batch_size);
        let batch = build_batch(
            &schema,
            containers,
            metrics,
            this_batch,
            base_time_ms + (rows_written as i64 * time_step),
            time_step,
        )?;
        writer.write(&batch)?;
        rows_written += this_batch;
    }

    writer.close()?;
    Ok(())
}

fn build_batch(
    schema: &Arc<Schema>,
    containers: &[Container],
    metrics: &[&str],
    num_rows: usize,
    base_time_ms: i64,
    time_step_ms: i64,
) -> Result<RecordBatch> {
    let mut metric_name_builder = StringBuilder::new();
    let mut time_builder = TimestampMillisecondBuilder::new();
    let mut value_int_builder = UInt64Builder::new();
    let mut value_float_builder = Float64Builder::new();

    // Map builder for labels
    let key_builder = StringBuilder::new();
    let value_builder = StringBuilder::new();
    let mut labels_builder = MapBuilder::new(None, key_builder, value_builder);

    for i in 0..num_rows {
        // Distribute rows across containers and metrics
        let container = &containers[i % containers.len()];
        let metric = metrics[i % metrics.len()];
        let time_ms = base_time_ms + (i as i64 * time_step_ms);

        metric_name_builder.append_value(metric);
        time_builder.append_value(time_ms);

        // Generate realistic values based on metric type
        let value = generate_value(metric, i);
        if metric.contains("usec") || metric.contains("bytes") || metric.ends_with("_count") {
            value_int_builder.append_value(value as u64);
            value_float_builder.append_null();
        } else {
            value_int_builder.append_null();
            value_float_builder.append_value(value);
        }

        // Add labels
        labels_builder.keys().append_value("container_id");
        labels_builder.values().append_value(&container.id);
        labels_builder.keys().append_value("qos_class");
        labels_builder.values().append_value(&container.qos_class);
        labels_builder.keys().append_value("namespace");
        labels_builder.values().append_value(&container.namespace);
        labels_builder.keys().append_value("pod_name");
        labels_builder.values().append_value(&container.pod_name);
        labels_builder.append(true)?;
    }

    let arrays: Vec<ArrayRef> = vec![
        Arc::new(metric_name_builder.finish()),
        Arc::new(time_builder.finish()),
        Arc::new(value_int_builder.finish()),
        Arc::new(value_float_builder.finish()),
        Arc::new(labels_builder.finish()),
    ];

    Ok(RecordBatch::try_new(schema.clone(), arrays)?)
}

/// Generate realistic values for different metric types
fn generate_value(metric: &str, seed: usize) -> f64 {
    // Simple deterministic pseudo-random based on seed
    let noise = ((seed * 17 + 31) % 100) as f64 / 100.0;

    if metric.contains("percentage") || metric.contains("avg10") {
        // Percentages: 0-100 with some variation
        20.0 + noise * 60.0
    } else if metric.contains("memory") && metric.contains("current") {
        // Memory in bytes: 100MB - 2GB
        100_000_000.0 + noise * 1_900_000_000.0
    } else if metric.contains("memory") && metric.contains("max") {
        // Memory limit: 2GB - 8GB
        2_000_000_000.0 + noise * 6_000_000_000.0
    } else if metric.contains("millicores") {
        // CPU in millicores: 0-4000
        noise * 4000.0
    } else if metric.contains("usec") {
        // Microseconds (cumulative): increases with seed
        (seed as f64 * 1000.0) + noise * 10000.0
    } else if metric.contains("bytes") {
        // Bytes (cumulative): increases with seed
        (seed as f64 * 10000.0) + noise * 100000.0
    } else if metric.contains("pid") || metric.contains("thread") {
        // Process/thread counts: 1-100
        1.0 + noise * 99.0
    } else if metric.contains("pss") {
        // PSS in bytes: 10MB - 500MB
        10_000_000.0 + noise * 490_000_000.0
    } else {
        // Default: small positive number
        noise * 1000.0
    }
}
