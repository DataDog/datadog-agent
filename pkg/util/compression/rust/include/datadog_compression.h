/**
 * Datadog Compression Library - C FFI Interface
 *
 * This header provides C bindings for the Rust compression library.
 * Supports zstd, gzip, zlib, and noop compression algorithms.
 *
 * Memory Management:
 * - Buffers returned by dd_compress/dd_decompress must be freed with dd_buffer_free()
 * - Compressor handles must be freed with dd_compressor_free()
 * - Stream handles are freed automatically by dd_stream_close()
 *
 * Thread Safety:
 * - Compressor handles are NOT inherently thread-safe. The underlying Rust
 *   implementation uses internal mutable state for optimal performance.
 *   Callers must provide their own synchronization (e.g., mutex) if sharing
 *   a compressor handle between threads.
 * - The Go wrapper (impl-rust) provides thread safety via sync.Mutex.
 * - Stream handles are NOT thread-safe; use one stream per thread.
 */

#ifndef DATADOG_COMPRESSION_H
#define DATADOG_COMPRESSION_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/**
 * Compression algorithm identifiers.
 */
typedef enum {
    DD_COMPRESSION_ALGORITHM_ZSTD = 0,
    DD_COMPRESSION_ALGORITHM_GZIP = 1,
    DD_COMPRESSION_ALGORITHM_ZLIB = 2,
    DD_COMPRESSION_ALGORITHM_NOOP = 3,
} dd_compression_algorithm_t;

/**
 * Error codes returned by compression operations.
 */
typedef enum {
    DD_COMPRESSION_ERROR_OK = 0,
    DD_COMPRESSION_ERROR_INVALID_INPUT = 1,
    DD_COMPRESSION_ERROR_INVALID_HANDLE = 2,
    DD_COMPRESSION_ERROR_ALLOCATION_FAILED = 3,
    DD_COMPRESSION_ERROR_COMPRESSION_FAILED = 4,
    DD_COMPRESSION_ERROR_DECOMPRESSION_FAILED = 5,
    DD_COMPRESSION_ERROR_BUFFER_TOO_SMALL = 6,
    DD_COMPRESSION_ERROR_STREAM_CLOSED = 7,
    DD_COMPRESSION_ERROR_NOT_SUPPORTED = 8,
    DD_COMPRESSION_ERROR_INTERNAL = 9,
} dd_compression_error_t;

/**
 * Buffer structure for returning data from compression operations.
 * The data pointer is owned by Rust and must be freed with dd_buffer_free().
 */
typedef struct {
    uint8_t *data;      /**< Pointer to buffer data (NULL if empty/error) */
    size_t len;         /**< Length of valid data in bytes */
    size_t capacity;    /**< Total allocated capacity */
} dd_buffer_t;

/**
 * Opaque handle to a compressor instance.
 */
typedef struct dd_compressor_s dd_compressor_t;

/**
 * Opaque handle to a stream compressor instance.
 */
typedef struct dd_stream_s dd_stream_t;

/* ============================================================================
 * Compressor Functions
 * ============================================================================ */

/**
 * Creates a new compressor for the specified algorithm.
 *
 * @param algorithm The compression algorithm to use
 * @param level Compression level (algorithm-specific)
 * @return New compressor handle, or NULL on error
 *
 * @note The returned handle must be freed with dd_compressor_free()
 */
dd_compressor_t *dd_compressor_new(dd_compression_algorithm_t algorithm, int level);

/**
 * Frees a compressor handle.
 *
 * @param compressor Handle to free (NULL is safe)
 */
void dd_compressor_free(dd_compressor_t *compressor);

/**
 * Compresses data using the compressor.
 *
 * @param compressor Valid compressor handle
 * @param src Source data to compress
 * @param src_len Length of source data
 * @param out_buffer Pointer to receive compressed data
 * @return Error code (DD_COMPRESSION_ERROR_OK on success)
 *
 * @note The out_buffer must be freed with dd_buffer_free()
 */
dd_compression_error_t dd_compressor_compress(
    const dd_compressor_t *compressor,
    const uint8_t *src,
    size_t src_len,
    dd_buffer_t *out_buffer
);

/**
 * Decompresses data using the compressor.
 *
 * @param compressor Valid compressor handle
 * @param src Compressed data to decompress
 * @param src_len Length of compressed data
 * @param out_buffer Pointer to receive decompressed data
 * @return Error code (DD_COMPRESSION_ERROR_OK on success)
 *
 * @note The out_buffer must be freed with dd_buffer_free()
 */
