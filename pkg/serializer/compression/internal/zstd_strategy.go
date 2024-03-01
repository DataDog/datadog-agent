// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package compression provides a set of functions for compressing with zlib / zstd
package compression

import (
	"bytes"

	"github.com/DataDog/zstd"
)

// ZstdEncoding is the content-encoding value for Zstd
const ZstdEncoding = "zstd"

// ZlibStrategy is the strategy for when serializer_compressor_kind is zlib
type ZstdStrategy struct {
}

// NewZstdStrategy returns a new ZstdStrategy
func NewZstdStrategy() *ZstdStrategy {
	return &ZstdStrategy{}
}

// Compress will compress the data with zstd
func (s *ZstdStrategy) Compress(src []byte) ([]byte, error) {
	return zstd.Compress(nil, src)
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
	return ZstdEncoding
}

// NewZstdZipper returns a new zstd Writer
func NewZstdZipper(output *bytes.Buffer) *zstd.Writer {
	return zstd.NewWriter(output)
}
