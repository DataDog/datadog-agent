// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zstd provides zstd compression with two interchangeable backends:
//   - a CGO-accelerated backend backed by github.com/DataDog/zstd (the native
//     libzstd) when CGO is enabled, and
//   - a pure-Go backend backed by github.com/klauspost/compress/zstd when CGO
//     is disabled.
//
// Both backends produce and consume standard zstd frames and are
// interoperable: data compressed by one can be decompressed by the other.
// Callers should import this package instead of either underlying library
// directly, so that a single build can target both CGO and non-CGO
// environments.
package zstd

import "io"

// Writer is a streaming zstd encoder. It extends io.WriteCloser with a Flush
// method so callers can flush partial frames.
type Writer interface {
	io.WriteCloser
	Flush() error
}

// BestSpeed is the fastest zstd compression level.
const BestSpeed = 1

// BestCompression is the highest zstd compression level supported by the
// native libzstd backend.
const BestCompression = 20

// DefaultCompression is the default zstd compression level.
const DefaultCompression = 5

// CompressBound returns the worst-case size needed to compress srcSize bytes.
// It mirrors ZSTD_COMPRESSBOUND from the zstd specification and is a valid
// upper bound for the output of either backend.
func CompressBound(srcSize int) int {
	lowLimit := 128 << 10 // 128 kB
	margin := 0
	if srcSize < lowLimit {
		margin = (lowLimit - srcSize) >> 11
	}
	return srcSize + (srcSize >> 8) + margin
}

// Compress compresses src at DefaultCompression. If dst is large enough it is
// reused, otherwise a new buffer is allocated.
func Compress(dst, src []byte) ([]byte, error) {
	return CompressLevel(dst, src, DefaultCompression)
}
