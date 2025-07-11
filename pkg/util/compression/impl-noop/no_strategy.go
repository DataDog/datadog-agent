// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package noopimpl provides a set of functions for compressing with zlib / zstd
package noopimpl

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// NoopStrategy is the strategy for when serializer_compressor_kind is neither zlib nor zstd
type NoopStrategy struct{}

// New returns a new NoopStrategy for when kind is neither zlib nor zstd
func New() compression.Compressor {
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
	return "identity"
}

// NewStreamCompressor implements the NewStreamCompressor method for NoopStrategy to satisfy the Compressor interface
func (s *NoopStrategy) NewStreamCompressor(buf *bytes.Buffer) compression.StreamCompressor {
	return &noopStreamCompressor{buf}
}

// NoopStreamCompressor is a no-op implementation of StreamCompressor
type noopStreamCompressor struct {
	*bytes.Buffer
}

// Close closes the underlying writer
func (n *noopStreamCompressor) Close() error {
	return nil
}

// Flush is a no-op
func (n *noopStreamCompressor) Flush() error {
	return nil
}
