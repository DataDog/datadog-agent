// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the serializer/compression/metrics component
package fx

import (
	factory "github.com/DataDog/datadog-agent/comp/serializer/compression/factory/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/compression/logs/def"
)

// Requires contains the config for Compression
type Requires struct {
	Factory factory.Component
}

// Provides contains the compression component
type Provides struct {
	Comp logscompression.Component
}


func NewCompressorReq(req Requires) Provides {
	return Provides {
		req.Factory.NewCompressor
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
