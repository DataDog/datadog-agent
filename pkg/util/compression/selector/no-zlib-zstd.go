// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !zlib && zstd

// Package selector provides correct compression impl to fx
package selector

import (
	common "github.com/DataDog/datadog-agent/pkg/util/compression"
	implgzip "github.com/DataDog/datadog-agent/pkg/util/compression/impl-gzip"
	implnoop "github.com/DataDog/datadog-agent/pkg/util/compression/impl-noop"
	implzstd "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewCompressor returns a new Compressor based on serializer_compressor_kind.
// This function is called when zstd is included but zlib is not.
func NewCompressor(kind string, level int) common.Compressor {
	switch kind {
	case common.ZstdKind:
		return implzstd.New(implzstd.Requires{
			Level: common.ZstdCompressionLevel(level),
		})
	case common.ZlibKind:
		log.Warn("zlib build tag not included, falling back to gzip")
		return implgzip.New(implgzip.Requires{Level: level})
	case common.GzipKind:
		return implgzip.New(implgzip.Requires{Level: level})
	case common.NoneKind:
		return implnoop.New()
	default:
		log.Errorf("unknown compression kind %q, falling back to noop", kind)
		return implnoop.New()
	}
}

// NewNoopCompressor returns a new Noop Compressor.
func NewNoopCompressor() common.Compressor {
	return implnoop.New()
}
