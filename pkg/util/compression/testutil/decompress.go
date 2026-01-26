// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testutil provides TEST-ONLY decompression utilities for the compression package.
//
// DO NOT import this package in production code. It exists solely to support
// round-trip testing (compress -> decompress -> compare) in test files.
//
// These functions are intentionally not part of the Compressor interface because
// decompression is not needed in the agent's hot path - the Datadog backend
// handles decompression.
package testutil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"

	"github.com/DataDog/zstd"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// DecompressZstdInto decompresses zstd data from src into dst.
// Returns the number of bytes written to dst.
// TEST ONLY.
func DecompressZstdInto(src, dst []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}
	decompressed, err := zstd.Decompress(dst[:0], src)
	if err != nil {
		return 0, err
	}
	// Verify we didn't reallocate - if we did, copy back
	if cap(decompressed) != cap(dst) && len(decompressed) <= cap(dst) {
		copy(dst, decompressed)
	}
	return len(decompressed), nil
}

// DecompressZstd decompresses zstd data, allocating a new buffer.
// TEST ONLY.
func DecompressZstd(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return []byte{}, nil
	}
	return zstd.Decompress(nil, src)
}

// DecompressGzipInto decompresses gzip data from src into dst.
// Returns the number of bytes written to dst.
// TEST ONLY.
func DecompressGzipInto(src, dst []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(src))
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	n, err := io.ReadFull(reader, dst)
	if err == io.ErrUnexpectedEOF {
		return n, nil
	}
	return n, err
}

// DecompressGzip decompresses gzip data, allocating a new buffer.
// TEST ONLY.
func DecompressGzip(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return []byte{}, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// DecompressZlibInto decompresses zlib data from src into dst.
// Returns the number of bytes written to dst.
// TEST ONLY.
func DecompressZlibInto(src, dst []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}
	reader, err := zlib.NewReader(bytes.NewReader(src))
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	n, err := io.ReadFull(reader, dst)
	if err == io.ErrUnexpectedEOF {
		return n, nil
	}
	return n, err
}

// DecompressZlib decompresses zlib data, allocating a new buffer.
// TEST ONLY.
func DecompressZlib(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return []byte{}, nil
	}
	reader, err := zlib.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// Decompress decompresses data based on the content encoding.
// Supports "zstd", "gzip", "deflate" (zlib), and "identity" (no-op).
// TEST ONLY.
func Decompress(src []byte, contentEncoding string) ([]byte, error) {
	switch contentEncoding {
	case compression.ZstdEncoding:
		return DecompressZstd(src)
	case compression.GzipEncoding:
		return DecompressGzip(src)
	case compression.ZlibEncoding:
		return DecompressZlib(src)
	case "identity", "":
		return src, nil
	default:
		return DecompressZstd(src) // default to zstd
	}
}
