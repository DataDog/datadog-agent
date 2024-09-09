// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && zstd

// Package compressionimpl provides a set of functions for compressing with zlib / zstd
package compressionimpl

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl/strategy"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called when both zlib and zstd build tags are included
func (_ *CompressorFactory) NewCompressor(kind string, level int, option string, valid []string) compression.Component {
	if !slices.Contains(valid, kind) {
		log.Warn("invalid " + option + " set. use one of " + strings.Join(valid, ", "))
		return strategy.NewNoopStrategy()
	}

	switch kind {
	case ZlibKind:
		return strategy.NewZlibStrategy()
	case ZstdKind:
		return strategy.NewZstdStrategy(level)
	case GzipKind:
		return strategy.NewGzipStrategy(level)
	case NoneKind:
		log.Warn("no " + option + " set. use one of " + strings.Join(valid, ", "))
		return strategy.NewNoopStrategy()
	default:
		log.Warn("invalid " + option + " set. use one of " + strings.Join(valid, ", "))
		return strategy.NewNoopStrategy()
	}
}
