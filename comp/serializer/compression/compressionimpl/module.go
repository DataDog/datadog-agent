// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compressionimpl implements a component that provides a factory to return
// different compression algorithms.
package compressionimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl/strategy"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewCompressorFactory),
	)
}

// CompressorFactory is used to create a Compression strategy.
type CompressorFactory struct{}

// NewCompressorFactory creates a new compression factory.
func NewCompressorFactory() compression.Factory {
	return &CompressorFactory{}
}

// FromConfig is used to create a compressor based on fields defined
// in the given configuration.
func FromConfig(cfg config.Component) compression.Component {
	return NewCompressorFactory().NewCompressor(
		cfg.GetString("serializer_compressor_kind"),
		cfg.GetInt("serializer_zstd_compressor_level"),
		"serializer_compressor_kind",
		[]string{"zstd", "zlib"},
	)
}

// NewNoopCompressor creates a noop compressor that performs no compression.
func (*CompressorFactory) NewNoopCompressor() compression.Component {
	return strategy.NewNoopStrategy()
}
