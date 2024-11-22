// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !zlib && !zstd

// Package selector provides correct compression impl to fx
package selector

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/def"
	implnoop "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-noop"
)

func (*compressorFactory) NewCompressor(kind string, level int, option string, valid []string) compression.Component {
	return implnoop.NewComponent().Comp
}

// NewCompressorReq returns a new Compressor based on serializer_compressor_kind
// This function is called only when there is no zlib or zstd tag
func NewCompressorReq(_ compression.Requires) compression.Provides {
	return implnoop.NewComponent()
}

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called only when there is no zlib or zstd tag
func NewCompressor(cfg config.Component) compression.Component {
	return NewCompressorReq(compression.Requires{Cfg: cfg}).Comp
}
