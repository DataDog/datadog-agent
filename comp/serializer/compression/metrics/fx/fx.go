// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the serializer/compression/metrics component
package fx

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	factory "github.com/DataDog/datadog-agent/comp/serializer/compression/factory/def"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/compression/metrics/def"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Requires contains the config for Compression
type Requires struct {
	Cfg     config.Component
	Factory factory.Component
}

// Provides contains the compression component
type Provides struct {
	Comp metricscompression.Component
}

func NewCompressorReq(req Requires) Provides {
	kind := req.Cfg.GetString("serializer_compressor_kind")
	var level int

	switch kind {
	case compression.ZstdKind:
		level = req.Cfg.GetInt("serializer_zstd_compressor_level")
	case compression.GzipKind:
		// There is no configuration option for gzip compression level when set via this method.
		level = 6
	}

	return Provides{
		req.Factory.NewCompressor(kind, level),
	}
}

// Module defines the fx options for the component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			NewCompressorReq,
		),
	)
}
