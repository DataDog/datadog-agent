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
	implnoop "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-noop"
	implzlib "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-zlib"
	implzstd "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-zstd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewCompressorReq returns a new Compressor based on serializer_compressor_kind
// This function is called when both zlib and zstd build tags are included
func NewCompressorReq(req Requires) Provides {
	switch req.Cfg.GetString("serializer_compressor_kind") {
	case common.ZlibKind:
		return Provides{implzlib.NewComponent().Comp}
	case common.ZstdKind:
		level := req.Cfg.GetInt("serializer_zstd_compressor_level")
		return Provides{implzstd.NewComponent(implzstd.Requires{Level: compression.ZstdCompressionLevel(level)}).Comp}
	case common.NoneKind:
		log.Warn("no serializer_compressor_kind set. use zlib or zstd")
		return Provides{implnoop.NewComponent().Comp}
	default:
		log.Warn("invalid serializer_compressor_kind detected. use one of 'zlib', 'zstd'")
		return Provides{implnoop.NewComponent().Comp}
	}
}

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called when both zlib and zstd build tags are included
func NewCompressor(cfg config.Component) compression.Component {
	return NewCompressorReq(Requires{Cfg: cfg}).Comp
}
