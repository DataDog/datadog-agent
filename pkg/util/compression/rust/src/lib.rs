//! Datadog Compression Library
//!
//! This library provides compression and decompression functionality for the Datadog Agent.
//! It supports multiple compression algorithms:
//!
//! - **zstd**: Zstandard compression (default, best performance)
//! - **gzip**: Gzip compression (widely compatible)
//! - **zlib**: Zlib/deflate compression (HTTP deflate encoding)
//! - **noop**: No compression (passthrough)
//!
//! # FFI Interface
//!
//! The library exposes a C-compatible FFI interface for use from Go via CGO.
//! See the `ffi` module for the C API.
//!
//! # Example (Rust)
//!
//! ```rust
//! use datadog_compression::{ZstdCompressor, Compressor};
//!
//! let compressor = ZstdCompressor::new(3);
//! let data = b"Hello, World!";
//!
//! let compressed = compressor.compress(data).unwrap();
//! let decompressed = compressor.decompress(&compressed).unwrap();
//!
//! assert_eq!(decompressed, data);
//! ```

pub mod compressor;
pub mod error;
pub mod ffi;
pub mod gzip_impl;
pub mod noop_impl;
pub mod zlib_impl;
pub mod zstd_impl;

// Re-export main types for convenience
pub use compressor::{Compressor, DdCompressionAlgorithm, StreamCompressor, StreamVariant};
pub use error::{CompressionResult, DdCompressionError};
pub use gzip_impl::GzipCompressor;
pub use noop_impl::NoopCompressor;
pub use zlib_impl::ZlibCompressor;
pub use zstd_impl::ZstdCompressor;

/// Creates a new compressor for the specified algorithm.
///
/// # Arguments
/// * `algorithm` - The compression algorithm to use
/// * `level` - Compression level (algorithm-specific interpretation)
///
/// # Returns
/// A boxed compressor implementing the `Compressor` trait.
#[must_use]
pub fn new_compressor(algorithm: DdCompressionAlgorithm, level: i32) -> Box<dyn Compressor> {
    match algorithm {
        DdCompressionAlgorithm::Zstd => Box::new(ZstdCompressor::new(level)),
        DdCompressionAlgorithm::Gzip => Box::new(GzipCompressor::new(level)),
        DdCompressionAlgorithm::Zlib => Box::new(ZlibCompressor::new(level)),
        DdCompressionAlgorithm::Noop => Box::new(NoopCompressor::new()),
    }
}

/// Creates a new compressor from a string algorithm name.
///
/// # Arguments
/// * `name` - Algorithm name: "zstd", "gzip", "zlib", or "none"
/// * `level` - Compression level
///
/// # Returns
/// A boxed compressor, or `None` if the algorithm name is unknown.
#[must_use]
pub fn new_compressor_by_name(name: &str, level: i32) -> Option<Box<dyn Compressor>> {
    let algorithm = match name {
        "zstd" => DdCompressionAlgorithm::Zstd,
        "gzip" => DdCompressionAlgorithm::Gzip,
        "zlib" | "deflate" => DdCompressionAlgorithm::Zlib,
        "none" | "identity" => DdCompressionAlgorithm::Noop,
        _ => return None,
    };
    Some(new_compressor(algorithm, level))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_new_compressor() {
        let compressor = new_compressor(DdCompressionAlgorithm::Zstd, 3);
        assert_eq!(compressor.algorithm(), DdCompressionAlgorithm::Zstd);
        assert_eq!(compressor.content_encoding(), "zstd");
    }

    #[test]
    fn test_new_compressor_by_name() {
        let zstd = new_compressor_by_name("zstd", 3).unwrap();
        assert_eq!(zstd.algorithm(), DdCompressionAlgorithm::Zstd);

        let gzip = new_compressor_by_name("gzip", 6).unwrap();
        assert_eq!(gzip.algorithm(), DdCompressionAlgorithm::Gzip);

        let zlib = new_compressor_by_name("zlib", 6).unwrap();
        assert_eq!(zlib.algorithm(), DdCompressionAlgorithm::Zlib);

        let deflate = new_compressor_by_name("deflate", 6).unwrap();
        assert_eq!(deflate.algorithm(), DdCompressionAlgorithm::Zlib);

        let noop = new_compressor_by_name("none", 0).unwrap();
        assert_eq!(noop.algorithm(), DdCompressionAlgorithm::Noop);

        let identity = new_compressor_by_name("identity", 0).unwrap();
        assert_eq!(identity.algorithm(), DdCompressionAlgorithm::Noop);

        assert!(new_compressor_by_name("unknown", 0).is_none());
    }

    #[test]
    fn test_round_trip_all_algorithms() {
        let data = b"The quick brown fox jumps over the lazy dog. ".repeat(100);

        for algorithm in [
            DdCompressionAlgorithm::Zstd,
            DdCompressionAlgorithm::Gzip,
            DdCompressionAlgorithm::Zlib,
            DdCompressionAlgorithm::Noop,
        ] {
            let compressor = new_compressor(algorithm, 3);

            let compressed = compressor.compress(&data).expect("compress failed");
            let decompressed = compressor
                .decompress(&compressed)
                .expect("decompress failed");

            assert_eq!(decompressed, data, "round-trip failed for {:?}", algorithm);
        }
    }

    #[test]
    fn test_stream_round_trip_all_algorithms() {
        use crate::compressor::StreamVariant;
        use crate::gzip_impl::GzipStreamCompressor;
        use crate::noop_impl::NoopStreamCompressor;
        use crate::zlib_impl::ZlibStreamCompressor;
        use crate::zstd_impl::ZstdStreamCompressor;

        let chunks = [
            b"First chunk of data. ".to_vec(),
            b"Second chunk with more content. ".to_vec(),
            b"Third and final chunk.".to_vec(),
        ];
        let expected: Vec<u8> = chunks.iter().flat_map(|c| c.iter().copied()).collect();

        let test_cases: Vec<(DdCompressionAlgorithm, StreamVariant)> = vec![
            (
                DdCompressionAlgorithm::Zstd,
                StreamVariant::Zstd(ZstdStreamCompressor::new(3)),
            ),
            (
                DdCompressionAlgorithm::Gzip,
                StreamVariant::Gzip(GzipStreamCompressor::new(3)),
            ),
            (
                DdCompressionAlgorithm::Zlib,
                StreamVariant::Zlib(ZlibStreamCompressor::new(3)),
            ),
            (
                DdCompressionAlgorithm::Noop,
                StreamVariant::Noop(NoopStreamCompressor::new()),
            ),
        ];

        for (algorithm, mut stream) in test_cases {
            let compressor = new_compressor(algorithm, 3);

            for chunk in &chunks {
                stream.write(chunk).expect("write failed");
            }

            let compressed = stream.finish().expect("finish failed");
            let decompressed = compressor
                .decompress(&compressed)
                .expect("decompress failed");

            assert_eq!(
                decompressed, expected,
                "stream round-trip failed for {:?}",
                algorithm
            );
        }
    }
}
