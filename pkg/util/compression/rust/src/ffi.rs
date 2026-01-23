//! C FFI interface for the compression library.
//!
//! This module provides C-compatible functions that can be called from Go via CGO.
//! Memory management follows these rules:
//! - Buffers returned by compression functions must be freed with `dd_buffer_free()`
//! - Compressor handles must be freed with `dd_compressor_free()`
//! - Stream handles must be closed with `dd_stream_close()` which also frees them
//!
//! # Safety
//! All FFI functions in this module expect callers to provide valid pointers.
//! This is standard for C FFI - the caller is responsible for pointer validity.
#![allow(clippy::not_unsafe_ptr_arg_deref)]

use crate::compressor::{Compressor, DdCompressionAlgorithm, StreamCompressor};
use crate::error::DdCompressionError;
use crate::gzip_impl::GzipCompressor;
use crate::noop_impl::NoopCompressor;
use crate::zlib_impl::ZlibCompressor;
use crate::zstd_impl::ZstdCompressor;
use libc::{c_char, c_int, size_t};
use std::ptr;
use std::slice;

/// Buffer structure for returning data to C/Go.
#[repr(C)]
pub struct DdBuffer {
    /// Pointer to the data. NULL if empty or on error.
    pub data: *mut u8,
    /// Length of the data in bytes.
    pub len: size_t,
    /// Capacity of the allocated buffer.
    pub capacity: size_t,
}

impl DdBuffer {
    /// Creates a new buffer from a Vec.
    fn from_vec(v: Vec<u8>) -> Self {
        if v.is_empty() {
            return Self {
                data: ptr::null_mut(),
                len: 0,
                capacity: 0,
            };
        }

        let mut v = v.into_boxed_slice();
        let data = v.as_mut_ptr();
        let len = v.len();
        std::mem::forget(v);

        Self {
            data,
            len,
            capacity: len,
        }
    }

    /// Creates an empty/null buffer.
    fn null() -> Self {
        Self {
            data: ptr::null_mut(),
            len: 0,
            capacity: 0,
        }
    }
}

/// Opaque handle to a compressor.
pub struct DdCompressor {
    inner: Box<dyn Compressor>,
}

/// Opaque handle to a stream compressor.
pub struct DdStream {
    inner: Option<Box<dyn StreamCompressor>>,
}

// ============================================================================
// Compressor FFI Functions
// ============================================================================

/// Creates a new compressor for the specified algorithm.
///
/// # Arguments
/// * `algorithm` - The compression algorithm to use
/// * `level` - Compression level (algorithm-specific interpretation)
///
/// # Returns
/// A pointer to a new compressor handle, or NULL on error.
///
/// # Safety
/// The returned handle must be freed with `dd_compressor_free()`.
#[no_mangle]
pub extern "C" fn dd_compressor_new(
    algorithm: DdCompressionAlgorithm,
    level: c_int,
) -> *mut DdCompressor {
    let compressor: Box<dyn Compressor> = match algorithm {
        DdCompressionAlgorithm::Zstd => Box::new(ZstdCompressor::new(level)),
        DdCompressionAlgorithm::Gzip => Box::new(GzipCompressor::new(level)),
        DdCompressionAlgorithm::Zlib => Box::new(ZlibCompressor::new(level)),
        DdCompressionAlgorithm::Noop => Box::new(NoopCompressor::new()),
    };

    Box::into_raw(Box::new(DdCompressor { inner: compressor }))
}

/// Frees a compressor handle.
///
/// # Safety
/// - `compressor` must be a valid handle from `dd_compressor_new()` or NULL.
/// - The handle must not be used after this call.
#[no_mangle]
pub extern "C" fn dd_compressor_free(compressor: *mut DdCompressor) {
    if !compressor.is_null() {
        unsafe {
            let _ = Box::from_raw(compressor);
        }
    }
}

