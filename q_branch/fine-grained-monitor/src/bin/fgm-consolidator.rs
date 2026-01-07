//! Streaming Parquet Consolidator for fine-grained-monitor.
//!
//! REQ-CON-001: Reduces query latency by merging many small parquet files into
//! fewer larger files.
//!
//! REQ-CON-002: Uses streaming approach to maintain bounded memory usage
//! (~80MB) regardless of data volume.

use std::collections::HashMap;
use std::fs::File;
use std::io::BufWriter;
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime};

use anyhow::{Context, Result};
use clap::Parser;
use glob::glob;
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;
use tokio::signal::unix::{signal, SignalKind};
use tokio::sync::watch;
use tracing::{error, info, warn};

/// Streaming Parquet Consolidator
///
/// Merges small parquet files into larger consolidated files to reduce
/// query latency in the metrics viewer.
#[derive(Parser, Debug, Clone)]
#[command(name = "fgm-consolidator")]
#[command(about = "Consolidate fine-grained-monitor parquet files")]
struct Args {
    /// Data directory containing parquet files
    #[arg(long, default_value = "/data")]
    data_dir: PathBuf,

    /// Seconds between consolidation checks
    #[arg(long, default_value = "300")]
    check_interval: u64,

    /// Minimum files in a partition to trigger consolidation
    #[arg(long, default_value = "10")]
    min_files: usize,

    /// Maximum consolidated file size in MB
    #[arg(long, default_value = "500")]
    max_output_size_mb: u64,

    /// Minimum file age in seconds before consolidation
    #[arg(long, default_value = "300")]
    min_age_secs: u64,

    /// ZSTD compression level (1-22)
    #[arg(long, default_value = "3")]
    compression_level: i32,

    /// Show what would be consolidated without doing it
    #[arg(long)]
    dry_run: bool,
}

/// Information about a parquet file for consolidation planning
#[derive(Debug, Clone)]
struct FileInfo {
    path: PathBuf,
    size: u64,
    #[allow(dead_code)] // Used for debugging
    modified: SystemTime,
    timestamp: String, // Extracted from filename for sorting
}

/// Partition key for grouping files
#[derive(Debug, Clone, Hash, Eq, PartialEq)]
struct PartitionKey {
    date: String,
    identifier: String,
}

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let args = Args::parse();

    info!(
        data_dir = %args.data_dir.display(),
        check_interval = args.check_interval,
        min_files = args.min_files,
        max_output_size_mb = args.max_output_size_mb,
        min_age_secs = args.min_age_secs,
        dry_run = args.dry_run,
        "Starting fgm-consolidator"
    );

    run(args).await
}

async fn run(args: Args) -> Result<()> {
    // Set up shutdown signal handling (REQ-CON-005)
    let (shutdown_tx, mut shutdown_rx) = watch::channel(false);

    let mut sigint = signal(SignalKind::interrupt())?;
    let mut sigterm = signal(SignalKind::terminate())?;

    tokio::spawn(async move {
        tokio::select! {
            _ = sigint.recv() => {
                info!("Received SIGINT, initiating graceful shutdown");
            }
            _ = sigterm.recv() => {
                info!("Received SIGTERM, initiating graceful shutdown");
            }
        }
        let _ = shutdown_tx.send(true);
    });

    // Main consolidation loop (REQ-CON-005)
    let mut interval = tokio::time::interval(Duration::from_secs(args.check_interval));

    // Run first check immediately
    if let Err(e) = consolidation_pass(&args).await {
        error!(error = %e, "Initial consolidation pass failed");
    }

    loop {
        tokio::select! {
            _ = interval.tick() => {
                if let Err(e) = consolidation_pass(&args).await {
                    error!(error = %e, "Consolidation pass failed");
                }
            }
            _ = shutdown_rx.changed() => {
                if *shutdown_rx.borrow() {
                    info!("Shutdown requested, exiting");
                    break;
                }
            }
        }
    }

    Ok(())
}

