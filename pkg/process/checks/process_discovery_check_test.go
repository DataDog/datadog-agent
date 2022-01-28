// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/stretchr/testify/assert"
)

func TestProcessDiscoveryCheck(t *testing.T) {
	maxBatchSize := 10
	mockConfig := ddconfig.Mock()
	mockConfig.Set("process_config.max_per_message", maxBatchSize)
	cfg := &config.AgentConfig{}
	ProcessDiscovery.Init(cfg, &model.SystemInfo{
		Cpus:        []*model.CPUInfo{{Number: 0}},
		TotalMemory: 0,
	})

	// Test check runs without error
	result, err := ProcessDiscovery.Run(cfg, 0)
	assert.NoError(t, err)

	// Test that result has the proper number of chunks, and that those chunks are of the correct type
	for _, elem := range result {
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

func TestProcessDiscoveryCheckInit(t *testing.T) {
	tests := []struct {
		name                 string
		override             bool
		maxPerMessage        int
		expectedMaxBatchSize int
	}{
		{
			name:                 "default batch size",
			override:             false,
			maxPerMessage:        50,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "valid batch size override",
			override:             true,
			maxPerMessage:        50,
			expectedMaxBatchSize: 50,
		},
		{
			name:                 "negative max batch size",
			override:             true,
			maxPerMessage:        -1,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "0 max batch size",
			override:             true,
			maxPerMessage:        0,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "big max batch size",
			override:             true,
			maxPerMessage:        2000,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
	}

	assert := assert.New(t)
	cfg := config.NewDefaultAgentConfig(false)
	sysInfo := &model.SystemInfo{}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock()
			if tc.override {
				mockConfig.Set("process_config.max_per_message", tc.maxPerMessage)
			}

			ProcessDiscovery.Init(cfg, sysInfo)
			assert.Equal(tc.expectedMaxBatchSize, ProcessDiscovery.maxBatchSize)
		})
	}
}
