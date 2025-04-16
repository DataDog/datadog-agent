// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package fx provides the fx module for the serializer/compression component
package fx

import (
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	common "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			NewMockCompressor,
		),
	)
}

// NewMockCompressor returns a noop compressor.
func NewMockCompressor() compression.Component {
	return selector.NewCompressor(common.NoneKind, 1)
}
