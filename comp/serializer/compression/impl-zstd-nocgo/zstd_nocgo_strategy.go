// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zstdimpl provides a set of functions for compressing with zstd
package zstdimpl

import (
	"bytes"

	"github.com/DataDog/zstd"

	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/def"
)

// Requires contains the compression level for zstd compression
type Requires struct {
	Level int
}

// Provides contains the compression component
type Provides struct {
	Comp compression.Component
}

// ZstdNoCgoStrategy is the strategy for when serializer_compressor_kind is zstd
type ZstdNoCgoStrategy struct {
	level int
}

// NewComponent returns a new ZstdNoCgoStrategy
func NewComponent(reqs Requires) Provides {
	return Provides{
		Comp: &ZstdNoCgoStrategy{
			level: reqs.Level,
		},
	}
}

// Compress will compress the data with zstd
func (s *ZstdNoCgoStrategy) Compress(src []byte) ([]byte, error) {
	return zstd.CompressLevel(nil, src, s.level)
}

// Decompress will decompress the data with zstd
func (s *ZstdNoCgoStrategy) Decompress(src []byte) ([]byte, error) {
	return zstd.Decompress(nil, src)
}

// CompressBound returns the worst case size needed for a destination buffer when using zstd
func (s *ZstdNoCgoStrategy) CompressBound(sourceLen int) int {
	return zstd.CompressBound(sourceLen)
}

// ContentEncoding returns the content encoding value for zstd
func (s *ZstdNoCgoStrategy) ContentEncoding() string {
	return compression.ZstdEncoding
}

// NewStreamCompressor returns a new zstd Writer
func (s *ZstdNoCgoStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	return zstd.NewWriterLevel(output, s.level)
}
