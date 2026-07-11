// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

// Package crimock implements a mock Container Runtime Interface (CRI) client.
package crimock

import (
	"context"
	"time"

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

// ExecSync is a mock of ExecSync
func (m *MockCRIClient) ExecSync(ctx context.Context, containerID string, cmd []string, timeout time.Duration) ([]byte, []byte, int32, error) {
	args := m.Called(ctx, containerID, cmd, timeout)
	stdout, _ := args.Get(0).([]byte)
	stderr, _ := args.Get(1).([]byte)
	exitCode, _ := args.Get(2).(int32)
	return stdout, stderr, exitCode, args.Error(3)
}

// GetRuntime is a mock of GetRuntime
func (m *MockCRIClient) GetRuntime() string {
	return "fakeruntime"
}

// GetRuntimeVersion is a mock of GetRuntimeVersion
func (m *MockCRIClient) GetRuntimeVersion() string {
	return "1.0"
}
