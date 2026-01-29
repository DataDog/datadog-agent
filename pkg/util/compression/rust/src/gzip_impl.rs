//! Gzip compression implementation using flate2 with zlib-ng backend.
//!
//! This implementation uses smart pre-allocation based on:
//! - Input size for compression (compressed output is typically smaller)
//! - ISIZE field from gzip trailer for decompression (original size mod 2^32)

use crate::compressor::{Compressor, DdCompressionAlgorithm, StreamCompressor};
use crate::error::{CompressionResult, DdCompressionError};
use flate2::read::GzDecoder;
use flate2::write::GzEncoder;
use flate2::Compression;
use std::io::{self, Read, Write};

/// Maximum decompressed size we will pre-allocate (256 MB).
const MAX_PREALLOC_SIZE: usize = 256 * 1024 * 1024;

/// Minimum gzip file size (10 byte header + 8 byte trailer).
const MIN_GZIP_SIZE: usize = 18;

/// Get the expected decompressed size from the gzip trailer.
///
/// The gzip format stores ISIZE (original size mod 2^32) in the last 4 bytes.
/// This allows us to pre-allocate the correct buffer size and avoid reallocations.
///
/// Returns a reasonable default capacity if the size cannot be determined.
#[inline]
fn get_gzip_decompressed_size(src: &[u8]) -> usize {
    if src.len() < MIN_GZIP_SIZE {
        // Too small to be valid gzip, use default
        return src.len().saturating_mul(4).min(MAX_PREALLOC_SIZE);
    }

    // ISIZE is stored as little-endian u32 in the last 4 bytes
    let isize_bytes = &src[src.len() - 4..];
    let isize = u32::from_le_bytes([isize_bytes[0], isize_bytes[1], isize_bytes[2], isize_bytes[3]]);

    // Note: ISIZE is mod 2^32, so for files > 4GB this wraps.
    // We cap at MAX_PREALLOC_SIZE to handle this safely.
    (isize as usize).min(MAX_PREALLOC_SIZE)
}

/// A writer that writes to a fixed-size slice, tracking the position.
/// Returns an error if the buffer is too small.
struct SliceWriter<'a> {
    buf: &'a mut [u8],
    pos: usize,
}

impl<'a> SliceWriter<'a> {
    fn new(buf: &'a mut [u8]) -> Self {
        Self { buf, pos: 0 }
    }

    fn position(&self) -> usize {
        self.pos
    }
}

