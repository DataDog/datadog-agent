// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && zstd

// Package compressionimpl provides a set of functions for compressing with zlib / zstd
package fx

import (
	"github.com/DataDog/datadog-agent/comp/serializer/compression/common"
	implnoop "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-noop"
	implzlib "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-zlib"
	implzstd "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-zstd"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/def"
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
// This function is called when both zlib and zstd build tags are included
func NewCompressor(cfg config.Component) compression.Component {
	switch cfg.GetString("serializer_compressor_kind") {
	case common.ZlibKind:
		return implzlib.NewZlibStrategy()
	case common.ZstdKind:
		level := cfg.GetInt("serializer_zstd_compressor_level")
		return implzstd.NewZstdStrategy(level)
	case common.NoneKind:
		log.Warn("no serializer_compressor_kind set. use zlib or zstd")
		return implnoop.NewNoopStrategy()
	default:
		log.Warn("invalid serializer_compressor_kind detected. use one of 'zlib', 'zstd'")
		return implnoop.NewNoopStrategy()
	}
}
