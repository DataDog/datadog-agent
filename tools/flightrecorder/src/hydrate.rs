/// Converts contexts.bin files into queryable Parquet files.
///
/// For each recording directory, reads contexts.bin and writes a
/// contexts.parquet with decomposed name, reserved tags, and overflow tags MAP.
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
use arrow::array::{ArrayRef, MapBuilder, StringArray, StringBuilder, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;
use rayon::prelude::*;

use flightrecorder::writers::context_store::read_contexts_bin;

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
        eprintln!("Usage: hydrate --batch <top-dir> [-j N] [--skip-existing]");
        std::process::exit(1);
    }
    let top_dir = Path::new(&args[0]);

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

    let mut subdirs: Vec<PathBuf> = Vec::new();
    let mut skipped = 0usize;
    for entry in std::fs::read_dir(top_dir)? {
        let entry = entry?;
        let path = entry.path();
        if path.is_dir() && path.join("signals").is_dir() {
            if skip_existing && path.join("hydrated").join("contexts.parquet").exists() {
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
        match hydrate_single(&input, &output) {
            Ok(()) => {
                let n = done.fetch_add(1, Ordering::Relaxed) + 1;
                eprintln!("OK [{}/{}]: {}", n, total, name);
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
        "Done in {:.1}s. {} succeeded, {} failed.",
        elapsed.as_secs_f64(),
        total - f,
        f
    );
    Ok(())
}

fn hydrate_single(input_dir: &Path, output_dir: &Path) -> Result<()> {
    std::fs::create_dir_all(output_dir)?;

    let contexts_path = input_dir.join("contexts.bin");
    if !contexts_path.exists() {
        eprintln!("  SKIP: no contexts.bin in {}", input_dir.display());
        return Ok(());
    }

    let contexts = read_contexts_bin(&contexts_path)
        .with_context(|| format!("reading {}", contexts_path.display()))?;

    write_contexts_parquet(&contexts, &output_dir.join("contexts.parquet"))?;

    eprintln!(
        "  {} contexts -> {}",
        contexts.len(),
        output_dir.join("contexts.parquet").display()
    );
    Ok(())
}

fn write_contexts_parquet(
    contexts: &[(u64, String, String)],
    output: &Path,
) -> Result<()> {
    let dt = DataType::Utf8;
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

    let schema = Arc::new(Schema::new(vec![
        Field::new("context_key", DataType::UInt64, false),
        Field::new("name", dt.clone(), false),
        Field::new("tag_host", dt.clone(), false),
        Field::new("tag_device", dt.clone(), false),
        Field::new("tag_source", dt.clone(), false),
        Field::new("tag_service", dt.clone(), false),
        Field::new("tag_env", dt.clone(), false),
        Field::new("tag_version", dt.clone(), false),
        Field::new("tag_team", dt.clone(), false),
        map_field,
    ]));

    let props = WriterProperties::builder()
        .set_compression(Compression::SNAPPY)
        .set_dictionary_enabled(true)
        .build();

    let out_file = File::create(output)?;
    let mut writer = ArrowWriter::try_new(out_file, schema.clone(), Some(props))?;

    let num_rows = contexts.len();

    let mut context_keys = Vec::with_capacity(num_rows);
    let mut names = Vec::with_capacity(num_rows);
    let mut tag_host = Vec::with_capacity(num_rows);
    let mut tag_device = Vec::with_capacity(num_rows);
    let mut tag_source = Vec::with_capacity(num_rows);
    let mut tag_service = Vec::with_capacity(num_rows);
    let mut tag_env = Vec::with_capacity(num_rows);
    let mut tag_version = Vec::with_capacity(num_rows);
    let mut tag_team = Vec::with_capacity(num_rows);
    let mut map_builder = MapBuilder::new(None, StringBuilder::new(), StringBuilder::new());

    for (key, name, tags_joined) in contexts {
        context_keys.push(*key);
        names.push(name.as_str());

        let (reserved, overflow_kv) = decompose_tags(tags_joined);
        tag_host.push(reserved[0].clone());
        tag_device.push(reserved[1].clone());
        tag_source.push(reserved[2].clone());
        tag_service.push(reserved[3].clone());
        tag_env.push(reserved[4].clone());
        tag_version.push(reserved[5].clone());
        tag_team.push(reserved[6].clone());

        for (k, v) in &overflow_kv {
            map_builder.keys().append_value(k);
            map_builder.values().append_value(v);
        }
        map_builder.append(true).unwrap();
    }

    let columns: Vec<ArrayRef> = vec![
        Arc::new(UInt64Array::from(context_keys)),
        Arc::new(StringArray::from(names)),
        Arc::new(StringArray::from(tag_host.iter().map(|s| s.as_str()).collect::<Vec<_>>())),
        Arc::new(StringArray::from(tag_device.iter().map(|s| s.as_str()).collect::<Vec<_>>())),
        Arc::new(StringArray::from(tag_source.iter().map(|s| s.as_str()).collect::<Vec<_>>())),
        Arc::new(StringArray::from(tag_service.iter().map(|s| s.as_str()).collect::<Vec<_>>())),
        Arc::new(StringArray::from(tag_env.iter().map(|s| s.as_str()).collect::<Vec<_>>())),
        Arc::new(StringArray::from(tag_version.iter().map(|s| s.as_str()).collect::<Vec<_>>())),
        Arc::new(StringArray::from(tag_team.iter().map(|s| s.as_str()).collect::<Vec<_>>())),
        Arc::new(map_builder.finish()) as ArrayRef,
    ];

    let batch = RecordBatch::try_new(schema, columns)?;
    writer.write(&batch)?;
    writer.close()?;

    Ok(())
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
            overflow_kv.push((tag.to_string(), String::new()));
        }
    }

    (reserved, overflow_kv)
}
