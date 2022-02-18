// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri
// +build cri

package cri

import (
	"time"

	"k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	criCollectorID = "cri"
)

func init() {
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID:       criCollectorID,
		Priority: 1, // Less than the "system" collector, so we can rely on cgroups directly if possible
		Runtimes: []string{provider.RuntimeNameCRIO},
		Factory: func() (provider.Collector, error) {
			return newCRICollector()
		},
		DelegateCache: true,
	})
}

type criCollector struct {
	client cri.CRIClient
}

func newCRICollector() (*criCollector, error) {
	if !config.IsFeaturePresent(config.Cri) {
		return nil, provider.ErrPermaFail
	}

	client, err := cri.GetUtil()
	if err != nil {
		return nil, provider.ConvertRetrierErr(err)
	}

	return &criCollector{client: client}, nil
}

// ID returns the collector ID.
func (collector *criCollector) ID() string {
	return criCollectorID
}

// GetContainerStats returns stats by container ID.
func (collector *criCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	stats, err := collector.getCriContainerStats(containerID)
	if err != nil {
		return nil, err
	}

	return &provider.ContainerStats{
		Timestamp: time.Now(),
		CPU: &provider.ContainerCPUStats{
			Total: pointer.UIntToFloatPtr(stats.GetCpu().GetUsageCoreNanoSeconds().GetValue()),
		},
		Memory: &provider.ContainerMemStats{
			RSS: pointer.UIntToFloatPtr(stats.GetMemory().GetWorkingSetBytes().GetValue()),
		},
	}, nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (collector *criCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	// Not available
	return nil, nil
}

// GetContainerIDForPID returns the container ID for given PID
func (collector *criCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	// Not available
	return "", nil
}

// GetSelfContainerID returns current process container ID
func (collector *criCollector) GetSelfContainerID() (string, error) {
	// Not available
	return "", nil
}

func (collector *criCollector) getCriContainerStats(containerID string) (*v1alpha2.ContainerStats, error) {
	stats, err := collector.client.GetContainerStats(containerID)
	if err != nil {
		return nil, err
	}

	return stats, nil
}
