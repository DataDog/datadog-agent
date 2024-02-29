// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compression

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	compression "github.com/DataDog/datadog-agent/pkg/serializer/compression/internal"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const ZlibKind = "zlib"
const ZstdKind = "zstd"

type Compressor interface {
	Compress(src []byte) ([]byte, error)
	Decompress(src []byte) ([]byte, error)
	CompressBound(sourceLen int) int
	ContentEncoding() string
}

func NewCompressorStrategy(cfg config.Component) Compressor {
	kind := cfg.GetString("serializer_compressor_kind")
	switch kind {
	case ZlibKind:
		return compression.NewZlibStrategy()
	case ZstdKind:
		return compression.NewZstdStrategy()
	default:
		log.Warn("invalid serializer_compressor_kind detected. use zlib or zstd")
		return compression.NewNoopStrategy()
	}
}
