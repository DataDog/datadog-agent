// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selector provides correct compression impl to fx
package selector

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	common "github.com/DataDog/datadog-agent/pkg/util/compression"
)

// FromConfig will return the compression algorithm specified in the provided config
// under the `serializer_compressor_kind` key.
// If `zstd` the compression level is taken from the serializer_zstd_compressor_level
// key.
func FromConfig(cfg config.Reader) common.Compressor {
	kind := cfg.GetString("serializer_compressor_kind")
	var level int

	switch kind {
	case common.ZstdKind:
		level = cfg.GetInt("serializer_zstd_compressor_level")
	case common.GzipKind:
		// There is no configuration option for gzip compression level when set via this method.
		level = 6
	}

	return NewCompressor(kind, level)
}
