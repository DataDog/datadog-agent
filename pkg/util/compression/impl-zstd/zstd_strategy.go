// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zstdimpl provides a set of functions for compressing with zstd
package zstdimpl

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/zstd"
)

// Requires contains the compression level for zstd compression
type Requires struct {
	Level compression.ZstdCompressionLevel
}

// ZstdStrategy is the strategy for when serializer_compressor_kind is zstd
type ZstdStrategy struct {
	level zstd.Level
}

// New returns a new ZstdStrategy
func New(reqs Requires) compression.Compressor {
	return &ZstdStrategy{
		level: zstd.LevelFromInt(int(reqs.Level)),
	}
}

// Compress will compress the data with zstd
func (s *ZstdStrategy) Compress(src []byte) ([]byte, error) {
	return zstd.CompressLevel(nil, src, s.level)
}

// Decompress will decompress the data with zstd
func (s *ZstdStrategy) Decompress(src []byte) ([]byte, error) {
	return zstd.Decompress(nil, src)
}

// CompressBound returns the worst case size needed for a destination buffer when using zstd
func (s *ZstdStrategy) CompressBound(sourceLen int) int {
	return zstd.CompressBound(sourceLen)
}

// ContentEncoding returns the content encoding value for zstd
func (s *ZstdStrategy) ContentEncoding() string {
	return compression.ZstdEncoding
}

// NewStreamCompressor returns a new zstd Writer
func (s *ZstdStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	w, err := zstd.NewWriterLevel(output, s.level)
	if err != nil {
		// zstd.NewWriterLevel only returns an error for invalid options, which
		// we do not pass. A non-nil error here is unexpected; return a writer
		// that fails on first use so the caller surfaces it.
		return &errStreamCompressor{err: err}
	}
	return w
}

// errStreamCompressor is a StreamCompressor that fails every operation with err.
type errStreamCompressor struct{ err error }

func (e *errStreamCompressor) Write(_ []byte) (int, error) { return 0, e.err }
func (e *errStreamCompressor) Close() error                { return e.err }
func (e *errStreamCompressor) Flush() error                { return e.err }
