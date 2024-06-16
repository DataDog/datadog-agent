// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package strategy provides a set of functions for compressing with zlib / zstd
package strategy

import (
	"bytes"
	"log"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/zstd"
)

// ZstdStrategy is the strategy for when serializer_compressor_kind is zstd
type ZstdStrategy struct {
	ctx    zstd.Ctx
	output []byte
	level  int
}

// NewZstdStrategy returns a new ZstdStrategy
func NewZstdStrategy(level int) *ZstdStrategy {
	return &ZstdStrategy{
		ctx:    zstd.NewCtx(),
		output: make([]byte, 0),
		level:  level,
	}
}

// Compress will compress the data with zstd
func (s *ZstdStrategy) Compress(src []byte) ([]byte, error) {
	bound := zstd.CompressBound(len(src))

	if cap(s.output) < bound {
		// We need to reallocate the buffer to accomodate the larger size
		s.output = make([]byte, bound)
		log.Debugf("Reallocating zstd buffer to %d bytes", bound)
	}

	return s.ctx.CompressLevel(s.output, src, s.level)
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
	return zstd.NewWriterLevel(output, s.level)
}