/// Compresses data using the compressor.
///
/// # Arguments
/// * `compressor` - Valid compressor handle
/// * `src` - Source data to compress
/// * `src_len` - Length of source data
/// * `out_buffer` - Pointer to receive the compressed buffer
///
/// # Returns
/// Error code indicating success or failure.
///
/// # Safety
/// - `compressor` must be a valid handle from `dd_compressor_new()`.
/// - `src` must be valid for `src_len` bytes (can be NULL if `src_len` is 0).
/// - `out_buffer` must be a valid pointer.
/// - The returned buffer must be freed with `dd_buffer_free()`.
#[no_mangle]
pub extern "C" fn dd_compressor_compress(
    compressor: *const DdCompressor,
    src: *const u8,
    src_len: size_t,
    out_buffer: *mut DdBuffer,
) -> DdCompressionError {
    if compressor.is_null() || out_buffer.is_null() {
        return DdCompressionError::InvalidHandle;
    }

    let compressor = unsafe { &*compressor };
    let src_slice = if src.is_null() || src_len == 0 {
        &[]
    } else {
        unsafe { slice::from_raw_parts(src, src_len) }
    };

    match compressor.inner.compress(src_slice) {
        Ok(data) => {
            unsafe {
                *out_buffer = DdBuffer::from_vec(data);
            }
            DdCompressionError::Ok
        }
        Err(e) => {
            unsafe {
                *out_buffer = DdBuffer::null();
            }
            e
        }
    }
}

/// Decompresses data using the compressor.
///
/// # Arguments
/// * `compressor` - Valid compressor handle
/// * `src` - Compressed data to decompress
/// * `src_len` - Length of compressed data
/// * `out_buffer` - Pointer to receive the decompressed buffer
///
/// # Returns
/// Error code indicating success or failure.
///
/// # Safety
/// - `compressor` must be a valid handle from `dd_compressor_new()`.
/// - `src` must be valid for `src_len` bytes (can be NULL if `src_len` is 0).
/// - `out_buffer` must be a valid pointer.
/// - The returned buffer must be freed with `dd_buffer_free()`.
#[no_mangle]
pub extern "C" fn dd_compressor_decompress(
    compressor: *const DdCompressor,
    src: *const u8,
    src_len: size_t,
    out_buffer: *mut DdBuffer,
) -> DdCompressionError {
    if compressor.is_null() || out_buffer.is_null() {
        return DdCompressionError::InvalidHandle;
    }

    let compressor = unsafe { &*compressor };
    let src_slice = if src.is_null() || src_len == 0 {
        &[]
    } else {
        unsafe { slice::from_raw_parts(src, src_len) }
    };

    match compressor.inner.decompress(src_slice) {
        Ok(data) => {
            unsafe {
                *out_buffer = DdBuffer::from_vec(data);
            }
            DdCompressionError::Ok
        }
        Err(e) => {
            unsafe {
                *out_buffer = DdBuffer::null();
            }
            e
        }
    }
}

/// Returns the worst-case compressed size for the given input length.
///
/// # Arguments
/// * `compressor` - Valid compressor handle
/// * `source_len` - Length of the source data
///
/// # Returns
/// The worst-case compressed size, or 0 if the compressor handle is invalid.
#[no_mangle]
pub extern "C" fn dd_compressor_compress_bound(
    compressor: *const DdCompressor,
    source_len: size_t,
) -> size_t {
    if compressor.is_null() {
        return 0;
    }

    let compressor = unsafe { &*compressor };
    compressor.inner.compress_bound(source_len)
}

/// Returns the content-encoding string for this compressor.
///
/// # Arguments
/// * `compressor` - Valid compressor handle
///
/// # Returns
/// A static null-terminated string, or NULL if the compressor handle is invalid.
///
/// # Safety
/// The returned string is valid for the lifetime of the program.
/// Do not free or modify it.
#[no_mangle]
pub extern "C" fn dd_compressor_content_encoding(compressor: *const DdCompressor) -> *const c_char {
    if compressor.is_null() {
        return ptr::null();
    }

    let compressor = unsafe { &*compressor };
    match compressor.inner.algorithm() {
        DdCompressionAlgorithm::Zstd => b"zstd\0".as_ptr() as *const c_char,
        DdCompressionAlgorithm::Gzip => b"gzip\0".as_ptr() as *const c_char,
        DdCompressionAlgorithm::Zlib => b"deflate\0".as_ptr() as *const c_char,
        DdCompressionAlgorithm::Noop => b"identity\0".as_ptr() as *const c_char,
    }
}

