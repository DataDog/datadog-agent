//! Realistic synthetic parquet data generator for benchmarks.
//!
//! Models actual fine-grained-monitor data patterns:
//! - Directory structure: dt=YYYY-MM-DD/identifier=<pod>/
//! - Fresh files: metrics-YYYYMMDDTHHMMSSZ.parquet (~200K rows, 90s of data)
//! - Consolidated files: consolidated-START-END.parquet (~2M rows, ~14min of data)
//! - Mix of file types matching real 1-hour query patterns
//!
//! Usage:
//!   cargo run --release --bin generate-bench-data -- --scenario realistic-1h
//!   cargo run --release --bin generate-bench-data -- --scenario stress-test
//!   cargo run --release --bin generate-bench-data -- --help

use anyhow::Result;
use arrow::array::{ArrayRef, BinaryBuilder, Float64Builder, StringBuilder, TimestampMillisecondBuilder, UInt64Builder};
use arrow::datatypes::{DataType, Field, Schema, TimeUnit};
use arrow::record_batch::RecordBatch;
use chrono::{DateTime, Duration, Utc};
use clap::{Parser, ValueEnum};
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;
use std::fs::{self, File};
use std::path::PathBuf;
use std::sync::Arc;

/// Benchmark scenarios modeling real data patterns
#[derive(Debug, Clone, Copy, ValueEnum)]
enum Scenario {
    /// Realistic 1-hour window: 4 fresh + 4 consolidated files per identifier
    Realistic1h,
    /// Single consolidated file for isolated testing
    SingleConsolidated,
    /// Single fresh file for isolated testing
    SingleFresh,
    /// Stress test: 24 hours of data with realistic consolidation
    StressTest,
    /// Legacy format for backwards compatibility
    Legacy,
}

#[derive(Parser)]
#[command(name = "generate-bench-data")]
#[command(about = "Generate realistic synthetic parquet data for benchmarks")]
struct Args {
    /// Benchmark scenario to generate
    #[arg(short, long, value_enum, default_value = "realistic-1h")]
    scenario: Scenario,

    /// Output directory (default: testdata/bench/<scenario>)
    #[arg(short, long)]
    output: Option<PathBuf>,

    /// Number of identifiers (pods) to generate
    #[arg(long, default_value = "1")]
    identifiers: usize,

    /// Number of containers per identifier
    #[arg(long, default_value = "10")]
    containers: usize,

    /// Use ZSTD compression (like consolidated files) instead of Snappy
    #[arg(long)]
    zstd: bool,
}

/// Real metrics from fine-grained-monitor
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
const NAMESPACES: &[&str] = &["default", "kube-system", "monitoring", "app-production", "app-staging"];

fn main() -> Result<()> {
    let args = Args::parse();

    let output_dir = args.output.unwrap_or_else(|| {
        PathBuf::from("testdata/bench").join(format!("{:?}", args.scenario).to_lowercase().replace("_", "-"))
    });

    println!("Generating {:?} scenario:", args.scenario);
    println!("  Output: {}", output_dir.display());
    println!("  Identifiers: {}", args.identifiers);
    println!("  Containers per identifier: {}", args.containers);
    println!("  Compression: {}", if args.zstd { "ZSTD" } else { "Snappy" });

    // Clean and create output directory
    if output_dir.exists() {
        fs::remove_dir_all(&output_dir)?;
    }
    fs::create_dir_all(&output_dir)?;

    // Generate containers
    let containers: Vec<Container> = (0..args.containers)
        .map(|i| Container::new(i))
        .collect();

    // Base time: now minus 1 hour (so data is "recent")
    let now = Utc::now();
    let base_time = now - Duration::hours(1);

    match args.scenario {
        Scenario::Realistic1h => {
            generate_realistic_1h(&output_dir, &containers, args.identifiers, base_time, args.zstd)?;
        }
        Scenario::SingleConsolidated => {
            generate_single_file(&output_dir, &containers, "consolidated", 2_000_000, args.zstd)?;
        }
        Scenario::SingleFresh => {
            generate_single_file(&output_dir, &containers, "fresh", 200_000, args.zstd)?;
        }
        Scenario::StressTest => {
            generate_stress_test(&output_dir, &containers, args.identifiers, base_time, args.zstd)?;
        }
        Scenario::Legacy => {
            generate_legacy(&output_dir, &containers, args.zstd)?;
        }
    }

    // Print summary
    let total_size: u64 = walkdir::WalkDir::new(&output_dir)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter_map(|e| e.metadata().ok())
        .filter(|m| m.is_file())
        .map(|m| m.len())
        .sum();

    let file_count: usize = walkdir::WalkDir::new(&output_dir)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| e.path().extension().map(|x| x == "parquet").unwrap_or(false))
        .count();

    println!("\nSummary:");
    println!("  Files: {}", file_count);
    println!("  Total size: {:.1} MB", total_size as f64 / 1_000_000.0);

    Ok(())
}

