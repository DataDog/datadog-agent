// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func testGroupId(groupID int32) func() int32 {
	return func() int32 {
		return groupID
	}
}

func TestProcessDiscoveryCheck(t *testing.T) {
	prev := getMaxBatchSize
	defer func() {
		getMaxBatchSize = prev
	}()

	maxBatchSize := 10
	getMaxBatchSize = func(config.ConfigReader) int { return maxBatchSize }

	check := NewProcessDiscoveryCheck(config.Mock(t))
	check.Init(
		&SysProbeConfig{},
		&HostInfo{
			SystemInfo: &model.SystemInfo{
				Cpus:        []*model.CPUInfo{{Number: 0}},
				TotalMemory: 0,
			},
		},
	)

	// Test check runs without error
	result, err := check.Run(testGroupId(0), nil)
	assert.NoError(t, err)

	// Test that result has the proper number of chunks, and that those chunks are of the correct type
	for _, elem := range result.Payloads() {
		assert.IsType(t, &model.CollectorProcDiscovery{}, elem)
		collectorProcDiscovery := elem.(*model.CollectorProcDiscovery)
		for _, proc := range collectorProcDiscovery.ProcessDiscoveries {
			assert.Empty(t, proc.Host)
		}
		if len(collectorProcDiscovery.ProcessDiscoveries) > maxBatchSize {
			t.Errorf("Expected less than %d messages in chunk, got %d",
				maxBatchSize, len(collectorProcDiscovery.ProcessDiscoveries))
		}
	}
}

func TestProcessDiscoveryChunking(t *testing.T) {
	tests := []struct{ procs, chunkSize, expectedChunks int }{
		{100, 10, 10}, // Normal behavior
		{50, 30, 2},   // Number of chunks does not split cleanly
		{10, 100, 1},  // Larger chunk size than there are procs
		{0, 100, 0},   // No procs
	}

	for _, test := range tests {
		procs := make([]*model.ProcessDiscovery, test.procs)
		chunkedProcs := chunkProcessDiscoveries(procs, test.chunkSize)
		assert.Len(t, chunkedProcs, test.expectedChunks)
	}
}
