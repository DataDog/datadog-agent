// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !zlib && !zstd

// Package selector provides correct compression impl to fx
package selector

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	compression "github.com/DataDog/datadog-agent/comp/serializer/compressionfactory/def"
	implnoop "github.com/DataDog/datadog-agent/comp/serializer/compressionfactory/impl-noop"
)

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called only when there is no zlib or zstd tag
func NewCompressorKind(_kind string, _level int) compression.Component {
	return implnoop.NewComponent(implnoop.Requires{
		NewKind: NewCompressorKind,
	}).Comp
}

// NewCompressorReq returns a new Compressor based on serializer_compressor_kind
// This function is called only when there is no zlib or zstd tag
func NewCompressorReq(_ Requires) Provides {
	return Provides{Comp: implnoop.NewComponent(implnoop.Requires{
		NewKind: NewCompressorKind,
	}).Comp}
}

// NewNoopCompressorReq returns a new Noop Compressor. It does not do any
// compression, but can be used to create a compressor that does at a later
// point.
// This function is called only when there is no zlib or zstd tag
func NewNoopCompressorReq() Provides {
	return Provides{Comp: implnoop.NewComponent(implnoop.Requires{
		NewKind: NewCompressorKind,
	}).Comp}
}

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called only when there is no zlib or zstd tag
func NewCompressor(cfg config.Component) compression.Component {
	return NewCompressorReq(Requires{Cfg: cfg}).Comp
}
