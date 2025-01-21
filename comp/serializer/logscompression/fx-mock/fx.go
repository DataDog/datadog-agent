// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package fx provides the fx module for the serializer/compression component
package fx

import (
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type component struct{}

func (*component) NewCompressor(_kind string, _level int) compression.Compressor {
	return selector.NewNoopCompressor()
}

// NewMockCompressor returns a mock component that will always return a noop compressor.
func NewMockCompressor() logscompression.Component {
	return &component{}
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			NewMockCompressor,
		),
	)
}
