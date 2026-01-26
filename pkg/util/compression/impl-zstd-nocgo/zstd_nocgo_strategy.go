// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zstdimpl provides zstd compression without CGO
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
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
		zstd.WithEncoderConcurrency(conc),
		zstd.WithLowerEncoderMem(true),
		zstd.WithWindowSize(window))
	if err != nil {
		_ = log.Errorf("Error creating zstd encoder: %v", err)
		return nil
	}

	return &ZstdNoCgoStrategy{
		level:   level,
		encoder: encoder,
	}
}

// CompressInto compresses src directly into dst, returning the number of bytes written.
// The dst buffer must have capacity >= CompressBound(len(src)).
// Returns ErrBufferTooSmall if dst doesn't have sufficient capacity.
func (s *ZstdNoCgoStrategy) CompressInto(src, dst []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}

	// Check if dst has enough capacity before compression.
	// EncodeAll will reallocate if cap(dst) is insufficient, which would
	// result in compressed data NOT being in dst - a subtle bug.
	bound := s.encoder.MaxEncodedSize(len(src))
	if cap(dst) < bound {
		return 0, compression.ErrBufferTooSmall
	}

	// Use dst[:0] to start writing at the beginning of dst while preserving capacity.
	// Since we checked capacity above, EncodeAll will write directly to dst's backing array.
	compressed := s.encoder.EncodeAll(src, dst[:0])

	// Verify the compressed data is actually in dst (same backing array).
	// This guards against any edge cases where EncodeAll might reallocate.
	if cap(compressed) != cap(dst) {
		// This should not happen given the capacity check above, but guard against it.
		// If it does happen, we need to copy the data to dst.
		if len(compressed) > cap(dst) {
			return 0, compression.ErrBufferTooSmall
		}
		copy(dst, compressed)
	}

	return len(compressed), nil
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
	writer, _ := zstd.NewWriter(output, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(s.level)))
	return writer
}
