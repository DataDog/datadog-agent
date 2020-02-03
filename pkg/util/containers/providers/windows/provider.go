// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inmetrics.

// +build windows

package windows

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

// Provider is a Windows implementation of the ContainerImplementation interface
type Provider struct {
}

// Prefetch gets data from all cgroups in one go
// If not successful all other calls will fail
func (mp *Provider) Prefetch() error {
	//FIXME: To be implemented for Windows containers
	return nil
}

// ContainerExists returns true if a cgroup exists for this containerID
func (mp *Provider) ContainerExists(containerID string) bool {
	//FIXME: To be implemented for Windows containers
	return true
}

// GetContainerStartTime returns container start time
func (mp *Provider) GetContainerStartTime(containerID string) (int64, error) {
	//FIXME: To be implemented for Windows containers
	return 1581512596, nil
}

// GetContainerMetrics returns CPU, IO and Memory metrics
func (mp *Provider) GetContainerMetrics(containerID string) (*metrics.ContainerMetrics, error) {
	//FIXME: To be implemented for Windows containers
	metrics := metrics.ContainerMetrics{
		CPU: &metrics.ContainerCPUStats{
			User:   10,
			System: 10,
		},
		Memory: &metrics.ContainerMemStats{},
		IO:     &metrics.ContainerIOStats{},
	}
	return &metrics, nil
}

// GetContainerLimits returns CPU, Thread and Memory limits
func (mp *Provider) GetContainerLimits(containerID string) (*metrics.ContainerLimits, error) {
	//FIXME: To be implemented for Windows containers
	return nil, fmt.Errorf("Not implemented")
}

// GetNetworkMetrics return network metrics for all PIDs in container
func (mp *Provider) GetNetworkMetrics(containerID string, networks map[string]string) (metrics.ContainerNetStats, error) {
	//FIXME: To be implemented for Windows containers
	return nil, fmt.Errorf("Not implemented")
}

// GetAgentCID returns the container ID where the current agent is running
func (mp *Provider) GetAgentCID() (string, error) {
	//FIXME: To be implemented for Windows containers
	return "", fmt.Errorf("Not implemented")
}

// ContainerIDForPID return ContainerID for a given pid
func (mp *Provider) ContainerIDForPID(pid int) (string, error) {
	//FIXME: To be implemented for Windows containers
	return "", fmt.Errorf("Not implemented")
}

// DetectNetworkDestinations lists all the networks available
// to a given PID and parses them in NetworkInterface objects
func (mp *Provider) DetectNetworkDestinations(pid int) ([]containers.NetworkDestination, error) {
	//FIXME: To be implemented for Windows containers
	return nil, fmt.Errorf("Not implemented")
}
