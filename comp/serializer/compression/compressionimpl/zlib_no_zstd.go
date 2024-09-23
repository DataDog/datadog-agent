// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && !zstd

// Package compressionimpl provides a set of functions for compressing with zlib / zstd
package compressionimpl

import (

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl/strategy"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called only when the zlib build tag is included
func (*CompressorFactory) NewCompressor(kind string, level int, option string, valid string[]) compression.Component {
	if !slices.Contains(valid, kind) {
		log.Warn("invalid " + option + " set. use one of " + strings.Join(valid, ", "))
		return strategy.NewNoopStrategy()
	}

	switch kind {
	case ZlibKind:
		return strategy.NewZlibStrategy()
	case ZstdKind:
		log.Warn("zstd build tag not included. using zlib")
		return strategy.NewZlibStrategy()
	case NoneKind:
		log.Warn("no " + option + " set. use one of " + strings.Join(valid, ", "))
		return strategy.NewNoopStrategy()
	default:
		log.Warn("invalid " + option + " set. use one of " + strings.Join(valid, ", "))
		return strategy.NewNoopStrategy()
	}
}