/// Single consolidation pass - scan and consolidate eligible partitions
async fn consolidation_pass(args: &Args) -> Result<()> {
    info!("Starting consolidation pass");

    // Discover eligible files
    let files = discover_files(&args.data_dir, args.min_age_secs)?;

    if files.is_empty() {
        info!("No eligible files found");
        return Ok(());
    }

    // Group by partition (REQ-CON-006)
    let partitions = group_by_partition(files);

    info!(
        partitions = partitions.len(),
        "Found partitions with eligible files"
    );

    // Process each partition that exceeds threshold
    for (key, mut files) in partitions {
        if files.len() < args.min_files {
            continue;
        }

        info!(
            date = %key.date,
            identifier = %key.identifier,
            file_count = files.len(),
            "Partition eligible for consolidation"
        );

        // Sort files by timestamp for consistent ordering
        files.sort_by(|a, b| a.timestamp.cmp(&b.timestamp));

        // Plan consolidation batches respecting size limit (REQ-CON-006)
        let batches = plan_consolidation(&files, args.max_output_size_mb * 1024 * 1024);

        for batch in batches {
            if batch.len() < 2 {
                // No point consolidating a single file
                continue;
            }

            let start_ts = &batch.first().unwrap().timestamp;
            let end_ts = &batch.last().unwrap().timestamp;
            let output_name = format!("consolidated-{}-{}.parquet", start_ts, end_ts);
            let partition_dir = args
                .data_dir
                .join(format!("dt={}", key.date))
                .join(format!("identifier={}", key.identifier));
            let output_path = partition_dir.join(&output_name);

            let batch_paths: Vec<PathBuf> = batch.iter().map(|f| f.path.clone()).collect();
            let total_size: u64 = batch.iter().map(|f| f.size).sum();

            info!(
                files = batch.len(),
                total_size_mb = total_size / (1024 * 1024),
                output = %output_path.display(),
                "Consolidating batch"
            );

            if args.dry_run {
                for file in &batch {
                    info!(file = %file.path.display(), "Would consolidate");
                }
                continue;
            }

            // Perform consolidation (REQ-CON-002, REQ-CON-003)
            match consolidate_files(&batch_paths, &output_path, args.compression_level) {
                Ok(()) => {
                    info!(output = %output_path.display(), "Consolidation successful");

                    // Delete source files (REQ-CON-003)
                    for file in &batch {
                        if let Err(e) = std::fs::remove_file(&file.path) {
                            warn!(
                                file = %file.path.display(),
                                error = %e,
                                "Failed to delete source file"
                            );
                        }
                    }
                }
                Err(e) => {
                    error!(
                        output = %output_path.display(),
                        error = %e,
                        "Consolidation failed, source files preserved"
                    );
                }
            }
        }
    }

    info!("Consolidation pass complete");
    Ok(())
}

/// Discover parquet files eligible for consolidation
fn discover_files(data_dir: &Path, min_age_secs: u64) -> Result<Vec<FileInfo>> {
    let pattern = format!("{}/**/metrics-*.parquet", data_dir.display());
    let now = SystemTime::now();
    let min_age = Duration::from_secs(min_age_secs);

    let mut files = Vec::new();

    for entry in glob(&pattern)? {
        let path = match entry {
            Ok(p) => p,
            Err(e) => {
                warn!(error = %e, "Failed to read glob entry");
                continue;
            }
        };

        let metadata = match std::fs::metadata(&path) {
            Ok(m) => m,
            Err(e) => {
                warn!(path = %path.display(), error = %e, "Failed to read metadata");
                continue;
            }
        };

        let modified = match metadata.modified() {
            Ok(m) => m,
            Err(e) => {
                warn!(path = %path.display(), error = %e, "Failed to get mtime");
                continue;
            }
        };

        // REQ-CON-003: Exclude files modified within min_age
        let age = now.duration_since(modified).unwrap_or(Duration::ZERO);
        if age < min_age {
            continue;
        }

        // Extract timestamp from filename (metrics-YYYYMMDDTHHMMSSZ.parquet)
        let timestamp = path
            .file_stem()
            .and_then(|s| s.to_str())
            .and_then(|s| s.strip_prefix("metrics-"))
            .unwrap_or("")
            .to_string();

        files.push(FileInfo {
            path,
            size: metadata.len(),
            modified,
            timestamp,
        });
    }

    Ok(files)
}

/// Group files by partition key (date, identifier)
fn group_by_partition(files: Vec<FileInfo>) -> HashMap<PartitionKey, Vec<FileInfo>> {
    let mut partitions: HashMap<PartitionKey, Vec<FileInfo>> = HashMap::new();

    for file in files {
        // Extract partition from path: /data/dt=YYYY-MM-DD/identifier=xxx/file.parquet
        let key = extract_partition_key(&file.path);
        if let Some(key) = key {
            partitions.entry(key).or_default().push(file);
        }
    }

    partitions
}

/// Extract partition key from file path
fn extract_partition_key(path: &Path) -> Option<PartitionKey> {
    let path_str = path.to_str()?;

    // Find dt= and identifier= components
    let date = path_str
        .split('/')
        .find(|s| s.starts_with("dt="))?
        .strip_prefix("dt=")?
        .to_string();

    let identifier = path_str
        .split('/')
        .find(|s| s.starts_with("identifier="))?
        .strip_prefix("identifier=")?
        .to_string();

    Some(PartitionKey { date, identifier })
}

