// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metricscompressionimpl provides the implementation for the serializer/metricscompression component
package metricscompressionimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	zlib "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
)

// Requires contains the config for Compression
type Requires struct {
	Cfg config.Component
}

// NewCompressorReq returns the compression component
func NewCompressorReq(req Requires) Provides {
	return Provides{
		selector.FromConfig(req.Cfg),
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
