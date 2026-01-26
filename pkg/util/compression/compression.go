// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compression provides a set of constants describing the compression options
package compression

import (
	"bytes"
	"errors"
	"io"
)

// ErrBufferTooSmall is returned when the destination buffer is too small for the compressed data.
var ErrBufferTooSmall = errors.New("output buffer too small")

// ZlibKind defines a const value for the zlib compressor
const ZlibKind = "zlib"

// ZstdKind  defines a const value for the zstd compressor
const ZstdKind = "zstd"

// GzipKind  defines a const value for the gzip compressor
const GzipKind = "gzip"

// NoneKind defines a const value for disabling compression
const NoneKind = "none"

// ZlibEncoding is the content-encoding value for Zlib
const ZlibEncoding = "deflate"

// ZstdEncoding is the content-encoding value for Zstd
const ZstdEncoding = "zstd"

// GzipEncoding is the content-encoding value for Gzip
const GzipEncoding = "gzip"

// Compressor is the interface for compression algorithms.
// All methods are zero-copy for maximum performance.
type Compressor interface {
	// CompressInto compresses src directly into dst, returning bytes written.
	// dst must be at least CompressBound(len(src)) bytes.
	CompressInto(src, dst []byte) (int, error)

	// CompressBound returns the maximum compressed size for the given input length.
	CompressBound(sourceLen int) int

	// ContentEncoding returns the HTTP Content-Encoding header value.
	ContentEncoding() string

	// NewStreamCompressor creates a new streaming compressor.
	NewStreamCompressor(output *bytes.Buffer) StreamCompressor
}

// StreamCompressor is the interface that the compression algorithm
// should implement for streaming
type StreamCompressor interface {
	io.WriteCloser
	Flush() error
}

// ZstdCompressionLevel is a wrapper type over int for the compression level for zstd compression, if that is selected.
type ZstdCompressionLevel int