/// Returns the algorithm used by this compressor.
///
/// # Arguments
/// * `compressor` - Valid compressor handle
///
/// # Returns
/// The algorithm enum value, or Noop if the compressor handle is invalid.
#[no_mangle]
pub extern "C" fn dd_compressor_algorithm(
    compressor: *const DdCompressor,
) -> DdCompressionAlgorithm {
    if compressor.is_null() {
        return DdCompressionAlgorithm::Noop;
    }

    let compressor = unsafe { &*compressor };
    compressor.inner.algorithm()
}

// ============================================================================
// Stream Compressor FFI Functions
// ============================================================================

/// Creates a new stream compressor from a compressor handle.
///
/// # Arguments
/// * `compressor` - Valid compressor handle (used for algorithm selection)
///
/// # Returns
/// A pointer to a new stream compressor handle, or NULL on error.
///
/// # Safety
/// The returned handle must be closed with `dd_stream_close()`.
#[no_mangle]
pub extern "C" fn dd_stream_new(compressor: *const DdCompressor) -> *mut DdStream {
    if compressor.is_null() {
        return ptr::null_mut();
    }

    let compressor = unsafe { &*compressor };
    let stream = compressor.inner.new_stream();

    Box::into_raw(Box::new(DdStream {
        inner: Some(stream),
    }))
}

/// Writes data to the stream compressor.
///
/// # Arguments
/// * `stream` - Valid stream compressor handle
/// * `data` - Data to write
/// * `data_len` - Length of data
///
/// # Returns
/// Number of bytes written, or 0 on error.
///
/// # Safety
/// - `stream` must be a valid handle from `dd_stream_new()`.
/// - `data` must be valid for `data_len` bytes (can be NULL if `data_len` is 0).
#[no_mangle]
pub extern "C" fn dd_stream_write(
    stream: *mut DdStream,
    data: *const u8,
    data_len: size_t,
) -> size_t {
    if stream.is_null() {
        return 0;
    }

    let stream = unsafe { &mut *stream };
    let data_slice = if data.is_null() || data_len == 0 {
        &[]
    } else {
        unsafe { slice::from_raw_parts(data, data_len) }
    };

    if let Some(ref mut inner) = stream.inner {
        inner.write(data_slice).unwrap_or(0)
    } else {
        0
    }
}

/// Flushes buffered data in the stream compressor.
///
/// # Arguments
/// * `stream` - Valid stream compressor handle
///
/// # Returns
/// Error code indicating success or failure.
///
/// # Safety
/// `stream` must be a valid handle from `dd_stream_new()`.
#[no_mangle]
pub extern "C" fn dd_stream_flush(stream: *mut DdStream) -> DdCompressionError {
    if stream.is_null() {
        return DdCompressionError::InvalidHandle;
    }

    let stream = unsafe { &mut *stream };
    if let Some(ref mut inner) = stream.inner {
        match inner.flush() {
            Ok(()) => DdCompressionError::Ok,
            Err(e) => e,
        }
    } else {
        DdCompressionError::StreamClosed
    }
}

/// Closes the stream compressor and returns the final compressed data.
///
/// After calling this function, the stream handle is freed and must not be used.
///
/// # Arguments
/// * `stream` - Valid stream compressor handle
/// * `out_buffer` - Pointer to receive the compressed buffer
///
/// # Returns
/// Error code indicating success or failure.
///
/// # Safety
/// - `stream` must be a valid handle from `dd_stream_new()`.
/// - `out_buffer` must be a valid pointer.
/// - The returned buffer must be freed with `dd_buffer_free()`.
/// - The stream handle is freed by this call and must not be used afterward.
#[no_mangle]
pub extern "C" fn dd_stream_close(
    stream: *mut DdStream,
    out_buffer: *mut DdBuffer,
) -> DdCompressionError {
    if stream.is_null() || out_buffer.is_null() {
        return DdCompressionError::InvalidHandle;
    }

    // Take ownership of the stream
    let mut stream_box = unsafe { Box::from_raw(stream) };

    if let Some(inner) = stream_box.inner.take() {
        match inner.finish() {
            Ok(data) => {
                unsafe {
                    *out_buffer = DdBuffer::from_vec(data);
                }
                DdCompressionError::Ok
            }
            Err(e) => {
                unsafe {
                    *out_buffer = DdBuffer::null();
                }
                e
            }
        }
    } else {
        unsafe {
            *out_buffer = DdBuffer::null();
        }
        DdCompressionError::StreamClosed
    }
}

