// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package strategy provides a set of functions for compressing with zlib / zstd
package strategy

import (
	"bytes"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
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

// NewStreamCompressor returns a new NoopZipper when serializer_compressor_kind is neither zlib or zstd
func (s *NoopStrategy) NewStreamCompressor(_ *bytes.Buffer) compression.StreamCompressor {
	return NoopZipper{}
}

// NoopZipper is the zipper for when the serializer_compressor_kind is neither zlib nor zstd
type NoopZipper struct{}

// Write implements the Write method for NoopZipper to satisfy the Zipper interface
func (s NoopZipper) Write([]byte) (int, error) {
	return 0, nil
}

// Flush implements the Flush method for NoopStrategy to satisfy the Zipper interface
func (s NoopZipper) Flush() error {
	return nil
}

// Close implements the Close method for NoopStrategy to satisfy the Zipper interface
func (s NoopZipper) Close() error {
	return nil
}
