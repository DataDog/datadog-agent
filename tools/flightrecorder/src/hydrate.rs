/// Hydrates a flight recorder recording directory into single queryable
/// Parquet files — one per signal type.
///
/// For metrics in context-key mode, resolves context_keys against
/// contexts.bin to produce inline name+tags columns.
/// For logs and trace_stats, concatenates all row groups into one file.
///
/// Usage:
///   hydrate <input-dir> <output-dir>           # single directory
///   hydrate --batch <top-dir> [-j N]           # parallel over subdirs
use std::collections::HashMap;
use std::fs::File;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Instant;

use anyhow::{Context, Result};
use arrow::array::{Array, ArrayRef, Float64Array, Int64Array, MapBuilder, StringArray, StringBuilder, UInt16Array, UInt64Array};
use arrow::array::DictionaryArray;
use arrow::compute;
use arrow::datatypes::{DataType, Field, Schema, UInt16Type};
use arrow::record_batch::RecordBatch;
use bytes::Bytes;
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;
use parquet::schema::types::ColumnPath;
use rayon::prelude::*;

use flightrecorder::signal_files::{scan_signal_files_sync, FileType};
use flightrecorder::writers::context_store::read_contexts_bin;

/// Read batch size — large to reduce per-batch overhead.
/// 17M+ rows per file means we want very large batches.
const READ_BATCH_SIZE: usize = 1_048_576;

/// Global memory budget for pre-reading files (default 40GB).
const MEMORY_BUDGET: usize = 40 * 1024 * 1024 * 1024;
static MEMORY_USED: AtomicUsize = AtomicUsize::new(0);

/// Acquire memory from the global budget, blocking until available.
fn acquire_memory(size: usize) {
    loop {
        let current = MEMORY_USED.load(Ordering::Relaxed);
        // Always allow at least one allocation (so a single large pod doesn't deadlock).
        if current == 0 || current + size <= MEMORY_BUDGET {
            if MEMORY_USED
                .compare_exchange(current, current + size, Ordering::AcqRel, Ordering::Relaxed)
                .is_ok()
            {
                return;
            }
        }
        std::thread::yield_now();
    }
}

fn release_memory(size: usize) {
    MEMORY_USED.fetch_sub(size, Ordering::AcqRel);
}

/// A signal file pre-loaded into memory.
struct PreloadedFile {
    path: PathBuf,
    file_type: FileType,
    data: Bytes,
}

/// The 7 reserved metric tag keys, in column order.
const RESERVED_KEYS: &[&str] = &["host", "device", "source", "service", "env", "version", "team"];

fn main() -> Result<()> {
    let args: Vec<String> = std::env::args().collect();

    if args.len() >= 3 && args[1] == "--batch" {
        return run_batch(&args[2..]);
    }

    if args.len() < 3 {
        eprintln!("Usage:");
        eprintln!("  hydrate <input-dir> <output-dir>           # single directory");
        eprintln!("  hydrate --batch <top-dir> [-j N]           # parallel over subdirs");
        std::process::exit(1);
    }

    let input_dir = Path::new(&args[1]);
    let output_dir = Path::new(&args[2]);
    hydrate_single(input_dir, output_dir)
}

