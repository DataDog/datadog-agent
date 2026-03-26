/// Hydrates a flight recorder recording directory into single queryable
/// Parquet files — one per signal type.
///
/// For metrics in context-key mode, resolves context_keys against
/// contexts.bin to produce inline name+tags columns.
/// For logs and trace_stats, concatenates all row groups into one file.
///
/// Usage: hydrate <input-dir> <output-dir>
use std::collections::HashMap;
use std::fs::File;
use std::path::Path;
use std::sync::Arc;

use anyhow::{Context, Result};
use arrow::array::{Array, ArrayRef, Float64Array, Int64Array, StringArray, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema, UInt32Type};
use arrow::record_batch::RecordBatch;
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;

use flightrecorder::signal_files::{scan_signal_files_sync, FileType};
use flightrecorder::writers::context_store::read_contexts_bin;

/// The 7 reserved metric tag keys, in column order.
const RESERVED_KEYS: &[&str] = &["host", "device", "source", "service", "env", "version", "team"];

fn main() -> Result<()> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 3 {
        eprintln!("Usage: hydrate <input-dir> <output-dir>");
        std::process::exit(1);
    }
    let input_dir = Path::new(&args[1]);
    let output_dir = Path::new(&args[2]);

    std::fs::create_dir_all(output_dir)?;

    let entries = scan_signal_files_sync(input_dir)?;
    let metrics_files: Vec<_> = entries
        .iter()
        .filter(|e| e.file_type == FileType::Metrics)
        .collect();
    let logs_files: Vec<_> = entries
        .iter()
        .filter(|e| e.file_type == FileType::Logs)
        .collect();
    let trace_stats_files: Vec<_> = entries
        .iter()
        .filter(|e| e.file_type == FileType::TraceStats)
        .collect();

    let total_input_bytes: u64 = entries.iter().map(|e| e.size).sum();
    eprintln!(
        "Input: {} files ({:.1} MB)",
        entries.len(),
        total_input_bytes as f64 / 1_048_576.0
    );

    if !metrics_files.is_empty() {
        let out = output_dir.join("metrics.parquet");
        let rows = hydrate_metrics(input_dir, &metrics_files, &out)?;
        let size = std::fs::metadata(&out).map(|m| m.len()).unwrap_or(0);
        eprintln!(
            "  metrics.parquet: {} rows ({:.1} MB)",
            rows,
            size as f64 / 1_048_576.0
        );
    }

    if !logs_files.is_empty() {
        let out = output_dir.join("logs.parquet");
        let rows = concatenate_files(&logs_files, &out)?;
        let size = std::fs::metadata(&out).map(|m| m.len()).unwrap_or(0);
        eprintln!(
            "  logs.parquet: {} rows ({:.1} MB)",
            rows,
            size as f64 / 1_048_576.0
        );
    }

    if !trace_stats_files.is_empty() {
        let out = output_dir.join("trace_stats.parquet");
        let rows = concatenate_files(&trace_stats_files, &out)?;
        let size = std::fs::metadata(&out).map(|m| m.len()).unwrap_or(0);
        eprintln!(
            "  trace_stats.parquet: {} rows ({:.1} MB)",
            rows,
            size as f64 / 1_048_576.0
        );
    }

    eprintln!("Done.");
    Ok(())
}

/// Detect whether the metrics files use context-key or inline mode,
/// then hydrate accordingly.
fn hydrate_metrics(
    input_dir: &Path,
    files: &[&flightrecorder::signal_files::SignalEntry],
    output: &Path,
) -> Result<u64> {
    // Peek at the first file's schema.
    let first_file = File::open(&files[0].path)?;
    let first_reader = ParquetRecordBatchReaderBuilder::try_new(first_file)?;
    let schema = first_reader.schema().clone();

    let has_context_key = schema.column_with_name("context_key").is_some();

    if has_context_key {
        hydrate_metrics_contextkey(input_dir, files, output)
    } else {
        // Inline mode — just concatenate.
        concatenate_files(files, output)
    }
}

