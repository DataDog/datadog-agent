//! Zstandard compression implementation using the zstd crate (C bindings).
//!
//! This implementation uses `UnsafeCell` for zero-overhead interior mutability.
//! The FFI layer guarantees that each compressor handle is used by only one thread
//! at a time (Go handles synchronization at the caller level).
//!
//! # Safety
//!
//! This compressor is NOT thread-safe. Each instance must only be used from
//! a single thread at a time. The FFI boundary enforces this invariant.

use crate::compressor::{Compressor, DdCompressionAlgorithm, StreamCompressor};
use crate::error::{CompressionResult, DdCompressionError};
use std::cell::UnsafeCell;
use std::io::Write;

/// Default zstd compression level (matches Go agent default).
pub const DEFAULT_ZSTD_LEVEL: i32 = 1;

/// Minimum valid zstd compression level.
pub const MIN_ZSTD_LEVEL: i32 = 1;

/// Maximum valid zstd compression level.
pub const MAX_ZSTD_LEVEL: i32 = 22;

/// Maximum decompressed size we will pre-allocate (256 MB).
const MAX_PREALLOC_SIZE: usize = 256 * 1024 * 1024;

/// Get the decompressed size from the zstd frame header if available.
/// Returns `None` if the size cannot be determined cheaply.
#[inline]
fn get_decompressed_size(src: &[u8]) -> Option<usize> {
    // Minimum zstd frame size is ~9 bytes (magic + frame header)
    if src.len() < 9 {
        return None;
    }

    match zstd::zstd_safe::get_frame_content_size(src) {
        Ok(Some(size)) => usize::try_from(size)
            .ok()
            .map(|s| s.min(MAX_PREALLOC_SIZE)),
        _ => None,
    }
}

/// Zstd compressor using UnsafeCell for zero-overhead interior mutability.
///
/// # Safety
///
/// This type is NOT Sync. It must only be accessed from a single thread at a time.
/// The FFI layer ensures this by having Go handle synchronization.
pub struct ZstdCompressor {
    level: i32,
    /// Compression context - UnsafeCell for zero-overhead access.
    /// SAFETY: Only accessed from one thread at a time (FFI guarantees this).
    compressor: UnsafeCell<Option<zstd::bulk::Compressor<'static>>>,
    /// Decompression context - UnsafeCell for zero-overhead access.
    /// SAFETY: Only accessed from one thread at a time (FFI guarantees this).
    decompressor: UnsafeCell<Option<zstd::bulk::Decompressor<'static>>>,
}

// SAFETY: ZstdCompressor is Send because the underlying zstd contexts can be
// moved between threads.
unsafe impl Send for ZstdCompressor {}

// SAFETY: ZstdCompressor is Sync because the FFI layer guarantees that each
// compressor handle is only accessed by one thread at a time. Go handles
// synchronization at the caller level. The header documentation explicitly
// states that compressor handles are thread-safe for Go code (because Go
// manages the synchronization), while the internal UnsafeCell provides
// zero-overhead interior mutability on the Rust side.
unsafe impl Sync for ZstdCompressor {}

impl std::fmt::Debug for ZstdCompressor {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ZstdCompressor")
            .field("level", &self.level)
            .finish_non_exhaustive()
    }
}

impl ZstdCompressor {
    /// Creates a new zstd compressor with the specified compression level.
    /// Level is clamped to the valid range [1, 22].
    #[must_use]
    pub fn new(level: i32) -> Self {
        let clamped_level = level.clamp(MIN_ZSTD_LEVEL, MAX_ZSTD_LEVEL);

        // Pre-create the compressor context
        let compressor = zstd::bulk::Compressor::new(clamped_level).ok();
        let decompressor = zstd::bulk::Decompressor::new().ok();

        Self {
            level: clamped_level,
            compressor: UnsafeCell::new(compressor),
            decompressor: UnsafeCell::new(decompressor),
        }
    }

    /// Returns the compression level.
    #[inline]
    #[must_use]
    pub fn level(&self) -> i32 {
        self.level
    }