fn run_batch(args: &[String]) -> Result<()> {
    if args.is_empty() {
        eprintln!("Usage: hydrate --batch <top-dir> [-j N]");
        std::process::exit(1);
    }
    let top_dir = Path::new(&args[0]);

    // Parse optional -j N and --skip-existing
    let mut num_threads: Option<usize> = None;
    let mut skip_existing = false;
    let mut i = 1;
    while i < args.len() {
        if args[i] == "-j" && i + 1 < args.len() {
            num_threads = Some(args[i + 1].parse().context("-j requires a number")?);
            i += 2;
        } else if args[i] == "--skip-existing" {
            skip_existing = true;
            i += 1;
        } else {
            i += 1;
        }
    }

    if let Some(n) = num_threads {
        rayon::ThreadPoolBuilder::new()
            .num_threads(n)
            .build_global()
            .ok();
    }

    // Collect subdirs that contain a "signals" subdirectory.
    let mut subdirs: Vec<PathBuf> = Vec::new();
    let mut skipped = 0usize;
    for entry in std::fs::read_dir(top_dir)? {
        let entry = entry?;
        let path = entry.path();
        if path.is_dir() && path.join("signals").is_dir() {
            if skip_existing && path.join("hydrated").is_dir() {
                skipped += 1;
                continue;
            }
            subdirs.push(path);
        }
    }
    subdirs.sort();
    if skipped > 0 {
        eprintln!("Skipped {} already-hydrated subdirs", skipped);
    }

    let total = subdirs.len();
    let done = AtomicUsize::new(0);
    let failed = AtomicUsize::new(0);
    let batch_start = Instant::now();

    eprintln!(
        "Hydrating {} subdirs with {} threads",
        total,
        num_threads.unwrap_or_else(rayon::current_num_threads)
    );

    subdirs.par_iter().for_each(|subdir| {
        let name = subdir.file_name().unwrap().to_string_lossy();
        let input = subdir.join("signals");
        let output = subdir.join("hydrated");
        let t0 = Instant::now();
        match hydrate_single(&input, &output) {
            Ok(()) => {
                let n = done.fetch_add(1, Ordering::Relaxed) + 1;
                let elapsed = batch_start.elapsed().as_secs();
                eprintln!(
                    "OK [{}/{}] ({:.1}s, total {}m{}s): {}",
                    n, total, t0.elapsed().as_secs_f64(),
                    elapsed / 60, elapsed % 60, name
                );
            }
            Err(e) => {
                failed.fetch_add(1, Ordering::Relaxed);
                let n = done.fetch_add(1, Ordering::Relaxed) + 1;
                eprintln!("FAIL [{}/{}]: {} — {}", n, total, name, e);
            }
        }
    });

    let f = failed.load(Ordering::Relaxed);
    let elapsed = batch_start.elapsed();
    eprintln!(
        "Done in {}m{:.1}s. {} succeeded, {} failed.",
        elapsed.as_secs() / 60,
        elapsed.as_secs_f64() % 60.0,
        total - f,
        f
    );
    Ok(())
}

fn hydrate_single(input_dir: &Path, output_dir: &Path) -> Result<()> {
    std::fs::create_dir_all(output_dir)?;

    let entries = scan_signal_files_sync(input_dir)?;

    // Calculate total size and acquire memory budget.
    let total_size: usize = entries.iter().map(|e| e.size as usize).sum();
    acquire_memory(total_size);

    // Pre-read all files into memory in parallel.
    let t0 = Instant::now();
    let preloaded: Vec<PreloadedFile> = entries
        .par_iter()
        .filter_map(|e| {
            match std::fs::read(&e.path) {
                Ok(data) => Some(PreloadedFile {
                    path: e.path.clone(),
                    file_type: e.file_type.clone(),
                    data: Bytes::from(data),
                }),
                Err(err) => {
                    eprintln!("  SKIP (read): {} — {}", e.path.display(), err);
                    None
                }
            }
        })
        .collect();
    let preread_ms = t0.elapsed().as_millis();

    let metrics_files: Vec<_> = preloaded.iter().filter(|f| f.file_type == FileType::Metrics).collect();
    let logs_files: Vec<_> = preloaded.iter().filter(|f| f.file_type == FileType::Logs).collect();
    let trace_stats_files: Vec<_> = preloaded.iter().filter(|f| f.file_type == FileType::TraceStats).collect();

    eprintln!(
        "  preread: {} files ({:.1} MB) in {}ms | metrics={} logs={} trace_stats={}",
        preloaded.len(),
        total_size as f64 / 1_048_576.0,
        preread_ms,
        metrics_files.len(),
        logs_files.len(),
        trace_stats_files.len(),
    );

    let result = (|| {
        if !metrics_files.is_empty() {
            let t = Instant::now();
            let rows = hydrate_metrics(input_dir, &metrics_files, output_dir)?;
            eprintln!(
                "  metrics: {} rows in {:.1}s",
                rows,
                t.elapsed().as_secs_f64(),
            );
        }

        if !logs_files.is_empty() {
            let t = Instant::now();
            let out = output_dir.join("logs.parquet");
            let rows = concatenate_files(&logs_files, &out)?;
            let size = std::fs::metadata(&out).map(|m| m.len()).unwrap_or(0);
            eprintln!(
                "  logs: {} rows -> {:.1} MB in {:.1}s",
                rows,
                size as f64 / 1_048_576.0,
                t.elapsed().as_secs_f64(),
            );
        }

        if !trace_stats_files.is_empty() {
            let t = Instant::now();
            let out = output_dir.join("trace_stats.parquet");
            let rows = concatenate_files(&trace_stats_files, &out)?;
            let size = std::fs::metadata(&out).map(|m| m.len()).unwrap_or(0);
            eprintln!(
                "  trace_stats: {} rows -> {:.1} MB in {:.1}s",
                rows,
                size as f64 / 1_048_576.0,
                t.elapsed().as_secs_f64(),
            );
        }

        Ok(())
    })();

    // Release memory budget (even on error).
    release_memory(total_size);
    result
}