/// Generate realistic 1-hour window data
fn generate_realistic_1h(
    output_dir: &PathBuf,
    containers: &[Container],
    num_identifiers: usize,
    base_time: DateTime<Utc>,
    use_zstd: bool,
) -> Result<()> {
    // Real pattern for 1-hour window:
    // - 4 consolidated files (~14 min each, covering minutes 0-54)
    // - 4 fresh files (90 sec each, covering minutes 54-60)

    for id_idx in 0..num_identifiers {
        let identifier = format!("bench-pod-{}", id_idx);
        let date_str = base_time.format("%Y-%m-%d").to_string();
        let id_dir = output_dir
            .join(format!("dt={}", date_str))
            .join(format!("identifier={}", identifier));
        fs::create_dir_all(&id_dir)?;

        println!("  Generating identifier: {}", identifier);

        // Generate 4 consolidated files (each ~14 minutes, ~2M rows)
        for i in 0..4 {
            let start_offset = Duration::minutes(i * 14);
            let end_offset = Duration::minutes((i + 1) * 14 - 1);
            let file_start = base_time + start_offset;
            let file_end = base_time + end_offset;

            let filename = format!(
                "consolidated-{}-{}.parquet",
                file_start.format("%Y%m%dT%H%M%SZ"),
                file_end.format("%Y%m%dT%H%M%SZ")
            );
            let path = id_dir.join(&filename);

            print!("    {} ", filename);
            let rows = generate_file(&path, containers, &identifier, file_start, file_end, true)?;
            println!("({} rows)", rows);
        }

        // Generate 4 fresh files (each ~90 seconds, ~200K rows)
        for i in 0..4 {
            let start_offset = Duration::minutes(54) + Duration::seconds(i * 90);
            let file_start = base_time + start_offset;

            let filename = format!("metrics-{}.parquet", file_start.format("%Y%m%dT%H%M%SZ"));
            let path = id_dir.join(&filename);

            let file_end = file_start + Duration::seconds(90);
            print!("    {} ", filename);
            let rows = generate_file(&path, containers, &identifier, file_start, file_end, use_zstd)?;
            println!("({} rows)", rows);
        }
    }

    Ok(())
}

/// Generate a single test file
fn generate_single_file(
    output_dir: &PathBuf,
    containers: &[Container],
    file_type: &str,
    target_rows: usize,
    use_zstd: bool,
) -> Result<()> {
    let now = Utc::now();
    let identifier = "bench-single";

    // Calculate duration based on target rows
    // Real data: ~30 metrics × 10 containers × 1 sample/sec = 300 rows/sec
    // For 2M rows, need ~6600 seconds (~110 minutes, ~14 min × 10 containers)
    let rows_per_sec = containers.len() * METRIC_NAMES.len();
    let duration_secs = (target_rows / rows_per_sec).max(90) as i64;

    let file_start = now - Duration::seconds(duration_secs);
    let file_end = now;

    let filename = match file_type {
        "consolidated" => format!(
            "consolidated-{}-{}.parquet",
            file_start.format("%Y%m%dT%H%M%SZ"),
            file_end.format("%Y%m%dT%H%M%SZ")
        ),
        _ => format!("metrics-{}.parquet", file_start.format("%Y%m%dT%H%M%SZ")),
    };

    let path = output_dir.join(&filename);
    print!("  {} ", filename);
    let rows = generate_file(&path, containers, identifier, file_start, file_end, use_zstd)?;
    println!("({} rows)", rows);

    Ok(())
}

