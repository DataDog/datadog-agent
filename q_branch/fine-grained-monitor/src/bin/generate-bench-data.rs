//! Benchmark data generator for fine-grained-monitor.
//!
//! Generates realistic parquet data matching observed production patterns.
//!
//! ## Scenarios
//!
//! **Realistic**: Models stable workload with occasional pod restarts.
//! - ~20 containers per identifier
//! - 2-3 pod identifiers per day (DaemonSet restarts)
//! - ~150-200 MB/day pattern
//!
//! **Stress**: Models heavy churn with many containers and frequent restarts.
//! - ~50 containers per identifier
//! - 5-7 pod identifiers per day
//! - ~500-800 MB/day pattern
//!
//! ## Usage
//!
//! ```bash
//! cargo run --release --bin generate-bench-data -- --scenario realistic
//! cargo run --release --bin generate-bench-data -- --scenario stress
//! cargo run --release --bin generate-bench-data -- --scenario realistic --duration 24h
//! ```
//!
//! ## Running benchmarks
//!
//! ```bash
//! BENCH_DATA=testdata/bench/realistic cargo bench
//! BENCH_DATA=testdata/bench/stress cargo bench
//! ```

use anyhow::Result;
use arrow::array::{
    ArrayRef, BinaryBuilder, Float64Builder, StringBuilder, TimestampMillisecondBuilder,
    UInt64Builder,
};
use arrow::datatypes::{DataType, Field, Schema, TimeUnit};
use arrow::record_batch::RecordBatch;
use chrono::{DateTime, Duration, Utc};
use clap::{Parser, ValueEnum};
use fine_grained_monitor::sidecar::{ContainerSidecar, SidecarContainer, sidecar_path_for_parquet};
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;
use std::collections::HashMap;
use std::collections::hash_map::DefaultHasher;
use std::fs::{self, File};
use std::hash::{Hash, Hasher};
use std::path::{Path, PathBuf};
use std::sync::Arc;

/// Benchmark scenarios based on observed production patterns.
#[derive(Debug, Clone, Copy, ValueEnum)]
enum Scenario {
    /// Stable workload: ~20 containers, 2-3 identifiers/day, ~150-200 MB/day
    Realistic,
    /// Heavy churn: ~50 containers, 5-7 identifiers/day, ~500-800 MB/day
    Stress,
}

impl Scenario {
    /// Containers per pod identifier
    fn containers(&self) -> usize {
        match self {
            Scenario::Realistic => 20,
            Scenario::Stress => 50,
        }
    }

    /// Pod identifiers per day (from DaemonSet restarts)
    fn identifiers_per_day(&self) -> usize {
        match self {
            Scenario::Realistic => 3,  // Worst-case stable: 2-3 restarts/day
            Scenario::Stress => 7,     // Heavy churn: ~7 restarts/day
        }
    }

    /// Whether containers churn within each identifier's lifetime
    fn container_churn(&self) -> bool {
        match self {
            Scenario::Realistic => false, // Same containers throughout
            Scenario::Stress => true,     // Containers restart mid-identifier
        }
    }
}

#[derive(Parser)]
#[command(name = "generate-bench-data")]
#[command(about = "Generate benchmark data for fine-grained-monitor")]
struct Args {
    /// Scenario: realistic (stable workload) or stress (heavy churn)
    #[arg(short, long, value_enum, default_value = "realistic")]
    scenario: Scenario,

    /// Duration of data to generate (e.g., "1h", "6h", "24h", "7d")
    #[arg(short, long, default_value = "1h")]
    duration: String,
}

