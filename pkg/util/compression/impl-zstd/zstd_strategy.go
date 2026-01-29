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
// The dst buffer must have capacity >= CompressBound(len(src)).
// Returns ErrBufferTooSmall if dst doesn't have sufficient capacity.
func (s *ZstdStrategy) CompressInto(src, dst []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}

	// Check if dst has enough capacity before compression.
	// CompressLevel will reallocate if cap(dst) is insufficient, which would
	// result in compressed data NOT being in dst - a subtle bug.
	bound := zstd.CompressBound(len(src))
	if cap(dst) < bound {
		return 0, compression.ErrBufferTooSmall
	}

	// Use dst[:0] to start writing at the beginning of dst while preserving capacity.
	// Since we checked capacity above, CompressLevel will write directly to dst's backing array.
	compressed, err := zstd.CompressLevel(dst[:0], src, s.level)
	if err != nil {
		return 0, err
	}

	// Verify the compressed data is actually in dst (same backing array).
	// This guards against any edge cases where CompressLevel might reallocate.
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
