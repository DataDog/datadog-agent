// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zlibimpl provides a set of functions for compressing with zlib
package zlibimpl

import (
	"bytes"
	"compress/zlib"
	"io"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// ZlibStrategy is the strategy for when serializer_compressor_kind is zlib
type ZlibStrategy struct{}

// New returns a new ZlibStrategy
func New() compression.Compressor {
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

// CompressBound returns the worst case size needed for a destination buffer.
// Return value will be > `sourceLen`.
func (s *ZlibStrategy) CompressBound(sourceLen int) int {
	// The formula is: sourceLen + (sourceLen >> 12) + (sourceLen >> 14) + (sourceLen >> 25) + 18
	//
	// Bit shifts (approximate stored block overhead for incompressible
	// data):
	//
	//   - sourceLen >> 12: divides by 4096, ~0.024% overhead
	//   - sourceLen >> 14: divides by 16384, ~0.006% overhead
	//   - sourceLen >> 25: divides by 33554432, negligible but covers edge cases
	//
	// These approximate the 5-byte-per-block overhead when deflate falls
	// back to stored (uncompressed) blocks. Each stored block holds up to
	// 65535 bytes and has a 5-byte header (3 bits type + 5 bits padding +
	// 16 bits LEN + 16 bits NLEN).
	//
	// Constant 18 breakdown:
	//
	//   - 2 bytes: zlib header (CMF + FLG)
	//   - 4 bytes: Adler-32 checksum trailer
	//   - 7 bytes: deflate framing overhead
	//   - 5 bytes: Go's compress/flate empty final block on Close()
	//
	// Go's compress/flate writes an empty stored block (01 00 00 ff ff)
	// when Close() is called, adding 5 bytes that C zlib does not.
	//
	// REF github.com/madler/zlib/blob/75133f8599b7b4509db50e673c66a42c1da1be03/compress.c#L87
	// REF compress/flate/deflate.go compressor.close() -> writeStoredHeader(0, true)
	return sourceLen + (sourceLen >> 12) + (sourceLen >> 14) + (sourceLen >> 25) + 18
}

// ContentEncoding returns the content encoding value for zlib
func (s *ZlibStrategy) ContentEncoding() string {
	return compression.ZlibEncoding
}

// NewStreamCompressor returns a new zlib writer
func (s *ZlibStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	return zlib.NewWriter(output)
}