/// Parse duration string like "1h", "6h", "24h", "7d"
fn parse_duration(s: &str) -> Result<Duration> {
    let s = s.trim().to_lowercase();
    if let Some(hours) = s.strip_suffix('h') {
        Ok(Duration::hours(hours.parse()?))
    } else if let Some(days) = s.strip_suffix('d') {
        Ok(Duration::days(days.parse()?))
    } else if let Some(mins) = s.strip_suffix('m') {
        Ok(Duration::minutes(mins.parse()?))
    } else {
        anyhow::bail!("Invalid duration format. Use: 1h, 6h, 24h, 7d, etc.")
    }
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
const NAMESPACES: &[&str] = &["default", "kube-system", "monitoring", "app-prod", "app-staging"];

fn main() -> Result<()> {
    let args = Args::parse();
    let duration = parse_duration(&args.duration)?;

    let output_dir = PathBuf::from("testdata/bench").join(format!("{:?}", args.scenario).to_lowercase());

    println!("Generating {:?} scenario:", args.scenario);
    println!("  Duration: {} ({} hours)", args.duration, duration.num_hours());
    println!("  Containers per identifier: {}", args.scenario.containers());
    println!("  Identifiers per day: {}", args.scenario.identifiers_per_day());
    println!("  Container churn: {}", args.scenario.container_churn());
    println!("  Output: {}", output_dir.display());

    // Clean and create output directory
    if output_dir.exists() {
        fs::remove_dir_all(&output_dir)?;
    }
    fs::create_dir_all(&output_dir)?;

    // Generate data
    generate_scenario(&output_dir, args.scenario, duration)?;

    // Print summary
    let total_size: u64 = walkdir::WalkDir::new(&output_dir)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter_map(|e| e.metadata().ok())
        .filter(|m| m.is_file())
        .map(|m| m.len())
        .sum();

    let parquet_count: usize = walkdir::WalkDir::new(&output_dir)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| e.path().extension().map(|x| x == "parquet").unwrap_or(false))
        .count();

    let sidecar_count: usize = walkdir::WalkDir::new(&output_dir)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| e.path().extension().map(|x| x == "containers").unwrap_or(false))
        .count();

    println!("\nGenerated:");
    println!("  Parquet files: {}", parquet_count);
    println!("  Sidecar files: {} (v2 with labels)", sidecar_count);
    println!("  Total size: {:.1} MB", total_size as f64 / 1_000_000.0);
    println!("\nRun benchmarks with:");
    println!("  BENCH_DATA={} cargo bench", output_dir.display());

    Ok(())
}

/// Generate data for the given scenario and duration.
fn generate_scenario(output_dir: &Path, scenario: Scenario, duration: Duration) -> Result<()> {
    let now = Utc::now();
    let start_time = now - duration;

    // Calculate number of identifiers based on duration
    let duration_days = (duration.num_hours() as f64 / 24.0).max(1.0 / 24.0); // At least 1 hour
    let num_identifiers = ((duration_days * scenario.identifiers_per_day() as f64).ceil() as usize).max(1);

    println!("  Generating {} identifiers for {:.1} days of data", num_identifiers, duration_days);

    // Each identifier covers a portion of the total duration
    let identifier_duration = duration / num_identifiers as i32;

    for id_idx in 0..num_identifiers {
        let identifier = format!("fgm-bench-{:05}", id_idx);
        let id_start = start_time + identifier_duration * id_idx as i32;
        let id_end = (id_start + identifier_duration).min(now);

        println!("\n  Identifier {}/{}: {}", id_idx + 1, num_identifiers, identifier);
        println!("    Time range: {} to {}",
            id_start.format("%Y-%m-%d %H:%M"),
            id_end.format("%Y-%m-%d %H:%M"));

        generate_identifier_data(
            output_dir,
            &identifier,
            scenario,
            id_start,
            id_end,
            id_idx,
        )?;
    }

    Ok(())
}

