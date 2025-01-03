// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && zstd

// Package selector provides correct compression impl to fx
package selector

import (
	implgzip "github.com/DataDog/datadog-agent/comp/serializer/compressionfactory/impl-gzip"
	implnoop "github.com/DataDog/datadog-agent/comp/serializer/compressionfactory/impl-noop"
	implzlib "github.com/DataDog/datadog-agent/comp/serializer/compressionfactory/impl-zlib"
	implzstd "github.com/DataDog/datadog-agent/comp/serializer/compressionfactory/impl-zstd"
	common "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewCompressorKind returns a new Compressor based on serializer_compressor_kind
// This function is called only when the zlib build tag is included
func (*Factory) NewCompressor(kind string, level int) common.Compressor {
	switch kind {
	case common.ZlibKind:
		return implzlib.NewComponent().Comp
	case common.ZstdKind:
		return implzstd.NewComponent(implzstd.Requires{
			Level: level,
		}).Comp
	case common.GzipKind:
		return implgzip.NewComponent(implgzip.Requires{
			Level: level,
		}).Comp
	case common.NoneKind:
		return implnoop.NewComponent().Comp
	default:
		log.Warn("invalid compression set")
		return implnoop.NewComponent().Comp
	}
}

// NewCompressorReq returns a new Compressor based on serializer_compressor_kind
// This function is called when both zlib and zstd build tags are included
func NewCompressorReq() Provides {
	return Provides{
		Comp: &Factory{},
	}
}

/*
// NewNoopCompressorReq returns a new Noop Compressor. It does not do any
// compression, but can be used to create a compressor that does at a later
// point.
// This function is called only when there is no zlib or zstd tag
/*
func NewNoopCompressorReq() Provides {
	return Provides{Comp: implnoop.NewComponent(implnoop.Requires{
		NewKind: NewCompressorKind,
	}).Comp}
}

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called when both zlib and zstd build tags are included
func NewCompressor(cfg config.Component) compression.Component {
	return NewCompressorReq(Requires{Cfg: cfg}).Comp
}
*/
