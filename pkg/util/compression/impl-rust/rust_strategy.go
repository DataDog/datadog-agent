// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && !no_rust_compression

// Package rustimpl provides compression using the Rust compression library via CGO.
package rustimpl

/*
#cgo CFLAGS: -I${SRCDIR}/../rust/include
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../rust/target/release -ldatadog_compression -framework Security -lm
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../rust/target/release -ldatadog_compression -framework Security -lm
#cgo linux LDFLAGS: -L${SRCDIR}/../rust/target/release -ldatadog_compression -lm -ldl -lpthread
#cgo windows LDFLAGS: -L${SRCDIR}/../rust/target/release -ldatadog_compression -lws2_32 -lbcrypt -luserenv -lntdll

#include <stdlib.h>
#include "datadog_compression.h"
*/
import "C"

import (
	"bytes"
	"errors"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Errors returned by the Rust compression library
var (
	ErrInvalidHandle       = errors.New("invalid compressor handle")
	ErrCompressionFailed   = errors.New("compression failed")
	ErrDecompressionFailed = errors.New("decompression failed")
	ErrStreamClosed        = errors.New("stream already closed")
)

// RustCompressor wraps the Rust compression library.
type RustCompressor struct {
	handle *C.dd_compressor_t
	algo   string
}

// Requires specifies the configuration for creating a RustCompressor.
type Requires struct {
	Algorithm string // "zstd", "gzip", "zlib", or "none"
	Level     int
}

// New creates a new RustCompressor with the specified configuration.
func New(req Requires) compression.Compressor {
	var algo C.dd_compression_algorithm_t
	switch req.Algorithm {
	case compression.ZstdKind:
		algo = C.DD_COMPRESSION_ALGORITHM_ZSTD
	case compression.GzipKind:
		algo = C.DD_COMPRESSION_ALGORITHM_GZIP
	case compression.ZlibKind, "deflate":
		algo = C.DD_COMPRESSION_ALGORITHM_ZLIB
	default:
		algo = C.DD_COMPRESSION_ALGORITHM_NOOP
	}

	handle := C.dd_compressor_new(algo, C.int(req.Level))
	if handle == nil {
		// Fallback to noop if creation fails
		handle = C.dd_compressor_new(C.DD_COMPRESSION_ALGORITHM_NOOP, 0)
	}

	return &RustCompressor{
		handle: handle,
		algo:   req.Algorithm,
	}
}

// NewZstd creates a new zstd compressor with the specified level.
func NewZstd(level int) compression.Compressor {
	return New(Requires{Algorithm: "zstd", Level: level})
}

// NewGzip creates a new gzip compressor with the specified level.
func NewGzip(level int) compression.Compressor {
	return New(Requires{Algorithm: "gzip", Level: level})
}

// NewZlib creates a new zlib compressor with the specified level.
func NewZlib(level int) compression.Compressor {
	return New(Requires{Algorithm: "zlib", Level: level})
}

// NewNoop creates a new no-op compressor.
func NewNoop() compression.Compressor {
	return New(Requires{Algorithm: "none", Level: 0})
}

// Compress compresses the input data.
func (c *RustCompressor) Compress(src []byte) ([]byte, error) {
	if c.handle == nil {
		return nil, ErrInvalidHandle
	}

	if len(src) == 0 {
		return []byte{}, nil
	}

	var outBuffer C.dd_buffer_t
	result := C.dd_compressor_compress(
		c.handle,
		(*C.uint8_t)(unsafe.Pointer(&src[0])),
		C.size_t(len(src)),
		&outBuffer,
	)

	if result != C.DD_COMPRESSION_ERROR_OK {
		return nil, ErrCompressionFailed
	}

	if outBuffer.data == nil || outBuffer.len == 0 {
		return []byte{}, nil
	}

	// Copy data from Rust-allocated buffer to Go slice
	out := C.GoBytes(unsafe.Pointer(outBuffer.data), C.int(outBuffer.len))

	// Free the Rust buffer
	C.dd_buffer_free(outBuffer)

	return out, nil
}

// Decompress decompresses the input data.
func (c *RustCompressor) Decompress(src []byte) ([]byte, error) {
	if c.handle == nil {
		return nil, ErrInvalidHandle
	}

	if len(src) == 0 {
		return []byte{}, nil
	}

	var outBuffer C.dd_buffer_t
	result := C.dd_compressor_decompress(
		c.handle,
		(*C.uint8_t)(unsafe.Pointer(&src[0])),
		C.size_t(len(src)),
		&outBuffer,
	)

	if result != C.DD_COMPRESSION_ERROR_OK {
		return nil, ErrDecompressionFailed
	}

	if outBuffer.data == nil || outBuffer.len == 0 {
		return []byte{}, nil
	}

	// Copy data from Rust-allocated buffer to Go slice
	out := C.GoBytes(unsafe.Pointer(outBuffer.data), C.int(outBuffer.len))

	// Free the Rust buffer
	C.dd_buffer_free(outBuffer)

	return out, nil
}

// CompressBound returns the worst-case compressed size for the given input length.
func (c *RustCompressor) CompressBound(sourceLen int) int {
	if c.handle == nil {
		return sourceLen
	}

	return int(C.dd_compressor_compress_bound(c.handle, C.size_t(sourceLen)))
}

// ContentEncoding returns the HTTP Content-Encoding header value.
func (c *RustCompressor) ContentEncoding() string {
	if c.handle == nil {
		return "identity"
	}

	encoding := C.dd_compressor_content_encoding(c.handle)
	if encoding == nil {
		return "identity"
	}

	return C.GoString(encoding)
}

// NewStreamCompressor creates a new streaming compressor.
func (c *RustCompressor) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	if c.handle == nil {
		return &rustStreamCompressor{
			output: output,
			closed: true,
		}
	}

	stream := C.dd_stream_new(c.handle)
	if stream == nil {
		return &rustStreamCompressor{
			output: output,
			closed: true,
		}
	}

	return &rustStreamCompressor{
		stream: stream,
		output: output,
		closed: false,
	}
}

