// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mocks implements a component to provide gpusubscriber used by the core agent.
package gpusubscribermock

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/process/gpusubscriber"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule is the mock module for process forwarders
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockGpuSubscriber),
	)
}

// NoopSubscriber is a no-op implementation of the gpusubscriber.Component interface.
type MockSubscriber struct{}

// GetGPUTags returns an empty map as a no-op implementation.
func (m *MockSubscriber) GetGPUTags() map[int32][]string {
	return nil
}

func newMockGpuSubscriber() gpusubscriber.Component {
	return &MockSubscriber{}
}
