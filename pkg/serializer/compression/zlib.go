// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && !zstd

// Package compression provides a set of functions for compressing with zlib / zstd
package compression

import (
	"bytes"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compression "github.com/DataDog/datadog-agent/pkg/serializer/compression/internal"
	"github.com/DataDog/datadog-agent/pkg/serializer/compression/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewCompressorStrategy returns a new Compressor based on serializer_compressor_kind
// This function is called only when the zlib build tag is included
func NewCompressorStrategy(cfg config.Component) utils.Compressor {
	kind := cfg.GetString("serializer_compressor_kind")
	switch kind {
	case utils.ZlibKind:
		return compression.NewZlibStrategy()
	case utils.ZstdKind:
		log.Warn("zstd build tag not included. using zlib")
		return compression.NewZlibStrategy()
	case utils.NoneKind:
		log.Warn("no serializer_compressor_kind set. use zlib or zstd")
		return compression.NewNoopStrategy()
	default:
		log.Warn("invalid serializer_compressor_kind detected. use zlib or zstd")
		return compression.NewNoopStrategy()
	}
}

// NewZipper returns a Zipper to be used by the stream Compressor
func NewStreamCompressor(output *bytes.Buffer, cfg config.Component) utils.StreamCompressor {
	kind := cfg.GetString("serializer_compressor_kind")
	switch kind {
	case utils.ZlibKind:
		return compression.NewZlibStreamCompressor(output)
	case utils.ZstdKind:
		log.Warn("zstd build tag not included. using zlib")
		return compression.NewZlibStreamCompressor(output)
	default:
		log.Warn("invalid serializer_compressor_kind detected. use zlib or zstd")
		return compression.NewNoopStreamCompressor(output)
	}
}