/// Generate data for a single identifier (pod instance).
fn generate_identifier_data(
    output_dir: &Path,
    identifier: &str,
    scenario: Scenario,
    start_time: DateTime<Utc>,
    end_time: DateTime<Utc>,
    id_idx: usize,
) -> Result<()> {
    let duration = end_time - start_time;
    let duration_mins = duration.num_minutes();

    // Consolidated files: ~15 minutes each (like production)
    let consolidated_duration = Duration::minutes(15);
    let num_consolidated = (duration_mins / 15).max(1) as usize;

    // Fresh files: last 6 minutes (4 files × 90 seconds)
    let fresh_duration = Duration::minutes(6);
    let has_fresh = duration > fresh_duration;

    // Consolidated files cover all but the last 6 minutes
    let consolidated_end = if has_fresh {
        end_time - fresh_duration
    } else {
        end_time
    };

    // Container generations (for churn scenario)
    let num_generations = if scenario.container_churn() {
        (duration_mins / 30).max(1) as usize // New containers every ~30 mins
    } else {
        1
    };

    println!("    Consolidated files: {} ({}-min each)", num_consolidated, consolidated_duration.num_minutes());
    if has_fresh {
        println!("    Fresh files: 4 (90-sec each)");
    }
    if scenario.container_churn() {
        println!("    Container generations: {} (churn every ~30 mins)", num_generations);
    }

    // Generate consolidated files
    let mut current_time = start_time;
    let mut file_idx = 0;

    while current_time < consolidated_end {
        let file_end = (current_time + consolidated_duration).min(consolidated_end);

        // Determine which container generation this file uses
        let generation = if scenario.container_churn() {
            let elapsed_mins = (current_time - start_time).num_minutes();
            (elapsed_mins / 30) as usize
        } else {
            0
        };

        let containers = generate_containers(scenario.containers(), id_idx, generation);

        let date_str = current_time.format("%Y-%m-%d").to_string();
        let id_dir = output_dir
            .join(format!("dt={}", date_str))
            .join(format!("identifier={}", identifier));
        fs::create_dir_all(&id_dir)?;

        let filename = format!(
            "consolidated-{}-{}.parquet",
            current_time.format("%Y%m%dT%H%M%SZ"),
            file_end.format("%Y%m%dT%H%M%SZ")
        );
        let path = id_dir.join(&filename);

        let rows = generate_file(&path, &containers, identifier, current_time, file_end)?;
        print!("\r    [{}/{}] {} ({} rows)          ",
            file_idx + 1, num_consolidated, filename, rows);

        current_time = file_end;
        file_idx += 1;
    }
    println!();

    // Generate fresh files (most recent 6 minutes)
    if has_fresh {
        let latest_generation = if scenario.container_churn() {
            num_generations - 1
        } else {
            0
        };
        let containers = generate_containers(scenario.containers(), id_idx, latest_generation);

        for i in 0..4 {
            let file_start = end_time - Duration::seconds((4 - i) * 90);
            let file_end = file_start + Duration::seconds(90);

            if file_start < start_time {
                continue;
            }

            let date_str = file_start.format("%Y-%m-%d").to_string();
            let id_dir = output_dir
                .join(format!("dt={}", date_str))
                .join(format!("identifier={}", identifier));
            fs::create_dir_all(&id_dir)?;

            let filename = format!("metrics-{}.parquet", file_start.format("%Y%m%dT%H%M%SZ"));
            let path = id_dir.join(&filename);

            let rows = generate_file(&path, &containers, identifier, file_start, file_end)?;
            println!("    Fresh: {} ({} rows)", filename, rows);
        }
    }

    Ok(())
}

/// Generate container metadata for a given configuration.
fn generate_containers(count: usize, id_idx: usize, generation: usize) -> Vec<Container> {
    (0..count)
        .map(|c| Container::new(c, id_idx, generation))
        .collect()
}

/// Generate a single parquet file with realistic data and companion sidecar file.
fn generate_file(
    path: &PathBuf,
    containers: &[Container],
    identifier: &str,
    start_time: DateTime<Utc>,
    end_time: DateTime<Utc>,
) -> Result<usize> {
    let schema = create_schema();

    // Use ZSTD compression like production consolidated files
    let props = WriterProperties::builder()
        .set_compression(Compression::ZSTD(parquet::basic::ZstdLevel::try_new(3).unwrap()))
        .build();

    let file = File::create(path)?;
    let mut writer = ArrowWriter::try_new(file, schema.clone(), Some(props))?;

    let mut total_rows = 0;
    let mut current_time = start_time;
    let mut fetch_index = 0u64;
    let batch_duration = Duration::seconds(60); // Build batches of 60 seconds

    while current_time < end_time {
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

    // Write companion sidecar file (v2 format with labels)
    let sidecar_containers: Vec<SidecarContainer> = containers
        .iter()
        .map(|c| c.to_sidecar_container())
        .collect();
    let sidecar = ContainerSidecar::new(sidecar_containers);
    let sidecar_path = sidecar_path_for_parquet(path);
    sidecar.write(&sidecar_path)?;

    Ok(total_rows)
}

/// Create the schema matching real fine-grained-monitor files.
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

/// Build a record batch for a time range.
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

                let kind = if metric.contains("usec") || metric.contains("bytes") {
                    "counter"
                } else {
                    "gauge"
                };
                metric_kind.append_value(kind);

                let value = generate_value(metric, seed);
                if kind == "counter" {
                    value_int.append_value(value as u64);
                    value_float.append_null();
                } else {
                    value_int.append_null();
                    value_float.append_value(value);
                }

                l_cluster_name.append_value("bench-cluster");
                l_container_id.append_value(&container.id);
                l_container_name.append_value(&container.name);
                l_device.append_null();
                l_namespace.append_value(&container.namespace);
                l_node_name.append_value("bench-node");
                l_pid.append_null();
                l_pod_name.append_value(&container.pod_name);
                l_pod_uid.append_value(&container.pod_uid);
                l_qos_class.append_value(&container.qos_class);
                value_histogram.append_null();

                seed += 1;
            }
        }

        *fetch_index += 1;
        current_time += Duration::seconds(1);
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