/// Detect whether the metrics files use context-key or inline mode,
/// then hydrate accordingly.
fn hydrate_metrics(
    input_dir: &Path,
    files: &[&PreloadedFile],
    output: &Path,
) -> Result<u64> {
    // Peek at the first readable file's schema.
    let mut has_context_key = false;
    for entry in files {
        if let Ok(reader) = ParquetRecordBatchReaderBuilder::try_new(entry.data.clone()) {
            has_context_key = reader.schema().column_with_name("context_key").is_some();
            break;
        }
    }

    if has_context_key {
        hydrate_metrics_contextkey(input_dir, files, output)
    } else {
        // Inline mode — just concatenate.
        concatenate_files(files, output)
    }
}

/// Pre-decomposed context using dictionary indices for fast array construction.
/// Each string column (name + 7 tags) is represented as a u16 index into a
/// per-column dictionary. With 17.5M rows and ~31 unique contexts, this reduces
/// the string column data from ~330MB to ~17.5MB per column.
struct DecomposedContext {
    /// Dictionary indices for: name, tag_host, tag_device, tag_source, tag_service, tag_env, tag_version, tag_team
    dict_indices: [u16; 8],
    /// Overflow tags stored as pre-parsed (key, value) pairs for MAP column.
    overflow_tags: Vec<(Arc<str>, Arc<str>)>,
}

/// Dictionaries for the 8 string columns (name + 7 reserved tags).
/// Each dictionary maps string → u16 index, with a parallel Vec for the reverse mapping.
struct ColumnDictionaries {
    /// For each of 8 columns: the unique string values (index → string).
    values: [Vec<String>; 8],
    /// For each of 8 columns: string → index mapping.
    indices: [HashMap<String, u16>; 8],
}

impl ColumnDictionaries {
    fn new() -> Self {
        Self {
            values: Default::default(),
            indices: Default::default(),
        }
    }

    /// Insert a string into a column dictionary, returning its index.
    fn insert(&mut self, col: usize, s: &str) -> u16 {
        if let Some(&idx) = self.indices[col].get(s) {
            idx
        } else {
            let idx = self.values[col].len() as u16;
            self.values[col].push(s.to_string());
            self.indices[col].insert(s.to_string(), idx);
            idx
        }
    }

    /// Build Arrow StringArray for a column's dictionary values.
    fn arrow_values(&self, col: usize) -> StringArray {
        StringArray::from(self.values[col].iter().map(|s| s.as_str()).collect::<Vec<_>>())
    }
}

/// Special index for unknown context keys.
const UNKNOWN_IDX: u16 = u16::MAX;

