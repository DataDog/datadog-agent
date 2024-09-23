// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package strategy provides a set of functions for compressing with zlib / zstd / gzip
package strategy

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
)

// GzipStrategy is the strategy for when serializer_compression_kind is gzip
type GzipStrategy struct {
	level int
}

// NewGzipStrategy returns a new GzipStrategy
func NewGzipStrategy(level int) *GzipStrategy {
	if level < gzip.NoCompression {
		level = gzip.NoCompression
	} else if level > gzip.BestCompression {
		level = gzip.BestCompression
	}

	return &GzipStrategy{
		level: level,
	}
}

// Compress will compress the data with gzip
func (s *GzipStrategy) Compress(src []byte) ([]byte, error) {
	var compressedPayload bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&compressedPayload, s.level)
	if err != nil {
		return nil, err
	}
	_, err = gzipWriter.Write(src)
	if err != nil {
		return nil, err
	}
	err = gzipWriter.Flush()
	if err != nil {
		return nil, err
	}
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

// CompressBound returns the worst case size needed for a destination buffer when using gzip
// The worst case expansion is a few bytes for the gzip file header, plus 5 bytes per 32 KiB block, or an expansion ratio of 0.015% for large files.
// Source: https://www.gnu.org/software/gzip/manual/html_node/Overview.html
func (s *GzipStrategy) CompressBound(sourceLen int) int {
	return sourceLen + (sourceLen/32768)*5 + 18
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
		level = gzip.HuffmanOnly
	}

	if level > gzip.BestCompression {
		level = gzip.BestCompression
	}

	writer, _ := gzip.NewWriterLevel(output, level)
	return writer
}
