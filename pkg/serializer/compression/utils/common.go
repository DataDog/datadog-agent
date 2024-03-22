// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils provides a set of constants for compressing with zlib / zstd
package utils

import "io"

// ZlibKind defines a const value for the zlib compressor
const ZlibKind = "zlib"

// ZstdKind  defines a const value for the zstd compressor
const ZstdKind = "zstd"

// NoneKind defines a const value for disabling compression
const NoneKind = "none"

// ZlibEncoding is the content-encoding value for Zlib
const ZlibEncoding = "deflate"

// ZstdEncoding is the content-encoding value for Zstd
const ZstdEncoding = "zstd"

// Compressor is the interface for the compressor used by the Serializer
type Compressor interface {
	Compress(src []byte) ([]byte, error)
	Decompress(src []byte) ([]byte, error)
	CompressBound(sourceLen int) int
	ContentEncoding() string
}

// StreamCompressor is the interface that zlib and zstd should implement
type StreamCompressor interface {
	io.WriteCloser
	Flush() error
}
