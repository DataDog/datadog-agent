/// Shared logic for writing contexts to Parquet format.
///
/// Used by the `archive` subcommand. The contexts are read from the append-only
/// `contexts.bin` file (via `read_contexts_bin`) and written to a
/// `contexts.parquet` file with decomposed name, reserved tags, and an overflow
/// tags MAP column.
use std::fs::File;
use std::path::Path;
use std::sync::Arc;

use anyhow::Result;
use arrow::array::{ArrayRef, MapBuilder, StringArray, StringBuilder, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;

/// The 7 reserved metric tag keys, in column order.
pub const RESERVED_KEYS: &[&str] = &[
    "host", "device", "source", "service", "env", "version", "team",
];

/// Write a slice of `(context_key, name, tags_joined)` tuples to a Parquet file.
///
/// The output schema has one column per reserved tag key plus an overflow MAP
/// column for all remaining tags.
pub fn write_contexts_parquet(contexts: &[(u64, String, String)], output: &Path) -> Result<()> {
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
        Arc::new(StringArray::from(
            tag_host.iter().map(|s| s.as_str()).collect::<Vec<_>>(),
        )),
        Arc::new(StringArray::from(
            tag_device.iter().map(|s| s.as_str()).collect::<Vec<_>>(),
        )),
        Arc::new(StringArray::from(
            tag_source.iter().map(|s| s.as_str()).collect::<Vec<_>>(),
        )),
        Arc::new(StringArray::from(
            tag_service.iter().map(|s| s.as_str()).collect::<Vec<_>>(),
        )),
        Arc::new(StringArray::from(
            tag_env.iter().map(|s| s.as_str()).collect::<Vec<_>>(),
        )),
        Arc::new(StringArray::from(
            tag_version.iter().map(|s| s.as_str()).collect::<Vec<_>>(),
        )),
        Arc::new(StringArray::from(
            tag_team.iter().map(|s| s.as_str()).collect::<Vec<_>>(),
        )),
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