/// Generate realistic metric values.
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
        time_factor * 1_000_000.0 + noise * 10000.0
    } else if metric.contains("bytes") {
        time_factor * 100_000.0 + noise * 100000.0
    } else if metric.contains("pid") || metric.contains("thread") {
        1.0 + noise * 99.0
    } else if metric.contains("pss") {
        10_000_000.0 + noise * 490_000_000.0
    } else {
        noise * 1000.0
    }
}

/// Generate a deterministic 64-character hex container ID with unique prefix.
/// Uses hashing to ensure the first 12 characters (short_id) are unique per container.
fn generate_container_id(container_idx: usize, id_idx: usize, generation: usize) -> String {
    // Hash the inputs to get well-distributed bits
    let mut hasher = DefaultHasher::new();
    container_idx.hash(&mut hasher);
    id_idx.hash(&mut hasher);
    generation.hash(&mut hasher);
    let h1 = hasher.finish();

    // Generate additional hashes for the full 64-char ID
    hasher.write_u64(h1);
    let h2 = hasher.finish();

    hasher.write_u64(h2);
    let h3 = hasher.finish();

    hasher.write_u64(h3);
    let h4 = hasher.finish();

    // Combine into 64-character hex string (256 bits)
    format!("{:016x}{:016x}{:016x}{:016x}", h1, h2, h3, h4)
}

/// Container metadata.
struct Container {
    id: String,
    name: String,
    namespace: String,
    pod_name: String,
    pod_uid: String,
    qos_class: String,
    labels: HashMap<String, String>,
}

impl Container {
    /// Create container with unique ID based on identifier index, container index, and generation.
    fn new(container_idx: usize, id_idx: usize, generation: usize) -> Self {
        // Create deterministic but unique container ID using hash-based approach
        // This ensures each container has a unique 12-char prefix (short_id)
        let id = generate_container_id(container_idx, id_idx, generation);
        let unique_seed = (id_idx * 10000) + (generation * 1000) + container_idx;

        // Generate realistic K8s labels
        let app_name = format!("app-{}", container_idx);
        let namespace = NAMESPACES[container_idx % NAMESPACES.len()].to_string();
        let labels = HashMap::from([
            ("app".to_string(), app_name.clone()),
            ("app.kubernetes.io/name".to_string(), app_name.clone()),
            ("app.kubernetes.io/instance".to_string(), format!("{}-{}", app_name, id_idx)),
            ("app.kubernetes.io/component".to_string(), "backend".to_string()),
            ("pod-template-hash".to_string(), format!("{:08x}", unique_seed)),
            ("team".to_string(), format!("team-{}", container_idx % 5)),
            ("env".to_string(), if namespace.contains("prod") { "production" } else { "development" }.to_string()),
        ]);

        Container {
            id,
            name: app_name,
            namespace,
            pod_name: format!("app-{}-pod", container_idx),
            pod_uid: format!(
                "{:08x}-{:04x}-{:04x}-{:04x}-{:012x}",
                id_idx, generation, container_idx, 0, unique_seed
            ),
            qos_class: QOS_CLASSES[container_idx % QOS_CLASSES.len()].to_string(),
            labels,
        }
    }

    /// Convert to sidecar format
    fn to_sidecar_container(&self) -> SidecarContainer {
        SidecarContainer {
            container_id: self.id[..12].to_string(), // Short ID (first 12 chars)
            pod_name: Some(self.pod_name.clone()),
            container_name: Some(self.name.clone()),
            namespace: Some(self.namespace.clone()),
            pod_uid: Some(self.pod_uid.clone()),
            qos_class: self.qos_class.clone(),
            labels: Some(self.labels.clone()),
        }
    }
}
