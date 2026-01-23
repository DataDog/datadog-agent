//! Gzip compression implementation using flate2 (pure Rust).

use crate::compressor::{Compressor, DdCompressionAlgorithm, StreamCompressor};
use crate::error::{CompressionResult, DdCompressionError};
use flate2::read::GzDecoder;
use flate2::write::GzEncoder;
use flate2::Compression;
use std::io::{self, Read, Write};

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
pub struct GzipCompressor {
    level: i32,
}

impl GzipCompressor {
    /// Creates a new gzip compressor with the specified compression level.
    /// Level is clamped to the valid range [0, 9].
    pub fn new(level: i32) -> Self {
        let clamped_level = level.clamp(MIN_GZIP_LEVEL, MAX_GZIP_LEVEL);
        Self {
            level: clamped_level,
        }
    }

    fn compression(&self) -> Compression {
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
    #[inline(always)]
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Gzip
    }

    #[inline(always)]
    fn level(&self) -> i32 {
        self.level
    }

    #[inline(always)]
    fn compress(&self, src: &[u8]) -> CompressionResult<Vec<u8>> {
        if src.is_empty() {
            return Ok(Vec::new());
        }

        let mut encoder = GzEncoder::new(Vec::new(), self.compression());
        encoder
            .write_all(src)
            .map_err(|_| DdCompressionError::CompressionFailed)?;
        encoder
            .finish()
            .map_err(|_| DdCompressionError::CompressionFailed)
    }

    #[inline(always)]
    fn compress_into(&self, src: &[u8], dst: &mut [u8]) -> CompressionResult<usize> {
        if src.is_empty() {
            return Ok(0);
        }

        let writer = SliceWriter::new(dst);
        let mut encoder = GzEncoder::new(writer, self.compression());
        encoder
            .write_all(src)
            .map_err(|_| DdCompressionError::BufferTooSmall)?;
        let writer = encoder
            .finish()
            .map_err(|_| DdCompressionError::BufferTooSmall)?;
        Ok(writer.position())
    }

    #[inline(always)]
    fn decompress(&self, src: &[u8]) -> CompressionResult<Vec<u8>> {
        if src.is_empty() {
            return Ok(Vec::new());
        }

        let mut decoder = GzDecoder::new(src);
        let mut output = Vec::new();
        decoder
            .read_to_end(&mut output)
            .map_err(|_| DdCompressionError::DecompressionFailed)?;
        Ok(output)
    }

    #[inline(always)]
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

impl GzipStreamCompressor {
    /// Creates a new streaming gzip compressor.
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
    #[inline(always)]
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Gzip
    }

    #[inline(always)]
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

    #[inline(always)]
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
    fn finish(mut self: Box<Self>) -> CompressionResult<Vec<u8>> {
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

    #[inline(always)]
    fn get_output(&self) -> &[u8] {
        if let Some(ref encoder) = self.encoder {
            encoder.get_ref()
        } else {
            &[]
        }
    }

    #[inline(always)]
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
        let mut stream = compressor.new_stream();

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
