// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zlibimpl provides zlib compression
package zlibimpl

import (
	"bytes"
	"compress/zlib"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// ZlibStrategy is the strategy for when serializer_compressor_kind is zlib
type ZlibStrategy struct{}

// New returns a new ZlibStrategy
func New() compression.Compressor {
	return &ZlibStrategy{}
}

// CompressInto compresses src directly into dst, returning the number of bytes written.
func (s *ZlibStrategy) CompressInto(src, dst []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}

	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(src); err != nil {
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}

	compressed := buf.Bytes()
	if len(compressed) > len(dst) {
		return 0, compression.ErrBufferTooSmall
	}

	copy(dst, compressed)
	return len(compressed), nil
}

// CompressBound returns the worst case size needed for a destination buffer.
func (s *ZlibStrategy) CompressBound(sourceLen int) int {
	return sourceLen + (sourceLen >> 12) + (sourceLen >> 14) + (sourceLen >> 25) + 13
}

// ContentEncoding returns the content encoding value for zlib
func (s *ZlibStrategy) ContentEncoding() string {
	return compression.ZlibEncoding
}

// NewStreamCompressor returns a new zlib writer
func (s *ZlibStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	return zlib.NewWriter(output)
}