/// Generate 24-hour stress test data
fn generate_stress_test(
    output_dir: &PathBuf,
    containers: &[Container],
    num_identifiers: usize,
    base_time: DateTime<Utc>,
    use_zstd: bool,
) -> Result<()> {
    // 24 hours of data:
    // - ~100 consolidated files per identifier
    // - 4 fresh files (most recent)

    let base_time = base_time - Duration::hours(23); // Start 24 hours ago

    for id_idx in 0..num_identifiers {
        let identifier = format!("bench-pod-{}", id_idx);
        println!("  Generating identifier: {}", identifier);

        // Generate consolidated files for first 23.9 hours
        let num_consolidated = 100;
        let consolidated_duration = Duration::minutes(14);

        for i in 0..num_consolidated {
            let file_start = base_time + Duration::minutes(i as i64 * 14);
            let file_end = file_start + consolidated_duration;

            let date_str = file_start.format("%Y-%m-%d").to_string();
            let id_dir = output_dir
                .join(format!("dt={}", date_str))
                .join(format!("identifier={}", identifier));
            fs::create_dir_all(&id_dir)?;

            let filename = format!(
                "consolidated-{}-{}.parquet",
                file_start.format("%Y%m%dT%H%M%SZ"),
                file_end.format("%Y%m%dT%H%M%SZ")
            );
            let path = id_dir.join(&filename);

            print!("\r    Consolidated {}/{}", i + 1, num_consolidated);
            generate_file(&path, containers, &identifier, file_start, file_end, use_zstd)?;
        }
        println!();

        // Generate fresh files for last 6 minutes
        let now = Utc::now();
        for i in 0..4 {
            let file_start = now - Duration::seconds((4 - i) * 90);
            let file_end = file_start + Duration::seconds(90);

            let date_str = file_start.format("%Y-%m-%d").to_string();
            let id_dir = output_dir
                .join(format!("dt={}", date_str))
                .join(format!("identifier={}", identifier));
            fs::create_dir_all(&id_dir)?;

            let filename = format!("metrics-{}.parquet", file_start.format("%Y%m%dT%H%M%SZ"));
            let path = id_dir.join(&filename);

            print!("    Fresh {} ", filename);
            let rows = generate_file(&path, containers, &identifier, file_start, file_end, false)?;
            println!("({} rows)", rows);
        }
    }

    Ok(())
}

/// Generate legacy flat file format
fn generate_legacy(output_dir: &PathBuf, containers: &[Container], use_zstd: bool) -> Result<()> {
    let now = Utc::now();
    let file_start = now - Duration::hours(1);

    // Generate 50 flat files like the old generator
    for i in 0..50 {
        let file_time = file_start + Duration::minutes(i as i64);
        let file_end = file_time + Duration::seconds(60);
        let filename = format!("bench-{:04}.parquet", i);
        let path = output_dir.join(&filename);

        print!("\r  Generating {}", filename);
        generate_file(&path, containers, "legacy", file_time, file_end, use_zstd)?;
    }
    println!();

    Ok(())
}

/// Generate a single parquet file with realistic data
fn generate_file(
    path: &PathBuf,
    containers: &[Container],
    identifier: &str,
    start_time: DateTime<Utc>,
    end_time: DateTime<Utc>,
    use_zstd: bool,
) -> Result<usize> {
    let schema = create_schema();

    let compression = if use_zstd {
        Compression::ZSTD(parquet::basic::ZstdLevel::try_new(3).unwrap())
    } else {
        Compression::SNAPPY
    };

    let props = WriterProperties::builder()
        .set_compression(compression)
        .build();

    let file = File::create(path)?;
    let mut writer = ArrowWriter::try_new(file, schema.clone(), Some(props))?;

    // Generate data: 1 sample per second per container per metric
    let duration_secs = (end_time - start_time).num_seconds() as usize;
    let samples_per_container = duration_secs.max(1);
    let batch_size = 65536;

    let mut total_rows = 0;
    let mut current_time = start_time;
    let mut fetch_index = 0u64;

    while current_time < end_time {
        let batch_duration = Duration::seconds((batch_size / (containers.len() * METRIC_NAMES.len())).max(1) as i64);
        let batch_end = (current_time + batch_duration).min(end_time);

        let batch = build_batch(
            &schema,
            containers,
            identifier,
            current_time,
            batch_end,
            &mut fetch_index,
        )?;

        total_rows += batch.num_rows();
        writer.write(&batch)?;
        current_time = batch_end;
    }

    writer.close()?;
    Ok(total_rows)
}

