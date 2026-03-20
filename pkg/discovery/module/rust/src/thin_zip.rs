// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! A lightweight ZIP central directory parser that avoids the memory overhead of
//! the full `zip` crate's `ZipArchive`.
//!
//! `ZipArchive::new()` loads rich per-entry metadata into an `IndexMap`: three
//! copies of each filename, extra fields, comments, etc.  For a Spring Boot fat
//! JAR with 20 000 entries this easily reaches 10+ MiB of small heap
//! allocations that fragment glibc arenas and never get returned to the OS.
//!
//! This module instead parses only the fields we need (name, offset,
//! compression, sizes) and only stores entries that match a caller-provided
//! filter — typically fewer than a hundred out of tens of thousands.

use flate2::bufread::DeflateDecoder;
use std::io::{self, BufReader, Read, Seek, SeekFrom};

const MAX_ENTRY_SIZE: u64 = 1024 * 1024; // 1 MiB – same limit as fs.rs

// ZIP signatures
const EOCD_SIGNATURE: u32 = 0x06054b50;
const ZIP64_EOCD_LOCATOR_SIGNATURE: u32 = 0x07064b50;
const ZIP64_EOCD_SIGNATURE: u32 = 0x06064b50;
const CD_ENTRY_SIGNATURE: u32 = 0x02014b50;
const LOCAL_FILE_HEADER_SIGNATURE: u32 = 0x04034b50;

// Compression methods
const COMPRESSION_STORED: u16 = 0;
const COMPRESSION_DEFLATED: u16 = 8;

// ZIP64 extra field header id
const ZIP64_EXTRA_FIELD_ID: u16 = 0x0001;

/// Minimal metadata for a single ZIP entry.
pub struct ThinZipEntry {
    name: String,
    local_header_offset: u64,
    compressed_size: u64,
    uncompressed_size: u64,
    compression_method: u16,
}

impl ThinZipEntry {
    pub fn name(&self) -> &str {
        &self.name
    }
}

/// A memory-efficient ZIP reader that only stores entries matching a filter.
///
/// Unlike `zip::ZipArchive`, this avoids storing rich metadata for every entry
/// in the archive, dramatically reducing memory usage for large JARs.
pub struct FilteredZipReader<R> {
    reader: R,
    is_spring_boot: bool,
    entries: Vec<ThinZipEntry>,
}

impl<R: Read + Seek> FilteredZipReader<R> {
    /// Parse the central directory, keeping only entries where `filter(name)`
    /// returns `true`.  Also records whether any entry starts with `"BOOT-INF/"`
    /// (for Spring Boot detection).
    pub fn new(mut reader: R, filter: impl Fn(&str) -> bool) -> io::Result<Self> {
        let (cd_offset, cd_size) = find_central_directory(&mut reader)?;

        reader.seek(SeekFrom::Start(cd_offset))?;

        let mut is_spring_boot = false;
        let mut entries = Vec::new();
        let cd_end = cd_offset.saturating_add(cd_size);

        while reader.stream_position()? < cd_end {
            let entry = parse_cd_entry(&mut reader)?;

            if !is_spring_boot && entry.name.starts_with("BOOT-INF/") {
                is_spring_boot = true;
            }

            if filter(&entry.name) {
                entries.push(entry);
            }
        }

        Ok(Self {
            reader,
            is_spring_boot,
            entries,
        })
    }

    pub fn is_spring_boot(&self) -> bool {
        self.is_spring_boot
    }

    pub fn entries(&self) -> &[ThinZipEntry] {
        &self.entries
    }

