// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package strategy provides a set of functions for compressing with zlib / zstd /gzip
package strategy

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/klauspost/compress/zstd"
)

// ZstdStrategy is the strategy for when serializer_compressor_kind is zstd
type ZstdNativeStrategy struct {
	level   int
	encoder *zstd.Encoder
}

// NewZstdStrategy returns a new ZstdStrategy
func NewZstdNativeStrategy(level int) *ZstdNativeStrategy {
	log.Debugf("Compressing native zstd at level %d", level)

	encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))

	return &ZstdNativeStrategy{
		level:   level,
		encoder: encoder,
	}
}

// Compress will compress the data with zstd
func (s *ZstdNativeStrategy) Compress(src []byte) ([]byte, error) {
	return s.encoder.EncodeAll(src, make([]byte, 0, len(src))), nil
}

// Decompress will decompress the data with zstd
func (s *ZstdNativeStrategy) Decompress(src []byte) ([]byte, error) {
	//return zstd.Decompress(nil, src)
	return nil, nil
}

// CompressBound returns the worst case size needed for a destination buffer when using zstd
func (s *ZstdNativeStrategy) CompressBound(sourceLen int) int {
	return s.encoder.MaxEncodedSize(sourceLen)
}

// ContentEncoding returns the content encoding value for zstd
func (s *ZstdNativeStrategy) ContentEncoding() string {
	return compression.ZstdEncoding
}

func (s *ZstdNativeStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	//return zstd.NewWriterLevel(output, s.level)
	writer, _ := zstd.NewWriter(output, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(s.level)))
	if writer == nil {
		fmt.Println("wut wut wut")
	}
	return writer
}
