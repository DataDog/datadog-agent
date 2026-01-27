// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compression provides a set of constants describing the compression options
package compression

import (
	"bytes"
	"io"
)

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

// Compressor is the interface that a given compression algorithm needs to
// implement.
//
// Thread-Safety: Compressor implementations are safe for concurrent use by
// multiple goroutines. The Compress(), Decompress(), CompressBound(), and
// ContentEncoding() methods can be called concurrently from different
// goroutines.
//
// StreamCompressor instances returned by NewStreamCompressor() are NOT
// thread-safe and should not be shared between goroutines.
type Compressor interface {
	Compress(src []byte) ([]byte, error)
	Decompress(src []byte) ([]byte, error)
	CompressBound(sourceLen int) int
	ContentEncoding() string
	NewStreamCompressor(output *bytes.Buffer) StreamCompressor
}

// StreamCompressor is the interface that the compression algorithm should
// implement for streaming.
//
// Lifecycle:
//
//   - Write() can be called multiple times to add data to the compressed stream
//   - Flush() can be called to ensure buffered data is written (optional)
//   - Close() finalizes the compressed stream and must be called when done
//   - Write() calls after Close() will return an error
//   - Close() is idempotent and can be safely called multiple times
//
// Thread-Safety: StreamCompressor instances are NOT thread-safe and must not be
// used concurrently by multiple goroutines. Each goroutine should create its
// own StreamCompressor instance via Compressor.NewStreamCompressor().
type StreamCompressor interface {
	io.WriteCloser
	Flush() error
}

// ZstdCompressionLevel is a wrapper type over int for the compression level for zstd compression, if that is selected.
type ZstdCompressionLevel int