    /// Read and decompress an entry by index, returning its contents.
    ///
    /// Rejects entries larger than 1 MiB (same limit as [`crate::fs`]).
    pub fn read_entry_by_index(&mut self, index: usize) -> io::Result<Vec<u8>> {
        let Some(entry) = self.entries.get(index) else {
            return Err(io::Error::new(
                io::ErrorKind::NotFound,
                "entry index out of bounds",
            ));
        };

        if entry.uncompressed_size > MAX_ENTRY_SIZE {
            return Err(io::Error::new(
                io::ErrorKind::InvalidInput,
                format!(
                    "ZIP entry too large ({} bytes, max {} bytes)",
                    entry.uncompressed_size, MAX_ENTRY_SIZE
                ),
            ));
        }

        let local_header_offset = entry.local_header_offset;
        let compressed_size = entry.compressed_size;
        let uncompressed_size = entry.uncompressed_size;
        let compression_method = entry.compression_method;

        self.reader
            .seek(SeekFrom::Start(local_header_offset))?;
        let data_start = skip_local_file_header(&mut self.reader)?;
        self.reader.seek(SeekFrom::Start(data_start))?;

        match compression_method {
            COMPRESSION_STORED => {
                let mut buf = vec![0u8; uncompressed_size as usize];
                self.reader.read_exact(&mut buf)?;
                Ok(buf)
            }
            COMPRESSION_DEFLATED => {
                let limited = (&mut self.reader).take(compressed_size);
                let mut decoder = DeflateDecoder::new(BufReader::new(limited));
                let mut buf = Vec::with_capacity(uncompressed_size as usize);
                decoder.take(uncompressed_size).read_to_end(&mut buf)?;
                Ok(buf)
            }
            _ => Err(io::Error::new(
                io::ErrorKind::InvalidData,
                format!("unsupported compression method: {}", compression_method),
            )),
        }
    }

    /// Find an entry by name and return its index.
    pub fn index_for_name(&self, name: &str) -> Option<usize> {
        self.entries.iter().position(|e| e.name == name)
    }
}

// ---------------------------------------------------------------------------
// ZIP parsing helpers
// ---------------------------------------------------------------------------

fn read_u16_le<R: Read>(r: &mut R) -> io::Result<u16> {
    let mut buf = [0u8; 2];
    r.read_exact(&mut buf)?;
    Ok(u16::from_le_bytes(buf))
}

fn read_u32_le<R: Read>(r: &mut R) -> io::Result<u32> {
    let mut buf = [0u8; 4];
    r.read_exact(&mut buf)?;
    Ok(u32::from_le_bytes(buf))
}

fn read_u64_le<R: Read>(r: &mut R) -> io::Result<u64> {
    let mut buf = [0u8; 8];
    r.read_exact(&mut buf)?;
    Ok(u64::from_le_bytes(buf))
}

/// Locate the central directory by reading the End of Central Directory record
/// (and the ZIP64 variant if needed).  Returns `(cd_offset, cd_size)`.
fn find_central_directory<R: Read + Seek>(reader: &mut R) -> io::Result<(u64, u64)> {
    let file_size = reader.seek(SeekFrom::End(0))?;
    let eocd_pos = find_eocd_signature(reader, file_size)?;

    // Read EOCD fixed fields (18 bytes after the 4-byte signature).
    reader.seek(SeekFrom::Start(eocd_pos + 4))?;
    let mut eocd = [0u8; 18];
    reader.read_exact(&mut eocd)?;

    let num_entries_total = u16::from_le_bytes([eocd[6], eocd[7]]);
    let cd_size_32 = u32::from_le_bytes([eocd[8], eocd[9], eocd[10], eocd[11]]);
    let cd_offset_32 = u32::from_le_bytes([eocd[12], eocd[13], eocd[14], eocd[15]]);

    let needs_zip64 = num_entries_total == 0xFFFF
        || cd_size_32 == 0xFFFFFFFF
        || cd_offset_32 == 0xFFFFFFFF;

    if needs_zip64 {
        if let Some((offset, size)) = try_zip64_eocd(reader, eocd_pos)? {
            return Ok((offset, size));
        }
    }

    Ok((cd_offset_32 as u64, cd_size_32 as u64))
}

/// Search backward from the end of the file for the EOCD signature.
fn find_eocd_signature<R: Read + Seek>(reader: &mut R, file_size: u64) -> io::Result<u64> {
    // EOCD is at most 22 + 65535 bytes from the end.  Start with a small
    // search window that covers the overwhelming majority of ZIP files (no
    // trailing comment).
    for &window in &[1024u64, 22 + 65535] {
        let search_start = file_size.saturating_sub(window);
        let buf_size = (file_size - search_start) as usize;
        if buf_size < 22 {
            continue;
        }

        reader.seek(SeekFrom::Start(search_start))?;
        let mut buf = vec![0u8; buf_size];
        reader.read_exact(&mut buf)?;

        // Scan backward for 0x06054b50.
        if let Some(pos) = buf
            .windows(4)
            .rposition(|w| w == [0x50, 0x4b, 0x05, 0x06])
        {
            return Ok(search_start + pos as u64);
        }
    }

    Err(io::Error::new(
        io::ErrorKind::InvalidData,
        "end of central directory record not found",
    ))
}