/// Resolve context_keys to inline name+tags columns.
fn hydrate_metrics_contextkey(
    input_dir: &Path,
    files: &[&flightrecorder::signal_files::SignalEntry],
    output: &Path,
) -> Result<u64> {
    // Load context definitions.
    let contexts_path = input_dir.join("contexts.bin");
    let contexts = read_contexts_bin(&contexts_path)
        .with_context(|| format!("reading {}", contexts_path.display()))?;
    let ctx_map: HashMap<u64, (&str, &str)> = contexts
        .iter()
        .map(|(k, n, t)| (*k, (n.as_str(), t.as_str())))
        .collect();
    eprintln!("  Loaded {} contexts from contexts.bin", ctx_map.len());

    // Output schema: inline 13 columns.
    let dt = DataType::Utf8;
    let inline_schema = Arc::new(Schema::new(vec![
        Field::new("name", dt.clone(), false),
        Field::new("tag_host", dt.clone(), false),
        Field::new("tag_device", dt.clone(), false),
        Field::new("tag_source", dt.clone(), false),
        Field::new("tag_service", dt.clone(), false),
        Field::new("tag_env", dt.clone(), false),
        Field::new("tag_version", dt.clone(), false),
        Field::new("tag_team", dt.clone(), false),
        Field::new("tags_overflow", dt, false),
        Field::new("value", DataType::Float64, false),
        Field::new("timestamp_ns", DataType::Int64, false),
        Field::new("sample_rate", DataType::Float64, false),
        Field::new("source", DataType::Utf8, false),
    ]));

    let props = WriterProperties::builder()
        .set_compression(Compression::SNAPPY)
        .set_dictionary_enabled(true)
        .build();
    let out_file = File::create(output)?;
    let mut writer = ArrowWriter::try_new(out_file, inline_schema.clone(), Some(props))?;

    let mut total_rows = 0u64;

    for entry in files {
        let file = File::open(&entry.path)?;
        let reader = ParquetRecordBatchReaderBuilder::try_new(file)?.build()?;

        for batch in reader {
            let batch = batch?;
            let num_rows = batch.num_rows();

            // Extract input columns.
            let ckeys = batch
                .column_by_name("context_key")
                .unwrap()
                .as_any()
                .downcast_ref::<UInt64Array>()
                .unwrap();
            let values = batch
                .column_by_name("value")
                .unwrap()
                .as_any()
                .downcast_ref::<Float64Array>()
                .unwrap();
            let timestamps = batch
                .column_by_name("timestamp_ns")
                .unwrap()
                .as_any()
                .downcast_ref::<Int64Array>()
                .unwrap();
            let sample_rates = batch
                .column_by_name("sample_rate")
                .unwrap()
                .as_any()
                .downcast_ref::<Float64Array>()
                .unwrap();

            // Source column may be dictionary-encoded or plain string.
            let source_strings: Vec<String> = extract_string_column(&batch, "source");

            // Resolve context_keys → name + decomposed tags.
            let mut names = Vec::with_capacity(num_rows);
            let mut tag_host = Vec::with_capacity(num_rows);
            let mut tag_device = Vec::with_capacity(num_rows);
            let mut tag_source = Vec::with_capacity(num_rows);
            let mut tag_service = Vec::with_capacity(num_rows);
            let mut tag_env = Vec::with_capacity(num_rows);
            let mut tag_version = Vec::with_capacity(num_rows);
            let mut tag_team = Vec::with_capacity(num_rows);
            let mut tags_overflow = Vec::with_capacity(num_rows);

            for i in 0..num_rows {
                let ckey = ckeys.value(i);
                if let Some(&(name, tags_joined)) = ctx_map.get(&ckey) {
                    names.push(name.to_string());
                    let (reserved, overflow) = decompose_tags(tags_joined);
                    tag_host.push(reserved[0].clone());
                    tag_device.push(reserved[1].clone());
                    tag_source.push(reserved[2].clone());
                    tag_service.push(reserved[3].clone());
                    tag_env.push(reserved[4].clone());
                    tag_version.push(reserved[5].clone());
                    tag_team.push(reserved[6].clone());
                    tags_overflow.push(overflow);
                } else {
                    names.push("<unknown>".to_string());
                    tag_host.push(String::new());
                    tag_device.push(String::new());
                    tag_source.push(String::new());
                    tag_service.push(String::new());
                    tag_env.push(String::new());
                    tag_version.push(String::new());
                    tag_team.push(String::new());
                    tags_overflow.push(String::new());
                }
            }

            let columns: Vec<ArrayRef> = vec![
                Arc::new(StringArray::from(names)),
                Arc::new(StringArray::from(tag_host)),
                Arc::new(StringArray::from(tag_device)),
                Arc::new(StringArray::from(tag_source)),
                Arc::new(StringArray::from(tag_service)),
                Arc::new(StringArray::from(tag_env)),
                Arc::new(StringArray::from(tag_version)),
                Arc::new(StringArray::from(tag_team)),
                Arc::new(StringArray::from(tags_overflow)),
                Arc::new(values.clone()) as ArrayRef,
                Arc::new(timestamps.clone()) as ArrayRef,
                Arc::new(sample_rates.clone()) as ArrayRef,
                Arc::new(StringArray::from(source_strings)),
            ];

            let out_batch = RecordBatch::try_new(inline_schema.clone(), columns)
                .context("building inline RecordBatch")?;
            writer.write(&out_batch)?;
            total_rows += num_rows as u64;
        }
    }

    writer.close()?;
    Ok(total_rows)
}