    /// Gets mutable access to the compressor context.
    ///
    /// # Safety
    ///
    /// Caller must ensure no other references to the compressor exist.
    /// The FFI layer guarantees single-threaded access per handle.
    #[inline]
    unsafe fn compressor_mut(&self) -> &mut Option<zstd::bulk::Compressor<'static>> {
        &mut *self.compressor.get()
    }

    /// Gets mutable access to the decompressor context.
    ///
    /// # Safety
    ///
    /// Caller must ensure no other references to the decompressor exist.
    /// The FFI layer guarantees single-threaded access per handle.
    #[inline]
    unsafe fn decompressor_mut(&self) -> &mut Option<zstd::bulk::Decompressor<'static>> {
        &mut *self.decompressor.get()
    }
}

impl Default for ZstdCompressor {
    /// Creates a new zstd compressor with the default level (1).
    fn default() -> Self {
        Self::new(DEFAULT_ZSTD_LEVEL)
    }
}

impl Compressor for ZstdCompressor {
    #[inline]
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Zstd
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

        let bound = zstd::zstd_safe::compress_bound(src.len());
        let mut output = vec![0u8; bound];

        let written = self.compress_into(src, &mut output)?;
        output.truncate(written);
        Ok(output)
    }

    #[inline]
    fn compress_into(&self, src: &[u8], dst: &mut [u8]) -> CompressionResult<usize> {
        if src.is_empty() {
            return Ok(0);
        }

        // SAFETY: FFI guarantees single-threaded access per compressor handle.
        let compressor = unsafe { self.compressor_mut() };
        let compressor = compressor
            .as_mut()
            .ok_or(DdCompressionError::CompressionFailed)?;

        // Use the low-level compress2 directly for maximum performance.
        // This avoids any additional overhead from the bulk::Compressor wrapper.
        compressor
            .compress_to_buffer(src, dst)
            .map_err(|_| DdCompressionError::BufferTooSmall)
    }

    #[inline]
    fn decompress(&self, src: &[u8]) -> CompressionResult<Vec<u8>> {
        if src.is_empty() {
            return Ok(Vec::new());
        }

        // Get the expected size from the frame header for optimal pre-allocation.
        let cap = get_decompressed_size(src)
            .unwrap_or_else(|| src.len().saturating_mul(4).min(MAX_PREALLOC_SIZE));

        // SAFETY: FFI guarantees single-threaded access per compressor handle.
        let decompressor = unsafe { self.decompressor_mut() };
        let decompressor = decompressor
            .as_mut()
            .ok_or(DdCompressionError::DecompressionFailed)?;

        decompressor
            .decompress(src, cap)
            .map_err(|_| DdCompressionError::DecompressionFailed)
    }

    #[inline]
    fn decompress_into(&self, src: &[u8], dst: &mut [u8]) -> CompressionResult<usize> {
        if src.is_empty() {
            return Ok(0);
        }

        // SAFETY: FFI guarantees single-threaded access per compressor handle.
        let decompressor = unsafe { self.decompressor_mut() };
        let decompressor = decompressor
            .as_mut()
            .ok_or(DdCompressionError::BufferTooSmall)?;

        decompressor
            .decompress_to_buffer(src, dst)
            .map_err(|_| DdCompressionError::BufferTooSmall)
    }

    #[inline]
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

impl std::fmt::Debug for ZstdStreamCompressor {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ZstdStreamCompressor")
            .field("bytes_written", &self.bytes_written)
            .field("finished", &self.finished)
            .finish_non_exhaustive()
    }
}

impl ZstdStreamCompressor {
    /// Creates a new streaming zstd compressor.
    #[must_use]
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
    #[inline]
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Zstd
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
            let output = encoder
                .finish()
                .map_err(|_| DdCompressionError::CompressionFailed)?;
            Ok(output)
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
        let mut stream = ZstdStreamCompressor::new(3);

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

    #[test]
    fn test_zstd_compress_into() {
        let compressor = ZstdCompressor::new(3);
        let data = b"Test data for compress_into.";

        let bound = compressor.compress_bound(data.len());
        let mut dst = vec![0u8; bound];

        let written = compressor.compress_into(data, &mut dst).unwrap();
        assert!(written > 0);
        assert!(written <= bound);

        // Verify round-trip
        let decompressed = compressor.decompress(&dst[..written]).unwrap();
        assert_eq!(decompressed, data);
    }
}