/// Create the schema matching real fine-grained-monitor files
fn create_schema() -> Arc<Schema> {
    Arc::new(Schema::new(vec![
        Field::new("run_id", DataType::Utf8, false),
        Field::new("time", DataType::Timestamp(TimeUnit::Millisecond, None), false),
        Field::new("fetch_index", DataType::UInt64, false),
        Field::new("metric_name", DataType::Utf8, false),
        Field::new("metric_kind", DataType::Utf8, false),
        Field::new("value_int", DataType::UInt64, true),
        Field::new("value_float", DataType::Float64, true),
        Field::new("l_cluster_name", DataType::Utf8, true),
        Field::new("l_container_id", DataType::Utf8, true),
        Field::new("l_container_name", DataType::Utf8, true),
        Field::new("l_device", DataType::Utf8, true),
        Field::new("l_namespace", DataType::Utf8, true),
        Field::new("l_node_name", DataType::Utf8, true),
        Field::new("l_pid", DataType::Utf8, true),
        Field::new("l_pod_name", DataType::Utf8, true),
        Field::new("l_pod_uid", DataType::Utf8, true),
        Field::new("l_qos_class", DataType::Utf8, true),
        Field::new("value_histogram", DataType::Binary, true),
    ]))
}

/// Build a record batch for a time range
fn build_batch(
    schema: &Arc<Schema>,
    containers: &[Container],
    identifier: &str,
    start_time: DateTime<Utc>,
    end_time: DateTime<Utc>,
    fetch_index: &mut u64,
) -> Result<RecordBatch> {
    let duration_secs = (end_time - start_time).num_seconds().max(1) as usize;
    let estimated_rows = duration_secs * containers.len() * METRIC_NAMES.len();

    let mut run_id = StringBuilder::with_capacity(estimated_rows, estimated_rows * 20);
    let mut time = TimestampMillisecondBuilder::with_capacity(estimated_rows);
    let mut fetch_idx = UInt64Builder::with_capacity(estimated_rows);
    let mut metric_name = StringBuilder::with_capacity(estimated_rows, estimated_rows * 30);
    let mut metric_kind = StringBuilder::with_capacity(estimated_rows, estimated_rows * 10);
    let mut value_int = UInt64Builder::with_capacity(estimated_rows);
    let mut value_float = Float64Builder::with_capacity(estimated_rows);
    let mut l_cluster_name = StringBuilder::with_capacity(estimated_rows, estimated_rows * 15);
    let mut l_container_id = StringBuilder::with_capacity(estimated_rows, estimated_rows * 64);
    let mut l_container_name = StringBuilder::with_capacity(estimated_rows, estimated_rows * 20);
    let mut l_device = StringBuilder::with_capacity(estimated_rows, estimated_rows * 10);
    let mut l_namespace = StringBuilder::with_capacity(estimated_rows, estimated_rows * 15);
    let mut l_node_name = StringBuilder::with_capacity(estimated_rows, estimated_rows * 20);
    let mut l_pid = StringBuilder::with_capacity(estimated_rows, estimated_rows * 8);
    let mut l_pod_name = StringBuilder::with_capacity(estimated_rows, estimated_rows * 30);
    let mut l_pod_uid = StringBuilder::with_capacity(estimated_rows, estimated_rows * 36);
    let mut l_qos_class = StringBuilder::with_capacity(estimated_rows, estimated_rows * 12);
    let mut value_histogram = BinaryBuilder::with_capacity(estimated_rows, 0);

    let mut current_time = start_time;
    let mut seed = 0usize;

    while current_time < end_time {
        let time_ms = current_time.timestamp_millis();

        for container in containers {
            for metric in METRIC_NAMES.iter() {
                run_id.append_value(identifier);
                time.append_value(time_ms);
                fetch_idx.append_value(*fetch_index);
                metric_name.append_value(*metric);

                // Determine metric kind
                let kind = if metric.contains("usec") || metric.contains("bytes") {
                    "counter"
                } else {
                    "gauge"
                };
                metric_kind.append_value(kind);

                // Generate value
                let value = generate_value(metric, seed);
                if kind == "counter" {
                    value_int.append_value(value as u64);
                    value_float.append_null();
                } else {
                    value_int.append_null();
                    value_float.append_value(value);
                }

                // Labels
                l_cluster_name.append_value("bench-cluster");
                l_container_id.append_value(&container.id);
                l_container_name.append_value(&container.name);
                l_device.append_null(); // Only for I/O metrics with device
                l_namespace.append_value(&container.namespace);
                l_node_name.append_value("bench-node-0");
                l_pid.append_null(); // Only for process metrics
                l_pod_name.append_value(&container.pod_name);
                l_pod_uid.append_value(&container.pod_uid);
                l_qos_class.append_value(&container.qos_class);
                value_histogram.append_null();

                seed += 1;
            }
        }

        *fetch_index += 1;
        current_time = current_time + Duration::seconds(1);
    }

    let arrays: Vec<ArrayRef> = vec![
        Arc::new(run_id.finish()),
        Arc::new(time.finish()),
        Arc::new(fetch_idx.finish()),
        Arc::new(metric_name.finish()),
        Arc::new(metric_kind.finish()),
        Arc::new(value_int.finish()),
        Arc::new(value_float.finish()),
        Arc::new(l_cluster_name.finish()),
        Arc::new(l_container_id.finish()),
        Arc::new(l_container_name.finish()),
        Arc::new(l_device.finish()),
        Arc::new(l_namespace.finish()),
        Arc::new(l_node_name.finish()),
        Arc::new(l_pid.finish()),
        Arc::new(l_pod_name.finish()),
        Arc::new(l_pod_uid.finish()),
        Arc::new(l_qos_class.finish()),
        Arc::new(value_histogram.finish()),
    ];

    Ok(RecordBatch::try_new(schema.clone(), arrays)?)
}

