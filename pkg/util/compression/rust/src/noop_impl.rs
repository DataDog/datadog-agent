//! No-op (passthrough) compression implementation.

use crate::compressor::{Compressor, DdCompressionAlgorithm, StreamCompressor};
use crate::error::{CompressionResult, DdCompressionError};

/// No-op compressor that passes data through unchanged.
pub struct NoopCompressor;

impl NoopCompressor {
    /// Creates a new no-op compressor.
    pub fn new() -> Self {
        Self
    }
}

impl Default for NoopCompressor {
    fn default() -> Self {
        Self::new()
    }
}

impl Compressor for NoopCompressor {
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Noop
    }

    fn level(&self) -> i32 {
        0
    }

    fn compress(&self, src: &[u8]) -> CompressionResult<Vec<u8>> {
        Ok(src.to_vec())
    }

    fn decompress(&self, src: &[u8]) -> CompressionResult<Vec<u8>> {
        Ok(src.to_vec())
    }

    fn compress_bound(&self, source_len: usize) -> usize {
        // No expansion for passthrough
        source_len
    }

    fn new_stream(&self) -> Box<dyn StreamCompressor> {
        Box::new(NoopStreamCompressor::new())
    }
}

/// Streaming no-op compressor.
pub struct NoopStreamCompressor {
    buffer: Vec<u8>,
    finished: bool,
}

impl NoopStreamCompressor {
    /// Creates a new streaming no-op compressor.
    pub fn new() -> Self {
        Self {
            buffer: Vec::with_capacity(4096),
            finished: false,
        }
    }
}

impl Default for NoopStreamCompressor {
    fn default() -> Self {
        Self::new()
    }
}

impl StreamCompressor for NoopStreamCompressor {
    fn algorithm(&self) -> DdCompressionAlgorithm {
        DdCompressionAlgorithm::Noop
    }

    fn write(&mut self, data: &[u8]) -> CompressionResult<usize> {
        if self.finished {
            return Err(DdCompressionError::StreamClosed);
        }

        self.buffer.extend_from_slice(data);
        Ok(data.len())
    }

    fn flush(&mut self) -> CompressionResult<()> {
        if self.finished {
            return Err(DdCompressionError::StreamClosed);
        }
        // No-op: nothing to flush
        Ok(())
    }

    fn finish(mut self: Box<Self>) -> CompressionResult<Vec<u8>> {
        if self.finished {
            return Err(DdCompressionError::StreamClosed);
        }

        self.finished = true;
        Ok(std::mem::take(&mut self.buffer))
    }

    fn get_output(&self) -> &[u8] {
        &self.buffer
    }

    fn bytes_written(&self) -> usize {
        self.buffer.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_noop_compress_decompress() {
        let compressor = NoopCompressor::new();
        let data = b"Hello, World! This is a test of no-op compression.";

        let compressed = compressor.compress(data).unwrap();
        assert_eq!(compressed, data); // Should be identical

        let decompressed = compressor.decompress(&compressed).unwrap();
        assert_eq!(decompressed, data);
    }

    #[test]
    fn test_noop_empty_input() {
        let compressor = NoopCompressor::new();

        let compressed = compressor.compress(&[]).unwrap();
        assert!(compressed.is_empty());

        let decompressed = compressor.decompress(&[]).unwrap();
        assert!(decompressed.is_empty());
    }

    #[test]
    fn test_noop_compress_bound() {
        let compressor = NoopCompressor::new();

        let bound = compressor.compress_bound(1000);
        assert_eq!(bound, 1000); // No expansion

        let bound_zero = compressor.compress_bound(0);
        assert_eq!(bound_zero, 0);
    }

    #[test]
    fn test_noop_content_encoding() {
        let compressor = NoopCompressor::new();
        assert_eq!(compressor.content_encoding(), "identity");
    }

    #[test]
    fn test_noop_stream_compressor() {
        let compressor = NoopCompressor::new();
        let mut stream = compressor.new_stream();

        let data1 = b"First chunk of data. ";
        let data2 = b"Second chunk of data. ";
        let data3 = b"Third chunk of data.";

        stream.write(data1).unwrap();
        stream.write(data2).unwrap();
        stream.write(data3).unwrap();

        let output = stream.finish().unwrap();

        // Should be unchanged concatenation
        let expected: Vec<u8> = [data1.as_slice(), data2.as_slice(), data3.as_slice()].concat();
        assert_eq!(output, expected);
    }

    #[test]
    fn test_noop_stream_get_output() {
        let compressor = NoopCompressor::new();
        let mut stream = compressor.new_stream();

        stream.write(b"Hello").unwrap();
        assert_eq!(stream.get_output(), b"Hello");

        stream.write(b" World").unwrap();
        assert_eq!(stream.get_output(), b"Hello World");
    }

    #[test]
    fn test_noop_stream_bytes_written() {
        let compressor = NoopCompressor::new();
        let mut stream = compressor.new_stream();

        assert_eq!(stream.bytes_written(), 0);

        stream.write(b"Hello").unwrap();
        assert_eq!(stream.bytes_written(), 5);

        stream.write(b" World").unwrap();
        assert_eq!(stream.bytes_written(), 11);
    }
}
