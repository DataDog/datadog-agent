// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package strategy provides a set of functions for compressing with zlib / zstd /gzip
package strategy

import (
	"bytes"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/zstd"
)

// ZstdStrategy is the strategy for when serializer_compressor_kind is zstd
type ZstdStrategy struct {
	level int
	ctx   zstd.Ctx
}

// NewZstdStrategy returns a new ZstdStrategy
func NewZstdStrategy(level int) *ZstdStrategy {
	log.Debugf("Compressing zstd at level %d", level)
	ctx := zstd.NewCtx()

	window, err := strconv.Atoi(os.Getenv("WAKKAS_WINDOW"))
	if err == nil {
		ctx.SetParameter(zstd.WindowLog, window)
	}

	return &ZstdStrategy{
		level: level,
		ctx:   ctx,
	}
}

// Compress will compress the data with zstd
func (s *ZstdStrategy) Compress(src []byte) ([]byte, error) {
	return s.ctx.CompressLevel(nil, src, s.level)
}

// Decompress will decompress the data with zstd
func (s *ZstdStrategy) Decompress(src []byte) ([]byte, error) {
	return s.ctx.Decompress(nil, src)
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
