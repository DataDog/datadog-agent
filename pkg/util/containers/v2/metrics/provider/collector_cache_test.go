// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestCollectorCache(t *testing.T) {
	actualCollector := dummyCollector{
		id: "foo",
		cStats: map[string]*ContainerStats{
			"cID1": {
				CPU: &ContainerCPUStats{
					Total: util.Float64Ptr(100),
				},
			},
			"cID2": {
				CPU: &ContainerCPUStats{
					Total: util.Float64Ptr(200),
				},
			},
		},
		cNetStats: map[string]*ContainerNetworkStats{
			"cID1": {
				BytesSent: util.Float64Ptr(110),
			},
			"cID2": {
				BytesSent: util.Float64Ptr(210),
			},
		},
	}

	collectorCache := NewCollectorCache(actualCollector)
	assert.Equal(t, collectorCache.ID(), actualCollector.ID())

	cStats, err := collectorCache.GetContainerStats("cID1", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 100.0, *cStats.CPU.Total)

	ncStats, err := collectorCache.GetContainerNetworkStats("cID1", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 110.0, *ncStats.BytesSent)

	cStats2, err := collectorCache.GetContainerStats("cID2", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 200.0, *cStats2.CPU.Total)

	ncStats2, err := collectorCache.GetContainerNetworkStats("cID2", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 210.0, *ncStats2.BytesSent)

	// Changing underlying source
	actualCollector.cStats["cID1"] = &ContainerStats{
		CPU: &ContainerCPUStats{
			Total: util.Float64Ptr(150),
		},
	}
	actualCollector.cStats["cID2"] = &ContainerStats{
		CPU: &ContainerCPUStats{
			Total: util.Float64Ptr(250),
		},
	}
	actualCollector.cNetStats["cID2"] = &ContainerNetworkStats{
		BytesSent: util.Float64Ptr(260),
	}

	cStats, err = collectorCache.GetContainerStats("cID1", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 100.0, *cStats.CPU.Total)

	ncStats, err = collectorCache.GetContainerNetworkStats("cID1", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 110.0, *ncStats.BytesSent)

	// Force refresh
	cStats2, err = collectorCache.GetContainerStats("cID2", 0)
	assert.NoError(t, err)
	assert.Equal(t, 250.0, *cStats2.CPU.Total)

	// Verify networkStats was not refreshed
	ncStats2, err = collectorCache.GetContainerNetworkStats("cID2", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 210.0, *ncStats2.BytesSent)
}