/// Resolve context_keys to inline name+tags columns.
fn hydrate_metrics_contextkey(
    input_dir: &Path,
    files: &[&PreloadedFile],
    output: &Path,
) -> Result<u64> {
    // Load context definitions.
    let contexts_path = input_dir.join("contexts.bin");
    let contexts = read_contexts_bin(&contexts_path)
        .with_context(|| format!("reading {}", contexts_path.display()))?;

    // Pre-decompose all contexts and build per-column dictionaries.
    let mut dicts = ColumnDictionaries::new();
    // Insert the "unknown" / empty sentinel values first.
    let unknown_idx = dicts.insert(0, "<unknown>");
    for col in 1..8 {
        dicts.insert(col, "");
    }

    let ctx_map: HashMap<u64, DecomposedContext> = contexts
        .iter()
        .map(|(k, name, tags_joined)| {
            let (reserved_vec, overflow_kv) = decompose_tags(tags_joined);
            let name_idx = dicts.insert(0, name);
            let mut dict_indices = [0u16; 8];
            dict_indices[0] = name_idx;
            for (i, val) in reserved_vec.iter().enumerate() {
                dict_indices[i + 1] = dicts.insert(i + 1, val);
            }
            let overflow_tags: Vec<(Arc<str>, Arc<str>)> = overflow_kv
                .into_iter()
                .map(|(k, v)| (Arc::from(k.as_str()), Arc::from(v.as_str())))
                .collect();
            (*k, DecomposedContext {
                dict_indices,
                overflow_tags,
            })
        })
        .collect();
    eprintln!(
        "  contexts: {} entries, dict sizes: name={} host={} device={} source={} service={} env={} version={} team={}",
        ctx_map.len(),
        dicts.values[0].len(), dicts.values[1].len(), dicts.values[2].len(), dicts.values[3].len(),
        dicts.values[4].len(), dicts.values[5].len(), dicts.values[6].len(), dicts.values[7].len(),
    );

    // Output schema: dictionary-encoded string columns + MAP for overflow tags.
    // Using DictionaryArray<UInt16> for the repeated string columns — with ~31 unique
    // contexts, this stores just indices (17.5MB) instead of full strings (330MB) per column.
    let dict_dt = DataType::Dictionary(Box::new(DataType::UInt16), Box::new(DataType::Utf8));
    let map_field = Field::new(
        "tags",
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
    );
    let inline_schema = Arc::new(Schema::new(vec![
        Field::new("name", dict_dt.clone(), false),
        Field::new("tag_host", dict_dt.clone(), false),
        Field::new("tag_device", dict_dt.clone(), false),
        Field::new("tag_source", dict_dt.clone(), false),
        Field::new("tag_service", dict_dt.clone(), false),
        Field::new("tag_env", dict_dt.clone(), false),
        Field::new("tag_version", dict_dt.clone(), false),
        Field::new("tag_team", dict_dt.clone(), false),
        map_field,
        Field::new("value", DataType::Float64, false),
        Field::new("timestamp_ns", DataType::Int64, false),
        Field::new("sample_rate", DataType::Float64, false),
        Field::new("source", DataType::Utf8, false),
    ]));

    // Enable dictionary encoding for string columns — the writer will leverage our
    // pre-built DictionaryArrays (tiny dict page + RLE-encoded indices for 17.5M rows).
    // Disable dictionary for MAP and numeric columns where it's not useful.
    let make_props = || {
        WriterProperties::builder()
            .set_compression(Compression::SNAPPY)
            .set_dictionary_enabled(true)
            .set_column_dictionary_enabled(
                ColumnPath::from(vec!["tags".to_string(), "entries".to_string(), "keys".to_string()]),
                false,
            )
            .set_column_dictionary_enabled(
                ColumnPath::from(vec!["tags".to_string(), "entries".to_string(), "values".to_string()]),
                false,
            )
            .set_column_dictionary_enabled(ColumnPath::from("value"), false)
            .set_column_dictionary_enabled(ColumnPath::from("timestamp_ns"), false)
            .set_column_dictionary_enabled(ColumnPath::from("sample_rate"), false)
            .set_column_dictionary_enabled(ColumnPath::from("source"), false)
            .set_data_page_row_count_limit(1_000_000)
            .build()
    };

    // Pre-build Arrow dictionary values arrays (shared across all threads).
    let dict_values: [Arc<dyn Array>; 8] = std::array::from_fn(|i| Arc::new(dicts.arrow_values(i)) as Arc<dyn Array>);
    let unknown_indices: [u16; 8] = std::array::from_fn(|i| if i == 0 { unknown_idx } else { 0 });

    let ctx_map = &ctx_map;
    let dict_values = &dict_values;
    let unknown_indices = &unknown_indices;

    let metrics_start = Instant::now();
    let files_done = AtomicUsize::new(0);
    let total_rows_atomic = AtomicUsize::new(0);
    let num_files = files.len();

    // Process each input file in parallel — each produces its own output parquet.
    // This parallelizes the expensive write phase across all cores.
    let results: Vec<Result<u64>> = files.par_iter().map(|entry| {
        let file_start = Instant::now();
        let file_name = entry.path.file_name().unwrap_or_default().to_string_lossy();
        let file_size = entry.data.len();

        // Each input file gets its own output file in the output directory.
        let stem = entry.path.file_stem().unwrap_or_default().to_string_lossy();
        let out_path = output.join(format!("metrics_{}.parquet", stem));

        let out_file = File::create(&out_path)?;
        let mut writer = ArrowWriter::try_new(out_file, inline_schema.clone(), Some(make_props()))?;

        let reader = match ParquetRecordBatchReaderBuilder::try_new(entry.data.clone())
            .and_then(|b| b.with_batch_size(READ_BATCH_SIZE).build())
        {
            Ok(r) => r,
            Err(e) => {
                eprintln!("  SKIP (corrupt): {} — {}", entry.path.display(), e);
                return Ok(0);
            }
        };

        let mut file_rows = 0u64;
        let mut idx_bufs: [Vec<u16>; 8] = Default::default();

        for batch in reader {
            let batch = match batch {
                Ok(b) => b,
                Err(e) => {
                    eprintln!("  SKIP (batch): {} — {}", entry.path.display(), e);
                    break;
                }
            };
            let num_rows = batch.num_rows();

            let ckeys = batch.column_by_name("context_key").unwrap()
                .as_any().downcast_ref::<UInt64Array>().unwrap();
            let values = batch.column_by_name("value").unwrap()
                .as_any().downcast_ref::<Float64Array>().unwrap();
            let timestamps = batch.column_by_name("timestamp_ns").unwrap()
                .as_any().downcast_ref::<Int64Array>().unwrap();
            let sample_rates = batch.column_by_name("sample_rate").unwrap()
                .as_any().downcast_ref::<Float64Array>().unwrap();

            let source_col = batch.column_by_name("source").unwrap();
            let source_arr: ArrayRef = if source_col.data_type() == &DataType::Utf8 {
                source_col.clone()
            } else {
                compute::cast(source_col, &DataType::Utf8)?
            };

            for buf in &mut idx_bufs {
                buf.clear();
                buf.reserve(num_rows);
            }

            let mut map_builder = MapBuilder::new(
                None,
                StringBuilder::with_capacity(num_rows * 2, num_rows * 20),
                StringBuilder::with_capacity(num_rows * 2, num_rows * 40),
            );

            for i in 0..num_rows {
                let ckey = ckeys.value(i);
                if let Some(ctx) = ctx_map.get(&ckey) {
                    for (j, buf) in idx_bufs.iter_mut().enumerate() {
                        buf.push(ctx.dict_indices[j]);
                    }
                    for (k, v) in &ctx.overflow_tags {
                        map_builder.keys().append_value(&**k);
                        map_builder.values().append_value(&**v);
                    }
                } else {
                    for (j, buf) in idx_bufs.iter_mut().enumerate() {
                        buf.push(unknown_indices[j]);
                    }
                }
                map_builder.append(true).unwrap();
            }

            let tags_map = map_builder.finish();

            let mut columns: Vec<ArrayRef> = Vec::with_capacity(13);
            for i in 0..8 {
                let keys = UInt16Array::from(idx_bufs[i].clone());
                let dict = DictionaryArray::<UInt16Type>::try_new(keys, dict_values[i].clone())?;
                columns.push(Arc::new(dict) as ArrayRef);
            }
            columns.push(Arc::new(tags_map) as ArrayRef);
            columns.push(Arc::new(values.clone()) as ArrayRef);
            columns.push(Arc::new(timestamps.clone()) as ArrayRef);
            columns.push(Arc::new(sample_rates.clone()) as ArrayRef);
            columns.push(source_arr);

            let out_batch = RecordBatch::try_new(inline_schema.clone(), columns)
                .context("building inline RecordBatch")?;
            writer.write(&out_batch)?;
            file_rows += num_rows as u64;
        }

        writer.close()?;
        total_rows_atomic.fetch_add(file_rows as usize, Ordering::Relaxed);
        let n = files_done.fetch_add(1, Ordering::Relaxed) + 1;
        eprintln!(
            "    [{}/{}] {} ({:.1} KB, {} rows) in {:.1}s",
            n, num_files, file_name,
            file_size as f64 / 1024.0, file_rows,
            file_start.elapsed().as_secs_f64(),
        );
        Ok(file_rows)
    }).collect();

    // Check for errors.
    for r in &results {
        if let Err(e) = r {
            eprintln!("  ERROR: {}", e);
        }
    }

    let total_rows = total_rows_atomic.load(Ordering::Relaxed) as u64;
    eprintln!(
        "  metrics total: {:.1}s for {} rows across {} files",
        metrics_start.elapsed().as_secs_f64(),
        total_rows,
        num_files,
    );
    Ok(total_rows)
}

