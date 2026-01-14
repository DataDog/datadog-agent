//! Inspect parquet file for bloom filter presence
//!
//! Usage: cargo run --bin inspect-bloom -- <parquet-file>

use parquet::file::reader::{FileReader, SerializedFileReader};
use std::env;
use std::fs::File;

fn main() {
    let args: Vec<String> = env::args().collect();
    if args.len() < 2 {
        eprintln!("Usage: {} <parquet-file>", args[0]);
        std::process::exit(1);
    }

    let path = &args[1];
    let file = File::open(path).expect("Failed to open file");
    let reader = SerializedFileReader::new(file).expect("Failed to create reader");
    let metadata = reader.metadata();

    println!("File: {}", path);
    println!("Schema: {} columns", metadata.file_metadata().schema().get_fields().len());
    println!("Row groups: {}", metadata.num_row_groups());
    println!();

    // Find l_container_id column index
    let schema = metadata.file_metadata().schema();
    let mut container_id_idx: Option<usize> = None;
    for (i, field) in schema.get_fields().iter().enumerate() {
        if field.name() == "l_container_id" {
            container_id_idx = Some(i);
            println!("l_container_id column index: {}", i);
            break;
        }
    }

    let container_id_idx = match container_id_idx {
        Some(idx) => idx,
        None => {
            println!("l_container_id column not found!");
            return;
        }
    };

    println!();
    println!("Bloom filter info per row group:");
    for rg_idx in 0..metadata.num_row_groups() {
        let rg = metadata.row_group(rg_idx);
        let col = rg.column(container_id_idx);

        let has_bloom = col.bloom_filter_offset().is_some();
        let bloom_offset = col.bloom_filter_offset();
        let bloom_length = col.bloom_filter_length();

        println!(
            "  RG {}: bloom_filter_offset={:?}, bloom_filter_length={:?}, has_bloom={}",
            rg_idx, bloom_offset, bloom_length, has_bloom
        );
    }
}
