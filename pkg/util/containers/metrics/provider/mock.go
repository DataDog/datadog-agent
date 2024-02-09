// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package provider

import (
	"time"
)

var _ Collector = &dummyCollector{}

type dummyCollector struct {
	id              string
	cStats          map[string]*ContainerStats
	cPIDStats       map[string]*ContainerPIDStats
	cPIDs           map[string][]int
	cOpenFilesCount map[string]*uint64
	cNetStats       map[string]*ContainerNetworkStats
	cIDForPID       map[int]string
	selfContainerID string
	err             error
}

func (d *dummyCollector) constructor(priority uint8, runtimes ...RuntimeMetadata) (CollectorMetadata, error) {
	metadata := CollectorMetadata{
		ID:         d.id,
		Collectors: make(CollectorCatalog),
	}
	collectors := d.getCollectors(priority)

	for _, runtime := range runtimes {
		metadata.Collectors[runtime] = collectors
	}

	return metadata, d.err
}

func (d *dummyCollector) GetContainerStats(containerNS, containerID string, _ time.Duration) (*ContainerStats, error) {
	return d.cStats[containerNS+containerID], d.err
}

func (d *dummyCollector) GetContainerPIDStats(containerNS, containerID string, _ time.Duration) (*ContainerPIDStats, error) {
	return d.cPIDStats[containerNS+containerID], d.err
}

func (d *dummyCollector) GetContainerOpenFilesCount(containerNS, containerID string, _ time.Duration) (*uint64, error) {
	return d.cOpenFilesCount[containerNS+containerID], d.err
}

func (d *dummyCollector) GetContainerNetworkStats(containerNS, containerID string, _ time.Duration) (*ContainerNetworkStats, error) {
	return d.cNetStats[containerNS+containerID], d.err
}

func (d *dummyCollector) GetContainerIDForPID(pid int, _ time.Duration) (string, error) {
	return d.cIDForPID[pid], d.err
}

func (d *dummyCollector) GetPIDs(containerNS, containerID string, _ time.Duration) ([]int, error) {
	return d.cPIDs[containerNS+containerID], nil
}

func (d *dummyCollector) GetSelfContainerID() (string, error) {
	return d.selfContainerID, nil
}

// Helpers not part of Collector interface
func (d *dummyCollector) getCollectors(priority uint8) *Collectors {
	return &Collectors{
		Stats: CollectorRef[ContainerStatsGetter]{
			Collector: d,
			Priority:  priority,
		},
		Network: CollectorRef[ContainerNetworkStatsGetter]{
			Collector: d,
			Priority:  priority,
		},
		OpenFilesCount: CollectorRef[ContainerOpenFilesCountGetter]{
			Collector: d,
			Priority:  priority,
		},
		PIDs: CollectorRef[ContainerPIDsGetter]{
			Collector: d,
			Priority:  priority,
		},
		ContainerIDForPID: CollectorRef[ContainerIDForPIDRetriever]{
			Collector: d,
			Priority:  priority,
		},
		SelfContainerID: CollectorRef[SelfContainerIDRetriever]{
			Collector: d,
			Priority:  priority,
		},
	}
}
