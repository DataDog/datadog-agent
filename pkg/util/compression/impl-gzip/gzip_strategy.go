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

// CompressBound returns the worst case size needed for a destination buffer
// when using gzip
//
// The worst case expansion is a few bytes for the gzip file header, plus
// 5 bytes per 32 KiB block, or an expansion ratio of 0.015% for large files.
// The additional 18 bytes comes from the header (10 bytes) and trailer
// (8 bytes). There is no theoretical maximum to the header,
// but we don't set any extra header fields so it is safe to assume
//
// Source: https://www.gnu.org/software/gzip/manual/html_node/Overview.html
// More details are in the linked RFC: https://www.ietf.org/rfc/rfc1952.txt
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
