// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compression_test

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/zstd"
	klauspostzstd "github.com/klauspost/compress/zstd"
)

// DecompressZlib decompresses data compressed with zlib - used for testing round-trips
func DecompressZlib(src []byte) ([]byte, error) {
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

// DecompressGzip decompresses data compressed with gzip - used for testing round-trips
func DecompressGzip(src []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var result bytes.Buffer
	_, err = io.Copy(&result, reader)
	if err != nil {
		return nil, err
	}

	return result.Bytes(), nil
}

// DecompressZstd decompresses data compressed with zstd (cgo version) - used for testing round-trips
func DecompressZstd(src []byte) ([]byte, error) {
	return zstd.Decompress(nil, src)
}

// DecompressZstdNoCgo decompresses data compressed with zstd (no-cgo version) - used for testing round-trips
func DecompressZstdNoCgo(src []byte) ([]byte, error) {
	decoder, _ := klauspostzstd.NewReader(nil)
	return decoder.DecodeAll(src, nil)
}

// Decompress decompresses data based on the compressor's content encoding - used for testing round-trips
// This function is only for testing and should not be used in production code.
func Decompress(c compression.Compressor, src []byte) ([]byte, error) {
	switch c.ContentEncoding() {
	case compression.ZlibEncoding:
		return DecompressZlib(src)
	case compression.GzipEncoding:
		return DecompressGzip(src)
	case compression.ZstdEncoding:
		return DecompressZstd(src)
	case "identity": // NoopStrategy
		return src, nil
	default:
		// Try zstd as fallback
		return DecompressZstd(src)
	}
}
