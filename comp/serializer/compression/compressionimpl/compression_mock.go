// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package compressionimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func MockModuleFactory() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMockCompressorFactory),
	)
}

type mockFactory struct{}

func (*mockFactory) NewNoopCompressor() compression.Component {
	return NewCompressorFactory().NewNoopCompressor()
}

func (*mockFactory) NewCompressor(_ string, _ int, _ string, _ []string) compression.Component {
	return NewCompressorFactory().NewNoopCompressor()
}

// NewMockCompressorFactory returns a factory that always return a Noop Compressor
func NewMockCompressorFactory() compression.Factory {
	return &mockFactory{}
}

// NewMockCompressor returns a new Mock
func NewMockCompressor() compression.Component {
	return NewCompressorFactory().NewNoopCompressor()
}
