// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package providers

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

// FakeContainerImpl is a fake implementation of a container provider
type FakeContainerImpl struct {
}

// Prefetch mocks the prefetch interface method
func (f FakeContainerImpl) Prefetch() error {
	return nil
}

// ContainerExists mocks the ContainerExists interface method
func (f FakeContainerImpl) ContainerExists(containerID string) bool {
	return true
}

// GetContainerStartTime mocks the GetContainerStartTime interface method
func (f FakeContainerImpl) GetContainerStartTime(containerID string) (int64, error) {
	panic("implement me")
}

// DetectNetworkDestinations mocks the DetectNetworkDestinations interface method
func (f FakeContainerImpl) DetectNetworkDestinations(pid int) ([]containers.NetworkDestination, error) {
	panic("implement me")
}

// GetAgentCID mocks the GetAgentCID interface method
func (f FakeContainerImpl) GetAgentCID() (string, error) {
	panic("implement me")
}

// GetPIDs mocks the GetPIDs interface method
func (f FakeContainerImpl) GetPIDs(containerID string) ([]int32, error) {
	return []int32{123}, nil
}

// ContainerIDForPID mocks the ContainerIDForPID interface method
func (f FakeContainerImpl) ContainerIDForPID(pid int) (string, error) {
	panic("implement me")
}

// GetDefaultGateway the GetDefaultGateway interface method
func (f FakeContainerImpl) GetDefaultGateway() (net.IP, error) {
	panic("implement me")
}

// GetDefaultHostIPs mocks the GetDefaultHostIPs interface method
func (f FakeContainerImpl) GetDefaultHostIPs() ([]string, error) {
	panic("implement me")
}

// GetContainerMetrics mocks the GetContainerMetrics interface method
func (f FakeContainerImpl) GetContainerMetrics(containerID string) (*metrics.ContainerMetrics, error) {
	return nil, nil
}

// GetContainerLimits mocks the GetContainerLimits interface method
func (f FakeContainerImpl) GetContainerLimits(containerID string) (*metrics.ContainerLimits, error) {
	return nil, nil
}

// GetNetworkMetrics mocks the GetNetworkMetrics interface method
func (f FakeContainerImpl) GetNetworkMetrics(containerID string, networks map[string]string) (metrics.ContainerNetStats, error) {
	return nil, nil
}
