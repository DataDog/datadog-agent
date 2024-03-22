// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compression provides a set of functions for compressing with zlib / zstd
package compression

import (
	"bytes"
)

// NoopStrategy is the strategy for when serializer_compressor_kind is neither zlib nor zstd
type NoopStrategy struct {
}

// NewNoopStrategy returns a new NoopStrategy for when kind is neither zlib nor zstd
func NewNoopStrategy() *NoopStrategy {
	return &NoopStrategy{}
}

// Compress implements the Compress method for NoopStrategy to satisfy the Compressor interface
func (s *NoopStrategy) Compress(src []byte) ([]byte, error) {
	return src, nil
}

// Decompress implements the Decompress method for NoopStrategy to satisfy the Compressor interface
func (s *NoopStrategy) Decompress(src []byte) ([]byte, error) {
	return src, nil
}

// CompressBound implements the CompressBound method for NoopStrategy to satisfy the Compressor interface
func (s *NoopStrategy) CompressBound(sourceLen int) int {
	return sourceLen
}

// ContentEncoding implements the ContentEncoding method for NoopStrategy to satisfy the Compressor interface
func (s *NoopStrategy) ContentEncoding() string {
	return ""
}

// NoopStreamCompressor is the zipper for when the serializer_compressor_kind is neither zlib nor zstd
type NoopStreamCompressor struct{}

// Write implements the Write method for NoopStreamCompressor to satisfy the StreamCompressor interface
func (s NoopStreamCompressor) Write([]byte) (int, error) {
	return 0, nil
}

// Flush implements the Flush method for NoopStrategy to satisfy the StreamCompressor interface
func (s NoopStreamCompressor) Flush() error {
	return nil
}

// Close implements the Close method for NoopStrategy to satisfy the StreamCompressor interface
func (s NoopStreamCompressor) Close() error {
	return nil
}

// NewNoopStreamCompressor returns a new NoopStreamCompressor when serializer_compressor_kind is neither zlib or zstd
func NewNoopStreamCompressor(_ *bytes.Buffer) NoopStreamCompressor {
	return NoopStreamCompressor{}
}