impl<'a> Write for SliceWriter<'a> {
    fn write(&mut self, data: &[u8]) -> io::Result<usize> {
        let available = self.buf.len() - self.pos;
        if data.len() > available {
            return Err(io::Error::new(
                io::ErrorKind::WriteZero,
                "buffer too small",
            ));
        }
        self.buf[self.pos..self.pos + data.len()].copy_from_slice(data);
        self.pos += data.len();
        Ok(data.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

/// Default gzip compression level (matches Go agent default).
pub const DEFAULT_GZIP_LEVEL: i32 = 6;

/// Minimum valid gzip compression level.
pub const MIN_GZIP_LEVEL: i32 = 0;

/// Maximum valid gzip compression level.
pub const MAX_GZIP_LEVEL: i32 = 9;

/// Gzip compressor using flate2.
#[derive(Debug, Clone, Copy)]
pub struct GzipCompressor {
    level: i32,
}

impl GzipCompressor {
    /// Creates a new gzip compressor with the specified compression level.
    /// Level is clamped to the valid range [0, 9].
    #[must_use]
    pub fn new(level: i32) -> Self {
        let clamped_level = level.clamp(MIN_GZIP_LEVEL, MAX_GZIP_LEVEL);
        Self {
            level: clamped_level,
        }
    }

    #[allow(clippy::cast_sign_loss)] // level is already clamped to [0, 9]
    fn compression(self) -> Compression {
        Compression::new(self.level as u32)
    }
}

impl Default for GzipCompressor {
    /// Creates a new gzip compressor with the default level (6).
    fn default() -> Self {
        Self::new(DEFAULT_GZIP_LEVEL)
    }
}

impl Compressor for GzipCompressor {
    #[inline]
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Gzip
    }

    #[inline]
    fn level(&self) -> i32 {
        self.level
    }

    #[inline]
    fn compress(&self, src: &[u8]) -> CompressionResult<Vec<u8>> {
        if src.is_empty() {
            return Ok(Vec::new());
        }

        // Pre-allocate with estimated compressed size.
        // Typical compression ratio is ~3:1 for text, but we're conservative.
        // Add gzip header/trailer overhead (18 bytes).
        let estimated_size = (src.len() / 2) + 18;
        let mut encoder = GzEncoder::new(Vec::with_capacity(estimated_size), (*self).compression());
        encoder
            .write_all(src)
            .map_err(|_| DdCompressionError::CompressionFailed)?;
        encoder
            .finish()
            .map_err(|_| DdCompressionError::CompressionFailed)
    }

    #[inline]
    fn compress_into(&self, src: &[u8], dst: &mut [u8]) -> CompressionResult<usize> {
        if src.is_empty() {
            return Ok(0);
        }

        let writer = SliceWriter::new(dst);
        let mut encoder = GzEncoder::new(writer, (*self).compression());
        encoder
            .write_all(src)
            .map_err(|_| DdCompressionError::BufferTooSmall)?;
        let writer = encoder
            .finish()
            .map_err(|_| DdCompressionError::BufferTooSmall)?;
        Ok(writer.position())
    }

    #[inline]
    fn decompress(&self, src: &[u8]) -> CompressionResult<Vec<u8>> {
        if src.is_empty() {
            return Ok(Vec::new());
        }

        // Pre-allocate based on ISIZE from gzip trailer.
        // This avoids reallocations during decompression.
        let expected_size = get_gzip_decompressed_size(src);
        let mut decoder = GzDecoder::new(src);
        let mut output = Vec::with_capacity(expected_size);
        decoder
            .read_to_end(&mut output)
            .map_err(|_| DdCompressionError::DecompressionFailed)?;
        Ok(output)
    }

    #[inline]
    fn decompress_into(&self, src: &[u8], dst: &mut [u8]) -> CompressionResult<usize> {
        if src.is_empty() {
            return Ok(0);
        }

        let mut decoder = GzDecoder::new(src);
        let mut pos = 0;
        loop {
            if pos >= dst.len() {
                // Check if there's more data
                let mut check = [0u8; 1];
                if decoder.read(&mut check).unwrap_or(0) > 0 {
                    return Err(DdCompressionError::BufferTooSmall);
                }
                break;
            }
            match decoder.read(&mut dst[pos..]) {
                Ok(0) => break,
                Ok(n) => pos += n,
                Err(_) => return Err(DdCompressionError::DecompressionFailed),
            }
        }
        Ok(pos)
    }

    #[inline]
    fn compress_bound(&self, source_len: usize) -> usize {
        // Gzip worst case: zlib deflateBound + 18 bytes for gzip header/trailer
        // The deflate compression can expand incompressible data by:
        // - 5 bytes for each 16KB block + 1 byte per 32KB block
        // - Plus gzip header (10 bytes) and trailer (8 bytes)
        // Using a conservative formula that matches real-world behavior:
        // For small inputs, the overhead can be significant, so we add extra safety margin.
        //
        // More conservative formula: input + 5 * (input / 16383 + 1) + 18
        // This ensures enough room for block headers even for small inputs.
        let num_blocks = (source_len / 16383) + 1;
        source_len + (5 * num_blocks) + 18
    }

    #[inline]
    fn new_stream(&self) -> Box<dyn StreamCompressor> {
        Box::new(GzipStreamCompressor::new(self.level))
    }
}

/// Streaming gzip compressor.
pub struct GzipStreamCompressor {
    encoder: Option<GzEncoder<Vec<u8>>>,
    bytes_written: usize,
    finished: bool,
}

impl std::fmt::Debug for GzipStreamCompressor {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("GzipStreamCompressor")
            .field("bytes_written", &self.bytes_written)
            .field("finished", &self.finished)
            .finish_non_exhaustive()
    }
}

impl GzipStreamCompressor {
    /// Creates a new streaming gzip compressor.
    #[must_use]
    #[allow(clippy::cast_sign_loss)] // clamped_level is guaranteed to be [0, 9]
    pub fn new(level: i32) -> Self {
        let clamped_level = level.clamp(MIN_GZIP_LEVEL, MAX_GZIP_LEVEL);
        let compression = Compression::new(clamped_level as u32);
        let encoder = GzEncoder::new(Vec::with_capacity(4096), compression);

        Self {
            encoder: Some(encoder),
            bytes_written: 0,
            finished: false,
        }
    }
}

impl StreamCompressor for GzipStreamCompressor {
    #[inline]
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Gzip
    }

