// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gzipimpl provides gzip compression
package gzipimpl

import (
	"bytes"
	"compress/gzip"

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

// CompressInto compresses src directly into dst, returning the number of bytes written.
func (s *GzipStrategy) CompressInto(src, dst []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}

	var buf bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&buf, s.level)
	if err != nil {
		return 0, err
	}

	if _, err = gzipWriter.Write(src); err != nil {
		return 0, err
	}
	if err = gzipWriter.Close(); err != nil {
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
// Gzip worst case is ~0.015% expansion plus 18 bytes header/trailer.
func (s *GzipStrategy) CompressBound(sourceLen int) int {
	return sourceLen + (sourceLen/32768)*5 + 18
}

// ContentEncoding returns the content encoding value for gzip
func (s *GzipStrategy) ContentEncoding() string {
	return compression.GzipEncoding
}

// NewStreamCompressor returns a new gzip Writer
func (s *GzipStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
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
