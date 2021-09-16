package checks

import (
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/stretchr/testify/assert"
)

func TestProcessDiscoveryCheck(t *testing.T) {
	cfg := &config.AgentConfig{MaxPerMessage: 10}
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

		if len(collectorProcDiscovery.ProcessDiscoveries) > cfg.MaxPerMessage {
			t.Errorf("Expected less than %d messages in chunk, got %d",
				cfg.MaxPerMessage, len(collectorProcDiscovery.ProcessDiscoveries))
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
