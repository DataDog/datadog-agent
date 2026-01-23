//! Core compressor traits and types.

use crate::error::{CompressionResult, DdCompressionError};
use std::io::Write;

/// Compression algorithm identifiers.
#[repr(C)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DdCompressionAlgorithm {
    /// Zstandard compression.
    Zstd = 0,
    /// Gzip compression.
    Gzip = 1,
    /// Zlib/deflate compression.
    Zlib = 2,
    /// No compression (passthrough).
    Noop = 3,
}

impl DdCompressionAlgorithm {
    /// Returns the HTTP Content-Encoding header value for this algorithm.
    pub fn content_encoding(&self) -> &'static str {
        match self {
            DdCompressionAlgorithm::Zstd => "zstd",
            DdCompressionAlgorithm::Gzip => "gzip",
            DdCompressionAlgorithm::Zlib => "deflate",
            DdCompressionAlgorithm::Noop => "identity",
        }
    }

    /// Returns the algorithm name as used in configuration.
    pub fn name(&self) -> &'static str {
        match self {
            DdCompressionAlgorithm::Zstd => "zstd",
            DdCompressionAlgorithm::Gzip => "gzip",
            DdCompressionAlgorithm::Zlib => "zlib",
            DdCompressionAlgorithm::Noop => "none",
        }
    }
}

/// Trait for one-shot compression/decompression operations.
pub trait Compressor: Send + Sync {
    /// Returns the algorithm used by this compressor.
    fn algorithm(&self) -> DdCompressionAlgorithm;

    /// Returns the compression level (algorithm-specific interpretation).
    fn level(&self) -> i32;

    /// Compresses the input data and returns the compressed output.
    fn compress(&self, src: &[u8]) -> CompressionResult<Vec<u8>>;

    /// Decompresses the input data and returns the decompressed output.
    fn decompress(&self, src: &[u8]) -> CompressionResult<Vec<u8>>;

    /// Returns the worst-case compressed size for the given input length.
    /// This is used to pre-allocate output buffers.
    fn compress_bound(&self, source_len: usize) -> usize;

    /// Returns the HTTP Content-Encoding header value for this compressor.
    fn content_encoding(&self) -> &'static str {
        self.algorithm().content_encoding()
    }

    /// Creates a new stream compressor for incremental compression.
    fn new_stream(&self) -> Box<dyn StreamCompressor>;
}

/// Trait for streaming compression operations.
/// Implements a write-flush-close pattern similar to Go's io.WriteCloser.
pub trait StreamCompressor: Send {
    /// Returns the algorithm used by this stream compressor.
    fn algorithm(&self) -> DdCompressionAlgorithm;

    /// Writes data to the compression stream.
    /// Returns the number of bytes consumed from the input.
    fn write(&mut self, data: &[u8]) -> CompressionResult<usize>;

    /// Flushes any buffered data to the output without finalizing the stream.
    fn flush(&mut self) -> CompressionResult<()>;

    /// Finalizes the compression stream and returns the compressed data.
    /// After calling this method, the stream cannot be used again.
    fn finish(self: Box<Self>) -> CompressionResult<Vec<u8>>;

    /// Returns the current compressed output without finalizing.
    /// Useful for checking progress or implementing chunked output.
    fn get_output(&self) -> &[u8];

    /// Returns the total number of uncompressed bytes written so far.
    fn bytes_written(&self) -> usize;
}

/// A wrapper that provides streaming compression by buffering writes
/// and compressing on finish.
pub struct BufferedStreamCompressor<W: Write> {
    algorithm: DdCompressionAlgorithm,
    writer: W,
    bytes_written: usize,
}

impl<W: Write> BufferedStreamCompressor<W> {
    pub fn new(algorithm: DdCompressionAlgorithm, writer: W) -> Self {
        Self {
            algorithm,
            writer,
            bytes_written: 0,
        }
    }

    pub fn into_inner(self) -> W {
        self.writer
    }
}

impl<W: Write + Send> StreamCompressor for BufferedStreamCompressor<W>
where
    W: AsRef<[u8]>,
{
    fn algorithm(&self) -> DdCompressionAlgorithm {
        self.algorithm
    }

    fn write(&mut self, data: &[u8]) -> CompressionResult<usize> {
        self.writer
            .write_all(data)
            .map_err(|_| DdCompressionError::CompressionFailed)?;
        self.bytes_written += data.len();
        Ok(data.len())
    }

    fn flush(&mut self) -> CompressionResult<()> {
        self.writer
            .flush()
            .map_err(|_| DdCompressionError::CompressionFailed)
    }

    fn finish(self: Box<Self>) -> CompressionResult<Vec<u8>> {
        Ok(self.writer.as_ref().to_vec())
    }

    fn get_output(&self) -> &[u8] {
        self.writer.as_ref()
    }

    fn bytes_written(&self) -> usize {
        self.bytes_written
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_algorithm_content_encoding() {
        assert_eq!(DdCompressionAlgorithm::Zstd.content_encoding(), "zstd");
        assert_eq!(DdCompressionAlgorithm::Gzip.content_encoding(), "gzip");
        assert_eq!(DdCompressionAlgorithm::Zlib.content_encoding(), "deflate");
        assert_eq!(DdCompressionAlgorithm::Noop.content_encoding(), "identity");
    }

    #[test]
    fn test_algorithm_name() {
        assert_eq!(DdCompressionAlgorithm::Zstd.name(), "zstd");
        assert_eq!(DdCompressionAlgorithm::Gzip.name(), "gzip");
        assert_eq!(DdCompressionAlgorithm::Zlib.name(), "zlib");
        assert_eq!(DdCompressionAlgorithm::Noop.name(), "none");
    }
}
