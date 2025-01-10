// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the serializer/otelcompression component
package fx

import (
	otelcompression "github.com/DataDog/datadog-agent/comp/serializer/otelcompression/def"
	zlib "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Provides contains the compression component
type Provides struct {
	Comp otelcompression.Component
}

// NewCompressorReq returns the compression component
func NewCompressorReq() Provides {
	return Provides{
		Comp: zlib.New(),
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
