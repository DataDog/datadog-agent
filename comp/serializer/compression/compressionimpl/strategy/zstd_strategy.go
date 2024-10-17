// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package strategy provides a set of functions for compressing with zlib / zstd
package strategy

import (
	"bytes"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	ddzstd "github.com/DataDog/zstd"
	"github.com/klauspost/compress/zstd"
)

// ZstdStrategy is the strategy for when serializer_compressor_kind is zstd
type ZstdStrategy struct {
	level   int
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

// NewZstdStrategy returns a new ZstdStrategy
func NewZstdStrategy(level int) *ZstdStrategy {
	encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
	decoder, _ := zstd.NewReader(nil)
	return &ZstdStrategy{
		level:   level,
		encoder: encoder,
		decoder: decoder,
	}
}

// Compress will compress the data with zstd
func (s *ZstdStrategy) Compress(src []byte) ([]byte, error) {
	return s.encoder.EncodeAll(src, make([]byte, 0, len(src))), nil
}

// Decompress will decompress the data with zstd
func (s *ZstdStrategy) Decompress(src []byte) ([]byte, error) {
	return s.decoder.DecodeAll(src, nil)
}

// CompressBound returns the worst case size needed for a destination buffer when using zstd
func (s *ZstdStrategy) CompressBound(sourceLen int) int {
	return ddzstd.CompressBound(sourceLen)
}

// ContentEncoding returns the content encoding value for zstd
func (s *ZstdStrategy) ContentEncoding() string {
	return compression.ZstdEncoding
}

// NewStreamCompressor returns a new zstd Writer
func (s *ZstdStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	encoder, _ := zstd.NewWriter(output, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(s.level)))
	return encoder
}
