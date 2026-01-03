#!/usr/bin/env -S cargo +nightly -Zscript

---
[package]
edition = "2024"

[dependencies]
parquet = { version = "54", features = ["arrow"] }
arrow = "54"
anyhow = "1"
clap = { version = "4", features = ["derive"] }
walkdir = "2"

[profile.dev]
opt-level = 3
---

//! Merge multiple fine-grained-monitor Parquet files into a single file.
//!
//! Uses streaming writes to handle arbitrarily large datasets without
//! loading all data into memory.
//!
//! Usage:
//!     ./merge_parquet.rs /path/to/parquet/dir -o merged.parquet
//!     ./merge_parquet.rs file1.parquet file2.parquet -o merged.parquet

use anyhow::{Context, Result};
use clap::{Parser, ValueEnum};
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;
use std::collections::HashSet;
use std::fs::File;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Instant;
use walkdir::WalkDir;

#[derive(Parser, Debug)]
#[command(name = "merge_parquet")]
#[command(about = "Merge multiple Parquet files into one")]
struct Args {
    /// Input parquet files or directories
    #[arg(required = true)]
    inputs: Vec<PathBuf>,

    /// Output file path
    #[arg(short, long, default_value = "merged.parquet")]
    output: PathBuf,

    /// Compression codec
    #[arg(long, value_enum, default_value = "zstd")]
    compression: CompressionCodec,
}

#[derive(Debug, Clone, ValueEnum)]
enum CompressionCodec {
    Zstd,
    Snappy,
    Gzip,
    None,
}

impl CompressionCodec {
    fn to_parquet(&self) -> Compression {
        match self {
            CompressionCodec::Zstd => Compression::ZSTD(Default::default()),
            CompressionCodec::Snappy => Compression::SNAPPY,
            CompressionCodec::Gzip => Compression::GZIP(Default::default()),
            CompressionCodec::None => Compression::UNCOMPRESSED,
        }
    }
}

fn find_parquet_files(paths: &[PathBuf]) -> Result<Vec<PathBuf>> {
    let mut files: HashSet<PathBuf> = HashSet::new();

    for path in paths {
        if path.is_file() {
            if path.extension().map_or(false, |e| e == "parquet") {
                files.insert(path.canonicalize().unwrap_or_else(|_| path.clone()));
            }
        } else if path.is_dir() {
            for entry in WalkDir::new(path).into_iter().filter_map(|e| e.ok()) {
                let p = entry.path();
                if p.is_file() && p.extension().map_or(false, |e| e == "parquet") {
                    files.insert(p.canonicalize().unwrap_or_else(|_| p.to_path_buf()));
                }
            }
        } else {
            eprintln!("Warning: {} does not exist, skipping", path.display());
        }
    }

    let mut sorted: Vec<_> = files.into_iter().collect();
    sorted.sort();
    Ok(sorted)
}

/// Extract schema from the first valid parquet file
fn get_schema_from_files(files: &[PathBuf]) -> Result<Arc<arrow::datatypes::Schema>> {
    for path in files {
        let metadata = std::fs::metadata(path)?;
        if metadata.len() == 0 {
            continue;
        }

        let file = match File::open(path) {
            Ok(f) => f,
            Err(_) => continue,
        };

        let builder = match ParquetRecordBatchReaderBuilder::try_new(file) {
            Ok(b) => b,
            Err(_) => continue,
        };

        return Ok(builder.schema().clone());
    }

    anyhow::bail!("No valid parquet files found to extract schema")
}

fn main() -> Result<()> {
    let args = Args::parse();
    let total_start = Instant::now();

    // Find all parquet files
    let input_files = find_parquet_files(&args.inputs)?;

    if input_files.is_empty() {
        anyhow::bail!("No parquet files found");
    }

    eprintln!("Found {} parquet files to merge", input_files.len());

    // Get schema from first valid file
    let schema = get_schema_from_files(&input_files)?;

    // Create output writer upfront - this enables streaming writes
    eprintln!("Creating output file {:?}...", args.output);
    let output_file = File::create(&args.output).context("Failed to create output file")?;

    let props = WriterProperties::builder()
        .set_compression(args.compression.to_parquet())
        .build();

    let mut writer = ArrowWriter::try_new(output_file, schema, Some(props))?;

    // Stream through all files, writing batches directly to output
    let stream_start = Instant::now();
    let mut total_rows: u64 = 0;
    let mut total_batches: u64 = 0;
    let mut files_read = 0;

    for (i, path) in input_files.iter().enumerate() {
        // Skip empty files
        let metadata = std::fs::metadata(path)?;
        if metadata.len() == 0 {
            eprintln!("  Skipping empty file: {}", path.display());
            continue;
        }

        let file = match File::open(path) {
            Ok(f) => f,
            Err(e) => {
                eprintln!("  Warning: Failed to open {}: {}", path.display(), e);
                continue;
            }
        };

        let builder = match ParquetRecordBatchReaderBuilder::try_new(file) {
            Ok(b) => b,
            Err(e) => {
                eprintln!("  Warning: Failed to read {}: {}", path.display(), e);
                continue;
            }
        };

        let reader = builder.with_batch_size(65536).build()?;

        // Stream batches directly to output - no buffering in memory
        for batch_result in reader {
            match batch_result {
                Ok(batch) => {
                    total_rows += batch.num_rows() as u64;
                    total_batches += 1;
                    writer.write(&batch)?;
                }
                Err(e) => {
                    eprintln!("  Warning: Error reading batch from {}: {}", path.display(), e);
                }
            }
        }

        files_read += 1;

        if (i + 1) % 100 == 0 {
            eprintln!(
                "  Processed {}/{} files ({} rows) [{:.1}s]",
                i + 1,
                input_files.len(),
                format_number(total_rows),
                stream_start.elapsed().as_secs_f64()
            );
        }
    }

    if total_rows == 0 {
        // Clean up empty output file
        drop(writer);
        let _ = std::fs::remove_file(&args.output);
        anyhow::bail!("No valid data found in parquet files");
    }

    // Finalize output file
    eprintln!("Finalizing output file...");
    writer.close()?;

    let output_size = std::fs::metadata(&args.output)?.len();

    eprintln!(
        "\nDone! Merged {} files, {} batches, {} rows [{:.2}s]",
        files_read,
        total_batches,
        format_number(total_rows),
        stream_start.elapsed().as_secs_f64()
    );

    eprintln!(
        "Output: {:?} ({:.2} MB) [{:.2}s total]",
        args.output,
        output_size as f64 / 1024.0 / 1024.0,
        total_start.elapsed().as_secs_f64()
    );

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
