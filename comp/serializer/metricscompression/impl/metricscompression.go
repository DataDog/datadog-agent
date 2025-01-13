// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metricscompressionimpl provides the implementation for the serializer/metricscompression component
package metricscompressionimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	zlib "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
)

// Requires contains the config for Compression
type Requires struct {
	Cfg config.Component
}

// NewCompressorReq returns the compression component
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
		selector.NewCompressor(kind, level),
	}
}

// Provides contains the compression component
type Provides struct {
	Comp metricscompression.Component
}

// NewCompressorReqOtel returns the compression component for Otel
func NewCompressorReqOtel() Provides {
	return Provides{
		Comp: zlib.New(),
	}
}