dd_compression_error_t dd_compressor_decompress(
    const dd_compressor_t *compressor,
    const uint8_t *src,
    size_t src_len,
    dd_buffer_t *out_buffer
);

/**
 * Returns the worst-case compressed size for the given input length.
 *
 * @param compressor Valid compressor handle
 * @param source_len Length of source data
 * @return Upper bound on compressed size, or 0 on error
 */
size_t dd_compressor_compress_bound(const dd_compressor_t *compressor, size_t source_len);

/**
 * Compresses data directly into a caller-provided buffer (zero-copy).
 *
 * This function eliminates the need for an intermediate allocation by
 * compressing directly into a buffer provided by the caller. Use
 * dd_compressor_compress_bound() to determine the required buffer size.
 *
 * @param compressor Valid compressor handle
 * @param src Source data to compress
 * @param src_len Length of source data
 * @param dst Destination buffer to write compressed data
 * @param dst_capacity Capacity of the destination buffer
 * @return On success: positive value indicating bytes written
 *         On error: negative value (negated dd_compression_error_t)
 *         Special: 0 when src_len is 0
 *
 * @note No memory is allocated; the caller owns the dst buffer
 * @note Returning the value directly eliminates CGO allocation overhead
 */
int64_t dd_compressor_compress_into_fast(
    const dd_compressor_t *compressor,
    const uint8_t *src,
    size_t src_len,
    uint8_t *dst,
    size_t dst_capacity
);

/**
 * Ultra-fast stateless zstd compression for benchmarking.
 * This bypasses context reuse and uses the simplest possible path.
 *
 * @param src Source data to compress
 * @param src_len Length of source data
 * @param dst Destination buffer to write compressed data
 * @param dst_capacity Capacity of the destination buffer
 * @param level Compression level (1-22)
 * @return On success: positive value indicating bytes written
 *         On error: negative value (negated dd_compression_error_t)
 *         Special: 0 when src_len is 0
 */
int64_t dd_zstd_compress_stateless(
    const uint8_t *src,
    size_t src_len,
    uint8_t *dst,
    size_t dst_capacity,
    int level
);

/**
 * Direct zstd compression that bypasses enum dispatch.
 * Uses the compressor's stored zstd context directly.
 *
 * @param compressor Valid zstd compressor handle
 * @param src Source data to compress
 * @param src_len Length of source data
 * @param dst Destination buffer to write compressed data
 * @param dst_capacity Capacity of the destination buffer
 * @return On success: positive value indicating bytes written
 *         On error: negative value (negated dd_compression_error_t)
 *         Special: 0 when src_len is 0
 */
int64_t dd_zstd_compress_direct(
    const dd_compressor_t *compressor,
    const uint8_t *src,
    size_t src_len,
    uint8_t *dst,
    size_t dst_capacity
);

/**
 * Decompresses data directly into a caller-provided buffer (zero-copy).
 *
 * This function eliminates the need for an intermediate allocation by
 * decompressing directly into a buffer provided by the caller. Use
 * dd_get_decompressed_size() to determine the required buffer size.
 *
 * @param compressor Valid compressor handle
 * @param src Compressed source data
 * @param src_len Length of compressed data
 * @param dst Destination buffer to write decompressed data
 * @param dst_capacity Capacity of the destination buffer
 * @param out_written Pointer to receive the number of bytes written
 * @return Error code (DD_COMPRESSION_ERROR_OK on success,
 *         DD_COMPRESSION_ERROR_BUFFER_TOO_SMALL if dst is too small)
 *
 * @note No memory is allocated; the caller owns the dst buffer
 */
dd_compression_error_t dd_compressor_decompress_into(
    const dd_compressor_t *compressor,
    const uint8_t *src,
    size_t src_len,
    uint8_t *dst,
    size_t dst_capacity,
    size_t *out_written
);

