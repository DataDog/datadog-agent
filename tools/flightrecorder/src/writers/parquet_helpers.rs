use std::sync::Arc;

use arrow::array::{DictionaryArray, StringArray, UInt32Array};
use arrow::datatypes::{DataType, UInt32Type};
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;

/// Convert StringInterner output to an Arrow DictionaryArray, applying sort order.
pub fn interner_to_dict_array(
    vals: Vec<String>,
    codes: Vec<u32>,
    order: &[usize],
) -> DictionaryArray<UInt32Type> {
    let sorted_codes: UInt32Array = order.iter().map(|&i| codes[i]).collect();
    let values = Arc::new(StringArray::from(vals));
    DictionaryArray::try_new(sorted_codes, values).expect("DictionaryArray construction failed")
}

/// Standard dictionary-encoded UTF8 data type.
pub fn dict_utf8_type() -> DataType {
    DataType::Dictionary(Box::new(DataType::UInt32), Box::new(DataType::Utf8))
}

/// Default Parquet writer properties: Snappy compression, dictionary enabled.
pub fn default_writer_props() -> WriterProperties {
    WriterProperties::builder()
        .set_compression(Compression::SNAPPY)
        .set_dictionary_enabled(true)
        .build()
}