/// Try to read the ZIP64 End of Central Directory record.
fn try_zip64_eocd<R: Read + Seek>(
    reader: &mut R,
    eocd_pos: u64,
) -> io::Result<Option<(u64, u64)>> {
    // The ZIP64 EOCD locator sits immediately before the regular EOCD.
    if eocd_pos < 20 {
        return Ok(None);
    }

    reader.seek(SeekFrom::Start(eocd_pos - 20))?;
    let locator_sig = read_u32_le(reader)?;
    if locator_sig != ZIP64_EOCD_LOCATOR_SIGNATURE {
        return Ok(None);
    }

    // Skip disk number (4 bytes).
    reader.seek(SeekFrom::Current(4))?;
    let zip64_eocd_offset = read_u64_le(reader)?;

    // Read the ZIP64 EOCD record.
    reader.seek(SeekFrom::Start(zip64_eocd_offset))?;
    let sig = read_u32_le(reader)?;
    if sig != ZIP64_EOCD_SIGNATURE {
        return Ok(None);
    }

    // Skip: size of record (8), version made by (2), version needed (2),
    //        disk number (4), cd disk (4), entries this disk (8).
    reader.seek(SeekFrom::Current(28))?;
    let _num_entries = read_u64_le(reader)?;
    let cd_size = read_u64_le(reader)?;
    let cd_offset = read_u64_le(reader)?;

    Ok(Some((cd_offset, cd_size)))
}

/// Parse one central-directory file header and advance the reader past it.
fn parse_cd_entry<R: Read + Seek>(reader: &mut R) -> io::Result<ThinZipEntry> {
    let sig = read_u32_le(reader)?;
    if sig != CD_ENTRY_SIGNATURE {
        return Err(io::Error::new(
            io::ErrorKind::InvalidData,
            format!(
                "invalid central directory entry signature: 0x{:08x}",
                sig
            ),
        ));
    }

    // Fixed-size part after the signature: 42 bytes.
    //
    // Layout (offsets relative to this buffer):
    //   [0..2]   version made by
    //   [2..4]   version needed
    //   [4..6]   general purpose bit flag
    //   [6..8]   compression method
    //   [8..10]  last mod file time
    //   [10..12] last mod file date
    //   [12..16] crc-32
    //   [16..20] compressed size
    //   [20..24] uncompressed size
    //   [24..26] file name length
    //   [26..28] extra field length
    //   [28..30] file comment length
    //   [30..32] disk number start
    //   [32..34] internal file attributes
    //   [34..38] external file attributes
    //   [38..42] relative offset of local header
    let mut fixed = [0u8; 42];
    reader.read_exact(&mut fixed)?;

    let compression_method = u16::from_le_bytes([fixed[6], fixed[7]]);
    let compressed_size_32 = u32::from_le_bytes([fixed[16], fixed[17], fixed[18], fixed[19]]);
    let uncompressed_size_32 = u32::from_le_bytes([fixed[20], fixed[21], fixed[22], fixed[23]]);
    let file_name_length = u16::from_le_bytes([fixed[24], fixed[25]]) as usize;
    let extra_field_length = u16::from_le_bytes([fixed[26], fixed[27]]) as usize;
    let file_comment_length = u16::from_le_bytes([fixed[28], fixed[29]]) as usize;
    let local_header_offset_32 = u32::from_le_bytes([fixed[38], fixed[39], fixed[40], fixed[41]]);

    // Read file name.
    let mut name_buf = vec![0u8; file_name_length];
    reader.read_exact(&mut name_buf)?;
    let file_name = String::from_utf8(name_buf)
        .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;

    // Resolve sizes / offset — may come from a ZIP64 extra field.
    let mut compressed_size = compressed_size_32 as u64;
    let mut uncompressed_size = uncompressed_size_32 as u64;
    let mut local_header_offset = local_header_offset_32 as u64;

    let needs_zip64 = compressed_size_32 == 0xFFFFFFFF
        || uncompressed_size_32 == 0xFFFFFFFF
        || local_header_offset_32 == 0xFFFFFFFF;

    if needs_zip64 && extra_field_length > 0 {
        let mut extra_buf = vec![0u8; extra_field_length];
        reader.read_exact(&mut extra_buf)?;

        resolve_zip64_sizes(
            &extra_buf,
            uncompressed_size_32 == 0xFFFFFFFF,
            compressed_size_32 == 0xFFFFFFFF,
            local_header_offset_32 == 0xFFFFFFFF,
            &mut uncompressed_size,
            &mut compressed_size,
            &mut local_header_offset,
        );
    } else if extra_field_length > 0 {
        reader.seek(SeekFrom::Current(extra_field_length as i64))?;
    }

    // Skip file comment.
    if file_comment_length > 0 {
        reader.seek(SeekFrom::Current(file_comment_length as i64))?;
    }

    Ok(ThinZipEntry {
        name: file_name,
        local_header_offset,
        compressed_size,
        uncompressed_size,
        compression_method,
    })
}

