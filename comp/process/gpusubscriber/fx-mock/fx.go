// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package fx provides the fx module for the gpu subscriber component
package fx

import (
	"testing"

	gpusubscriber "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/def"
	gpusubscriberimpl "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for this component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			gpusubscriberimpl.NewGpuSubscriberMock,
		),
	)
}

// SetupMockGpuSubscriber calls fxutil.Test to create a mock subscriber for testing
func SetupMockGpuSubscriber(t testing.TB) gpusubscriber.Component {
	return fxutil.Test[gpusubscriber.Component](t, MockModule())
}
