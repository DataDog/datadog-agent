// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compression provides a set of functions for compressing with zlib / zstd
package compression

import (
	"bytes"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compression "github.com/DataDog/datadog-agent/pkg/serializer/compression/internal"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ZlibKind defines a const value for the zlib compressor
const ZlibKind = "zlib"

// ZstdKind  defines a const value for the zstd compressor
const ZstdKind = "zstd"

// ZlibEncoding is the content-encoding value for Zlib
const ZlibEncoding = compression.ZlibEncoding

// ZstdEncoding is the content-encoding value for Zstd
const ZstdEncoding = compression.ZstdEncoding

// Compressor is the interface for the compressor used by the Serializer
type Compressor interface {
	Compress(src []byte) ([]byte, error)
	Decompress(src []byte) ([]byte, error)
	CompressBound(sourceLen int) int
	ContentEncoding() string
}

// NewCompressorStrategy returns a new Compressor based on serializer_compressor_kind
func NewCompressorStrategy(cfg config.Component) Compressor {
	kind := cfg.GetString("serializer_compressor_kind")
	switch kind {
	case ZlibKind:
		return compression.NewZlibStrategy()
	case ZstdKind:
		return compression.NewZstdStrategy()
	default:
		log.Warn("invalid serializer_compressor_kind detected. use zlib or zstd")
		return compression.NewNoopStrategy()
	}
}

// StreamCompressor is the interface that zlib and zstd should implement
type StreamCompressor interface {
	io.WriteCloser
	Flush() error
}

// NewZipper returns a StreamCompressor
func NewStreamCompressor(output *bytes.Buffer, cfg config.Component) StreamCompressor {
	kind := cfg.GetString("serializer_compressor_kind")
	switch kind {
	case ZlibKind:
		return compression.NewZlibZipper(output)
	case ZstdKind:
		return compression.NewZstdZipper(output)
	default:
		log.Warn("invalid serializer_compressor_kind detected. use zlib or zstd")
		return compression.NewNoopZipper(output)
	}
}