/// Extract ZIP64 sizes from the extra field data.
fn resolve_zip64_sizes(
    extra: &[u8],
    need_uncompressed: bool,
    need_compressed: bool,
    need_offset: bool,
    uncompressed_size: &mut u64,
    compressed_size: &mut u64,
    local_header_offset: &mut u64,
) {
    let mut pos = 0;
    while pos + 4 <= extra.len() {
        let Some(header_bytes) = extra.get(pos..pos + 4) else {
            break;
        };
        let Ok(hdr): Result<[u8; 4], _> = header_bytes.try_into() else {
            break;
        };
        let header_id = u16::from_le_bytes([hdr[0], hdr[1]]);
        let data_size = u16::from_le_bytes([hdr[2], hdr[3]]) as usize;

        if header_id == ZIP64_EXTRA_FIELD_ID {
            let mut field_pos = pos + 4;

            if need_uncompressed {
                if let Some(bytes) = extra.get(field_pos..field_pos + 8) {
                    if let Ok(arr) = <[u8; 8]>::try_from(bytes) {
                        *uncompressed_size = u64::from_le_bytes(arr);
                    }
                }
                field_pos += 8;
            }

            if need_compressed {
                if let Some(bytes) = extra.get(field_pos..field_pos + 8) {
                    if let Ok(arr) = <[u8; 8]>::try_from(bytes) {
                        *compressed_size = u64::from_le_bytes(arr);
                    }
                }
                field_pos += 8;
            }

            if need_offset {
                if let Some(bytes) = extra.get(field_pos..field_pos + 8) {
                    if let Ok(arr) = <[u8; 8]>::try_from(bytes) {
                        *local_header_offset = u64::from_le_bytes(arr);
                    }
                }
            }

            return;
        }

        pos += 4 + data_size;
    }
}

/// Read past a local file header and return the file-data start position.
///
/// Layout:
///   [0..4]   signature (0x04034b50)
///   [4..30]  fixed fields (26 bytes)
///     [26..28] file name length
///     [28..30] extra field length
///   [30..]   file name, extra field
fn skip_local_file_header<R: Read + Seek>(reader: &mut R) -> io::Result<u64> {
    let sig = read_u32_le(reader)?;
    if sig != LOCAL_FILE_HEADER_SIGNATURE {
        return Err(io::Error::new(
            io::ErrorKind::InvalidData,
            format!(
                "invalid local file header signature: 0x{:08x}",
                sig
            ),
        ));
    }

    let mut fixed = [0u8; 26];
    reader.read_exact(&mut fixed)?;

    let file_name_length = u16::from_le_bytes([fixed[22], fixed[23]]) as i64;
    let extra_field_length = u16::from_le_bytes([fixed[24], fixed[25]]) as i64;

    reader.seek(SeekFrom::Current(file_name_length + extra_field_length))?;
    reader.stream_position()
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used, clippy::indexing_slicing)]
mod tests {
    use super::*;
    use std::io::Write;

    fn create_test_zip(files: &[(&str, &[u8])]) -> Vec<u8> {
        let mut buf = Vec::new();
        {
            let mut writer = zip::ZipWriter::new(std::io::Cursor::new(&mut buf));
            let options: zip::write::FileOptions<()> = zip::write::FileOptions::default()
                .compression_method(zip::CompressionMethod::Stored);
            for &(name, content) in files {
                writer.start_file(name, options).unwrap();
                writer.write_all(content).unwrap();
            }
            writer.finish().unwrap();
        }
        buf
    }

