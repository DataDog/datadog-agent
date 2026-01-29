// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package noopimpl provides a no-op compressor
package noopimpl

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// NoopStrategy is the strategy for when compression is disabled
type NoopStrategy struct{}

// New returns a new NoopStrategy
func New() compression.Compressor {
	return &NoopStrategy{}
}

// CompressInto copies src directly into dst (no compression).
func (s *NoopStrategy) CompressInto(src, dst []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}
	if len(dst) < len(src) {
		return 0, compression.ErrBufferTooSmall
	}
	copy(dst, src)
	return len(src), nil
}

// CompressBound returns the input length (no compression overhead).
func (s *NoopStrategy) CompressBound(sourceLen int) int {
	return sourceLen
}

// ContentEncoding returns "identity" (no encoding).
func (s *NoopStrategy) ContentEncoding() string {
	return "identity"
}

// NewStreamCompressor returns a no-op stream compressor.
func (s *NoopStrategy) NewStreamCompressor(buf *bytes.Buffer) compression.StreamCompressor {
	return &noopStreamCompressor{buf}
}

// noopStreamCompressor is a no-op implementation of StreamCompressor
type noopStreamCompressor struct {
	*bytes.Buffer
}

// Close is a no-op
func (n *noopStreamCompressor) Close() error {
	return nil
}

// Flush is a no-op
func (n *noopStreamCompressor) Flush() error {
	return nil
}
