//! Error types for the compression library.

use std::fmt;

/// Error codes returned by compression operations.
#[repr(C)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DdCompressionError {
    /// Operation completed successfully.
    Ok = 0,
    /// Invalid input data provided.
    InvalidInput = 1,
    /// Invalid compressor handle.
    InvalidHandle = 2,
    /// Memory allocation failed.
    AllocationFailed = 3,
    /// Compression operation failed.
    CompressionFailed = 4,
    /// Decompression operation failed.
    DecompressionFailed = 5,
    /// Output buffer is too small.
    BufferTooSmall = 6,
    /// Stream has already been closed.
    StreamClosed = 7,
    /// Requested algorithm is not supported.
    NotSupported = 8,
    /// Internal error occurred.
    InternalError = 9,
}

impl DdCompressionError {
    /// Returns a human-readable description of the error.
    pub fn as_str(&self) -> &'static str {
        match self {
            DdCompressionError::Ok => "success",
            DdCompressionError::InvalidInput => "invalid input data",
            DdCompressionError::InvalidHandle => "invalid handle",
            DdCompressionError::AllocationFailed => "memory allocation failed",
            DdCompressionError::CompressionFailed => "compression failed",
            DdCompressionError::DecompressionFailed => "decompression failed",
            DdCompressionError::BufferTooSmall => "output buffer too small",
            DdCompressionError::StreamClosed => "stream already closed",
            DdCompressionError::NotSupported => "algorithm not supported",
            DdCompressionError::InternalError => "internal error",
        }
    }

    /// Returns true if this represents a successful operation.
    pub fn is_ok(&self) -> bool {
        *self == DdCompressionError::Ok
    }
}

impl fmt::Display for DdCompressionError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.as_str())
    }
}

impl std::error::Error for DdCompressionError {}

/// Internal result type for compression operations.
pub type CompressionResult<T> = Result<T, DdCompressionError>;

impl From<std::io::Error> for DdCompressionError {
    fn from(_: std::io::Error) -> Self {
        DdCompressionError::InternalError
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_error_as_str() {
        assert_eq!(DdCompressionError::Ok.as_str(), "success");
        assert_eq!(
            DdCompressionError::CompressionFailed.as_str(),
            "compression failed"
        );
    }

    #[test]
    fn test_error_is_ok() {
        assert!(DdCompressionError::Ok.is_ok());
        assert!(!DdCompressionError::InvalidInput.is_ok());
    }
}
