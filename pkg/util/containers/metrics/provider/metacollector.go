// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"time"
)

// MetaCollector is a special collector that uses all available collectors, by priority order.
type MetaCollector interface {
	// GetContainerIDForPID returns a container ID for given PID.
	// ("", nil) will be returned if no error but the containerd ID was not found.
	GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error)

	// GetSelfContainerID returns the container ID for current container.
	// ("", nil) will be returned if not possible to get ID for current container.
	GetSelfContainerID() (string, error)
}

type metaCollector struct {
	orderedCollectorLister func() []*collectorReference
}

func newMetaCollector(collectorLister func() []*collectorReference) *metaCollector {
	return &metaCollector{
		orderedCollectorLister: collectorLister,
	}
}

// GetContainerIDForPID returns a container ID for given PID.
func (mc *metaCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	for _, collector := range mc.orderedCollectorLister() {
		val, err := collector.collector.GetContainerIDForPID(pid, cacheValidity)
		if err != nil {
			return "", err
		}

		if val != "" {
			return val, nil
		}
	}

	return "", nil
}

// GetSelfContainerID returns the container ID for current container.
func (mc *metaCollector) GetSelfContainerID() (string, error) {
	for _, collector := range mc.orderedCollectorLister() {
		val, err := collector.collector.GetSelfContainerID()
		if err != nil {
			return "", err
		}

		if val != "" {
			return val, nil
		}
	}

	return "", nil
}

// Not implemented as metaCollector implements Collector interface to allow wrapping in collectorCache
func (mc *metaCollector) ID() string {
	panic("Should never be called")
}

func (mc *metaCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	panic("Should never be called")
}

func (mc *metaCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) {
	panic("Should never be called")
}
