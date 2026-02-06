// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zstdimpl provides a set of functions for compressing with zstd
package zstdimpl

import (
	"bytes"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/klauspost/compress/zstd"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Requires contains the compression level for zstd compression
type Requires struct {
	Level compression.ZstdCompressionLevel
}

// ZstdNoCgoStrategy can be manually selected via component - it's not used by any selector / config option
type ZstdNoCgoStrategy struct {
	level   int
	encoder *zstd.Encoder
}

// New returns a new ZstdNoCgoStrategy
func New(reqs Requires) compression.Compressor {
	level := int(reqs.Level)
	log.Debugf("Compressing native zstd at level %d", level)

	conc, err := strconv.Atoi(os.Getenv("ZSTD_NOCGO_CONCURRENCY"))
	if err != nil {
		conc = 1
	}

	window, err := strconv.Atoi(os.Getenv("ZSTD_NOCGO_WINDOW"))
	if err != nil {
		window = 1 << 15
	}
	log.Debugf("native zstd concurrency %d", conc)
	log.Debugf("native zstd window size %d", window)
	// WithZeroFrames(true) ensures empty input produces a valid zstd frame.
	// Without this, klauspost/compress returns empty output for empty
	// input, which the CGO zstd library cannot decompress. By enabling
	// WithZeroFrames we ensure that the cgo and nocgo zstd strategies are
	// identical in their behavior.
	//
	// See also FuzzZstdCrossCompatibility.
	//
	// REF
	//  * https://github.com/klauspost/compress/pull/155 ->
	//  * https://github.com/IBM/sarama/pull/1477 ->
	//  * https://github.com/IBM/sarama/issues/1252
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
		zstd.WithEncoderConcurrency(conc),
		zstd.WithLowerEncoderMem(true),
		zstd.WithWindowSize(window),
		zstd.WithZeroFrames(true))
	if err != nil {
		_ = log.Errorf("Error creating zstd encoder: %v", err)
		return nil
	}

	return &ZstdNoCgoStrategy{
		level:   level,
		encoder: encoder,
	}
}

// Compress will compress the data with zstd
func (s *ZstdNoCgoStrategy) Compress(src []byte) ([]byte, error) {
	return s.encoder.EncodeAll(src, nil), nil
}

// Decompress will decompress the data with zstd
func (s *ZstdNoCgoStrategy) Decompress(src []byte) ([]byte, error) {
	decoder, _ := zstd.NewReader(nil)
	return decoder.DecodeAll(src, nil)
}

// CompressBound returns the worst case size needed for a destination buffer when using zstd
func (s *ZstdNoCgoStrategy) CompressBound(sourceLen int) int {
	return s.encoder.MaxEncodedSize(sourceLen)
}

// ContentEncoding returns the content encoding value for zstd
func (s *ZstdNoCgoStrategy) ContentEncoding() string {
	return compression.ZstdEncoding
}

// NewStreamCompressor returns a new zstd Writer
func (s *ZstdNoCgoStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	// WithZeroFrames(true) for CGO compatibility, see New() for details.
	writer, _ := zstd.NewWriter(output,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(s.level)),
		zstd.WithZeroFrames(true))
	return writer
}
