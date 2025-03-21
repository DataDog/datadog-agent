// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/stretchr/testify/assert"
)

func TestRuntimeMetadataString(t *testing.T) {
	for _, test := range []struct {
		name     string
		runtime  string
		flavor   string
		expected string
	}{
		{
			name:     "containerd",
			runtime:  string(RuntimeNameContainerd),
			flavor:   "",
			expected: "containerd",
		},
		{
			name:     "containerd with kata",
			runtime:  string(RuntimeNameContainerd),
			flavor:   "kata",
			expected: "containerd-kata",
		},
		{
			name:     "containerd with runc",
			runtime:  string(RuntimeNameContainerd),
			flavor:   "runc",
			expected: "containerd-runc",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			actual := NewRuntimeMetadata(test.runtime, test.flavor)
			assert.Equal(t, test.expected, actual.String())
		})
	}
}

func TestGenericProvider(t *testing.T) {
	provider := newProvider(optional.NewNoneOption[workloadmeta.Component]())

	// First collector is going to be priority 1 on stats and 2 on network
	statsCollector := &dummyCollector{
		id: "statsNet",
		cStats: map[string]*ContainerStats{
			"cID1": {
				CPU: &ContainerCPUStats{
					Total: pointer.Ptr(100.0),
				},
			},
			"cID2": {
				CPU: &ContainerCPUStats{
					Total: pointer.Ptr(200.0),
				},
			},
		},
		cNetStats: map[string]*ContainerNetworkStats{
			"cID1": {
				BytesSent: pointer.Ptr(110.0),
			},
			"cID2": {
				BytesSent: pointer.Ptr(210.0),
			},
		},
	}

	// Second collector is going to be implementing process at priority 1 and stats at priority 0 later
	processCollector := &dummyCollector{
		id: "process",
		cStats: map[string]*ContainerStats{
			"cID1": {
				CPU: &ContainerCPUStats{
					Total: pointer.Ptr(200.0),
				},
			},
			"cID2": {
				CPU: &ContainerCPUStats{
					Total: pointer.Ptr(400.0),
				},
			},
		},
		cOpenFilesCount: map[string]*uint64{
			"cID1": pointer.Ptr[uint64](10),
			"cID2": pointer.Ptr[uint64](20),
		},
	}

	runtimeMetadata := NewRuntimeMetadata(string(RuntimeNameDocker), "")

	// Verify that at first we get nothing
	actualCollector := provider.GetCollector(runtimeMetadata)
	assert.Nil(t, actualCollector)

	// Register collectors, one is empty (PIDs)
	provider.collectorsUpdatedCallback(CollectorCatalog{
		RuntimeMetadata{runtime: RuntimeNameDocker}: &Collectors{
			Stats: CollectorRef[ContainerStatsGetter]{
				Collector: statsCollector,
				Priority:  1,
			},
			Network: CollectorRef[ContainerNetworkStatsGetter]{
				Collector: statsCollector,
				Priority:  2,
			},
			OpenFilesCount: CollectorRef[ContainerOpenFilesCountGetter]{
				Collector: processCollector,
				Priority:  1,
			},
		},
	})

	actualCollector = provider.GetCollector(runtimeMetadata)
	assert.NotNil(t, actualCollector)

	// Verify that we get the data
	statsC1, err := actualCollector.GetContainerStats("", "cID1", 0)
	assert.NoError(t, err)
	assert.Equal(t, 100.0, *statsC1.CPU.Total)

	statsC2, err := actualCollector.GetContainerStats("", "cID2", 0)
	assert.NoError(t, err)
	assert.Equal(t, 200.0, *statsC2.CPU.Total)

	netStatsC1, err := actualCollector.GetContainerNetworkStats("", "cID1", 0)
	assert.NoError(t, err)
	assert.Equal(t, 110.0, *netStatsC1.BytesSent)

	netStatsC2, err := actualCollector.GetContainerNetworkStats("", "cID2", 0)
	assert.NoError(t, err)
	assert.Equal(t, 210.0, *netStatsC2.BytesSent)

	ofC1, err := actualCollector.GetContainerOpenFilesCount("", "cID1", 0)
	assert.NoError(t, err)
	assert.EqualValues(t, 10, *ofC1)

	ofC2, err := actualCollector.GetContainerOpenFilesCount("", "cID2", 0)
	assert.NoError(t, err)
	assert.EqualValues(t, 20, *ofC2)

	// Update priority of the second collector
	provider.collectorsUpdatedCallback(CollectorCatalog{
		RuntimeMetadata{runtime: RuntimeNameDocker}: &Collectors{
			Stats: CollectorRef[ContainerStatsGetter]{
				Collector: processCollector,
				Priority:  0,
			},
			Network: CollectorRef[ContainerNetworkStatsGetter]{
				Collector: statsCollector,
				Priority:  2,
			},
			OpenFilesCount: CollectorRef[ContainerOpenFilesCountGetter]{
				Collector: processCollector,
				Priority:  1,
			},
		},
	})

	actualCollector = provider.GetCollector(runtimeMetadata)
	assert.NotNil(t, actualCollector)

	statsC1, err = actualCollector.GetContainerStats("", "cID1", 0)
	assert.NoError(t, err)
	assert.Equal(t, 200.0, *statsC1.CPU.Total)

	statsC2, err = actualCollector.GetContainerStats("", "cID2", 0)
	assert.NoError(t, err)
	assert.Equal(t, 400.0, *statsC2.CPU.Total)
}