/// Plan consolidation batches respecting max output size (REQ-CON-006)
fn plan_consolidation(files: &[FileInfo], max_size: u64) -> Vec<Vec<FileInfo>> {
    let mut batches = Vec::new();
    let mut current_batch = Vec::new();
    let mut current_size = 0u64;

    for file in files {
        // If adding this file would exceed limit and we have files, start new batch
        if current_size + file.size > max_size && !current_batch.is_empty() {
            batches.push(std::mem::take(&mut current_batch));
            current_size = 0;
        }

        current_size += file.size;
        current_batch.push(file.clone());
    }

    if !current_batch.is_empty() {
        batches.push(current_batch);
    }

    batches
}

/// Consolidate multiple parquet files into one using streaming (REQ-CON-002)
fn consolidate_files(
    input_paths: &[PathBuf],
    output_path: &Path,
    compression_level: i32,
) -> Result<()> {
    if input_paths.is_empty() {
        anyhow::bail!("No input files provided");
    }

    // REQ-CON-003: Write to temporary file first
    let tmp_path = output_path.with_extension("parquet.tmp");

    // Read schema from first file
    let first_file = File::open(&input_paths[0]).context("Failed to open first input file")?;
    let first_reader = ParquetRecordBatchReaderBuilder::try_new(first_file)
        .context("Failed to create reader for schema")?;
    let schema = first_reader.schema().clone();

    // Create output writer with ZSTD compression
    let output_file = File::create(&tmp_path).context("Failed to create output file")?;
    let writer = BufWriter::new(output_file);

    let props = WriterProperties::builder()
        .set_compression(Compression::ZSTD(
            parquet::basic::ZstdLevel::try_new(compression_level).unwrap_or_default(),
        ))
        .build();

    let mut arrow_writer =
        ArrowWriter::try_new(writer, schema.clone(), Some(props)).context("Failed to create ArrowWriter")?;

    // REQ-CON-002: Stream through each input file, one row group at a time
    for input_path in input_paths {
        let file = match File::open(input_path) {
            Ok(f) => f,
            Err(e) => {
                warn!(path = %input_path.display(), error = %e, "Skipping unreadable file");
                continue;
            }
        };

        let builder = match ParquetRecordBatchReaderBuilder::try_new(file) {
            Ok(b) => b,
            Err(e) => {
                warn!(path = %input_path.display(), error = %e, "Skipping invalid parquet");
                continue;
            }
        };

        // REQ-CON-004: Validate schema matches
        if builder.schema() != &schema {
            warn!(
                path = %input_path.display(),
                "Schema mismatch, skipping file"
            );
            continue;
        }

        let reader = builder.with_batch_size(65536).build()?;

        // REQ-CON-002: Process one batch at a time
        for batch_result in reader {
            let batch = batch_result.context("Failed to read record batch")?;
            arrow_writer
                .write(&batch)
                .context("Failed to write record batch")?;
            // Batch is dropped here, freeing memory
        }
    }

    // REQ-CON-003: Close writer to finalize footer
    arrow_writer.close().context("Failed to close ArrowWriter")?;

    // Fsync to ensure data is physically written to disk before rename
    // This prevents corruption from system crashes, sleep/resume, or I/O errors
    let sync_file = File::open(&tmp_path).context("Failed to reopen temp file for sync")?;
    sync_file
        .sync_all()
        .context("Failed to sync temp file to disk")?;

    // REQ-CON-003: Atomic rename
    std::fs::rename(&tmp_path, output_path).context("Failed to rename temp file to final")?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_partition_key() {
        let path = PathBuf::from("/data/dt=2025-12-30/identifier=test-pod/metrics-20251230T120000Z.parquet");
        let key = extract_partition_key(&path).unwrap();
        assert_eq!(key.date, "2025-12-30");
        assert_eq!(key.identifier, "test-pod");
    }

    #[test]
    fn test_plan_consolidation_single_batch() {
        let files = vec![
            FileInfo {
                path: PathBuf::from("a.parquet"),
                size: 100,
                modified: SystemTime::now(),
                timestamp: "20251230T120000Z".to_string(),
            },
            FileInfo {
                path: PathBuf::from("b.parquet"),
                size: 100,
                modified: SystemTime::now(),
                timestamp: "20251230T120130Z".to_string(),
            },
        ];

        let batches = plan_consolidation(&files, 1000);
        assert_eq!(batches.len(), 1);
        assert_eq!(batches[0].len(), 2);
    }

    #[test]
    fn test_plan_consolidation_multiple_batches() {
        let files = vec![
            FileInfo {
                path: PathBuf::from("a.parquet"),
                size: 600,
                modified: SystemTime::now(),
                timestamp: "20251230T120000Z".to_string(),
            },
            FileInfo {
                path: PathBuf::from("b.parquet"),
                size: 600,
                modified: SystemTime::now(),
                timestamp: "20251230T120130Z".to_string(),
            },
        ];

        let batches = plan_consolidation(&files, 1000);
        assert_eq!(batches.len(), 2);
        assert_eq!(batches[0].len(), 1);
        assert_eq!(batches[1].len(), 1);
    }
}