/// Returns the number of uncompressed bytes written to the stream so far.
///
/// # Arguments
/// * `stream` - Valid stream compressor handle
///
/// # Returns
/// Number of bytes written, or 0 if the stream handle is invalid.
#[no_mangle]
pub extern "C" fn dd_stream_bytes_written(stream: *const DdStream) -> size_t {
    if stream.is_null() {
        return 0;
    }

    let stream = unsafe { &*stream };
    if let Some(ref inner) = stream.inner {
        inner.bytes_written()
    } else {
        0
    }
}

/// Returns the current size of compressed output in the stream buffer.
/// This can be used to track compression progress without finalizing the stream.
///
/// # Arguments
/// * `stream` - Valid stream compressor handle
///
/// # Returns
/// Number of compressed bytes currently in the output buffer, or 0 if invalid.
#[no_mangle]
pub extern "C" fn dd_stream_output_len(stream: *const DdStream) -> size_t {
    if stream.is_null() {
        return 0;
    }

    let stream = unsafe { &*stream };
    if let Some(ref inner) = stream.inner {
        inner.get_output().len()
    } else {
        0
    }
}

/// Copies the current compressed output from the stream without finalizing.
/// This is useful for checking progress or implementing chunked output.
///
/// # Arguments
/// * `stream` - Valid stream compressor handle
/// * `out_buffer` - Pointer to receive a copy of the current compressed output
///
/// # Returns
/// Error code indicating success or failure.
///
/// # Safety
/// - `stream` must be a valid handle from `dd_stream_new()`.
/// - `out_buffer` must be a valid pointer.
/// - The returned buffer must be freed with `dd_buffer_free()`.
#[no_mangle]
pub extern "C" fn dd_stream_get_output(
    stream: *const DdStream,
    out_buffer: *mut DdBuffer,
) -> DdCompressionError {
    if stream.is_null() || out_buffer.is_null() {
        return DdCompressionError::InvalidHandle;
    }

    let stream = unsafe { &*stream };
    if let Some(ref inner) = stream.inner {
        let output = inner.get_output();
        unsafe {
            *out_buffer = DdBuffer::from_vec(output.to_vec());
        }
        DdCompressionError::Ok
    } else {
        unsafe {
            *out_buffer = DdBuffer::null();
        }
        DdCompressionError::StreamClosed
    }
}

// ============================================================================
// Buffer FFI Functions
// ============================================================================

/// Frees a buffer allocated by the compression library.
///
/// # Arguments
/// * `buffer` - Buffer to free (if data is NULL, this is a no-op)
///
/// # Safety
/// The buffer must have been returned by a compression function.
/// Do not double-free or use after free.
#[no_mangle]
pub extern "C" fn dd_buffer_free(buffer: DdBuffer) {
    if !buffer.data.is_null() {
        unsafe {
            let _ = Box::from_raw(slice::from_raw_parts_mut(buffer.data, buffer.capacity));
        }
    }
}

// ============================================================================
// Utility FFI Functions
// ============================================================================

