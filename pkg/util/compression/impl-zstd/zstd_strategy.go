// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zstdimpl provides zstd compression
package zstdimpl

import (
	"bytes"

	"github.com/DataDog/zstd"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Requires contains the compression level for zstd compression
type Requires struct {
	Level compression.ZstdCompressionLevel
}

// ZstdStrategy is the strategy for when serializer_compressor_kind is zstd
type ZstdStrategy struct {
	level int
}

// New returns a new ZstdStrategy
func New(reqs Requires) compression.Compressor {
	return &ZstdStrategy{
		level: int(reqs.Level),
	}
}

// CompressInto compresses src directly into dst, returning the number of bytes written.
func (s *ZstdStrategy) CompressInto(src, dst []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}

	compressed, err := zstd.CompressLevel(dst[:0], src, s.level)
	if err != nil {
		return 0, err
	}

	if len(compressed) > len(dst) {
		return 0, compression.ErrBufferTooSmall
	}

	return len(compressed), nil
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
	return zstd.NewWriterLevel(output, s.level)
}