    fn create_deflated_test_zip(files: &[(&str, &[u8])]) -> Vec<u8> {
        let mut buf = Vec::new();
        {
            let mut writer = zip::ZipWriter::new(std::io::Cursor::new(&mut buf));
            let options: zip::write::FileOptions<()> = zip::write::FileOptions::default()
                .compression_method(zip::CompressionMethod::Deflated);
            for &(name, content) in files {
                writer.start_file(name, options).unwrap();
                writer.write_all(content).unwrap();
            }
            writer.finish().unwrap();
        }
        buf
    }

    #[test]
    fn test_basic_read() {
        let zip_data = create_test_zip(&[
            ("hello.txt", b"hello world"),
            ("sub/dir/file.txt", b"nested content"),
        ]);

        let cursor = std::io::Cursor::new(zip_data);
        let mut reader = FilteredZipReader::new(cursor, |_| true).unwrap();

        assert!(!reader.is_spring_boot());
        assert_eq!(reader.entries().len(), 2);
        assert_eq!(reader.entries()[0].name(), "hello.txt");
        assert_eq!(reader.entries()[1].name(), "sub/dir/file.txt");

        let data = reader.read_entry_by_index(0).unwrap();
        assert_eq!(&data, b"hello world");

        let data = reader.read_entry_by_index(1).unwrap();
        assert_eq!(&data, b"nested content");
    }

    #[test]
    fn test_deflated_read() {
        let content = b"this is some content that will be deflate-compressed";
        let zip_data = create_deflated_test_zip(&[("compressed.txt", content)]);

        let cursor = std::io::Cursor::new(zip_data);
        let mut reader = FilteredZipReader::new(cursor, |_| true).unwrap();

        let data = reader.read_entry_by_index(0).unwrap();
        assert_eq!(&data, content);
    }

    #[test]
    fn test_spring_boot_detection() {
        let zip_data = create_test_zip(&[
            ("META-INF/MANIFEST.MF", b"Manifest-Version: 1.0"),
            ("BOOT-INF/classes/application.properties", b"spring.application.name=test"),
            ("BOOT-INF/lib/dep.jar", b"fake jar"),
        ]);

        let cursor = std::io::Cursor::new(zip_data);
        let reader = FilteredZipReader::new(cursor, |name| {
            name.starts_with("BOOT-INF/classes/") || name == "META-INF/MANIFEST.MF"
        })
        .unwrap();

        assert!(reader.is_spring_boot());
        // Only BOOT-INF/classes/ and MANIFEST.MF are stored, not BOOT-INF/lib/
        assert_eq!(reader.entries().len(), 2);
        assert_eq!(
            reader.entries()[0].name(),
            "META-INF/MANIFEST.MF"
        );
        assert_eq!(
            reader.entries()[1].name(),
            "BOOT-INF/classes/application.properties"
        );
    }

    #[test]
    fn test_filter_reduces_stored_entries() {
        let zip_data = create_test_zip(&[
            ("a.txt", b"a"),
            ("b.txt", b"b"),
            ("c.txt", b"c"),
        ]);

        let cursor = std::io::Cursor::new(zip_data);
        let reader =
            FilteredZipReader::new(cursor, |name| name == "b.txt").unwrap();

        assert_eq!(reader.entries().len(), 1);
        assert_eq!(reader.entries()[0].name(), "b.txt");
    }

    #[test]
    fn test_entry_too_large() {
        let big_content = vec![0u8; (MAX_ENTRY_SIZE + 1) as usize];
        let zip_data = create_test_zip(&[("big.bin", &big_content)]);

        let cursor = std::io::Cursor::new(zip_data);
        let mut reader = FilteredZipReader::new(cursor, |_| true).unwrap();

        let result = reader.read_entry_by_index(0);
        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .to_string()
            .contains("too large"));
    }

    #[test]
    fn test_index_for_name() {
        let zip_data = create_test_zip(&[
            ("first.txt", b"1"),
            ("second.txt", b"2"),
        ]);

        let cursor = std::io::Cursor::new(zip_data);
        let reader = FilteredZipReader::new(cursor, |_| true).unwrap();

        assert_eq!(reader.index_for_name("first.txt"), Some(0));
        assert_eq!(reader.index_for_name("second.txt"), Some(1));
        assert_eq!(reader.index_for_name("missing.txt"), None);
    }
}
