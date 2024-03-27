// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && !zstd

// Package compressionimpl provides a set of functions for compressing with zlib / zstd
package compressionimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl/strategy"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Module defines the fx options for the component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewCompressor),
	)
}

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called only when the zlib build tag is included
func NewCompressor(cfg config.Component) compression.Component {
	switch cfg.GetString("serializer_compressor_kind") {
	case ZlibKind:
		return strategy.NewZlibStrategy()
	case ZstdKind:
		log.Warn("zstd build tag not included. using zlib")
		return strategy.NewZlibStrategy()
	case NoneKind:
		log.Warn("no serializer_compressor_kind set. use zlib or zstd")
		return strategy.NewNoopStrategy()
	default:
		log.Warn("invalid serializer_compressor_kind detected. use zlib or zstd")
		return strategy.NewNoopStrategy()
	}
}