/// Concatenate all Parquet files into a single output file (preserving schema).
/// Skips corrupted or unreadable files.
fn concatenate_files(
    files: &[&PreloadedFile],
    output: &Path,
) -> Result<u64> {
    // Find the first readable file to get the schema.
    let mut schema = None;
    for entry in files {
        match ParquetRecordBatchReaderBuilder::try_new(entry.data.clone()) {
            Ok(r) => { schema = Some(r.schema().clone()); break; }
            Err(e) => {
                eprintln!("  SKIP (schema): {} — {}", entry.path.display(), e);
            }
        }
    }
    let schema = schema.context("no readable parquet files found")?;

    let props = WriterProperties::builder()
        .set_compression(Compression::SNAPPY)
        .set_dictionary_enabled(true)
        .build();
    let out_file = File::create(output)?;
    let mut writer = ArrowWriter::try_new(out_file, schema, Some(props))?;

    let mut total_rows = 0u64;

    for entry in files {
        let reader = match ParquetRecordBatchReaderBuilder::try_new(entry.data.clone())
            .and_then(|b| b.with_batch_size(READ_BATCH_SIZE).build())
        {
            Ok(r) => r,
            Err(e) => {
                eprintln!("  SKIP (corrupt): {} — {}", entry.path.display(), e);
                continue;
            }
        };
        for batch in reader {
            match batch {
                Ok(batch) => {
                    total_rows += batch.num_rows() as u64;
                    writer.write(&batch)?;
                }
                Err(e) => {
                    eprintln!("  SKIP (batch): {} — {}", entry.path.display(), e);
                    break;
                }
            }
        }
    }

    writer.close()?;
    Ok(total_rows)
}

/// Decompose pipe-joined tags into reserved columns + overflow key-value pairs.
fn decompose_tags(tags_joined: &str) -> (Vec<String>, Vec<(String, String)>) {
    let mut reserved = vec![String::new(); RESERVED_KEYS.len()];
    let mut overflow_kv: Vec<(String, String)> = Vec::new();

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
            overflow_kv.push((key.to_string(), value.to_string()));
        } else {
            // Tag without a colon — store with empty value.
            overflow_kv.push((tag.to_string(), String::new()));
        }
    }

    (reserved, overflow_kv)
}
