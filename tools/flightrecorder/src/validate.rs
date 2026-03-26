/// Validates Parquet files written by flightrecorder.
///
/// Reads all .parquet metric files in a directory and outputs a report with:
///   - total row count
///   - file count
///   - column schema
///
/// Usage: validate <directory>
use std::collections::HashMap;
use std::fs::File;
use std::path::Path;

use arrow::array::Array;
use arrow::datatypes::UInt32Type;
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;

fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 2 {
        eprintln!("Usage: validate <directory>");
        std::process::exit(1);
    }
    let dir = Path::new(&args[1]);

    let json_output = args.iter().any(|a| a == "--json");

    let mut total_rows = 0u64;
    let mut files_read = 0u64;
    let mut rows_by_file: Vec<(String, u64)> = Vec::new();
    let mut schema_sample: Option<Vec<String>> = None;
    let mut context_counts: HashMap<String, u64> = HashMap::new();

    for entry in std::fs::read_dir(dir)? {
        let entry = entry?;
        let name = entry.file_name().into_string().unwrap_or_default();
        if !name.ends_with(".parquet") || !name.contains("metrics") {
            continue;
        }

        let path = entry.path();
        let file = match File::open(&path) {
            Ok(f) => f,
            Err(e) => {
                eprintln!("  WARN: cannot open {}: {}", path.display(), e);
                continue;
            }
        };

        let reader = match ParquetRecordBatchReaderBuilder::try_new(file) {
            Ok(b) => b.build()?,
            Err(e) => {
                eprintln!("  WARN: cannot read {}: {}", path.display(), e);
                continue;
            }
        };

        let mut file_rows = 0u64;
        for batch in reader {
            let batch = batch?;

            if schema_sample.is_none() {
                schema_sample = Some(
                    batch
                        .schema()
                        .fields()
                        .iter()
                        .map(|f| format!("{}: {:?}", f.name(), f.data_type()))
                        .collect(),
                );
            }

            // Try to extract "name" column for context counting (inline mode).
            if let Some(name_col) = batch.column_by_name("name") {
                if let Some(dict) = name_col
                    .as_any()
                    .downcast_ref::<arrow::array::DictionaryArray<UInt32Type>>()
                {
                    let values = dict
                        .values()
                        .as_any()
                        .downcast_ref::<arrow::array::StringArray>()
                        .unwrap();
                    for i in 0..dict.len() {
                        if !dict.is_null(i) {
                            let key = dict.keys().value(i) as usize;
                            let name = values.value(key);
                            *context_counts.entry(name.to_string()).or_insert(0) += 1;
                        }
                    }
                }
            }

            file_rows += batch.num_rows() as u64;
        }

        total_rows += file_rows;
        files_read += 1;
        rows_by_file.push((name, file_rows));
    }

    if json_output {
        let mut top_contexts: Vec<_> = context_counts.into_iter().collect();
        top_contexts.sort_by(|a, b| b.1.cmp(&a.1));
        top_contexts.truncate(20);

        println!("{{");
        println!("  \"total_rows\": {},", total_rows);
        println!("  \"files_read\": {},", files_read);
        println!("  \"top_contexts\": {{");
        for (i, (name, count)) in top_contexts.iter().enumerate() {
            let comma = if i + 1 < top_contexts.len() { "," } else { "" };
            println!("    \"{}\": {}{}", name, count, comma);
        }
        println!("  }}");
        println!("}}");
    } else {
        println!("=== Flight Recorder Validate ===");
        println!("Directory: {}", dir.display());
        println!("Files read: {}", files_read);
        println!("Total rows: {}", total_rows);

        if let Some(schema) = schema_sample {
            println!("\nSchema ({} columns):", schema.len());
            for col in &schema {
                println!("  {col}");
            }
        }

        if !context_counts.is_empty() {
            let mut top: Vec<_> = context_counts.into_iter().collect();
            top.sort_by(|a, b| b.1.cmp(&a.1));
            println!("\nTop 20 metric names ({} distinct):", top.len());
            for (name, count) in top.iter().take(20) {
                println!("  {count:>8}  {name}");
            }
        }

        println!("\nFiles:");
        for (name, rows) in &rows_by_file {
            println!("  {rows:>8} rows  {name}");
        }
    }

    Ok(())
}