/// Returns a human-readable string for an error code.
///
/// # Arguments
/// * `error` - Error code
///
/// # Returns
/// A static null-terminated string describing the error.
#[no_mangle]
pub extern "C" fn dd_compression_error_string(error: DdCompressionError) -> *const c_char {
    let msg: &[u8] = match error {
        DdCompressionError::Ok => b"success\0",
        DdCompressionError::InvalidInput => b"invalid input data\0",
        DdCompressionError::InvalidHandle => b"invalid handle\0",
        DdCompressionError::AllocationFailed => b"memory allocation failed\0",
        DdCompressionError::CompressionFailed => b"compression failed\0",
        DdCompressionError::DecompressionFailed => b"decompression failed\0",
        DdCompressionError::BufferTooSmall => b"output buffer too small\0",
        DdCompressionError::StreamClosed => b"stream already closed\0",
        DdCompressionError::NotSupported => b"algorithm not supported\0",
        DdCompressionError::InternalError => b"internal error\0",
    };
    msg.as_ptr() as *const c_char
}

/// Returns the library version string.
///
/// # Returns
/// A static null-terminated version string.
#[no_mangle]
pub extern "C" fn dd_compression_version() -> *const c_char {
    concat!(env!("CARGO_PKG_VERSION"), "\0").as_ptr() as *const c_char
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ffi_compressor_lifecycle() {
        let compressor = dd_compressor_new(DdCompressionAlgorithm::Zstd, 3);
        assert!(!compressor.is_null());
        dd_compressor_free(compressor);
    }

    #[test]
    fn test_ffi_compress_decompress() {
        let compressor = dd_compressor_new(DdCompressionAlgorithm::Zstd, 3);
        let data = b"Hello, World! This is a test of FFI compression.";

        let mut compressed = DdBuffer::null();
        let result = dd_compressor_compress(compressor, data.as_ptr(), data.len(), &mut compressed);
        assert_eq!(result, DdCompressionError::Ok);
        assert!(!compressed.data.is_null());

        let mut decompressed = DdBuffer::null();
        let result = dd_compressor_decompress(
            compressor,
            compressed.data,
            compressed.len,
            &mut decompressed,
        );
        assert_eq!(result, DdCompressionError::Ok);

        let decompressed_slice =
            unsafe { slice::from_raw_parts(decompressed.data, decompressed.len) };
        assert_eq!(decompressed_slice, data);

        dd_buffer_free(compressed);
        dd_buffer_free(decompressed);
        dd_compressor_free(compressor);
    }

    #[test]
    fn test_ffi_stream_compressor() {
        let compressor = dd_compressor_new(DdCompressionAlgorithm::Gzip, 6);
        let stream = dd_stream_new(compressor);
        assert!(!stream.is_null());

        let data1 = b"First chunk. ";
        let data2 = b"Second chunk. ";
        let data3 = b"Third chunk.";

        let written1 = dd_stream_write(stream, data1.as_ptr(), data1.len());
        assert_eq!(written1, data1.len());

        let written2 = dd_stream_write(stream, data2.as_ptr(), data2.len());
        assert_eq!(written2, data2.len());

        let written3 = dd_stream_write(stream, data3.as_ptr(), data3.len());
        assert_eq!(written3, data3.len());

        let mut output = DdBuffer::null();
        let result = dd_stream_close(stream, &mut output);
        assert_eq!(result, DdCompressionError::Ok);
        assert!(!output.data.is_null());

        // Decompress and verify
        let mut decompressed = DdBuffer::null();
        let result =
            dd_compressor_decompress(compressor, output.data, output.len, &mut decompressed);
        assert_eq!(result, DdCompressionError::Ok);

        let expected: Vec<u8> = [data1.as_slice(), data2.as_slice(), data3.as_slice()].concat();
        let decompressed_slice =
            unsafe { slice::from_raw_parts(decompressed.data, decompressed.len) };
        assert_eq!(decompressed_slice, expected.as_slice());

        dd_buffer_free(output);
        dd_buffer_free(decompressed);
        dd_compressor_free(compressor);
    }

    #[test]
    fn test_ffi_null_safety() {
        // These should not crash
        dd_compressor_free(ptr::null_mut());

        let bound = dd_compressor_compress_bound(ptr::null(), 1000);
        assert_eq!(bound, 0);

        let encoding = dd_compressor_content_encoding(ptr::null());
        assert!(encoding.is_null());
    }
}
