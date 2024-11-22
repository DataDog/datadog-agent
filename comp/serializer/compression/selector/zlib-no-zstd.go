// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && !zstd

// Package selector provides correct compression impl to fx
package selector

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/common"
	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/def"
	implnoop "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-noop"
	implzlib "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-zlib"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called only when the zlib build tag is included
func (*compressorFactory) NewCompressor(kind string, level int, option string, valid []string) compression.Component {
	if !slices.Contains(valid, kind) {
		log.Warn("invalid " + option + " set. use one of " + strings.Join(valid, ", "))
		return implnoop.NewComponent().Comp
	}

	switch kind {
	case common.ZlibKind:
		return implzlib.NewComponent().Comp
	case common.ZstdKind:
		log.Warn("zstd build tag not included. using zlib")
		return implzlib.NewComponent().Comp
	case common.NoneKind:
		log.Warn("no " + option + " set. use one of " + strings.Join(valid, ", "))
		return implnoop.NewComponent().Comp
	default:
		log.Warn("invalid " + option + " set. use one of " + strings.Join(valid, ", "))
		return implnoop.NewComponent().Comp
	}
}

// NewCompressorReq returns a new Compressor based on serializer_compressor_kind
// This function is called only when the zlib build tag is included
func NewCompressorReq(req compression.Requires) compression.Provides {
	switch req.Cfg.GetString("serializer_compressor_kind") {
	case common.ZlibKind:
		return implzlib.NewComponent()
	case common.ZstdKind:
		log.Warn("zstd build tag not included. using zlib")
		return implzlib.NewComponent()
	case common.NoneKind:
		log.Warn("no serializer_compressor_kind set. use zlib or zstd")
		return implnoop.NewComponent()
	default:
		log.Warn("invalid serializer_compressor_kind detected. use zlib or zstd")
		return implnoop.NewComponent()
	}
}

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called only when the zlib build tag is included
func NewCompressor(cfg config.Component) compression.Component {
	return NewCompressorReq(compression.Requires{Cfg: cfg}).Comp
}