// Close releases the compressor resources.
func (c *RustCompressor) Close() {
	if c.handle != nil {
		C.dd_compressor_free(c.handle)
		c.handle = nil
	}
}

// rustStreamCompressor wraps the Rust stream compressor.
type rustStreamCompressor struct {
	stream               *C.dd_stream_t
	output               *bytes.Buffer
	closed               bool
	bytesWrittenToOutput int // tracks bytes already written to output buffer during flush
}

// Write writes data to the compression stream.
func (s *rustStreamCompressor) Write(p []byte) (n int, err error) {
	if s.closed || s.stream == nil {
		return 0, ErrStreamClosed
	}

	if len(p) == 0 {
		return 0, nil
	}

	written := C.dd_stream_write(
		s.stream,
		(*C.uint8_t)(unsafe.Pointer(&p[0])),
		C.size_t(len(p)),
	)

	return int(written), nil
}

// Flush flushes any buffered data to the output buffer.
// This ensures output.Len() reflects the current compressed size,
// which is needed for the serializer's hasRoomForItem logic.
func (s *rustStreamCompressor) Flush() error {
	if s.closed || s.stream == nil {
		return ErrStreamClosed
	}

	result := C.dd_stream_flush(s.stream)
	if result != C.DD_COMPRESSION_ERROR_OK {
		return ErrCompressionFailed
	}

	// Get current compressed output size
	currentLen := int(C.dd_stream_output_len(s.stream))

	// Write any new bytes to the output buffer
	if currentLen > s.bytesWrittenToOutput {
		var outBuffer C.dd_buffer_t
		result = C.dd_stream_get_output(s.stream, &outBuffer)
		if result != C.DD_COMPRESSION_ERROR_OK {
			return ErrCompressionFailed
		}

		if outBuffer.data != nil && outBuffer.len > 0 {
			// Only write the new bytes (from bytesWrittenToOutput to currentLen)
			compressed := C.GoBytes(unsafe.Pointer(outBuffer.data), C.int(outBuffer.len))
			s.output.Write(compressed[s.bytesWrittenToOutput:])
			s.bytesWrittenToOutput = currentLen
			C.dd_buffer_free(outBuffer)
		}
	}

	return nil
}

// Close finalizes the stream and writes any remaining compressed data to the output buffer.
func (s *rustStreamCompressor) Close() error {
	if s.closed {
		return nil
	}

	s.closed = true

	if s.stream == nil {
		return nil
	}

	var outBuffer C.dd_buffer_t
	result := C.dd_stream_close(s.stream, &outBuffer)
	s.stream = nil

	if result != C.DD_COMPRESSION_ERROR_OK {
		return ErrCompressionFailed
	}

	if outBuffer.data != nil && outBuffer.len > 0 {
		// Copy only the remaining compressed data that wasn't written during flush
		compressed := C.GoBytes(unsafe.Pointer(outBuffer.data), C.int(outBuffer.len))
		if s.bytesWrittenToOutput < len(compressed) {
			s.output.Write(compressed[s.bytesWrittenToOutput:])
		}
		C.dd_buffer_free(outBuffer)
	}

	return nil
}