/// Concatenate all Parquet files into a single output file (preserving schema).
fn concatenate_files(
    files: &[&flightrecorder::signal_files::SignalEntry],
    output: &Path,
) -> Result<u64> {
    let first_file = File::open(&files[0].path)?;
    let first_reader = ParquetRecordBatchReaderBuilder::try_new(first_file)?;
    let schema = first_reader.schema().clone();

    let props = WriterProperties::builder()
        .set_compression(Compression::SNAPPY)
        .set_dictionary_enabled(true)
        .build();
    let out_file = File::create(output)?;
    let mut writer = ArrowWriter::try_new(out_file, schema, Some(props))?;

    let mut total_rows = 0u64;

    for entry in files {
        let file = File::open(&entry.path)?;
        let reader = ParquetRecordBatchReaderBuilder::try_new(file)?.build()?;
        for batch in reader {
            let batch = batch?;
            total_rows += batch.num_rows() as u64;
            writer.write(&batch)?;
        }
    }

    writer.close()?;
    Ok(total_rows)
}

/// Decompose pipe-joined tags into reserved columns + overflow.
fn decompose_tags(tags_joined: &str) -> (Vec<String>, String) {
    let mut reserved = vec![String::new(); RESERVED_KEYS.len()];
    let mut overflow_parts: Vec<&str> = Vec::new();

    for tag in tags_joined.split('|') {
        if tag.is_empty() {
            continue;
        }
        if let Some(colon_pos) = tag.find(':') {
            let key = &tag[..colon_pos];
            let value = &tag[colon_pos + 1..];
            if let Some(idx) = RESERVED_KEYS.iter().position(|&k| k == key) {
                reserved[idx] = value.to_string();
                continue;
            }
        }
        overflow_parts.push(tag);
    }

    let overflow = overflow_parts.join("|");
    (reserved, overflow)
}

/// Extract a string column, handling both plain StringArray and DictionaryArray.
fn extract_string_column(batch: &RecordBatch, name: &str) -> Vec<String> {
    let col = batch.column_by_name(name).unwrap();

    // Try dictionary first.
    if let Some(dict) = col
        .as_any()
        .downcast_ref::<arrow::array::DictionaryArray<UInt32Type>>()
    {
        let values = dict
            .values()
            .as_any()
            .downcast_ref::<StringArray>()
            .unwrap();
        return (0..dict.len())
            .map(|i| {
                let key = dict.keys().value(i) as usize;
                values.value(key).to_string()
            })
            .collect();
    }

    // Plain string.
    if let Some(arr) = col.as_any().downcast_ref::<StringArray>() {
        return (0..arr.len()).map(|i| arr.value(i).to_string()).collect();
    }

    vec!["<unsupported>".to_string(); batch.num_rows()]
}
