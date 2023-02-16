// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestCollectorCache(t *testing.T) {
	actualCollector := dummyCollector{
		id: "foo",
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
		cIDForPID: map[int]string{
			1: "cID1",
			2: "cID2",
		},
	}

	collectorCache := NewCollectorCache(actualCollector)
	assert.Equal(t, collectorCache.ID(), actualCollector.ID())

	cStats, err := collectorCache.GetContainerStats("", "cID1", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 100.0, *cStats.CPU.Total)

	ncStats, err := collectorCache.GetContainerNetworkStats("", "cID1", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 110.0, *ncStats.BytesSent)

	cStats2, err := collectorCache.GetContainerStats("", "cID2", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 200.0, *cStats2.CPU.Total)

	ncStats2, err := collectorCache.GetContainerNetworkStats("", "cID2", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 210.0, *ncStats2.BytesSent)

	cID1, err := collectorCache.GetContainerIDForPID(1, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "cID1", cID1)

	cID2, err := collectorCache.GetContainerIDForPID(2, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "cID2", cID2)

	// Changing underlying source
	actualCollector.cStats["cID1"] = &ContainerStats{
		CPU: &ContainerCPUStats{
			Total: pointer.Ptr(150.0),
		},
	}
	actualCollector.cStats["cID2"] = &ContainerStats{
		CPU: &ContainerCPUStats{
			Total: pointer.Ptr(250.0),
		},
	}
	actualCollector.cNetStats["cID2"] = &ContainerNetworkStats{
		BytesSent: pointer.Ptr(260.0),
	}
	actualCollector.cIDForPID[2] = "cID22"

	cStats, err = collectorCache.GetContainerStats("", "cID1", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 100.0, *cStats.CPU.Total)

	ncStats, err = collectorCache.GetContainerNetworkStats("", "cID1", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 110.0, *ncStats.BytesSent)

	// Force refresh
	cStats2, err = collectorCache.GetContainerStats("", "cID2", 0)
	assert.NoError(t, err)
	assert.Equal(t, 250.0, *cStats2.CPU.Total)

	cID2, err = collectorCache.GetContainerIDForPID(2, 0)
	assert.NoError(t, err)
	assert.Equal(t, "cID22", cID2)

	// Verify networkStats was not refreshed
	ncStats2, err = collectorCache.GetContainerNetworkStats("", "cID2", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 210.0, *ncStats2.BytesSent)

	cID1, err = collectorCache.GetContainerIDForPID(1, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "cID1", cID1)
}