    #[inline]
    fn write(&mut self, data: &[u8]) -> CompressionResult<usize> {
        if self.finished {
            return Err(DdCompressionError::StreamClosed);
        }

        if let Some(ref mut encoder) = self.encoder {
            encoder
                .write_all(data)
                .map_err(|_| DdCompressionError::CompressionFailed)?;
            self.bytes_written += data.len();
            Ok(data.len())
        } else {
            Err(DdCompressionError::InternalError)
        }
    }

    #[inline]
    fn flush(&mut self) -> CompressionResult<()> {
        if self.finished {
            return Err(DdCompressionError::StreamClosed);
        }

        if let Some(ref mut encoder) = self.encoder {
            encoder
                .flush()
                .map_err(|_| DdCompressionError::CompressionFailed)?;
            Ok(())
        } else {
            Err(DdCompressionError::InternalError)
        }
    }

    #[inline]
    fn finish(mut self) -> CompressionResult<Vec<u8>> {
        if self.finished {
            return Err(DdCompressionError::StreamClosed);
        }

        self.finished = true;

        if let Some(encoder) = self.encoder.take() {
            encoder
                .finish()
                .map_err(|_| DdCompressionError::CompressionFailed)
        } else {
            Err(DdCompressionError::InternalError)
        }
    }

    #[inline]
    fn get_output(&self) -> &[u8] {
        if let Some(ref encoder) = self.encoder {
            encoder.get_ref()
        } else {
            &[]
        }
    }

    #[inline]
    fn bytes_written(&self) -> usize {
        self.bytes_written
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_gzip_compress_decompress() {
        let compressor = GzipCompressor::new(6);
        let data = b"Hello, World! This is a test of gzip compression.";

        let compressed = compressor.compress(data).unwrap();
        assert!(!compressed.is_empty());

        let decompressed = compressor.decompress(&compressed).unwrap();
        assert_eq!(decompressed, data);
    }

    #[test]
    fn test_gzip_empty_input() {
        let compressor = GzipCompressor::new(6);

        let compressed = compressor.compress(&[]).unwrap();
        assert!(compressed.is_empty());

        let decompressed = compressor.decompress(&[]).unwrap();
        assert!(decompressed.is_empty());
    }

    #[test]
    fn test_gzip_compress_bound() {
        let compressor = GzipCompressor::new(6);

        let bound = compressor.compress_bound(1000);
        assert!(bound >= 1000);

        let bound_large = compressor.compress_bound(100_000);
        // 100000 + (100000/32768)*5 + 18 = 100000 + 15 + 18 = 100033
        assert!(bound_large >= 100_000);
    }

    #[test]
    fn test_gzip_level_clamping() {
        let low = GzipCompressor::new(-5);
        assert_eq!(low.level(), MIN_GZIP_LEVEL);

        let high = GzipCompressor::new(100);
        assert_eq!(high.level(), MAX_GZIP_LEVEL);

        let normal = GzipCompressor::new(5);
        assert_eq!(normal.level(), 5);
    }

    #[test]
    fn test_gzip_content_encoding() {
        let compressor = GzipCompressor::new(6);
        assert_eq!(compressor.content_encoding(), "gzip");
    }

    #[test]
    fn test_gzip_stream_compressor() {
        let compressor = GzipCompressor::new(6);
        let mut stream = GzipStreamCompressor::new(6);

        let data1 = b"First chunk of data. ";
        let data2 = b"Second chunk of data. ";
        let data3 = b"Third chunk of data.";

        stream.write(data1).unwrap();
        stream.write(data2).unwrap();
        stream.write(data3).unwrap();

        let compressed = stream.finish().unwrap();
        assert!(!compressed.is_empty());

        // Decompress and verify
        let decompressed = compressor.decompress(&compressed).unwrap();
        let expected: Vec<u8> = [data1.as_slice(), data2.as_slice(), data3.as_slice()].concat();
        assert_eq!(decompressed, expected);
    }

    #[test]
    fn test_gzip_large_data() {
        let compressor = GzipCompressor::new(6);

        // Create 1MB of compressible data
        let data: Vec<u8> = (0..1_000_000).map(|i| (i % 256) as u8).collect();

        let compressed = compressor.compress(&data).unwrap();
        assert!(compressed.len() < data.len());

        let decompressed = compressor.decompress(&compressed).unwrap();
        assert_eq!(decompressed, data);
    }
}
