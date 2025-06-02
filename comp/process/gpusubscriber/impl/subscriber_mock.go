// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package gpusubscriberimpl subscribes to GPU events
package gpusubscriberimpl

import (
	gpusubscriber "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/def"
)

// MockSubscriber is a mock implementation of the gpusubscriber.Component interface.
type MockSubscriber struct{}

// GetGPUTags returns empty in mocked implementation.
func (m *MockSubscriber) GetGPUTags() map[int32][]string {
	return map[int32][]string{}
}

// NewGpuSubscriberMock returns a new instance of the mock GPU subscriber.
func NewGpuSubscriberMock() gpusubscriber.Component {
	return &MockSubscriber{}
}
