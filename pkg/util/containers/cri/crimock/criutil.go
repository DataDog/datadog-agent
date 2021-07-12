// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build cri

package crimock

import (
	"github.com/stretchr/testify/mock"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// MockCRIClient is used for tests
type MockCRIClient struct {
	mock.Mock
}

// ListContainerStats sends a ListContainerStatsRequest to the server, and parses the returned response
func (m *MockCRIClient) ListContainerStats() (map[string]*pb.ContainerStats, error) {
	args := m.Called()
	return args.Get(0).(map[string]*pb.ContainerStats), args.Error(1)
}

// GetContainerStatus sends a ContainerStatusRequest to the server, and parses the returned response
func (m *MockCRIClient) GetContainerStatus(containerID string) (*pb.ContainerStatus, error) {
	args := m.Called(containerID)
	return args.Get(0).(*pb.ContainerStatus), args.Error(1)
}

func (m *MockCRIClient) GetRuntime() string {
	return "fakeruntime"
}

func (m *MockCRIClient) GetRuntimeVersion() string {
	return "1.0"
}
