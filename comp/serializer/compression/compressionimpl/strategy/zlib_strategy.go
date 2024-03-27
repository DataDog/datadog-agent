// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package strategy provides a set of functions for compressing with zlib / zstd
package strategy

import (
	"bytes"
	"compress/zlib"
	"io"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
)

// ZlibStrategy is the strategy for when serializer_compressor_kind is zlib
type ZlibStrategy struct {
}

// NewZlibStrategy returns a new ZlibStrategy
func NewZlibStrategy() *ZlibStrategy {
	return &ZlibStrategy{}
}

// Compress will compress the data with zlib
func (s *ZlibStrategy) Compress(src []byte) ([]byte, error) {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err := w.Write(src)
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	dst := b.Bytes()
	return dst, nil
}

// Decompress will decompress the data with zlib
func (s *ZlibStrategy) Decompress(src []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	dst, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return dst, nil
}

// CompressBound returns the worst case size needed for a destination buffer
// This is allowed to return a value _larger_ than 'sourceLen'.
// Ref: https://refspecs.linuxbase.org/LSB_3.0.0/LSB-Core-generic/LSB-Core-generic/zlib-compressbound-1.html
func (s *ZlibStrategy) CompressBound(sourceLen int) int {
	// From https://code.woboq.org/gcc/zlib/compress.c.html#compressBound
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
