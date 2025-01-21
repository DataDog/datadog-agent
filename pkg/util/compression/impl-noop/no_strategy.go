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

// NewStreamCompressor returns a nil when there is no compression implementation.
func (s *NoopStrategy) NewStreamCompressor(_ *bytes.Buffer) compression.StreamCompressor {
	return nil
}
