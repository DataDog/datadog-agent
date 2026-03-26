/// Flight recorder Parquet file reader.
///
/// Usage: reader <file.parquet> [--search <substring>] [--limit <n>]
use std::fs::File;

use arrow::array::Array;
use arrow::datatypes::UInt32Type;
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;

fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 2 {
        eprintln!("Usage: reader <file.parquet> [--search <substring>] [--limit <n>]");
        std::process::exit(1);
    }
    let path = &args[1];

    let mut search: Option<String> = None;
    let mut limit: usize = usize::MAX;
    let mut i = 2;
    while i < args.len() {
        match args[i].as_str() {
            "--search" if i + 1 < args.len() => {
                search = Some(args[i + 1].clone());
                i += 2;
            }
            "--limit" if i + 1 < args.len() => {
                limit = args[i + 1].parse().unwrap_or(usize::MAX);
                i += 2;
            }
            _ => {
                i += 1;
            }
        }
    }

    let file = File::open(path)?;
    let reader = ParquetRecordBatchReaderBuilder::try_new(file)?
        .build()?;

    let mut rows_printed = 0;

    for batch in reader {
        let batch = batch?;
        let schema = batch.schema();
        let num_rows = batch.num_rows();

        for row in 0..num_rows {
            if rows_printed >= limit {
                return Ok(());
            }

            let mut parts: Vec<String> = Vec::new();
            for (col_idx, field) in schema.fields().iter().enumerate() {
                let col = batch.column(col_idx);
                let val = format_value(col.as_ref(), row);
                parts.push(format!("{}={}", field.name(), val));
            }

            let line = parts.join(" | ");
            if let Some(ref s) = search {
                if !line.contains(s.as_str()) {
                    continue;
                }
            }
            println!("{line}");
            rows_printed += 1;
        }
    }

    Ok(())
}

fn format_value(col: &dyn Array, row: usize) -> String {
    if col.is_null(row) {
        return "null".to_string();
    }

    // Try dictionary first (most string columns use this).
    if let Some(dict) = col.as_any().downcast_ref::<arrow::array::DictionaryArray<UInt32Type>>() {
        let values = dict.values().as_any().downcast_ref::<arrow::array::StringArray>().unwrap();
        let key = dict.keys().value(row) as usize;
        return values.value(key).to_string();
    }

    // Primitive types.
    if let Some(a) = col.as_any().downcast_ref::<arrow::array::Int64Array>() {
        return a.value(row).to_string();
    }
    if let Some(a) = col.as_any().downcast_ref::<arrow::array::Float64Array>() {
        return format!("{:.6}", a.value(row));
    }
    if let Some(a) = col.as_any().downcast_ref::<arrow::array::UInt64Array>() {
        return a.value(row).to_string();
    }
    if let Some(a) = col.as_any().downcast_ref::<arrow::array::UInt32Array>() {
        return a.value(row).to_string();
    }

    // Binary.
    if let Some(a) = col.as_any().downcast_ref::<arrow::array::BinaryArray>() {
        let bytes = a.value(row);
        if let Ok(s) = std::str::from_utf8(bytes) {
            return s.to_string();
        }
        return format!("<{} bytes>", bytes.len());
    }

    // String.
    if let Some(a) = col.as_any().downcast_ref::<arrow::array::StringArray>() {
        return a.value(row).to_string();
    }

    format!("<unsupported type: {:?}>", col.data_type())
}
