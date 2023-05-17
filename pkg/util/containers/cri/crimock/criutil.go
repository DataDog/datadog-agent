// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

package crimock

import (
	"github.com/stretchr/testify/mock"
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// MockCRIClient is used for tests
type MockCRIClient struct {
	mock.Mock
}

// ListContainerStats sends a ListContainerStatsRequest to the server, and parses the returned response
func (m *MockCRIClient) ListContainerStats() (map[string]*criv1.ContainerStats, error) {
	args := m.Called()
	return args.Get(0).(map[string]*criv1.ContainerStats), args.Error(1)
}

// GetContainerStats returns the stats for the container with the given ID
func (m *MockCRIClient) GetContainerStats(containerID string) (*criv1.ContainerStats, error) {
	args := m.Called(containerID)
	return args.Get(0).(*criv1.ContainerStats), args.Error(1)
}

// GetRuntime is a mock of GetRuntime
func (m *MockCRIClient) GetRuntime() string {
	return "fakeruntime"
}

// GetRuntimeVersion is a mock of GetRuntimeVersion
func (m *MockCRIClient) GetRuntimeVersion() string {
	return "1.0"
}
