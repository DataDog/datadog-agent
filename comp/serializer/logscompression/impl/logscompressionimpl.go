// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logscompressionimpl provides the implementation for the serializer/logscompression component
package logscompressionimpl

import (
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
)

type component struct{}

// Provides contains the compression component
type Provides struct {
	Comp logscompression.Component
}

func (*component) NewCompressor(kind string, level int) compression.Compressor {
	return selector.NewCompressor(kind, level)
}

// NewComponent creates a new logscompression component.
func NewComponent() logscompression.Component {
	return &component{}
}
