// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && zstd

// Package selector provides correct compression impl to fx
package selector

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/common"
	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/def"
	implgzip "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-gzip"
	implnoop "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-noop"
	implzlib "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-zlib"
	implzstd "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-zstd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewCompressorKind returns a new Compressor based on serializer_compressor_kind
// This function is called only when the zlib build tag is included
func NewCompressorKind(kind string, level int) compression.Component {
	switch kind {
	case common.ZlibKind:
		return implzlib.NewComponent(implzlib.Requires{
			NewKind: NewCompressorKind,
		}).Comp
	case common.ZstdKind:
		return implzstd.NewComponent(implzstd.Requires{
			Level:   level,
			NewKind: NewCompressorKind,
		}).Comp
	case common.GzipKind:
		return implgzip.NewComponent(implgzip.Requires{
			Level:   level,
			NewKind: NewCompressorKind,
		}).Comp
	case common.NoneKind:
		return implnoop.NewComponent(
			implnoop.Requires{
				NewKind: NewCompressorKind,
			}).Comp
	default:
		log.Warn("invalid compression set")
		return implnoop.NewComponent(implnoop.Requires{
			NewKind: NewCompressorKind,
		}).Comp
	}
}

// NewCompressorReq returns a new Compressor based on serializer_compressor_kind
// This function is called when both zlib and zstd build tags are included
func NewCompressorReq(req Requires) Provides {
	switch req.Cfg.GetString("serializer_compressor_kind") {
	case common.ZlibKind:
		return Provides{implzlib.NewComponent(implzlib.Requires{
			NewKind: NewCompressorKind,
		}).Comp}
	case common.ZstdKind:
		level := req.Cfg.GetInt("serializer_zstd_compressor_level")
		return Provides{implzstd.NewComponent(implzstd.Requires{
			Level:   level,
			NewKind: NewCompressorKind,
		}).Comp}
	case common.NoneKind:
		log.Warn("no serializer_compressor_kind set. use zlib or zstd")
		return Provides{implnoop.NewComponent(implnoop.Requires{
			NewKind: NewCompressorKind,
		}).Comp}
	case common.GzipKind:
		// There is no configuration option for gzip compression level when set via this method.
		// This is set when called via `NewCompressorKind` for logs.
		return Provides{implgzip.NewComponent(implgzip.Requires{
			Level:   6,
			NewKind: NewCompressorKind,
		}).Comp}
	default:
		log.Warn("invalid serializer_compressor_kind detected. use one of 'zlib', 'zstd'")
		return Provides{implnoop.NewComponent(implnoop.Requires{
			NewKind: NewCompressorKind,
		}).Comp}
	}
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
// This function is called when both zlib and zstd build tags are included
func NewCompressor(cfg config.Component) compression.Component {
	return NewCompressorReq(Requires{Cfg: cfg}).Comp
}
