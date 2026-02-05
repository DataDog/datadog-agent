// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gzipimpl provides a set of functions for compressing with zlib / zstd / gzip
package gzipimpl

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires contains the compression level for gzip compression
type Requires struct {
	Level int
}

// GzipStrategy is the strategy for when serializer_compression_kind is gzip
type GzipStrategy struct {
	level int
}

// New returns a new GzipStrategy
func New(req Requires) compression.Compressor {
	level := req.Level
	if level < gzip.NoCompression {
		log.Warnf("Gzip log level set to %d, minimum is %d.", level, gzip.NoCompression)
		level = gzip.NoCompression
	} else if level > gzip.BestCompression {
		log.Warnf("Gzip log level set to %d, maximum is %d.", level, gzip.BestCompression)
		level = gzip.BestCompression
	}

	return &GzipStrategy{
		level: level,
	}
}

// Compress will compress the data with gzip
func (s *GzipStrategy) Compress(src []byte) (result []byte, err error) {
	var compressedPayload bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&compressedPayload, s.level)

	if err != nil {
		return nil, err
	}
	_, err = gzipWriter.Write(src)
	if err != nil {
		return nil, err
	}
	// Close calls Flush internally, writing out the final stored
	// block. There is no need to call Flush explicitly before Close. Doing
	// so only writes a 5-byte sync block, inflating the output but to not
	// purpose.
	//
	// Should Go's internals change FuzzGzipCompressBound will fail.
	err = gzipWriter.Close()
	if err != nil {
		return nil, err
	}

	return compressedPayload.Bytes(), nil
}

// Decompress will decompress the data with gzip
func (s *GzipStrategy) Decompress(src []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Read all decompressed data
	var result bytes.Buffer
	_, err = io.Copy(&result, reader)
	if err != nil {
		return nil, err
	}

	return result.Bytes(), nil
}

// CompressBound returns the worst case size needed for a destination buffer
// when using gzip. Return value will be > `sourceLen`.
func (s *GzipStrategy) CompressBound(sourceLen int) int {
	// The formula is: sourceLen + ceil(sourceLen/65535)*5 + 23
	//
	// Stored block overhead (ceil(sourceLen/65535)*5):
	//
	//   - 65535 bytes: maximum data per deflate stored block (16-bit LEN field)
	//   - 5 bytes per block: header (3 bits type + 5 bits padding + 16 bits LEN + 16 bits NLEN)
	//
	// When deflate cannot compress data, it falls back to stored blocks. Each
	// block holds up to 65535 bytes with a 5-byte header. We compute
	// ceil(sourceLen/65535) via (sourceLen+65534)/65535, minimum 1 block.
	//
	// Constant 23 breakdown:
	//
	//   - 10 bytes: gzip header (magic, method, flags, mtime, xfl, os)
	//   - 8 bytes: gzip trailer (CRC32 + ISIZE)
	//   - 5 bytes: Go's compress/flate empty final block on Close()
	//
	// Go's compress/flate writes an empty stored block (01 00 00 ff ff) when
	// Close() is called.
	//
	// REF https://www.ietf.org/rfc/rfc1952.txt
	// REF compress/flate/deflate.go compressor.close() -> writeStoredHeader(0, true)
	storedBlocks := (sourceLen + 65534) / 65535
	if storedBlocks == 0 {
		storedBlocks = 1
	}
	return sourceLen + storedBlocks*5 + 23
}

// ContentEncoding returns the content encoding value for gzip
func (s *GzipStrategy) ContentEncoding() string {
	return compression.GzipEncoding
}

// NewStreamCompressor returns a new gzip Writer
func (s *GzipStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	// Ensure level is within a range that doesn't cause NewWriterLevel to error.
	level := s.level
	if level < gzip.HuffmanOnly {
		log.Warnf("Gzip streaming log level set to %d, minimum is %d. Setting to minimum.", level, gzip.HuffmanOnly)
		level = gzip.HuffmanOnly
	}

	if level > gzip.BestCompression {
		log.Warnf("Gzip streaming log level set to %d, maximum is %d. Setting to maximum.", level, gzip.BestCompression)
		level = gzip.BestCompression
	}

	writer, err := gzip.NewWriterLevel(output, level)
	if err != nil {
		log.Warnf("Error creating gzip writer with level %d. Using default.", level)
		writer = gzip.NewWriter(output)
	}

	return writer
}
