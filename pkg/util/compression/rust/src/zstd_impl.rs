//! Zstandard compression implementation using the zstd crate (C bindings).

use crate::compressor::{Compressor, DdCompressionAlgorithm, StreamCompressor};
use crate::error::{CompressionResult, DdCompressionError};
use std::io::Write;

/// Default zstd compression level (matches Go agent default).
pub const DEFAULT_ZSTD_LEVEL: i32 = 1;

/// Minimum valid zstd compression level.
pub const MIN_ZSTD_LEVEL: i32 = 1;

/// Maximum valid zstd compression level.
pub const MAX_ZSTD_LEVEL: i32 = 22;

/// Zstd compressor using C bindings for maximum performance.
pub struct ZstdCompressor {
    level: i32,
}

impl ZstdCompressor {
    /// Creates a new zstd compressor with the specified compression level.
    /// Level is clamped to the valid range [1, 22].
    pub fn new(level: i32) -> Self {
        let clamped_level = level.clamp(MIN_ZSTD_LEVEL, MAX_ZSTD_LEVEL);
        Self {
            level: clamped_level,
        }
    }
}

impl Default for ZstdCompressor {
    /// Creates a new zstd compressor with the default level (1).
    fn default() -> Self {
        Self::new(DEFAULT_ZSTD_LEVEL)
    }
}

impl Compressor for ZstdCompressor {
    #[inline(always)]
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Zstd
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

        // Use bulk compression with pre-allocated output buffer for better performance.
        // This is more efficient than zstd::encode_all which creates intermediate buffers.
        let bound = zstd::zstd_safe::compress_bound(src.len());
        let mut output = vec![0u8; bound];

        let compressed_size = zstd::bulk::compress_to_buffer(src, &mut output, self.level)
            .map_err(|_| DdCompressionError::CompressionFailed)?;

        output.truncate(compressed_size);
        Ok(output)
    }

    #[inline(always)]
    fn compress_into(&self, src: &[u8], dst: &mut [u8]) -> CompressionResult<usize> {
        if src.is_empty() {
            return Ok(0);
        }

        // Use zstd's bulk compression API that writes directly to a buffer
        zstd::bulk::compress_to_buffer(src, dst, self.level)
            .map_err(|_| DdCompressionError::BufferTooSmall)
    }

    #[inline(always)]
    fn decompress(&self, src: &[u8]) -> CompressionResult<Vec<u8>> {
        if src.is_empty() {
            return Ok(Vec::new());
        }

        zstd::decode_all(src).map_err(|_| DdCompressionError::DecompressionFailed)
    }

    #[inline(always)]
    fn compress_bound(&self, source_len: usize) -> usize {
        // Use zstd's compress_bound which gives the worst-case size
        zstd::zstd_safe::compress_bound(source_len)
    }

    #[inline]
    fn new_stream(&self) -> Box<dyn StreamCompressor> {
        Box::new(ZstdStreamCompressor::new(self.level))
    }
}

/// Streaming zstd compressor.
pub struct ZstdStreamCompressor {
    encoder: Option<zstd::stream::Encoder<'static, Vec<u8>>>,
    bytes_written: usize,
    finished: bool,
}

impl ZstdStreamCompressor {
    /// Creates a new streaming zstd compressor.
    pub fn new(level: i32) -> Self {
        let output = Vec::with_capacity(4096);
        let encoder = zstd::stream::Encoder::new(output, level).ok();

        Self {
            encoder,
            bytes_written: 0,
            finished: false,
        }
    }
}

impl StreamCompressor for ZstdStreamCompressor {
    #[inline(always)]
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Zstd
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
            let output = encoder
                .finish()
                .map_err(|_| DdCompressionError::CompressionFailed)?;
            Ok(output)
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
    fn test_zstd_compress_decompress() {
        let compressor = ZstdCompressor::new(3);
        let data = b"Hello, World! This is a test of zstd compression.";

        let compressed = compressor.compress(data).unwrap();
        assert!(!compressed.is_empty());
        // Small data may not compress well - just verify round-trip works

        let decompressed = compressor.decompress(&compressed).unwrap();
        assert_eq!(decompressed, data);
    }

    #[test]
    fn test_zstd_empty_input() {
        let compressor = ZstdCompressor::new(1);

        let compressed = compressor.compress(&[]).unwrap();
        assert!(compressed.is_empty());

        let decompressed = compressor.decompress(&[]).unwrap();
        assert!(decompressed.is_empty());
    }

    #[test]
    fn test_zstd_compress_bound() {
        let compressor = ZstdCompressor::new(1);

        let bound = compressor.compress_bound(1000);
        assert!(bound >= 1000); // Bound should be at least input size

        let bound_zero = compressor.compress_bound(0);
        assert!(bound_zero > 0); // Even empty input has some minimum bound
    }

    #[test]
    fn test_zstd_level_clamping() {
        let low = ZstdCompressor::new(-5);
        assert_eq!(low.level(), MIN_ZSTD_LEVEL);

        let high = ZstdCompressor::new(100);
        assert_eq!(high.level(), MAX_ZSTD_LEVEL);

        let normal = ZstdCompressor::new(5);
        assert_eq!(normal.level(), 5);
    }

    #[test]
    fn test_zstd_content_encoding() {
        let compressor = ZstdCompressor::new(1);
        assert_eq!(compressor.content_encoding(), "zstd");
    }

    #[test]
    fn test_zstd_stream_compressor() {
        let compressor = ZstdCompressor::new(3);
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
    fn test_zstd_large_data() {
        let compressor = ZstdCompressor::new(3);

        // Create 1MB of compressible data
        let data: Vec<u8> = (0..1_000_000).map(|i| (i % 256) as u8).collect();

        let compressed = compressor.compress(&data).unwrap();
        assert!(compressed.len() < data.len()); // Should compress well

        let decompressed = compressor.decompress(&compressed).unwrap();
        assert_eq!(decompressed, data);
    }
}