/// Generate realistic metric values
fn generate_value(metric: &str, seed: usize) -> f64 {
    let noise = ((seed * 17 + 31) % 100) as f64 / 100.0;
    let time_factor = (seed / 1000) as f64;

    if metric.contains("percentage") || metric.contains("avg10") {
        20.0 + noise * 60.0
    } else if metric.contains("memory") && metric.contains("current") {
        100_000_000.0 + noise * 1_900_000_000.0
    } else if metric.contains("memory") && metric.contains("max") {
        2_000_000_000.0 + noise * 6_000_000_000.0
    } else if metric.contains("millicores") {
        noise * 4000.0
    } else if metric.contains("usec") {
        // Cumulative counter: increases over time
        time_factor * 1_000_000.0 + noise * 10000.0
    } else if metric.contains("bytes") {
        // Cumulative counter: increases over time
        time_factor * 100_000.0 + noise * 100000.0
    } else if metric.contains("pid") || metric.contains("thread") {
        1.0 + noise * 99.0
    } else if metric.contains("pss") {
        10_000_000.0 + noise * 490_000_000.0
    } else {
        noise * 1000.0
    }
}

/// Container metadata
struct Container {
    id: String,
    name: String,
    namespace: String,
    pod_name: String,
    pod_uid: String,
    qos_class: String,
}

impl Container {
    fn new(index: usize) -> Self {
        let short_id = format!("{:012x}", (index as u64 + 1) * 0x111111111111u64);
        let id = format!("{}{:052x}", short_id, index);

        Container {
            id,
            name: format!("container-{}", index),
            namespace: NAMESPACES[index % NAMESPACES.len()].to_string(),
            pod_name: format!("pod-{}", index),
            pod_uid: format!("00000000-0000-0000-0000-{:012x}", index),
            qos_class: QOS_CLASSES[index % QOS_CLASSES.len()].to_string(),
        }
    }
}