/**
 * Returns the decompressed size from the compressed data's frame header/trailer.
 *
 * This function reads algorithm-specific metadata to determine the original
 * uncompressed size without actually decompressing. This enables efficient
 * two-phase decompression: call this first to get the size, allocate a buffer
 * (possibly from a pool), then call dd_compressor_decompress_into().
 *
 * Algorithm behavior:
 * - Zstd: Reads frame content size from header (fast, always accurate)
 * - Gzip: Reads ISIZE from trailer (mod 2^32, may wrap for >4GB files)
 * - Zlib: Returns 0 (format doesn't store original size)
 * - Noop: Returns src_len (passthrough, no compression)
 *
 * @param compressor Valid compressor handle (used for algorithm selection)
 * @param src Compressed data
 * @param src_len Length of compressed data
 * @return The decompressed size if known, or 0 if unknown/invalid
 */
size_t dd_get_decompressed_size(
    const dd_compressor_t *compressor,
    const uint8_t *src,
    size_t src_len
);

/**
 * Returns the content-encoding string for this compressor.
 *
 * @param compressor Valid compressor handle
 * @return Static string (e.g., "zstd", "gzip", "deflate", "identity"), or NULL on error
 */
const char *dd_compressor_content_encoding(const dd_compressor_t *compressor);

/**
 * Returns the algorithm used by this compressor.
 *
 * @param compressor Valid compressor handle
 * @return Algorithm enum value
 */
dd_compression_algorithm_t dd_compressor_algorithm(const dd_compressor_t *compressor);

/* ============================================================================
 * Stream Compressor Functions
 * ============================================================================ */

/**
 * Creates a new stream compressor from a compressor handle.
 *
 * @param compressor Valid compressor handle
 * @return New stream handle, or NULL on error
 *
 * @note The returned handle must be closed with dd_stream_close()
 */
dd_stream_t *dd_stream_new(const dd_compressor_t *compressor);

/**
 * Writes data to the stream compressor.
 *
 * @param stream Valid stream handle
 * @param data Data to write
 * @param data_len Length of data
 * @return Number of bytes written, or 0 on error
 */
size_t dd_stream_write(dd_stream_t *stream, const uint8_t *data, size_t data_len);

/**
 * Flushes buffered data in the stream compressor.
 *
 * @param stream Valid stream handle
 * @return Error code (DD_COMPRESSION_ERROR_OK on success)
 */
dd_compression_error_t dd_stream_flush(dd_stream_t *stream);

/**
 * Closes the stream and returns the final compressed data.
 *
 * @param stream Valid stream handle (freed by this call)
 * @param out_buffer Pointer to receive compressed data
 * @return Error code (DD_COMPRESSION_ERROR_OK on success)
 *
 * @note The stream handle is freed by this call
 * @note The out_buffer must be freed with dd_buffer_free()
 */
dd_compression_error_t dd_stream_close(dd_stream_t *stream, dd_buffer_t *out_buffer);

/**
 * Returns the number of uncompressed bytes written to the stream.
 *
 * @param stream Valid stream handle
 * @return Bytes written, or 0 on error
 */
size_t dd_stream_bytes_written(const dd_stream_t *stream);

/**
 * Returns the current size of compressed output in the stream buffer.
 * This can be used to track compression progress without finalizing.
 *
 * @param stream Valid stream handle
 * @return Compressed bytes currently in buffer, or 0 on error
 */
size_t dd_stream_output_len(const dd_stream_t *stream);

/**
 * Copies the current compressed output from the stream without finalizing.
 * Use this to get intermediate compressed data for progress tracking.
 *
 * @param stream Valid stream handle
 * @param out_buffer Pointer to receive a copy of current compressed output
 * @return Error code (DD_COMPRESSION_ERROR_OK on success)
 *
 * @note The out_buffer must be freed with dd_buffer_free()
 */
dd_compression_error_t dd_stream_get_output(const dd_stream_t *stream, dd_buffer_t *out_buffer);

/* ============================================================================
 * Buffer Functions
 * ============================================================================ */

/**
 * Frees a buffer allocated by the compression library.
 *
 * @param buffer Buffer to free (NULL data is safe)
 */
void dd_buffer_free(dd_buffer_t buffer);

/* ============================================================================
 * Utility Functions
 * ============================================================================ */

/**
 * Returns a human-readable string for an error code.
 *
 * @param error Error code
 * @return Static string describing the error
 */
const char *dd_compression_error_string(dd_compression_error_t error);

/**
 * Returns the library version string.
 *
 * @return Static version string (e.g., "0.1.0")
 */
const char *dd_compression_version(void);

#ifdef __cplusplus
}
#endif

#endif /* DATADOG_COMPRESSION_H */
